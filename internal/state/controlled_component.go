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
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	spyreerr "github.com/ibm-aiu/spyre-operator/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
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
}

func NewControlledComponent(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme,
	componentPath, namespace, name string) (*ControlledComponent, error) {
	files, err := filePathWalkDir(componentPath, []string{".yaml"})
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
	}
	return nil
}

func (c *ControlledComponent) Sync(ctx context.Context) (bool, string, error) {
	logger := log.FromContext(ctx).WithValues("Component", c.name, "Target Namespace", c.namespace,
		"disable", c.disabled, "skipUpdate", c.skipUpdate)
	if c.disabled {
		var multiErr error
		if c.name == spyreconst.CardManagementResourceName {
			if err := c.deleteCardManagementRunners(ctx); err != nil {
				multiErr = multierror.Append(multiErr, err)
			}
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
	// component is not disabled, continue deploying
	for i, obj := range c.objects {
		// previous step not ready
		if i > 0 && !c.objects[i-1].Ready(ctx, c.client) {
			return false, fmt.Sprintf("previous step %s is not ready", c.objects[i-1].GetID()), nil
		}
		syncedObject, err := obj.Sync(ctx, c.client)
		if apiErrors.IsNotFound(err) {
			if err := c.client.Create(ctx, obj.GetObject()); err != nil && !apiErrors.IsAlreadyExists(err) {
				spyreerr.LogErrCreate(logger, err)
				msg := fmt.Sprintf("failed to create %s: %v", obj.GetID(), err)
				return false, msg, errors.New(msg)
			}
			continue
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
	}
	ready := c.Ready(ctx)
	message := ""
	if ready {
		logger.Info(fmt.Sprintf("%s is ready", c.name))
	} else {
		message = "last step is not ready"
	}
	return ready, message, nil
}

func (c *ControlledComponent) deleteAll(ctx context.Context, logger logr.Logger) error {
	failedList := []string{}
	for i := range c.objects {
		// delete in reverse order
		deleteObj := c.objects[len(c.objects)-1-i]
		if err := c.client.Delete(ctx, deleteObj.GetObject()); err != nil && !apiErrors.IsNotFound(err) {
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
		logger.V(2).Info("failed to list pods", "labels", labels, "error", err)
		return fmt.Errorf("failed to list pod with labels %v: %w", labels, err)
	}
	var deleteErr error
	// Delete each pod
	for _, pod := range podList.Items {
		if err := c.client.Delete(ctx, &pod); err != nil {
			logger.V(2).Info("failed to delete pod", "name", pod.Name, "error", err)
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
	case spyreconst.CardManagementResourceName:
		return c.deleteCardManagementRunners(ctx)
	case spyreconst.DevicePluginResourceName:
		return c.clearAllSpyreNodeStates(ctx)
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
