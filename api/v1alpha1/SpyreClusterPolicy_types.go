/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package v1alpha1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Component describes a operator-controlled component name
// +enum
// +kubebuilder:validation:Enum=commonInit;devicePlugin;cardManagement;metricsExporter;scheduler;podValidator;healthChecker
type Component string

const (
	ComponentCommonInit      Component = "commonInit"
	ComponentDevicePlugin    Component = "devicePlugin"
	ComponentCardManagement  Component = "cardManagement"
	ComponentMetricsExporter Component = "metricsExporter"
	ComponentScheduler       Component = "scheduler"
	ComponentPodValidator    Component = "podValidator"
	ComponentHealthChecker   Component = "healthChecker"
)

// DeploymentConfig defines common embedded fields of pod's deployment
type DeploymentConfig struct {
	// image repository
	// +kubebuilder:validation:Optional
	Repository string `json:"repository,omitempty"`

	// image name
	// +kubebuilder:validation:Pattern=[a-zA-Z0-9\-]+
	Image string `json:"image,omitempty"`

	// image tag
	// +kubebuilder:validation:Optional
	Version string `json:"version,omitempty"`

	// Image pull policy
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image Pull Policy",xDescriptors="urn:alm:descriptor:com.tectonic.ui:imagePullPolicy"
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`

	// Image pull secrets
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image pull secrets",xDescriptors="urn:alm:descriptor:io.kubernetes:Secret"
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`

	// Optional: Define resources requests and limits for each pod
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Resource Requirements",xDescriptors="urn:alm:descriptor:com.tectonic.ui:advanced,urn:alm:descriptor:com.tectonic.ui:resourceRequirements"
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Optional: List of arguments
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Arguments",xDescriptors="urn:alm:descriptor:com.tectonic.ui:advanced,urn:alm:descriptor:com.tectonic.ui:text"
	Args []string `json:"args,omitempty"`

	// Optional: List of environment variables
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Environment Variables",xDescriptors="urn:alm:descriptor:com.tectonic.ui:advanced,urn:alm:descriptor:com.tectonic.ui:text"
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Defines which Nodes the Pod is scheduled on
	// +kubebuilder:validation:Optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// SpyreClusterPolicySpec defines the desired state of IBM Spyre Operator
type SpyreClusterPolicySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// SpyreDevicePlugin component spec
	DevicePlugin DevicePluginSpec `json:"devicePlugin"`

	// CardManagement spec
	// +kubebuilder:validation:Optional
	CardManagement CardManagementSpec `json:"cardManagement"`

	// MetricsExporter spec
	// +kubebuilder:validation:Optional
	MetricsExporter MetricsExporterSpec `json:"metricsExporter"`

	// Scheduler spec
	// +kubebuilder:validation:Optional
	Scheduler SchedulerSpec `json:"scheduler"`

	// Pod Validator spec
	// +kubebuilder:validation:Optional
	PodValidator PodValidatorSpec `json:"podValidator"`

	// Health Checker spec
	// +kubebuilder:validation:Optional
	HealthChecker HealthCheckerSpec `json:"healthChecker"`

	// ExperimentalMode lists experimental modes to be enabled. Available modes are:
	//
	// `pseudoDevice` applies pseudo devices and pseudo topology for a testing purpose.
	//
	// `perDeviceAllocation` allows devices to be specified by PCI address suffix (e.g., `spyre_pf_000a`).
	//
	// `topologyAwareAllocation` enables policy-based topology-aware allocation (e.g., `spyre_pf_tier0`).
	//
	// `externalDeviceReservation` uses the second scheduler defined in `.spec.scheduler` to provide reservation.
	//
	// `disableVirtualFunction` ignores vf devices regardless of availability (no spyre_vf available).
	//
	ExperimentalMode []SpyreClusterPolicyExperimentalMode `json:"experimentalMode,omitempty"`

	// +kubebuilder:default=info
	// +kubebuilder:validation:Enum=debug;info;error
	// Loglevel of Operator (debug, info or error)
	LogLevel *string `json:"loglevel,omitempty"`

	// SkipUpdateComponents prevents syncing the listed component objects if they have already been deployed.
	// Allows admins to customize these components without reverting to the template.
	SkipUpdateComponents []Component `json:"skipUpdateComponents,omitempty"`
}

