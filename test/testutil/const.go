/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

const (
	TestConfigFilePathKey             = "TEST_CONFIG"
	KubeConfigFilePathKey             = "E2E_KUBECONFIG"
	OperatorNamespace                 = "spyre-operator"
	MarketPlaceNamespace              = "openshift-marketplace"
	OperatorLifecycleManagerNamespace = "openshift-operator-lifecycle-manager"
	NodeFeatureDiscoveryNamespace     = "openshift-nfd"
	ClusterPolicyName                 = "spyreclusterpolicy"
	SubscriptionName                  = "spyre-operator"
	OperatorName                      = "spyre-operator"
	devicePluginName                  = "spyre-device-plugin"
	CatalogSourceName                 = "ibm-spyre-operators"
	OperatorGroupName                 = "spyre-operator-group"
	managerContainerName              = "manager"
	monitorVolumeName                 = "monitor-data"
	nfdInstanceName                   = "nfd-instance"
	devicePluginContainerName         = "spyre-device-plugin"
	operatorLabel                     = "control-plane=spyre-operator"
	devicePluginLabel                 = "app=spyre-device-plugin"
	nfdWorkerLabel                    = "app=nfd-worker"
	cardManagementLabel               = "app=cardmgmt"
	metricsExporterLabel              = "app=spyre-metrics-exporter"
	podValidatorLabel                 = "app=spyre-webhook-validator"
	healthCheckerLabel                = "app=spyre-health-checker"
	packageServerLabel                = "app=packageserver"
	cardManagementName                = "spyre-card-management"
	metricsExporterName               = "spyre-metrics-exporter"
	podValidatorName                  = "spyre-webhook-validator"
	podResource                       = "pods.v1."
	catalogSourceResource             = "catalogsources.v1alpha1.operators.coreos.com"
	clusterServiceVersionResource     = "clusterserviceversions.v1alpha1.operators.coreos.com"
	installPlanResource               = "installplans.v1alpha1.operators.coreos.com"
	operatorGroupResource             = "operatorgroups.v1.operators.coreos.com"
	subscriptionResource              = "subscriptions.v1alpha1.operators.coreos.com"
	customresourcedefinitionResource  = "customresourcedefinitions.v1.apiextensions.k8s.io"
	SpyreResourcePrefix               = "ibm.com/spyre_pf"
	containerTestImage                = "registry.access.redhat.com/ubi9-minimal:9.4"
	Ubi9MicroTestImage                = "registry.access.redhat.com/ubi9/ubi-micro:latest"
	schedForcePullPodLabel            = "ibm-spyre-force-image-puller"
)
