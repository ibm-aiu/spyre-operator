/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

/*
 * Transforming order:
 * - initial asset
 * - indirect config (i.e., configuration based on other components) from SpyreClusterPolicy
 * - direct config (e.g., DeployConfig) from SpyreClusterPolicy
 */

package state

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HostPathMount contains mount name, host path, and container mount path
type HostPathMount struct {
	name          string
	hostPath      string
	containerPath string
}

const (
	blockingLabel = "spyre.ibm.com/card-management-disabled"
)

var (
	// map hw-related host mount for each architecture
	// architecture is getting from the label "kubernetes.io/arch"
	hwHostPathMounts = map[string][]HostPathMount{
		"amd64":   {{name: "hwdata", hostPath: "/usr/share/hwdata", containerPath: "/usr/share/hwdata"}},
		"ppc64le": {{name: "hwdata", hostPath: "/usr/share/hwdata", containerPath: "/usr/share/hwdata"}},
		"s390x":   {{name: "hwdata", hostPath: "/usr/share/hwdata", containerPath: "/usr/share/hwdata"}},
	}

	// map device host mount for each architecture,
	// used by init container to discover devices and generate topology
	// architecture is getting from the label "kubernetes.io/arch"
	deviceHostPathMounts = map[string][]HostPathMount{
		"amd64":   {{name: "vfio", hostPath: "/dev/vfio", containerPath: "/dev/vfio"}},
		"ppc64le": {{name: "vfio", hostPath: "/dev/vfio", containerPath: "/dev/vfio"}},
		"s390x":   {{name: "vfio", hostPath: "/dev/vfio", containerPath: "/dev/vfio"}},
	}
)

// TransformMetricsExporter transforms metrics exporter daemonset with required config as per Spyre device
func TransformMetricsExporter(obj *appsv1.DaemonSet,
	config *spyrev1alpha1.SpyreClusterPolicySpec, nodeArchitecture string) error {
	if config.MetricsExporter.MetricsPath != "" {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.MetricsContainerPathKey, config.MetricsExporter.MetricsPath)
	}

	if config.DevicePlugin.ConfigName != "" {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.DeviceConfigFileNameKey, config.DevicePlugin.ConfigName)
	}

	// update env from experimental modes
	applyExperimentalModes(&(obj.Spec.Template.Spec.Containers[0]), config.ExperimentalMode)
	if config.MetricsExporter.Port != nil {
		applyPort(&(obj.Spec.Template.Spec.Containers[0]),
			*config.MetricsExporter.Port, spyreconst.ExporterPortKey, spyreconst.MonitorPortName)
	}
	if !config.ExperimentalModeEnabled(spyrev1alpha1.PseudoDeviceMode) {
		// If not pseudoDeviceMode, add architecture-specific hardware host path
		addHwHostPathVolume(&(obj.Spec.Template.Spec), nodeArchitecture)
	}

	// apply common deploy config
	if err := applyDeployConfig(&obj.Spec.Template, &config.MetricsExporter.DeploymentConfig); err != nil {
		return err
	}
	return nil
}

// TransformDevicePlugin transforms k8s-device-plugin daemonset with required config as per Spyre device
func TransformDevicePlugin(obj *appsv1.DaemonSet, config *spyrev1alpha1.SpyreClusterPolicySpec, nodeArchitecture string,
	logLevel zapcore.Level, topologyConfigMapExist bool) error {
	// gracefully warning, skip adding init container if error
	if err := transformDevicePluginInitContainer(obj, config, nodeArchitecture); err != nil {
		return fmt.Errorf("failed to transform init container: %w", err)
	}

	setIgnoreMetadataTopology(obj)

	transformDevicePluginLogLevel(obj, logLevel)

	transformDevicePluginPath(obj, config)

	// update env from experimental modes
	applyExperimentalModes(&(obj.Spec.Template.Spec.Containers[0]), config.ExperimentalMode)

	if !config.HealthChecker.Enabled {
		// reset SpyreHealthSocketEnvKey
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0], spyreconst.SpyreHealthSocketEnvKey, "")
	}

	if config.CardManagement.Enabled {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.CardManagementEnabledKey, spyreconst.ModeEnabledValue)
	}

	if config.DevicePlugin.P2PDMA {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.P2PDMAKey, spyreconst.ModeEnabledValue)
	}

	if !config.ExperimentalModeEnabled(spyrev1alpha1.PseudoDeviceMode) {
		// If not pseudoDeviceMode, add architecture-specific hardware host path
		addHwHostPathVolume(&(obj.Spec.Template.Spec), nodeArchitecture)
	}

	if topologyConfigMapExist {
		mountTopologyConfig(&obj.Spec.Template, config.DevicePlugin.TopologyConfigMapName)
	}

	// apply common deploy config
	if err := applyDeployConfig(&obj.Spec.Template, &config.DevicePlugin.DeploymentConfig); err != nil {
		return fmt.Errorf("failed to apply config: %w", err)
	}
	return nil
}