// DevicePluginSpec defines the properties for device-plugin deployment
type DevicePluginSpec struct {
	// DeviceConfigSpec defines device config path and filename
	DeviceConfigSpec `json:",inline"`

	// DeploymentConfig defines embedded common fields
	DeploymentConfig `json:",inline"`

	// ExternalInitContainerSpec defines an init container
	// that generates topology and metadata.
	// This init container requires full physical device access.
	// It is mounted to device path and share results in device-plugin-config volume
	// +kubebuilder:validation:Optional
	InitContainer *ExternalInitContainerSpec `json:"initContainer,omitempty"`

	// +kubebuilder:default=false
	// +kubebuilder:validation:Optional
	// P2PDMA configuration enablement on device plugin
	// This field is applicable only for amd64 architecture.
	P2PDMA bool `json:"p2pDMA,omitempty"`
}

// ExecutePolicy describes a policy for if/when to generate the topology file
// +kubebuilder:validation:Enum=Always;IfNotPresent
type ExecutePolicy string

const (
	// ExecuteAlways means that the tool always executes generate command
	ExecuteAlways ExecutePolicy = "Always"
	// ExecuteIfNotPresent means that the tool will executes generate command if the topology file doesn't present
	ExecuteIfNotPresent ExecutePolicy = "IfNotPresent"
)

type ExternalInitContainerSpec struct {
	// ExecutePolicy defines the policy to generate topology file
	*ExecutePolicy `json:"executePolicy,omitempty"`

	// DeploymentConfig defines deployment configuration of init container
	DeploymentConfig `json:",inline"`
}

// DeviceConfigSpec defines the configuration for Spyre devices
type DeviceConfigSpec struct {
	// +kubebuilder:default=/etc/aiu
	// ConfigPath defines a target config path at container
	ConfigPath string `json:"configPath,omitempty"`

	// +kubebuilder:default=senlib_config.json
	// ConfigName defines a target config filename at container
	ConfigName string `json:"configName,omitempty"`

	// TopologyConfigMapName defines a topology config map name
	TopologyConfigMapName string `json:"topologyConfigMap,omitempty"`
}

// CardManagementSpec defines the properties for Spyre Card Management deployment
type CardManagementSpec struct {
	// Enabled indicates if deployment of Spyre Card Management through operator is enabled
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable Spyre Card Management through IBM Spyre Operator",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// DeploymentConfig defines embedded common fields
	DeploymentConfig `json:",inline"`

	// CardManagementConfig defines values to set for cardmanagement config (spyrecardmgmg.ini).
	// +kubebuilder:default={}
	Config *CardManagementConfig `json:"config,omitempty"`
}

// CardManagementConfig defines configurable values for card management component.
type CardManagementConfig struct {
	// SpyreFilter specifies spyrefilter value
	// +kubebuilder:default=.
	SpyreFilter *string `json:"spyreFilter,omitempty" yaml:"spyreFilter,omitempty"`

	// PfRunnerImage specifies pfimage_URL
	// +kubebuilder:default="docker.io/spyre-operator/spyredriver-image:latest"
	PfRunnerImage *string `json:"pfRunnerImage,omitempty" yaml:"pfRunnerImage,omitempty"`

	// VfRunnerImage specifies vfimage_URL
	// +kubebuilder:default="docker.io/spyre-operator/spyredriver-image:latest"
	VfRunnerImage *string `json:"vfRunnerImage,omitempty" yaml:"vfRunnerImage,omitempty"`
}

// MetricsExporterSpec defines the properties for Spyre Metrics Exporter deployment
type MetricsExporterSpec struct {
	// Enabled indicates if deployment of Spyre Metrics Exporter through operator is enabled
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable Spyre Metrics Exporter deployment through IBM Spyre Operator",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// +kubebuilder:default=/data
	// MetricsPath defines a target metrics path at container
	MetricsPath string `json:"metricsPath,omitempty"`

	// DeploymentConfig defines embedded common fields
	DeploymentConfig `json:",inline"`

	// +kubebuilder:default=8082
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Optional
	Port *int32 `json:"port,omitempty"`
}

