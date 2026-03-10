/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"
	"fmt"
	"slices"
	"strings"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	spyreerr "github.com/ibm-aiu/spyre-operator/pkg/errors"
	secv1 "github.com/openshift/api/security/v1"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	secondschedv1 "github.com/openshift/secondary-scheduler-operator/pkg/apis/secondaryscheduler/v1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	objectControllerMap = map[string]func(*DefaultControlledObject, runtime.Object, string) (ControlledObject, error){
		"ServiceAccount":                 NewServiceAccount,
		"Role":                           NewRole,
		"RoleBinding":                    NewRoleBinding,
		"ClusterRole":                    NewClusterRole,
		"ClusterRoleBinding":             NewClusterRoleBinding,
		"ConfigMap":                      NewConfigMap,
		"DaemonSet":                      NewDaemonSet,
		"Deployment":                     NewDeployment,
		"Service":                        NewService,
		"SecurityContextConstraints":     NewSecurityContextConstraints,
		"PrometheusRule":                 NewPrometheusRule,
		"ServiceMonitor":                 NewServiceMonitor,
		"Issuer":                         NewIssuer,
		"Certificate":                    NewCertificate,
		"SecondaryScheduler":             NewSecondScheduler,
		"ValidatingWebhookConfiguration": NewValidatingWebhookConfiguration,
		"NodeFeatureRule":                NewNodeFeatureRule,
		"PersistentVolume":               NewPersistentVolume,
		"PersistentVolumeClaim":          NewPersistentVolumeClaim,
	}
)

// ControlledObject defines abstract of controlled object
type ControlledObject interface {
	GetID() ControlledID
	GetObject() client.Object
	// Fetch gets object from API server and return
	Fetch(context.Context, client.Client) (client.Object, error)
	// Sync gets and checks whether the object is in sync or not, returns synced object
	Sync(context.Context, client.Client) (client.Object, error)
	// Ready checks whether the object is ready for next step
	Ready(context.Context, client.Client) bool
	// TransformByPolicy transform the object using the inputs from clusterPolicy
	TransformByPolicy(scheme *runtime.Scheme, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error
}

// ControlledID identifies controlled object by kind, name, and namespace
type ControlledID struct {
	Kind      string
	Name      string
	Namespace string
}

func (id ControlledID) String() string {
	return fmt.Sprintf("%s/%s (%s)", id.Namespace, id.Name, id.Kind)
}

// DefaultControlledObject defines common functions for default controlled object
type DefaultControlledObject struct {
	ControlledID
	client.Object
	logger logr.Logger
}

func (obj *DefaultControlledObject) GetID() ControlledID {
	return obj.ControlledID
}

// GetObject returns transformed object
func (obj *DefaultControlledObject) GetObject() client.Object {
	obj.Object.SetResourceVersion("")
	return obj.Object
}

// Ready always returns true by default
func (obj *DefaultControlledObject) Ready(context.Context, client.Client) bool {
	return true
}

// TransformByPolicy commonly adds SpyreClusterPolicy as controller reference.
// This will prevent pending controlled object by always deleting the controlled object
// when the SpyreClusterPolicy is deleted.
func (obj *DefaultControlledObject) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	if err := controllerutil.SetControllerReference(clusterPolicy, obj.Object, scheme); err != nil {
		spyreerr.LogErrControllerReferenceSet(obj.logger, err)
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	return nil
}

// syncOwners commonly sync owner reference when Sync is called
func (obj *DefaultControlledObject) syncOwners(metadata *metav1.ObjectMeta) {
	metadata.OwnerReferences = obj.Object.GetOwnerReferences()
}

// NewControlledObject calls new function according to objectControllerMap
func NewControlledObject(ctx context.Context,
	kind string, targetNamespace string, runtimeObj runtime.Object) (ControlledObject, error) {
	defaultObj, err := newDefaultObject(ctx, kind, targetNamespace, runtimeObj)
	if err != nil {
		return nil, fmt.Errorf("failed to get default object: %w", err)
	}
	newFunc, found := objectControllerMap[kind]
	if !found {
		return nil, fmt.Errorf("unsupported kind %s", kind)
	}
	return newFunc(defaultObj, runtimeObj, targetNamespace)
}

// newDefaultObject returns DefaultControlledObject set by runtime object
func newDefaultObject(ctx context.Context,
	kind string, targetNamespace string, runtimeObj runtime.Object) (*DefaultControlledObject, error) {
	clientObj, ok := runtimeObj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("clientObj: %w", spyreerr.ErrParseFile)
	}
	namespace := clientObj.GetNamespace()
	if namespace == spyreconst.OPERATOR_FILLED {
		namespace = targetNamespace
	}
	controlledID := ControlledID{
		Kind:      kind,
		Name:      clientObj.GetName(),
		Namespace: namespace,
	}
	logger := log.FromContext(ctx).WithValues("id", controlledID)
	return &DefaultControlledObject{
		ControlledID: controlledID,
		logger:       logger,
	}, nil
}

