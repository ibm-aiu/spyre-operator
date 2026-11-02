/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	spyreerr "github.com/ibm-aiu/spyre-operator/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ControlledComponent comprises of an ordered list of controlled objects.
// The objects in the same controlled component will be synced sequentially.
// If the previous object is not ready, the object in the next sequence will not be synched.
type ControlledComponent struct {
	name       string
	namespace  string
	objects    []ControlledObject
	client     client.Client
	scheme     *runtime.Scheme
	disabled   bool
	skipUpdate bool
	// degraded holds the nodes on which a DegradableObject is permanently unable to become ready.
	// It is updated on every Sync.
	degraded []DegradedNode
	// failing holds the nodes on which a DegradableObject is failing but not confirmed as degraded yet.
	// The nodes are held by the taint. It is updated on every Sync.
	failing []DegradedNode
	// holdTolerated lists the components, other than itself, whose taint on a failing node
	// the objects of this component must tolerate. It is the components in the same or later steps,
	// so that a failing component holds only the components deployed after it.
	holdTolerated []string
}

func NewControlledComponent(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme,
	componentPath, namespace, name string) (*ControlledComponent, error) {
	files, err := filePathWalkDir(componentPath, []string{yamlExtension})
	if err != nil {
		return nil, fmt.Errorf("failed to list object files in %s: %w", componentPath, err)
	}
	component := &ControlledComponent{
		name:      name,
		namespace: namespace,
		client:    k8sClient,
		scheme:    scheme,
	}
	for _, file := range files {
		runtimeObj, gvk, err := decodeFromFile(scheme, file)
		if err != nil {
			return nil, fmt.Errorf("failed to decode from file %s: %w", file, err)
		}
		obj, err := NewControlledObject(ctx, gvk.Kind, namespace, runtimeObj)
		if err != nil {
			return nil, fmt.Errorf("failed to init controlled object from file %s, %w", file, err)
		}
		component.objects = append(component.objects, obj)
	}
	return component, nil
}

func (c *ControlledComponent) GetName() string {
	return c.name
}

// FailingNodes returns the nodes found failing, but not confirmed as degraded yet, at the last Sync
func (c *ControlledComponent) FailingNodes() []DegradedNode {
	return c.failing
}

// DegradedNodes returns the nodes found degraded at the last Sync
func (c *ControlledComponent) DegradedNodes() []DegradedNode {
	return c.degraded
}

func (c *ControlledComponent) SetDisable(disabled bool) {
	c.disabled = disabled
}

func (c *ControlledComponent) SetSkipUpdate(skipUpdate bool) {
	c.skipUpdate = skipUpdate
}

func (c *ControlledComponent) Transform(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	if c.disabled {
		return nil
	}
	for _, obj := range c.objects {
		if err := obj.TransformByPolicy(c.scheme, clusterPolicy, cluster); err != nil {
			return fmt.Errorf("failed to transform %s: %w", obj.GetID(), err)
		}
		if tolerator, ok := obj.(degradedTolerator); ok {
			tolerator.addDegradedToleration(c.name)
			for _, name := range c.holdTolerated {
				tolerator.addDegradedToleration(name)
			}
		}
	}
	return nil
}

func (c *ControlledComponent) Sync(ctx context.Context) (bool, string, error) {
	logger := log.FromContext(ctx).WithValues("Component", c.name, "Target Namespace", c.namespace,
		"disable", c.disabled, "skipUpdate", c.skipUpdate)
	if c.disabled {
		return c.syncDisabled(ctx, logger)
	}
	return c.syncEnabled(ctx, logger)
}