// SchedulerSpec defines the properties for device-plugin deployment
type SchedulerSpec struct {
	// DeploymentConfig defines embedded common fields
	DeploymentConfig `json:",inline"`
}

// PodValidatorSpec defines the properties for pod validator deployment
type PodValidatorSpec struct {
	// Enabled indicates if deployment of Spyre webhook for pod validation through operator is enabled
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable Spyre Webhook deployment through IBM Spyre Operator",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// DeploymentConfig defines embedded common fields
	DeploymentConfig `json:",inline"`

	// Number of desired pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`
}

// HealthChecker defines the properties for auto-pilot daemonset
type HealthCheckerSpec struct {
	// Enabled indicates if deployment of Spyre Health Checker through operator is enabled
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable Spyre Health Checker deployment through IBM Spyre Operator",xDescriptors="urn:alm:descriptor:com.tectonic.ui:booleanSwitch"
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// DeploymentConfig defines embedded common fields
	DeploymentConfig `json:",inline"`
}

// State indicates state of IBM Spyre operator components
type State string

const (
	// Ignored indicates duplicate Spyre instances and rest are ignored.
	Ignored State = "ignored"
	// Ready indicates all components of Spyre are ready
	Ready State = "ready"
	// NotReady indicates some/all components of Spyre are not ready
	NotReady State = "not ready"
	// NoSpyreNodes indicates no worker nodes with Spyre cards
	NoSpyreNodes State = "no Spyre nodes"
	// NoNFD indicates that node feature discovery operator has not been installed successfully
	NoNFD State = "no node feature discovery"
)

// to add/remove a mode, you need to modify three places:
//   1. add/remove mode value to/from kubebuilder marker
//   2. add/remove constant
//   3. add/remove value to/from func EnvKey()

// SpyreClusterPolicyExperimentalMode specifies available experimental modes
// +kubebuilder:validation:Enum=pseudoDevice;perDeviceAllocation;topologyAwareAllocation;externalDeviceReservation;disableVirtualFunction
type SpyreClusterPolicyExperimentalMode string

const (
	// PseudoDeviceMode applies pseudo devices and pseudo topology for testing purpose
	PseudoDeviceMode SpyreClusterPolicyExperimentalMode = "pseudoDevice"
	// PerDeviceAllocationMode allows devices to be specified by PCI address suffix.
	PerDeviceAllocationMode SpyreClusterPolicyExperimentalMode = "perDeviceAllocation"
	// TopologyAwareAllocationMode enables policy-based topology-aware allocation.
	TopologyAwareAllocationMode SpyreClusterPolicyExperimentalMode = "topologyAwareAllocation"
	// ReservationMode uses second scheduler to provide reservation
	ReservationMode SpyreClusterPolicyExperimentalMode = "externalDeviceReservation"
	// DisableVfMode ignores vf devices regardless of availability
	DisableVfMode SpyreClusterPolicyExperimentalMode = "disableVirtualFunction"
)

func (m SpyreClusterPolicyExperimentalMode) EnvKey() string {
	switch m {
	case PerDeviceAllocationMode:
		return "PER_DEVICE_ALLOCATION_MODE"
	case PseudoDeviceMode:
		return "PSEUDO_DEVICE_MODE"
	case TopologyAwareAllocationMode:
		return "TOPOLOGY_AWARE_ALLOCATION_MODE"
	case ReservationMode:
		return "EXTERNAL_DEVICE_RESERVATION_MODE"
	case DisableVfMode:
		return "DISABLE_VF_MODE"
	}
	return ""
}

//--------------------------------->>

type Interface struct {
	PciAddress string `json:"pciAddress"`
	//	NumVfs     int    `json:"numVfs,omitempty"`
	//	Mtu         int       `json:"mtu,omitempty"`
	Name string `json:"name,omitempty"`
}