// newSpecificDaemonSet returns name-specific controlled DaemonSet
func newSpecificDaemonSet(obj *appsv1.DaemonSet, ds *DaemonSet) ControlledObject {
	switch {
	case obj.Name == spyreconst.DevicePluginResourceName:
		return &DevicePluginDaemonset{
			DaemonSet: ds,
		}
	case obj.Name == spyreconst.MonitorResourceName:
		return &MetricsExporterDaemonset{
			DaemonSet: ds,
		}
	case obj.Name == spyreconst.HealthCheckerResourceName:
		return &HealthCheckerDaemonset{
			DaemonSet: ds,
		}
	case strings.HasPrefix(obj.Name, spyreconst.CardManagementResourceName):
		return &CardManagementDaemonset{
			DaemonSet: ds,
		}
	default:
		return ds
	}
}

// objects with corev1 API

type ServiceAccount struct {
	*DefaultControlledObject
	loadedObj *corev1.ServiceAccount
}

func NewServiceAccount(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*corev1.ServiceAccount); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &ServiceAccount{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *ServiceAccount) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &corev1.ServiceAccount{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *ServiceAccount) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &corev1.ServiceAccount{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	for _, pullSecret := range obj.loadedObj.ImagePullSecrets { // pragma: allowlist secret
		if !slices.Contains(getObj.ImagePullSecrets, pullSecret) {
			getObj.ImagePullSecrets = append(getObj.ImagePullSecrets, pullSecret)
		}
	}
	return getObj, nil
}

type ConfigMap struct {
	*DefaultControlledObject
	loadedObj *corev1.ConfigMap
}

func NewConfigMap(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*corev1.ConfigMap); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		cmObj := ConfigMap{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}
		switch obj.Name {
		case spyreconst.SenlibConfigTmplName:
			return &SenlibConfigTemplateConfigMap{
				ConfigMap: cmObj,
				data:      obj.DeepCopy().Data,
			}, nil
		case spyreconst.CardManagementConfigName:
			return &CardManagementConfigMap{
				ConfigMap: cmObj,
				data:      obj.DeepCopy().Data,
			}, nil
		default:
			return &cmObj, nil
		}
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *ConfigMap) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *ConfigMap) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Data = obj.loadedObj.Data
	return getObj, nil
}

type SenlibConfigTemplateConfigMap struct {
	data map[string]string
	ConfigMap
}

func (obj *SenlibConfigTemplateConfigMap) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Data = map[string]string{}
	for k, v := range obj.data {
		obj.loadedObj.Data[k] = v
	}
	if err := obj.ConfigMap.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform config map: %w", err)
	}
	enabled := metricsExporterEnabled(clusterPolicy)
	if _, err := TransformSenlibConfigTemplate(enabled, obj.loadedObj,
		clusterPolicy.Spec.DevicePlugin.ConfigName); err != nil {
		return fmt.Errorf("failed to update senlib config template: %w", err)
	}
	return nil
}

type CardManagementConfigMap struct {
	data map[string]string
	ConfigMap
}