// syncDisabled handles sync when the component is disabled: cleans up any running resources.
func (c *ControlledComponent) syncDisabled(ctx context.Context, logger logr.Logger) (bool, string, error) {
	var multiErr error
	c.degraded = nil
	c.failing = nil
	if c.name == spyreconst.CardManagementResourceName {
		if err := c.deleteCardManagementRunners(ctx); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	if err := c.clearDegradedNodes(ctx, nil); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.deleteAll(ctx, logger); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	message := ""
	if multiErr != nil {
		message = multiErr.Error()
	}
	return true, message, nil
}

// syncEnabled deploys the component's objects in order and reports readiness.
func (c *ControlledComponent) syncEnabled(ctx context.Context, logger logr.Logger) (bool, string, error) {
	for i, obj := range c.objects {
		// previous step not ready.
		// The step of a DegradableObject (i.e., per-node object) does not block the next steps
		// so that a pending object of a node does not block the other nodes.
		if i > 0 && !c.stepSettled(ctx, c.objects[i-1]) {
			return false, fmt.Sprintf("previous step %s is not ready", c.objects[i-1].GetID()), nil
		}
		if ok, msg, err := c.syncObject(ctx, logger, obj); !ok || err != nil {
			return ok, msg, err
		}
	}
	// Per-node objects do not block each other, so the last step (e.g., Service) is ready as long as
	// some nodes are serving. Objects pending within the grace period still make the component not ready,
	// while permanently pending ones are reported as degraded nodes to the caller.
	pending, held := c.reconcileDegraded(ctx, logger)
	// When all unready degradable objects are confirmed permanently degraded (pending == 0, degraded > 0),
	// treat the component as successful so the caller can emit Degraded instead of NotReady.
	// The healthy nodes (if any) keep serving; the degraded ones are tainted and reported.
	// The same applies when the objects are held back from every node by the failure of an earlier component
	// (e.g., the only node is failing), since they cannot become ready until the node recovers.
	allDegraded := pending == 0 && (len(c.degraded) > 0 || held > 0)
	ready := (c.Ready(ctx) || allDegraded) && pending == 0
	message := ""
	if ready {
		logger.Info(fmt.Sprintf("%s is ready", c.name))
	} else {
		message = "last step is not ready"
	}
	return ready, message, nil
}

// syncObject creates or updates a single controlled object.
// Returns (true, "", nil) when the object was successfully synced or created.
func (c *ControlledComponent) syncObject(ctx context.Context, logger logr.Logger, obj ControlledObject) (bool, string, error) {
	syncedObject, err := obj.Sync(ctx, c.client)
	if apiErrors.IsNotFound(err) {
		if err := c.client.Create(ctx, obj.GetObject()); err != nil && !apiErrors.IsAlreadyExists(err) {
			spyreerr.LogErrCreate(logger, err)
			msg := fmt.Sprintf("failed to create %s: %v", obj.GetID(), err)
			return false, msg, errors.New(msg)
		}
		return true, "", nil
	}
	if err != nil {
		msg := fmt.Sprintf("failed to sync %s: %v", obj.GetID(), err)
		return false, msg, errors.New(msg)
	}
	if !c.skipUpdate {
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			return c.client.Update(ctx, syncedObject)
		}); err != nil {
			msg := fmt.Sprintf("failed to update %s: %v", obj.GetID(), err)
			return false, msg, errors.New(msg)
		}
	}
	return true, "", nil
}

func isDegradable(obj ControlledObject) bool {
	_, ok := obj.(DegradableObject)
	return ok
}

// degradedTolerator is implemented by the controlled objects whose Pods must tolerate
// the taint put on the degraded node by the operator to be able to recover from it.
type degradedTolerator interface {
	addDegradedToleration(component string)
}

// endpointDependent is implemented by the controlled objects which are ready only if the Pods behind them
// are serving (e.g., a Service which needs a ready endpoint).
type endpointDependent interface {
	dependsOnEndpoints()
}