// TransformHealthChecker transforms health checker daemonset
func TransformHealthChecker(obj *appsv1.DaemonSet, config *spyrev1alpha1.SpyreClusterPolicySpec,
	logLevel zapcore.Level) error {
	// update env from experimental modes
	applyExperimentalModes(&(obj.Spec.Template.Spec.Containers[0]), config.ExperimentalMode)

	// apply common deploy config
	if err := applyDeployConfig(&obj.Spec.Template, &config.HealthChecker.DeploymentConfig); err != nil {
		return fmt.Errorf("failed to apply config: %w", err)
	}
	return nil
}

// addHwHostPathVolume adds host path volumes to the pod spec based on the node architecture.
func addHwHostPathVolume(spec *corev1.PodSpec, nodeArchitecture string) {
	if mntPaths, found := hwHostPathMounts[nodeArchitecture]; found {
		for _, mnt := range mntPaths {
			hostVolumeSource := corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: mnt.hostPath,
				},
			}
			addVolume(spec, mnt.name, hostVolumeSource)
			addVolumeMount(&spec.Containers[0], mnt.name, mnt.containerPath, true)
		}
	}
}

// addDeviceHostPathMounts adds device path volumes to init container based on the node architecture.
// return true if device host path mounts are found for the given architecture.
func addDeviceHostPathMounts(spec *corev1.PodSpec, nodeArchitecture string) bool {
	mntPaths, found := deviceHostPathMounts[nodeArchitecture]
	if found {
		for _, mnt := range mntPaths {
			hostVolumeSource := corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: mnt.hostPath,
				},
			}
			addVolume(spec, mnt.name, hostVolumeSource)
			addVolumeMount(&spec.InitContainers[0], mnt.name, mnt.containerPath, true)
		}
	}
	return found
}

func transformDevicePluginLogLevel(obj *appsv1.DaemonSet, logLevel zapcore.Level) {
	hasLogLevel := false
	for _, arg := range obj.Spec.Template.Spec.Containers[0].Args {
		if strings.HasPrefix(arg, "--v=") {
			hasLogLevel = true
			break
		}
	}
	if !hasLogLevel {
		obj.Spec.Template.Spec.Containers[0].Args = append(obj.Spec.Template.Spec.Containers[0].Args,
			fmt.Sprintf("--v=%d", -logLevel)) // invert zap loglevel.
	}
}

func transformDevicePluginPath(obj *appsv1.DaemonSet, config *spyrev1alpha1.SpyreClusterPolicySpec) {
	if config.DevicePlugin.ConfigPath != "" {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.DeviceConfigOutputPathKey, config.DevicePlugin.ConfigPath)
	}

	if config.DevicePlugin.ConfigName != "" {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.DeviceConfigFileNameKey, config.DevicePlugin.ConfigName)
	}

	if config.MetricsExporter.MetricsPath != "" {
		setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
			spyreconst.MetricsContainerPathKey, config.MetricsExporter.MetricsPath)
	}

	setContainerEnv(&obj.Spec.Template.Spec.Containers[0],
		spyreconst.MetricsExportKey, fmt.Sprintf("%v", config.MetricsExporter.Enabled))
}

// transformDevicePluginInitContainer transforms the init container for device plugin.
func transformDevicePluginInitContainer(obj *appsv1.DaemonSet,
	config *spyrev1alpha1.SpyreClusterPolicySpec, nodeArchitecture string) error {
	noInitContainerTemplate := len(obj.Spec.Template.Spec.InitContainers) == 0
	// Skip if no init container template or no config provided
	if noInitContainerTemplate || config.DevicePlugin.InitContainer == nil {
		obj.Spec.Template.Spec.InitContainers = []corev1.Container{}
		return nil
	}

	if err := applyContainerConfig(&obj.Spec.Template.Spec.InitContainers[0],
		&config.DevicePlugin.InitContainer.DeploymentConfig); err != nil {
		obj.Spec.Template.Spec.InitContainers = []corev1.Container{}
		return err
	}
	found := addDeviceHostPathMounts(&obj.Spec.Template.Spec, nodeArchitecture)
	if !found {
		obj.Spec.Template.Spec.InitContainers = []corev1.Container{}
		return fmt.Errorf("unsupported architecture %s, please remove .spec.devicePlugin.initContainer", nodeArchitecture)
	}
	applyExecutePolicy(&obj.Spec.Template.Spec.InitContainers[0],
		config.DevicePlugin.InitContainer.ExecutePolicy)
	applyVerifyP2P(&obj.Spec.Template.Spec.InitContainers[0], nodeArchitecture,
		config.DevicePlugin.P2PDMA)

	// update env from experimental modes
	applyExperimentalModes(&(obj.Spec.Template.Spec.InitContainers[0]), config.ExperimentalMode)

	return nil
}

