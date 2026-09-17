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
	draDriverLabel                    = "app=dra-driver-spyre"
	nfdWorkerLabel                    = "app=nfd-worker"
	cardManagementLabel               = "app=cardmgmt"
	metricsExporterLabel              = "app=spyre-metrics-exporter"
	podValidatorLabel                 = "app=spyre-webhook-validator"
	healthCheckerLabel                = "app=spyre-health-checker"
	packageServerLabel                = "app=packageserver"
	cardManagementName                = "spyre-card-management"
	metricsExporterName               = "spyre-metrics-exporter"
	healthCheckerName                 = "spyre-health-checker"
	healthCheckerMetricsPort          = 8081
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

	// MockCardmgmtSidecarName is the name used for the mock aiu-cardmgmt-health-api
	// DaemonSet, ServiceAccount, and RBAC objects deployed during pseudo-device tests.
	MockCardmgmtSidecarName = "aiu-cardmgmt-health-check-api-mock"

	// MockCardmgmtSidecarLabel is the pod label selector for the mock DaemonSet.
	MockCardmgmtSidecarLabel = "app=aiu-cardmgmt-health-check-api-mock"

	// PseudoUnhealthyPCISlot is the PCI address that PSEUDO_DEVICE_MODE always
	// reports as "unhealthy".  It must match PSEUDO_UNHEALTHY_PCI_SLOT_PREFIX
	// ("0000:41") in aiu-cardmgmt-health-api/src/grpc_service.py.
	PseudoUnhealthyPCISlot = "0000:41:00.0"

	// mockCardmgmtHostPath is the hostPath the mock sidecar writes its UNIX
	// socket into.  It must match the cardmgmt-health-api-socket volume in
	// assets/state-init/spyre-health-checker/0400_daemonset.yaml so that the
	// health-checker pods on the same node can reach the socket.
	mockCardmgmtHostPath = "/var/run/cardmgmt-health-check-api"

	// mockCardmgmtSocketFile is the socket filename inside mockCardmgmtHostPath.
	mockCardmgmtSocketFile = "health-check-api.sock"
)