func (obj *CardManagementConfigMap) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Data = map[string]string{}
	for k, v := range obj.data {
		obj.loadedObj.Data[k] = v
	}
	if err := obj.ConfigMap.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform config map: %w", err)
	}
	if data, found := obj.loadedObj.Data[spyreconst.CardManagementConfigFile]; found {
		newData := replaceWithNamespace(data, cluster.OperatorNamespace)
		if !clusterPolicy.ExperimentalModeEnabled(spyrev1alpha1.DisableVfMode) {
			newData = replaceFirstFalseWithTrue(newData)
		}
		var err error
		if newData, err = obj.replaceConfig(newData, clusterPolicy.Spec.CardManagement.Config); err != nil {
			return fmt.Errorf("failed to replace config: %w", err)
		}
		obj.loadedObj.Data[spyreconst.CardManagementConfigFile] = newData
	}
	return nil
}

func (obj *CardManagementConfigMap) replaceConfig(data string, config *spyrev1alpha1.CardManagementConfig) (string, error) {
	if config == nil || config.SpyreFilter == nil || config.PfRunnerImage == nil || config.VfRunnerImage == nil {
		return data, fmt.Errorf("some values in config is nil %v", config)
	}
	data = replacePlaceHolder(data, spyreconst.FILTER_OPERATOR_FILLED, *config.SpyreFilter)
	data = replacePlaceHolder(data, spyreconst.PFIMAGE_OPERATOR_FILLED, *config.PfRunnerImage)
	data = replacePlaceHolder(data, spyreconst.VFIMAGE_OPERATOR_FILLED, *config.VfRunnerImage)
	return data, nil
}

type Service struct {
	*DefaultControlledObject
	loadedObj *corev1.Service
}

func NewService(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*corev1.Service); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		svc := Service{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}
		if obj.Name == spyreconst.MonitorResourceName {
			return &MetricsExporterService{
				spec:    obj.Spec,
				Service: svc,
			}, nil
		}
		return &svc, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *Service) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &corev1.Service{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *Service) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

func (obj *Service) get(ctx context.Context,
	k8sClient client.Client) (*corev1.Service, error) {
	getObj := &corev1.Service{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	return getObj, err //nolint:wrapcheck
}

func (obj *Service) Ready(ctx context.Context, k8sClient client.Client) bool {
	logger := log.FromContext(ctx)
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		logger.Info("failed to get %s: %w", obj.DefaultControlledObject.GetID(), err)
		return false
	}
	endpointsObj := types.NamespacedName{
		Name:      getObj.Name,
		Namespace: getObj.Namespace,
	}
	endpoints := &corev1.Endpoints{}

	err = k8sClient.Get(ctx, endpointsObj, endpoints)
	if err != nil {
		spyreerr.LogErrGet(logger, err)
		return false
	}
	if len(endpoints.Subsets) == 0 {
		logger.Info("no subsets of endpoints")
		return false
	}
	if len(endpoints.Subsets[0].Addresses) == 0 {
		logger.Info("no addresses of endpoints")
		return false
	}
	logger.Info(fmt.Sprintf("%s is ready", obj.ControlledID))
	return true
}

type MetricsExporterService struct {
	spec corev1.ServiceSpec
	Service
}

func (obj *MetricsExporterService) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec = *obj.spec.DeepCopy()
	if err := obj.Service.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform service: %w", err)
	}
	if port := clusterPolicy.Spec.MetricsExporter.Port; port != nil {
		for i, val := range obj.loadedObj.Spec.Ports {
			if val.Name != spyreconst.MonitorPortName {
				continue
			}
			obj.loadedObj.Spec.Ports[i].Port = *port
			return nil
		}
		newPort := corev1.ServicePort{
			Name:       spyreconst.MonitorPortName,
			Port:       *port,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromString(spyreconst.MonitorPortName),
		}
		obj.loadedObj.Spec.Ports = append(obj.loadedObj.Spec.Ports, newPort)
	}
	return nil
}

type PersistentVolume struct {
	*DefaultControlledObject
	loadedObj *corev1.PersistentVolume
}

func NewPersistentVolume(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*corev1.PersistentVolume); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		pv := PersistentVolume{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}
		return &pv, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *PersistentVolume) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &corev1.PersistentVolume{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *PersistentVolume) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