// TransformPodValidator transforms pod validator deployment
func TransformPodValidator(obj *appsv1.Deployment, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	if err := TransformDeployment(obj, &clusterPolicy.Spec.PodValidator.DeploymentConfig,
		clusterPolicy.Spec.PodValidator.Replicas); err != nil {
		return fmt.Errorf("failed to transform deployment: %w", err)
	}
	return nil
}

// TransformCardManagement transforms card management deployment
func TransformCardManagement(obj *appsv1.DaemonSet,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState, pvcExists bool) error {
	if err := applyDeployConfig(&obj.Spec.Template, &clusterPolicy.Spec.CardManagement.DeploymentConfig); err != nil {
		return fmt.Errorf("failed to transform deployment: %w", err)
	}
	if pvcExists {
		mountCardManagementClaim(obj, spyreconst.CardManagementClaimName)
	}
	// update env from experimental modes
	applyExperimentalModes(&(obj.Spec.Template.Spec.Containers[0]), clusterPolicy.Spec.ExperimentalMode)

	// Apply spyreFilter logic before cluster check
	nodeName, exists := obj.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"]
	if !exists {
		return fmt.Errorf("failed to transform cardmgmt: no hostname is designated in Daemonset: %s", obj.Name)
	}

	// Get spyreFilter regex pattern from config
	var spyreFilterPattern string
	if clusterPolicy.Spec.CardManagement.Config != nil && clusterPolicy.Spec.CardManagement.Config.SpyreFilter != nil {
		spyreFilterPattern = *clusterPolicy.Spec.CardManagement.Config.SpyreFilter
	} else {
		spyreFilterPattern = "." // default pattern matches all
	}

	// Compile the regex pattern
	spyreFilterRegex, err := regexp.Compile(spyreFilterPattern)
	if err != nil {
		return fmt.Errorf("failed to compile spyreFilter regex pattern \"%s\": %w", spyreFilterPattern, err)
	}

	// Check if the node name matches the spyreFilter pattern
	if spyreFilterRegex.MatchString(nodeName) {
		// Node matches the filter, remove blocking label if it exists
		if obj.Spec.Template.Spec.NodeSelector != nil {
			delete(obj.Spec.Template.Spec.NodeSelector, blockingLabel)
		}
	} else {
		// Node name doesn't match the filter, prevent Pod from running by adding
		// a node selector with a non-existent label
		if obj.Spec.Template.Spec.NodeSelector == nil {
			obj.Spec.Template.Spec.NodeSelector = make(map[string]string)
		}
		obj.Spec.Template.Spec.NodeSelector[blockingLabel] = "true"
		return nil
	}

	// update number of spyre cards
	if cluster == nil {
		return nil // tentatively skip
	}

	ctx := context.Background()
	nodeList := &corev1.NodeList{}
	err = cluster.k8sClient.List(ctx, nodeList, client.MatchingLabels{"node-role.kubernetes.io/worker": ""})
	if err != nil {
		return fmt.Errorf("failed to transform cardmgmt: %w", err)
	}

	for _, node := range nodeList.Items {
		if node.Name != nodeName {
			continue
		}

		if q, exists := node.Status.Capacity[spyreconst.ResourcePrefix+"/"+spyreconst.PfResourceName]; exists {
			v, ok := q.AsInt64()
			if !ok {
				return fmt.Errorf("failed to convert quantity of spyre_pf: %s", q.String())
			}
			if v > 0 {
				m := map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceName(spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName): q,
					corev1.ResourceName(spyreconst.ResourcePrefix + "/" + spyreconst.VfResourceName): q,
				}
				obj.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{Limits: m}
			}
		}
		break
	}
	return nil
}