// stepSettled returns true if the next step is allowed to be synced after the step.
// A step which is not ready blocks the next one, except for:
//   - a DegradableObject, which does not block the other nodes.
//   - an object depending on the endpoints of the Pods (e.g., a Service) when every DegradableObject of
//     the component which is not ready is degraded or held back, since it has no endpoint to wait for.
func (c *ControlledComponent) stepSettled(ctx context.Context, step ControlledObject) bool {
	if isDegradable(step) || step.Ready(ctx, c.client) {
		return true
	}
	if _, ok := step.(endpointDependent); !ok {
		return false
	}
	unready := 0
	for _, obj := range c.objects {
		degradable, ok := obj.(DegradableObject)
		if !ok || obj.Ready(ctx, c.client) {
			continue
		}
		unready++
		if len(degradable.Degraded(ctx, c.client)) == 0 && !c.heldByTaint(ctx, obj) {
			return false
		}
	}
	return unready > 0
}

// heldByTainter is implemented by the controlled objects which may be held back from every node
// by the degraded taint of another component.
type heldByTainter interface {
	heldByTaint(ctx context.Context, k8sClient client.Client) bool
}

// reconcileDegraded checks every DegradableObject, taints and reports the condition on the nodes
// where the object is permanently not ready, and restores the other nodes of the component.
// The nodes where the object is failing but not confirmed as degraded yet are held by the taint only,
// so that the later components are not deployed on the node during the wait.
// It updates c.degraded and returns the number of objects which are not ready but not (yet) degraded,
// and the number of objects which are not ready only because they are held back from every node by the taint
// of another component. The latter are not counted as pending.
func (c *ControlledComponent) reconcileDegraded(ctx context.Context, logger logr.Logger) (pending, held int) {
	degraded, failing, pending, held := c.collectUnready(ctx)
	keep := make(map[string]struct{}, len(degraded)+len(failing))
	for _, node := range degraded {
		keep[node.Name] = struct{}{}
		if err := markNodeDegraded(ctx, c.client, c.name, node); err != nil {
			logger.Info("failed to mark node degraded", "node", node.Name, "error", err)
		}
	}
	heldNodes := c.holdFailingNodes(ctx, logger, failing, keep)
	if err := c.clearDegradedNodes(ctx, keep); err != nil {
		logger.Info("failed to restore nodes from degraded", "error", err)
	}
	c.degraded = degraded
	c.failing = heldNodes
	return pending, held
}

// collectUnready checks the DegradableObjects which are not ready. It returns the degraded and the failing nodes,
// the number of the objects pending, and the number of the objects held back by the taint of another component.
func (c *ControlledComponent) collectUnready(ctx context.Context) (degraded, failing []DegradedNode, pending, held int) {
	degraded = []DegradedNode{}
	failing = []DegradedNode{}
	for _, obj := range c.objects {
		degradable, ok := obj.(DegradableObject)
		if !ok || obj.Ready(ctx, c.client) {
			continue
		}
		objDegraded := degradable.Degraded(ctx, c.client)
		switch {
		case len(objDegraded) > 0:
		case c.heldByTaint(ctx, obj):
			held++
		default:
			pending++
		}
		degraded = append(degraded, objDegraded...)
		failing = append(failing, degradable.Failing(ctx, c.client)...)
	}
	return degraded, failing, pending, held
}

// heldByTaint returns true if the object is held back from every node by the taint of another component
func (c *ControlledComponent) heldByTaint(ctx context.Context, obj ControlledObject) bool {
	holdable, ok := obj.(heldByTainter)
	return ok && holdable.heldByTaint(ctx, c.client)
}

// holdFailingNodes taints the failing nodes which are not degraded to hold the later components from them,
// adds them to keep, and returns them.
func (c *ControlledComponent) holdFailingNodes(ctx context.Context, logger logr.Logger,
	failing []DegradedNode, keep map[string]struct{}) []DegradedNode {
	held := []DegradedNode{}
	for _, node := range failing {
		if _, found := keep[node.Name]; found {
			continue
		}
		keep[node.Name] = struct{}{}
		held = append(held, node)
		logger.Info("holding node while the failure is not confirmed", "node", node.Name, "reason", node.Reason)
		if err := markNodeFailing(ctx, c.client, c.name, node.Name); err != nil {
			logger.Info("failed to hold failing node", "node", node.Name, "error", err)
		}
	}
	return held
}