func (obj *PersistentVolume) get(ctx context.Context,
	k8sClient client.Client) (*corev1.PersistentVolume, error) {
	getObj := &corev1.PersistentVolume{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	return getObj, err //nolint:wrapcheck
}

func (obj *PersistentVolume) Ready(ctx context.Context, k8sClient client.Client) bool {
	logger := log.FromContext(ctx)
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		logger.Info("failed to get %s: %w", obj.DefaultControlledObject.GetID(), err)
		return false
	}
	pvObj := types.NamespacedName{
		Name:      getObj.Name,
		Namespace: getObj.Namespace,
	}
	pv := &corev1.PersistentVolume{}

	err = k8sClient.Get(ctx, pvObj, pv)
	if err != nil {
		spyreerr.LogErrGet(logger, err)
		return false
	}
	return pv.Status.Phase == corev1.VolumeAvailable || pv.Status.Phase == corev1.VolumeBound
}

type PersistentVolumeClaim struct {
	*DefaultControlledObject
	loadedObj *corev1.PersistentVolumeClaim
}

func NewPersistentVolumeClaim(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*corev1.PersistentVolumeClaim); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		pvc := PersistentVolumeClaim{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}
		return &pvc, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *PersistentVolumeClaim) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &corev1.PersistentVolumeClaim{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *PersistentVolumeClaim) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

func (obj *PersistentVolumeClaim) get(ctx context.Context,
	k8sClient client.Client) (*corev1.PersistentVolumeClaim, error) {
	getObj := &corev1.PersistentVolumeClaim{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	return getObj, err //nolint:wrapcheck
}

func (obj *PersistentVolumeClaim) Ready(ctx context.Context, k8sClient client.Client) bool {
	logger := log.FromContext(ctx)
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		logger.Info("failed to get %s: %w", obj.DefaultControlledObject.GetID(), err)
		return false
	}
	pvcObj := types.NamespacedName{
		Name:      getObj.Name,
		Namespace: getObj.Namespace,
	}
	pvc := &corev1.PersistentVolumeClaim{}

	err = k8sClient.Get(ctx, pvcObj, pvc)
	if err != nil {
		spyreerr.LogErrGet(logger, err)
		return false
	}
	return pvc.Status.Phase == corev1.ClaimBound
}

// objects with appsv1 API

type DaemonSet struct {
	*DefaultControlledObject
	loadedObj *appsv1.DaemonSet
	spec      corev1.PodTemplateSpec
}

func NewDaemonSet(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*appsv1.DaemonSet); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		ds := &DaemonSet{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
			spec:                    obj.Spec.Template,
		}
		return newSpecificDaemonSet(obj, ds), nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *DaemonSet) get(ctx context.Context,
	k8sClient client.Client) (*appsv1.DaemonSet, error) {
	getObj := &appsv1.DaemonSet{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	return getObj, err //nolint:wrapcheck
}

func (obj *DaemonSet) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &appsv1.DaemonSet{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *DaemonSet) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

func (obj *DaemonSet) Ready(ctx context.Context, k8sClient client.Client) bool {
	logger := log.FromContext(ctx)
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		logger.Info("failed to get %s: %w", obj.DefaultControlledObject.GetID(), err)
		return false
	}

	if strings.HasPrefix(obj.Name, "spyre-card-management") {
		s := getObj.Status
		n := s.DesiredNumberScheduled
		ok := s.CurrentNumberScheduled == n && s.NumberReady == n &&
			s.UpdatedNumberScheduled == n && s.NumberAvailable == n
		if !ok {
			logger.Info("not in a desired status", "name", getObj.Name,
				"desired", n, "current", s.CurrentNumberScheduled,
				"ready", s.NumberReady, "up-to-date", s.UpdatedNumberScheduled,
				"available", getObj.Status.NumberAvailable)
		}
		return ok
	}

	if getObj.Status.NumberReady == 0 {
		logger.Info("number of ready pods in daemon set zero", "name", getObj.Name)
		return false
	}
	if getObj.Status.NumberUnavailable > 0 {
		logger.Info("number of unavailable pods in daemon set greater than zero",
			"name", getObj.Name, "unavailable", getObj.Status.NumberUnavailable)
		return false
	}
	return true
}

type DevicePluginDaemonset struct {
	*DaemonSet
}