// mountCardManagementClaim mounts a PersistentVolumeClaim to a Deployment.
func mountCardManagementClaim(obj *appsv1.DaemonSet, claimName string) {
	mntName := "cardmgmt"
	mntPath := "/cardmgmt"
	volumeSource := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: claimName,
		},
	}
	addVolumeMount(&obj.Spec.Template.Spec.Containers[0], mntName, mntPath, false)
	addVolume(&obj.Spec.Template.Spec, mntName, volumeSource)
}

func TransformDeployment(deployment *appsv1.Deployment, config *spyrev1alpha1.DeploymentConfig, replicas *int32) error {
	if replicas != nil {
		deployment.Spec.Replicas = replicas
	}
	return applyDeployConfig(&deployment.Spec.Template, config)
}

// TransformService transforms metrics exporter service
func TransformMetricsExporterService(obj *corev1.Service, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	if clusterPolicy.Spec.MetricsExporter.Port == nil {
		return nil
	}
	config := clusterPolicy.Spec
	port := *config.MetricsExporter.Port
	for i, val := range obj.Spec.Ports {
		if val.Name != spyreconst.MonitorPortName {
			continue
		}
		obj.Spec.Ports[i].Port = port
		return nil
	}
	newPort := corev1.ServicePort{
		Name:       spyreconst.MonitorPortName,
		Port:       port,
		Protocol:   corev1.ProtocolTCP,
		TargetPort: intstr.FromString(spyreconst.MonitorPortName),
	}
	obj.Spec.Ports = append(obj.Spec.Ports, newPort)
	return nil
}

// applyExperimentalModes applies environment variables corresponding to enabled experimental modes.
func applyExperimentalModes(c *corev1.Container, modes []spyrev1alpha1.SpyreClusterPolicyExperimentalMode) {
	for _, mode := range modes {
		setContainerEnv(c, mode.EnvKey(), spyreconst.ModeEnabledValue)
	}
}

// applyPort applies port to specific container for specific env key and port name
func applyPort(c *corev1.Container, port int32, envKey string, portName string) {
	setContainerEnv(c, envKey, fmt.Sprintf("%d", port))
	setContainerPort(c, portName, port)
}

// mountTopologyConfig mounts config map
func mountTopologyConfig(template *corev1.PodTemplateSpec, configmapName string) {
	volumeSource := corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: configmapName,
			},
		},
	}
	addVolumeMount(&(template.Spec.Containers[0]), configmapName, spyreconst.DefaultTopologyFolder, false)
	addVolume(&(template.Spec), configmapName, volumeSource)
}

// addVolumeMounts replaces or appends volume mount if not exists
func addVolumeMount(container *corev1.Container, mntName, mntPath string, readOnly bool) {
	for index, mount := range container.VolumeMounts {
		if mount.Name == mntName {
			container.VolumeMounts[index].MountPath = mntPath
			return
		}
	}
	mnt := corev1.VolumeMount{
		Name:      mntName,
		MountPath: mntPath,
		ReadOnly:  readOnly,
	}
	container.VolumeMounts = append(container.VolumeMounts, mnt)
}

// addVolume replaces or appends volume if not exists
func addVolume(podSpec *corev1.PodSpec, mntName string, volumeSource corev1.VolumeSource) {
	for index, volume := range podSpec.Volumes {
		if volume.Name == mntName {
			podSpec.Volumes[index].VolumeSource = volumeSource
			return
		}
	}
	volume := corev1.Volume{
		Name:         mntName,
		VolumeSource: volumeSource,
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
}

// setIgnoreMetadataTopology set IgnoreMetadataKey to `true` if init container is not enabled.
// Otherwise, set to `false`.
func setIgnoreMetadataTopology(devicePlugin *appsv1.DaemonSet) {
	ignoreMetadata := fmt.Sprintf("%v", len(devicePlugin.Spec.Template.Spec.InitContainers) == 0)
	setContainerEnv(&devicePlugin.Spec.Template.Spec.Containers[0], spyreconst.IgnoreMetadataKey, ignoreMetadata)
}

func setContainerPort(c *corev1.Container, portName string, port int32) {
	for i, val := range c.Ports {
		if val.Name != portName {
			continue
		}
		c.Ports[i].ContainerPort = port
		return
	}
	newPort := corev1.ContainerPort{
		Name:          portName,
		ContainerPort: port,
		Protocol:      corev1.ProtocolTCP,
	}
	c.Ports = append(c.Ports, newPort)
}

func applyDeployConfig(template *corev1.PodTemplateSpec, config *spyrev1alpha1.DeploymentConfig) error {
	// apply container config to the first container
	if err := applyContainerConfig(&(template.Spec.Containers[0]), config); err != nil {
		return err
	}
	// set image pull secrets
	if len(config.ImagePullSecrets) > 0 {
		for _, secret := range config.ImagePullSecrets { // pragma: allowlist secret
			template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets,
				corev1.LocalObjectReference{Name: secret})
		}
	}
	// set node selector
	if config.NodeSelector != nil {
		template.Spec.NodeSelector = config.NodeSelector
	}

	return nil
}