// clearDegradedNodes restores the nodes degraded by this component from degraded except for the nodes to keep.
// It does nothing if the component has no DegradableObject.
func (c *ControlledComponent) clearDegradedNodes(ctx context.Context, keep map[string]struct{}) error {
	if !slices.ContainsFunc(c.objects, isDegradable) {
		return nil
	}
	nodeList := &corev1.NodeList{}
	if err := c.client.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	taintKey := degradedTaintKey(c.name)
	var multiErr error
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if _, found := keep[node.Name]; found || !hasDegradedMark(node, taintKey, degradedConditionType(c.name)) {
			continue
		}
		if err := clearNodeDegraded(ctx, c.client, c.name, node.Name); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	if multiErr != nil {
		return fmt.Errorf("failed to clear degraded nodes: %w", multiErr)
	}
	return nil
}

func (c *ControlledComponent) deleteAll(ctx context.Context, logger logr.Logger) error {
	failedList := []string{}
	for i := range c.objects {
		// delete in reverse order
		deleteObj := c.objects[len(c.objects)-1-i]
		if err := c.client.Delete(ctx, deleteObj.GetObject()); err != nil &&
			!apiErrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			spyreerr.LogWarningDelete(logger, err)
			failedList = append(failedList, deleteObj.GetID().String())
		}
	}
	if len(failedList) > 0 {
		return fmt.Errorf("failed to delete objects: %v", failedList)
	}
	return nil
}

func (c *ControlledComponent) GetControlledObjects(ctx context.Context, scheme *runtime.Scheme,
	path string, openshiftVersion string) ([]ControlledObject, error) {
	logger := log.FromContext(ctx).WithName("assets-from")
	controlledObjects := []ControlledObject{}
	files, err := filePathWalkDir(path, []string{".yaml"})
	if err != nil {
		return controlledObjects, fmt.Errorf("failed to traverse path '%s': %w", path, err)
	}
	sort.Strings(files)
	for _, file := range files {
		if strings.Contains(file, "openshift") && openshiftVersion == "" {
			continue
		}

		runtimeObj, gvk, err := decodeFromFile(scheme, file)
		if err != nil {
			return controlledObjects, fmt.Errorf("failed to decode '%s': %w", path, err)
		}
		controlledObj, err := NewControlledObject(ctx, gvk.Kind, c.namespace, runtimeObj)
		if err != nil {
			return controlledObjects, fmt.Errorf("failed to get controlled object from '%s': %w", file, err)
		}
		controlledObjects = append(controlledObjects, controlledObj)
		logger.Info("controlledObj added", "objectID", controlledObj.GetID())
	}
	return controlledObjects, nil
}

// Ready returns true if last object is ready or component is disabled
func (c *ControlledComponent) Ready(ctx context.Context) bool {
	if c.disabled {
		return true
	}
	success := true
	if len(c.objects) > 0 {
		success = c.objects[len(c.objects)-1].Ready(ctx, c.client)
	}
	return success
}

// deletePodsByLabels deletes pods in the given namespace with the specified labels.
func (c *ControlledComponent) deletePodsByLabels(ctx context.Context, labels map[string]string) error {
	var err error
	logger := log.FromContext(ctx).WithName("delete-pods-by-labels")
	// Create a list option to filter pods by labels in the given namespace
	listOpts := []client.ListOption{
		client.InNamespace(c.namespace),
		client.MatchingLabels(labels),
	}
	// List the pods
	var podList corev1.PodList
	if err = c.client.List(ctx, &podList, listOpts...); err != nil {
		logger.V(1).Info("failed to list pods", "labels", labels, "error", err)
		return fmt.Errorf("failed to list pod with labels %v: %w", labels, err)
	}
	var deleteErr error
	// Delete each pod
	for _, pod := range podList.Items {
		if err := c.client.Delete(ctx, &pod); err != nil {
			logger.V(1).Info("failed to delete pod", "name", pod.Name, "error", err)
			if deleteErr == nil {
				deleteErr = err
			}
		}
	}
	if deleteErr != nil {
		return fmt.Errorf("failed to delete some pods: %w", deleteErr)
	}
	return nil
}