func (obj *DevicePluginDaemonset) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec.Template = *obj.spec.DeepCopy()
	if err := obj.DefaultControlledObject.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform default controlled object: %w", err)
	}
	if err := TransformDevicePlugin(obj.loadedObj, &clusterPolicy.Spec, cluster.nodeArchitecture,
		cluster.GetLogLevel(), cluster.topologyConfigMapExist); err != nil {
		return fmt.Errorf("failed to transform device plugin: %w", err)
	}
	return nil
}

type MetricsExporterDaemonset struct {
	*DaemonSet
}

func (obj *MetricsExporterDaemonset) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec.Template = *obj.spec.DeepCopy()
	if err := obj.DefaultControlledObject.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform default controlled object: %w", err)
	}
	if err := TransformMetricsExporter(obj.loadedObj, &clusterPolicy.Spec, cluster.nodeArchitecture); err != nil {
		return fmt.Errorf("failed to transform metrics exporter: %w", err)
	}
	return nil
}

type HealthCheckerDaemonset struct {
	*DaemonSet
}

func (obj *HealthCheckerDaemonset) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec.Template = *obj.spec.DeepCopy()
	if err := obj.DefaultControlledObject.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform default controlled object: %w", err)
	}
	if err := TransformHealthChecker(obj.loadedObj, &clusterPolicy.Spec, cluster.GetLogLevel()); err != nil {
		return fmt.Errorf("failed to transform device plugin: %w", err)
	}
	return nil
}

type Deployment struct {
	*DefaultControlledObject
	loadedObj *appsv1.Deployment
	spec      corev1.PodTemplateSpec
}

func NewDeployment(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*appsv1.Deployment); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		deploy := &Deployment{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
			spec:                    obj.Spec.Template,
		}
		switch obj.Name {
		case spyreconst.ValidatorResourceName:
			return &PodValidatorDeployment{
				Deployment: deploy,
			}, nil
		default:
			return deploy, nil
		}
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *Deployment) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &appsv1.Deployment{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *Deployment) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

func (obj *Deployment) get(ctx context.Context,
	k8sClient client.Client) (*appsv1.Deployment, error) {
	getObj := &appsv1.Deployment{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	return getObj, err //nolint:wrapcheck
}

func (obj *Deployment) Ready(ctx context.Context, k8sClient client.Client) bool {
	logger := log.FromContext(ctx)
	getObj, err := obj.get(ctx, k8sClient)
	if err != nil {
		logger.Info("failed to get object", "id", obj.DefaultControlledObject.GetID(), "error", err)
		return false
	}
	return getObj.Status.UnavailableReplicas == 0 && getObj.Status.AvailableReplicas > 0
}

type PodValidatorDeployment struct {
	*Deployment
}

func (obj *PodValidatorDeployment) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec.Template = *obj.spec.DeepCopy()
	if err := obj.DefaultControlledObject.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform default controlled object: %w", err)
	}
	if err := TransformPodValidator(obj.loadedObj, clusterPolicy); err != nil {
		return fmt.Errorf("failed to transform validator: %w", err)
	}
	return nil
}

// Ready of PodValidatorDeployment needs to skip to let service deployed next
func (obj *PodValidatorDeployment) Ready(ctx context.Context, k8sClient client.Client) bool {
	logger := log.FromContext(ctx)
	_, err := obj.get(ctx, k8sClient)
	if err != nil {
		logger.Info("failed to get %s: %w", obj.DefaultControlledObject.GetID(), err)
		return false
	}
	return true
}

type CardManagementDaemonset struct {
	*DaemonSet
}

func (obj *CardManagementDaemonset) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec.Template = *obj.spec.DeepCopy()
	if err := obj.DefaultControlledObject.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform default controlled object: %w", err)
	}
	if err := TransformCardManagement(obj.loadedObj, clusterPolicy, cluster, cluster.cardMgmtPvcExist); err != nil {
		return fmt.Errorf("failed to transform card management: %w", err)
	}
	return nil
}

// objects with rbacv1 API

type Role struct {
	*DefaultControlledObject
	loadedObj *rbacv1.Role
}