/*
type Interfaces []Interface
*/
type InterfaceExt struct {
	Name       string `json:"name,omitempty"`
	Driver     string `json:"driver,omitempty"`
	PciAddress string `json:"pciAddress"`
	Vendor     string `json:"vendor,omitempty"`
	DeviceID   string `json:"deviceID,omitempty"`
	//	NumVfs      int               `json:"numVfs,omitempty"`
	//	TotalVfs    int               `json:"totalvfs,omitempty"`
	//	VFs         []VirtualFunction `json:"Vfs,omitempty"`
}
type InterfaceExts []InterfaceExt

// DevicePluginsStateStatus defines the observed state of DevicePluginsState
type DevicePluginsStateStatus struct {
	Interfaces    InterfaceExts `json:"interfaces,omitempty"`
	SyncStatus    string        `json:"syncStatus,omitempty"`
	LastSyncError string        `json:"lastSyncError,omitempty"`
}

// SpyreClusterPolicyStatus defines the observed state of Spyre
type SpyreClusterPolicyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// State indicates status of Spyre device plugins
	DevicePlugins DevicePluginsStateStatus `json:"devicePluginsStateStatus"`

	// State indicates status of Spyre
	State State `json:"state"`

	// Namespace indicates a namespace in which the operator is installed
	Namespace string `json:"namespace,omitempty"`

	// Message describes reason of not-ready or ignored status (empty if ready)
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=spyrepol,scope=Cluster
//+kubebuilder:subresource:status

// SpyreClusterPolicy is the Schema for the Spyre API
// +operator-sdk:csv:customresourcedefinitions:displayName="Spyre Cluster Policy"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
type SpyreClusterPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SpyreClusterPolicySpec   `json:"spec,omitempty"`
	Status SpyreClusterPolicyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SpyreClusterPolicyList contains a list of SpyreClusterPolicy
type SpyreClusterPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SpyreClusterPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SpyreClusterPolicy{}, &SpyreClusterPolicyList{})
}

// SetStatus sets state and namespace of SpyreClusterPolicy instance
func (p *SpyreClusterPolicy) SetStatus(s State, ns, message string) {
	p.Status.State = s
	p.Status.Namespace = ns
	p.Status.Message = message
}

// ExperimentalModeEnabled checks whether a specific experimental mode is enabled
func (p *SpyreClusterPolicySpec) ExperimentalModeEnabled(mode SpyreClusterPolicyExperimentalMode) bool {
	if p.ExperimentalMode != nil {
		for _, enabledMode := range p.ExperimentalMode {
			if mode == enabledMode {
				return true
			}
		}
	}
	return false
}

// ExperimentalModeEnabled delegates to SpyreClusterPolicySpec for checking whether a specific experimental mode is enabled
func (p *SpyreClusterPolicy) ExperimentalModeEnabled(mode SpyreClusterPolicyExperimentalMode) bool {
	return p.Spec.ExperimentalModeEnabled(mode)
}

func ImagePath(repository string, image string, version string) (string, error) {
	// ImagePath is obtained as follows. (i.e through repository/image/path variables in CRD)
	var crdImagePath string

	if image == "" {
		if repository == "" && version == "" {
			return "", nil
		} else {
			return "", fmt.Errorf("invalid combination of image in SpyreClusterPolicy CR: repository: \"%s\", image: \"%s\", version: \"%s\"", repository, image, version)
		}
	}

	crdImagePath = image
	if version != "" {
		if strings.HasPrefix(version, "sha256:") {
			crdImagePath = crdImagePath + "@" + version
		} else {
			crdImagePath = crdImagePath + ":" + version
		}
	}
	if repository != "" {
		crdImagePath = repository + "/" + crdImagePath
	}
	return crdImagePath, nil
}

// ImagePullPolicy sets image pull policy
func ImagePullPolicy(pullPolicy string) corev1.PullPolicy {
	var imagePullPolicy corev1.PullPolicy
	switch pullPolicy {
	case "Always":
		imagePullPolicy = corev1.PullAlways
	case "Never":
		imagePullPolicy = corev1.PullNever
	case "IfNotPresent":
		imagePullPolicy = corev1.PullIfNotPresent
	default:
		imagePullPolicy = corev1.PullIfNotPresent
	}
	return imagePullPolicy
}