// Clear cleans up resources which are generated by controlled object in component without garbage collection mechanism
func (c *ControlledComponent) Clear(ctx context.Context) error {
	switch c.name {
	case "common":
		return c.deleteCertSecretFromCertificate(ctx)
	case spyreconst.ValidatorResourceName:
		return c.deleteCertSecretFromService(ctx)
	case spyreconst.CardManagementResourceName:
		var multiErr error
		if err := c.deleteCardManagementRunners(ctx); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
		if err := c.clearDegradedNodes(ctx, nil); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
		if multiErr != nil {
			return fmt.Errorf("failed to clear card management: %w", multiErr)
		}
		return nil
	case spyreconst.DevicePluginResourceName:
		if err := c.clearDegradedNodes(ctx, nil); err != nil {
			return fmt.Errorf("failed to clear device plugin: %w", err)
		}
		return c.clearAllSpyreNodeStates(ctx)
	}
	return c.clearDegradedNodes(ctx, nil)
}

func (c *ControlledComponent) deleteCertSecretFromService(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("delete-validator-secret-from-service")
	var multiErr error
	for _, obj := range c.objects {
		if obj.GetID().Kind != "Service" {
			continue
		}
		if err := c.deleteServiceAndCertSecret(ctx, logger, obj); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	if multiErr != nil {
		return fmt.Errorf("failed to complete cert secret cleanup: %w", multiErr)
	}
	return nil
}

func (c *ControlledComponent) deleteServiceAndCertSecret(ctx context.Context, logger logr.Logger, obj ControlledObject) error {
	service, ok := obj.GetObject().(*corev1.Service)
	if !ok || service.Annotations == nil {
		return nil
	}
	secretName := service.Annotations["service.beta.openshift.io/serving-cert-secret-name"]
	if secretName == "" {
		return nil
	}
	logger.Info("deleting Service resource", "name", obj.GetID().Name, "namespace", c.namespace)
	if err := c.client.Delete(ctx, obj.GetObject()); err != nil &&
		!apiErrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return fmt.Errorf("failed to delete Service resource: %s", obj.GetID().Name)
	}
	return c.deleteCertSecret(ctx, secretName)
}

func (c *ControlledComponent) deleteCertSecretFromCertificate(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("delete-cert-secret-from-certificate")
	var multiErr error
	for _, obj := range c.objects {
		if obj.GetID().Kind != "Certificate" {
			continue
		}
		if err := c.deleteCertificateAndCertSecret(ctx, logger, obj); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	if multiErr != nil {
		return fmt.Errorf("failed to complete cert secret cleanup: %w", multiErr)
	}
	return nil
}

func (c *ControlledComponent) deleteCertificateAndCertSecret(ctx context.Context, logger logr.Logger, obj ControlledObject) error {
	cert, ok := obj.GetObject().(*certmanagerv1.Certificate)
	if !ok {
		return nil
	}
	secretName := cert.Spec.SecretName
	if secretName == "" {
		return nil
	}
	logger.Info("deleting Certificate resource", "name", obj.GetID().Name, "namespace", c.namespace)
	if err := c.client.Delete(ctx, obj.GetObject()); err != nil &&
		!apiErrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return fmt.Errorf("failed to delete Certificate resource: %s", obj.GetID().Name)
	}
	return c.deleteCertSecret(ctx, secretName)
}

func (c *ControlledComponent) deleteCertSecret(ctx context.Context, secretName string) error {
	logger := log.FromContext(ctx).WithName("delete-cert-secret")
	logger.Info("deleting associated Secret resource", "name", secretName, "namespace", c.namespace)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: c.namespace,
		},
	}
	if err := c.client.Delete(ctx, secret); err != nil && !apiErrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Secret resource: %w", err)
	}
	return nil
}