func NewRole(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*rbacv1.Role); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &Role{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *Role) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &rbacv1.Role{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *Role) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &rbacv1.Role{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Rules = obj.loadedObj.Rules
	return getObj, nil
}

type RoleBinding struct {
	*DefaultControlledObject
	loadedObj *rbacv1.RoleBinding
}

func NewRoleBinding(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*rbacv1.RoleBinding); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		for idx := range obj.Subjects {
			if obj.Subjects[idx].Namespace == spyreconst.OPERATOR_FILLED {
				obj.Subjects[idx].Namespace = targetNamespace
			}
		}
		return &RoleBinding{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *RoleBinding) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &rbacv1.RoleBinding{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *RoleBinding) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &rbacv1.RoleBinding{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Subjects = obj.loadedObj.Subjects
	getObj.RoleRef = obj.loadedObj.RoleRef
	return getObj, nil
}

type ClusterRole struct {
	*DefaultControlledObject
	loadedObj *rbacv1.ClusterRole
}

func NewClusterRole(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*rbacv1.ClusterRole); ok {
		defaultObj.Object = obj
		return &ClusterRole{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *ClusterRole) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &rbacv1.ClusterRole{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *ClusterRole) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &rbacv1.ClusterRole{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Rules = obj.loadedObj.Rules
	getObj.AggregationRule = obj.loadedObj.AggregationRule
	return getObj, nil
}

type ClusterRoleBinding struct {
	*DefaultControlledObject
	loadedObj *rbacv1.ClusterRoleBinding
}

func NewClusterRoleBinding(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*rbacv1.ClusterRoleBinding); ok {
		defaultObj.Object = obj
		for idx := range obj.Subjects {
			if obj.Subjects[idx].Namespace == spyreconst.OPERATOR_FILLED {
				obj.Subjects[idx].Namespace = targetNamespace
			}
		}
		return &ClusterRoleBinding{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *ClusterRoleBinding) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &rbacv1.ClusterRoleBinding{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *ClusterRoleBinding) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &rbacv1.ClusterRoleBinding{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Subjects = obj.loadedObj.Subjects
	getObj.RoleRef = obj.loadedObj.RoleRef
	return getObj, nil
}

// objects with secv1 API

type SecurityContextConstraints struct {
	*DefaultControlledObject
	loadedObj *secv1.SecurityContextConstraints
}

func NewSecurityContextConstraints(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*secv1.SecurityContextConstraints); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &SecurityContextConstraints{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *SecurityContextConstraints) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &secv1.SecurityContextConstraints{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *SecurityContextConstraints) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &secv1.SecurityContextConstraints{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	// skip content sync
	return getObj, nil
}

// objects with promv1 API

type PrometheusRule struct {
	*DefaultControlledObject
	loadedObj *promv1.PrometheusRule
}

func NewPrometheusRule(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*promv1.PrometheusRule); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &PrometheusRule{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *PrometheusRule) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &promv1.PrometheusRule{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *PrometheusRule) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &promv1.PrometheusRule{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

type ServiceMonitor struct {
	*DefaultControlledObject
	loadedObj *promv1.ServiceMonitor
}

func NewServiceMonitor(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*promv1.ServiceMonitor); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		for idx := range obj.Spec.NamespaceSelector.MatchNames {
			if obj.Spec.NamespaceSelector.MatchNames[idx] != spyreconst.OPERATOR_FILLED {
				continue
			}
			obj.Spec.NamespaceSelector.MatchNames[idx] = targetNamespace
		}
		return &ServiceMonitor{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *ServiceMonitor) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &promv1.ServiceMonitor{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *ServiceMonitor) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &promv1.ServiceMonitor{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

// Issuer
type Issuer struct {
	*DefaultControlledObject
	loadedObj *certmanagerv1.Issuer
}

func NewIssuer(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*certmanagerv1.Issuer); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &Issuer{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *Issuer) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &certmanagerv1.Issuer{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *Issuer) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &certmanagerv1.Issuer{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

// Certificate
type Certificate struct {
	*DefaultControlledObject
	loadedObj *certmanagerv1.Certificate
}

func NewCertificate(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*certmanagerv1.Certificate); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &Certificate{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *Certificate) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &certmanagerv1.Certificate{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *Certificate) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &certmanagerv1.Certificate{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

// objects with admissionv1 API

type ValidatingWebhookConfiguration struct {
	*DefaultControlledObject
	loadedObj *admissionv1.ValidatingWebhookConfiguration
}

func NewValidatingWebhookConfiguration(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*admissionv1.ValidatingWebhookConfiguration); ok {
		for idx := range obj.Webhooks {
			if obj.Webhooks[idx].ClientConfig.Service.Namespace == spyreconst.OPERATOR_FILLED {
				obj.Webhooks[idx].ClientConfig.Service.Namespace = targetNamespace
			}
		}
		defaultObj.Object = obj
		return &ValidatingWebhookConfiguration{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *ValidatingWebhookConfiguration) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &admissionv1.ValidatingWebhookConfiguration{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *ValidatingWebhookConfiguration) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &admissionv1.ValidatingWebhookConfiguration{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Webhooks = obj.loadedObj.Webhooks
	return getObj, nil
}

// objects with nfdv1alpha1 API

type NodeFeatureRule struct {
	*DefaultControlledObject
	loadedObj *nfdv1alpha1.NodeFeatureRule
}

func NewNodeFeatureRule(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*nfdv1alpha1.NodeFeatureRule); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &NodeFeatureRule{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *NodeFeatureRule) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &nfdv1alpha1.NodeFeatureRule{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *NodeFeatureRule) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &nfdv1alpha1.NodeFeatureRule{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

// objects with secondschedv1 API

type SecondaryScheduler struct {
	*DefaultControlledObject
	loadedObj *secondschedv1.SecondaryScheduler
	spec      secondschedv1.SecondarySchedulerSpec
}

func NewSecondScheduler(defaultObj *DefaultControlledObject,
	runtimeObj runtime.Object, targetNamespace string) (ControlledObject, error) {
	if obj, ok := runtimeObj.(*secondschedv1.SecondaryScheduler); ok {
		obj.Namespace = defaultObj.Namespace
		defaultObj.Object = obj
		return &SecondaryScheduler{
			DefaultControlledObject: defaultObj,
			loadedObj:               obj,
			spec:                    obj.Spec,
		}, nil
	}
	return nil, spyreerr.ErrParseFile
}

func (obj *SecondaryScheduler) Fetch(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	fetchObj := &secondschedv1.SecondaryScheduler{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, fetchObj)
	return fetchObj, err //nolint:wrapcheck
}

func (obj *SecondaryScheduler) Sync(ctx context.Context,
	k8sClient client.Client) (client.Object, error) {
	getObj := &secondschedv1.SecondaryScheduler{}
	namespacedName := types.NamespacedName{Name: obj.loadedObj.Name, Namespace: obj.loadedObj.Namespace}
	err := k8sClient.Get(ctx, namespacedName, getObj)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	obj.syncOwners(&getObj.ObjectMeta)
	getObj.Spec = obj.loadedObj.Spec
	return getObj, nil
}

func (obj *SecondaryScheduler) TransformByPolicy(scheme *runtime.Scheme,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	obj.loadedObj.Spec = *obj.spec.DeepCopy()
	if err := obj.DefaultControlledObject.TransformByPolicy(scheme, clusterPolicy, cluster); err != nil {
		return fmt.Errorf("failed to transform default controlled object: %w", err)
	}
	config := clusterPolicy.Spec.Scheduler
	image, err := spyrev1alpha1.ImagePath(config.Repository, config.Image, config.Version)
	if err != nil {
		obj.logger.Error(err, "failed to get scheduler image path")
		return fmt.Errorf("failed o get scheduler image path: %w", err)
	}
	obj.loadedObj.Spec.SchedulerImage = image
	return nil
}

func replaceWithNamespace(content string, namespace string) string {
	return strings.ReplaceAll(content, spyreconst.OPERATOR_FILLED, namespace)
}

func replaceFirstFalseWithTrue(content string) string {
	return strings.Replace(content, "False", "True", 1)
}

func replacePlaceHolder(content, placeHolder, value string) string {
	return strings.ReplaceAll(content, placeHolder, value)
}