// applyContainerConfig applies the provided configuration to the given container.
func applyContainerConfig(container *corev1.Container, config *spyrev1alpha1.DeploymentConfig) error {
	img, err := spyrev1alpha1.ImagePath(config.Repository, config.Image, config.Version)
	if err != nil {
		return fmt.Errorf("failed to get image path: %w", err)
	}
	container.Image = img
	// update image pull policy
	container.ImagePullPolicy = spyrev1alpha1.ImagePullPolicy(config.ImagePullPolicy)

	// set resource limits
	if config.Resources != nil {
		// apply resource limits to all containers
		container.Resources = *config.Resources
	}
	// set arguments if specified for driver container
	if len(config.Args) > 0 {
		container.Args = config.Args
	}

	// set/append environment variables for exporter container
	if len(config.Env) > 0 {
		for _, env := range config.Env {
			setContainerEnv(container, env.Name, env.Value)
		}
	}
	return nil
}

// applyExecutePolicy applies executePolicy (default is IfNotPresent)
// If executePolicy is IfNotPresent, inserts condition to skip execution if complete file exists.
func applyExecutePolicy(initContainer *corev1.Container, executePolicy *spyrev1alpha1.ExecutePolicy) {
	policy := spyrev1alpha1.ExecuteIfNotPresent
	if executePolicy != nil {
		policy = *executePolicy
	}
	if policy == spyrev1alpha1.ExecuteAlways {
		setContainerEnv(initContainer, "SKIP_IF_COMPLETED", "false")
	} else {
		setContainerEnv(initContainer, "SKIP_IF_COMPLETED", "true")
	}
}

// applyVerifyP2P
// Set architecture-aware defaults for VERIFY_P2P and privileged
// For amd64: if p2pDMA=true, set VERIFY_P2P=1 and privileged=true
// For other architectures: VERIFY_P2P=0 and privileged=false (defaults from YAML, regardless of p2pDMA flag)
// These defaults can be overridden by user config in applyContainerConfig below
func applyVerifyP2P(initContainer *corev1.Container, nodeArchitecture string, p2pDMA bool) {
	if nodeArchitecture == "amd64" && p2pDMA {
		setContainerEnv(initContainer, "VERIFY_P2P", "1")
		if initContainer.SecurityContext == nil {
			initContainer.SecurityContext = &corev1.SecurityContext{}
		}
		privileged := true
		initContainer.SecurityContext.Privileged = &privileged
	} else {
		// For non-amd64 architectures, ensure VERIFY_P2P=0 (default from YAML) and privileged=false
		setContainerEnv(initContainer, "VERIFY_P2P", "0")
		if initContainer.SecurityContext == nil {
			initContainer.SecurityContext = &corev1.SecurityContext{}
		}
		privileged := false
		initContainer.SecurityContext.Privileged = &privileged
	}
}

func setContainerEnv(c *corev1.Container, key, value string) {
	for i, val := range c.Env {
		if val.Name != key {
			continue
		}
		c.Env[i].Value = value
		return
	}
	c.Env = append(c.Env, corev1.EnvVar{Name: key, Value: value})
}

func TransformSenlibConfigTemplate(enabled bool, obj *corev1.ConfigMap, configFileName string) (SenlibConfig, error) {
	var senlibConfig SenlibConfig
	senlibData, found := obj.Data[spyreconst.DefaultSenlibConfigFilename]
	if !found {
		return senlibConfig, fmt.Errorf("%s key not found in configmap", spyreconst.DefaultSenlibConfigFilename)
	}
	var err error
	if configFileName == "" {
		configFileName = spyreconst.DefaultSenlibConfigFilename
	}
	if err = json.Unmarshal([]byte(senlibData), &senlibConfig); err == nil {
		senlibConfig.Metric.General.Enable = enabled
		var updatedTemplate []byte
		if updatedTemplate, err = json.Marshal(senlibConfig); err == nil {
			obj.Data[configFileName] = string(updatedTemplate)
			return senlibConfig, nil
		}
	}
	return senlibConfig, fmt.Errorf("failed to decode: %w", err)
}