func (c *ControlledComponent) deleteCardManagementRunners(ctx context.Context) error {
	return c.deletePodsByLabels(ctx, map[string]string{
		spyreconst.CardManagementRunnerLabelKey: spyreconst.CardManagementRunnerLabelValue})
}

// clearAllSpyreNodeStates clean all discovered devices and topology
func (c *ControlledComponent) clearAllSpyreNodeStates(ctx context.Context) error {
	nodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
	if err := c.client.List(ctx, nodeStateList, []client.ListOption{}...); err != nil {
		return fmt.Errorf("failed to list SpyreNodeState: %w", err)
	}
	for _, nodeState := range nodeStateList.Items {
		if len(nodeState.Spec.SpyreInterfaces) > 0 || len(nodeState.Spec.SpyreSSAInterfaces) > 0 || nodeState.Spec.Pcitopo != "" {
			nodeState.Spec.SpyreInterfaces = []spyrev1alpha1.SpyreInterface{}
			nodeState.Spec.SpyreSSAInterfaces = []spyrev1alpha1.SpyreSSAInterface{}
			nodeState.Spec.Pcitopo = ""
			if err := c.client.Update(ctx, &nodeState); err != nil {
				return fmt.Errorf("failed to delete SpyreNodeState: %w", err)
			}
		}
	}
	return nil
}

// Remove assets which owner (SpyreClusterPolicy) not found
// return true if the number of deleted objects
func (c *ControlledComponent) RemoveZombieAssets(ctx context.Context) int {
	deletedCount := 0
	ownerGetResult := make(map[string]bool)
	logger := log.FromContext(ctx).WithName("remove-zombie-assets")
	for i := range c.objects {
		// delete in reverse order
		obj := c.objects[len(c.objects)-1-i]
		owner := c.getSpyreClusterPolicyOwner(ctx, logger, obj)
		if owner != nil {
			found, checked := ownerGetResult[owner.Name]
			if !checked {
				found = c.isOwnerExist(ctx, logger, owner.Name)
				logger.Info("set SpyreClusterPolicy", "name", owner.Name, "found", found)
				ownerGetResult[owner.Name] = found
			}
			if !found {
				logger.Info("try to delete", "id", obj.GetID())
				if err := c.client.Delete(ctx, obj.GetObject()); err == nil {
					deletedCount += 1
				} else if !apiErrors.IsNotFound(err) {
					logger.Info("skip deleting zombie", "id", obj.GetID(), "err", err)
				}
			}
		}
	}
	return deletedCount
}

// getSpyreClusterPolicyOwner returns owner reference with kind = SpyreClusterPolicy
// return  nil if there is no object or target owner reference not found
func (c *ControlledComponent) getSpyreClusterPolicyOwner(ctx context.Context,
	logger logr.Logger, obj ControlledObject) *metav1.OwnerReference {
	fetchObj, err := obj.Fetch(ctx, c.client)
	if err != nil {
		if !apiErrors.IsNotFound(err) {
			logger.Info("skip checking", "id", obj.GetID(), "err", err)
		}
		return nil
	}
	owners := fetchObj.GetOwnerReferences()
	for _, owner := range owners {
		if owner.Kind == "SpyreClusterPolicy" {
			return &owner
		}
	}
	return nil
}

// isOwnerExist returns true if the specified SpyreClusterPolicy exists
func (c *ControlledComponent) isOwnerExist(ctx context.Context, logger logr.Logger, name string) bool {
	cp := &spyrev1alpha1.SpyreClusterPolicy{}
	namespacedName := types.NamespacedName{Name: name}
	err := c.client.Get(ctx, namespacedName, cp)
	if err == nil {
		return true
	} else if apiErrors.IsNotFound(err) {
		return false
	}
	logger.Info("failed to check owner", "SpyreClusterPolicy", name, "err", err)
	return false
}
