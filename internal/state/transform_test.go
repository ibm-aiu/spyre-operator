/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"
	"encoding/json"
	"path/filepath"

	. "github.com/ibm-aiu/spyre-operator/internal/state"
	"go.uber.org/zap/zapcore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DefaultArchitecture = "amd64"
)

var (
	twoReplicas = int32(2)

	regenerateIfNotPresent = spyrev1alpha1.ExecuteIfNotPresent
	regenerateAlways       = spyrev1alpha1.ExecuteAlways
)

func newDaemonset(name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: OpNs,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "main",
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}
}

func newDeployment(name string, replica int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: OpNs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replica,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "main",
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Transform", func() {

	memQuantity, _ := resource.ParseQuantity("1Gi")

	type applyDeployConfigExpectedResult struct {
		Image             string
		ImagePullPolicy   corev1.PullPolicy
		ImagePullSecret   *string
		MemoryRequest     *resource.Quantity
		MemoryLimit       *resource.Quantity
		Args              []string
		Envs              map[string]string
		NodeSelectorLabel string
		NodeSelectorValue string
	}

	DescribeTable("applyDeployConfig", func(config *spyrev1alpha1.DeploymentConfig, expected applyDeployConfigExpectedResult, expectedErr bool) {
		ds := newDaemonset("test-apply-deploy-config")
		err := ApplyDeployConfig(&ds.Spec.Template, config)
		if expectedErr {
			Expect(err).NotTo(BeNil())
			return
		}
		Expect(err).To(BeNil())
		container := ds.Spec.Template.Spec.Containers[0]
		By("checking image")
		Expect(container.Image, expected.Image)
		By("checking image pull policy")
		Expect(container.ImagePullPolicy, expected.ImagePullPolicy)
		By("checking resource")
		if expected.MemoryLimit != nil {
			Expect(container.Resources.Limits.Memory()).To(BeEquivalentTo(expected.MemoryLimit))
		}
		if expected.MemoryRequest != nil {
			Expect(container.Resources.Requests.Memory()).To(BeEquivalentTo(expected.MemoryRequest))
		}
		By("checking args")
		Expect(container.Args).To(BeEquivalentTo(expected.Args))
		By("checking envs")
		if len(container.Env) > 0 {
			Expect(expected.Envs).ToNot(BeNil())
		}
		for _, env := range container.Env {
			val, ok := expected.Envs[env.Name]
			Expect(ok).To(BeTrue())
			Expect(env.Value).To(BeEquivalentTo(val))
		}
		By("checking node selector")
		if expected.NodeSelectorLabel != "" {
			Expect(len(ds.Spec.Template.Spec.NodeSelector)).To(Equal(1))
			Expect(ds.Spec.Template.Spec.NodeSelector[expected.NodeSelectorLabel]).To(Equal(expected.NodeSelectorValue))
		}
	},
		Entry("no image", &spyrev1alpha1.DeploymentConfig{Image: "", Repository: "", Version: ""}, applyDeployConfigExpectedResult{}, false),
		Entry("wrong image combination - no version", &spyrev1alpha1.DeploymentConfig{Image: "", Repository: "somevalue", Version: ""},
			applyDeployConfigExpectedResult{}, true),
		Entry("wrong image combination - no repository", &spyrev1alpha1.DeploymentConfig{Image: "", Repository: "", Version: "somevalue"},
			applyDeployConfigExpectedResult{}, true),
		Entry("some image", &spyrev1alpha1.DeploymentConfig{Image: "image", Repository: "repo", Version: "version"},
			applyDeployConfigExpectedResult{Image: "repo/image:version"}, false),
		Entry("some image with sha", &spyrev1alpha1.DeploymentConfig{Image: "image", Repository: "repo", Version: "sha256:xxx"},
			applyDeployConfigExpectedResult{Image: "repo/image@sha256:xxx"}, false),
		Entry("image pull policy", &spyrev1alpha1.DeploymentConfig{ImagePullPolicy: "Always"},
			applyDeployConfigExpectedResult{ImagePullPolicy: corev1.PullAlways}, false),
		Entry("resource limit", &spyrev1alpha1.DeploymentConfig{
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: memQuantity,
				},
			}}, applyDeployConfigExpectedResult{MemoryLimit: &memQuantity}, false),
		Entry("resource request", &spyrev1alpha1.DeploymentConfig{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: memQuantity,
				},
			}}, applyDeployConfigExpectedResult{MemoryRequest: &memQuantity}, false),
		Entry("both request and limit", &spyrev1alpha1.DeploymentConfig{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: memQuantity,
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: memQuantity,
				},
			}}, applyDeployConfigExpectedResult{MemoryRequest: &memQuantity, MemoryLimit: &memQuantity}, false),
		Entry("args", &spyrev1alpha1.DeploymentConfig{Args: []string{"some", "args"}},
			applyDeployConfigExpectedResult{Args: []string{"some", "args"}}, false),
		Entry("envs", &spyrev1alpha1.DeploymentConfig{Env: []corev1.EnvVar{
			{Name: "k1", Value: "v1"}, {Name: "k2", Value: "v2"}}},
			applyDeployConfigExpectedResult{Envs: map[string]string{"k1": "v1", "k2": "v2"}}, false),
		Entry("node selector", &spyrev1alpha1.DeploymentConfig{NodeSelector: map[string]string{"k": "v"}},
			applyDeployConfigExpectedResult{NodeSelectorLabel: "k", NodeSelectorValue: "v"}, false),
	)

	DescribeTable("P2PDMA", func(p2pdma bool, expectedEnvNum int, expectedValue string) {
		devicePlugin := newDaemonset("device-plugin")
		config := &spyrev1alpha1.SpyreClusterPolicySpec{
			DevicePlugin: spyrev1alpha1.DevicePluginSpec{
				DeploymentConfig: ValidDeploymentConfig("some-image"),
				P2PDMA:           p2pdma,
			},
		}
		err := TransformDevicePlugin(devicePlugin, config, "amd64", zapcore.InfoLevel, false)
		Expect(err).NotTo(HaveOccurred())
		env := devicePlugin.Spec.Template.Spec.Containers[0].Env
		Expect(env).To(HaveLen(expectedEnvNum))
		Expect(env[0].Name).To(Equal("IGNORE_EXTERNAL_METADATA"))
		Expect(env[0].Value).To(Equal("true"), "without init container, must ignore external metadata")
		if expectedEnvNum == 4 {
			Expect(env[3].Name).To(Equal("P2PDMA"))
			Expect(env[3].Value).To(Equal(expectedValue))
		}
	},
		Entry("P2PDMA=false", false, 3, ""),
		Entry("P2PDMA=true", true, 4, "1"),
	)

	Context("init container", func() {
		ctx := context.Background()

		It("init container can be loaded", func() {
			dsPath := filepath.Join(AssetsPath, "state-core-components", "spyre-device-plugin", "0500_daemonset.yaml")
			runtimeObj, gvk, err := DecodeFromFile(StateScheme, dsPath)
			Expect(err).To(BeNil())
			defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())
			controlledObj, err := NewDaemonSet(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			ds, ok := controlledObj.GetObject().(*appsv1.DaemonSet)
			Expect(ok).To(BeTrue())
			Expect(len(ds.Spec.Template.Spec.InitContainers)).To(BeNumerically(">", 0))
			controlledDs, ok := controlledObj.(*DevicePluginDaemonset)
			Expect(ok).To(BeTrue())
			spec := controlledDs.GetSpec()
			Expect(len(spec.Spec.InitContainers)).To(Equal(1))
			Expect(spec.Spec.InitContainers[0].Name).To(BeEquivalentTo("init-data"))
		})

		DescribeTable("transform init container config", func(nodeArchitecture string, pseudoMode, hasInit, hasImage, expectedInit bool) {
			mnts, found := DeviceHostPathMounts[nodeArchitecture]
			if !found {
				Expect(expectedInit).To(BeFalse(), "should not expect init if hw is not supported")
			} else {
				Expect(len(mnts)).To(BeNumerically(">", 0), "should contain at least one mount target")
			}

			initContainer := corev1.Container{
				Name: "app",
				Args: []string{"command"},
			}
			devicePlugin := newDaemonset("device-plugin")
			if hasInit {
				devicePlugin.Spec.Template.Spec.InitContainers = []corev1.Container{initContainer}
			}
			var config spyrev1alpha1.SpyreClusterPolicySpec
			if pseudoMode {
				config.ExperimentalMode = []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PseudoDeviceMode}
			}
			config.DevicePlugin.DeploymentConfig = ValidDeploymentConfig("some-image")
			initConfig := ValidDeploymentConfig("some-init-image")
			if hasImage {
				config.DevicePlugin.InitContainer = &spyrev1alpha1.ExternalInitContainerSpec{
					DeploymentConfig: initConfig,
				}
			}
			err := TransformDevicePlugin(devicePlugin, &config, nodeArchitecture, zapcore.InfoLevel, false)
			expectedErr := !found && hasImage
			if expectedErr {
				Expect(err).NotTo(BeNil(), "should throw an error if set container for unsupported architecture")
			} else {
				Expect(err).To(BeNil())
			}
			By("checking length")
			initLenExist := len(devicePlugin.Spec.Template.Spec.InitContainers) > 0
			Expect(initLenExist).To(Equal(expectedInit))
			By("checking hw mounts")
			if initLenExist {
				expectedImage, err := spyrev1alpha1.ImagePath(initConfig.Repository, initConfig.Image, initConfig.Version)
				Expect(err).To(BeNil())
				Expect(devicePlugin.Spec.Template.Spec.InitContainers[0].Image).To(BeEquivalentTo(expectedImage))
				mountsContains(devicePlugin.Spec.Template.Spec.InitContainers[0].VolumeMounts, mnts, true)
			}
			volumesContains(devicePlugin.Spec.Template.Spec.Volumes, mnts, expectedInit)
			By("checking IgnoreMetadataKey")
			ignore := false
			for _, env := range devicePlugin.Spec.Template.Spec.Containers[0].Env {
				if env.Name == spyreconst.IgnoreMetadataKey {
					if initLenExist {
						Expect(env.Value).To(BeEquivalentTo("false"))
					} else {
						Expect(env.Value).To(BeEquivalentTo("true"))
						ignore = true
					}
					break
				}
			}
			if !expectedErr {
				Expect(!ignore).To(Equal(initLenExist))
			}
		},
			Entry("pseudo, without init template, no image", "amd64", true, false, false, false),
			Entry("pseudo, without init template, with image", "amd64", true, false, true, false),
			Entry("pseudo, with init template, no image", "amd64", true, true, false, false),
			Entry("pseudo, with init template, with image", "amd64", true, true, true, true),
			Entry("without init template, no image", "amd64", false, false, false, false),
			Entry("without init template, with image", "amd64", false, false, true, false),
			Entry("with init template, no image", "amd64", false, true, false, false),
			Entry("amd64: with init template, with image", "amd64", false, true, true, true),
			Entry("unsupported: with init template, with image", "unsupported", false, true, true, false),
		)

		DescribeTable("apply execute policy", func(executePolicy *spyrev1alpha1.ExecutePolicy, existingEnv []corev1.EnvVar, expectedEnv []corev1.EnvVar) {
			initContainer := corev1.Container{
				Name: "app",
				Env:  existingEnv,
			}

			ApplyExecutePolicy(&initContainer, executePolicy)
			Expect(initContainer.Env).To(BeEquivalentTo(expectedEnv))
		},
			Entry("nil - empty env", nil, []corev1.EnvVar{}, []corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "true"}}),
			Entry("IfNotPresent - empty env", &regenerateIfNotPresent, []corev1.EnvVar{}, []corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "true"}}),
			Entry("IfNotPresent - replace env", &regenerateIfNotPresent,
				[]corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "false"}}, []corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "true"}}),
			Entry("IfNotPresent - other env", &regenerateIfNotPresent, []corev1.EnvVar{{Name: "foo", Value: "bar"}},
				[]corev1.EnvVar{{Name: "foo", Value: "bar"}, {Name: "SKIP_IF_COMPLETED", Value: "true"}}),
			Entry("Always - empty env", &regenerateAlways, []corev1.EnvVar{}, []corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "false"}}),
			Entry("Always - replace env", &regenerateAlways,
				[]corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "true"}}, []corev1.EnvVar{{Name: "SKIP_IF_COMPLETED", Value: "false"}}),
			Entry("Always - other env", &regenerateAlways, []corev1.EnvVar{{Name: "foo", Value: "bar"}},
				[]corev1.EnvVar{{Name: "foo", Value: "bar"}, {Name: "SKIP_IF_COMPLETED", Value: "false"}}),
		)

		DescribeTable("architecture-aware VERIFY_P2P and privileged settings", func(nodeArchitecture string, p2pdma bool, expectedVerifyP2P string, expectedPrivileged bool, hasExistingSecurityContext bool) {
			initContainer := corev1.Container{
				Name: "init-data",
			}
			// Test both cases: with and without existing SecurityContext
			if hasExistingSecurityContext {
				existingPrivileged := false
				runAsUser := int64(1001)
				initContainer.SecurityContext = &corev1.SecurityContext{
					Privileged: &existingPrivileged,
					RunAsUser:  &runAsUser,
				}
			}
			devicePlugin := newDaemonset("device-plugin")
			devicePlugin.Spec.Template.Spec.InitContainers = []corev1.Container{initContainer}

			config := spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					DeploymentConfig: ValidDeploymentConfig("device-plugin"),
					P2PDMA:           p2pdma,
					InitContainer: &spyrev1alpha1.ExternalInitContainerSpec{
						DeploymentConfig: ValidDeploymentConfig("init-container"),
					},
				},
			}

			err := TransformDevicePlugin(devicePlugin, &config, nodeArchitecture, zapcore.InfoLevel, false)
			Expect(err).To(BeNil())

			By("checking init container exists")
			Expect(len(devicePlugin.Spec.Template.Spec.InitContainers)).To(Equal(1))

			By("checking VERIFY_P2P environment variable")
			verifyP2PFound := false
			for _, env := range devicePlugin.Spec.Template.Spec.InitContainers[0].Env {
				if env.Name == "VERIFY_P2P" {
					Expect(env.Value).To(Equal(expectedVerifyP2P), "VERIFY_P2P should match expected value for architecture %s", nodeArchitecture)
					verifyP2PFound = true
					break
				}
			}
			Expect(verifyP2PFound).To(BeTrue(), "VERIFY_P2P environment variable should be set")

			By("checking privileged security context")
			Expect(devicePlugin.Spec.Template.Spec.InitContainers[0].SecurityContext).NotTo(BeNil(), "SecurityContext should be set")
			Expect(devicePlugin.Spec.Template.Spec.InitContainers[0].SecurityContext.Privileged).NotTo(BeNil(), "Privileged field should be set")
			Expect(*devicePlugin.Spec.Template.Spec.InitContainers[0].SecurityContext.Privileged).To(Equal(expectedPrivileged), "Privileged should match expected value for architecture %s", nodeArchitecture)
		},
			Entry("amd64 with p2pdma: VERIFY_P2P=1 and privileged=true, no existing SecurityContext", "amd64", true, "1", true, false),
			Entry("amd64 with p2pdma: VERIFY_P2P=1 and privileged=true, with existing SecurityContext", "amd64", true, "1", true, true),
			Entry("amd64 without p2pdma: VERIFY_P2P=0 and privileged=false, with existing SecurityContext", "amd64", false, "0", false, true),
			Entry("ppc64le: VERIFY_P2P=0 and privileged=false, no existing SecurityContext", "ppc64le", false, "0", false, false),
			Entry("ppc64le: VERIFY_P2P=0 and privileged=false, with existing SecurityContext", "ppc64le", false, "0", false, true),
			Entry("ppc64le with p2pdma: VERIFY_P2P=0 and privileged=false, with existing SecurityContext", "ppc64le", false, "0", false, true),
			Entry("s390x: VERIFY_P2P=0 and privileged=false, no existing SecurityContext", "s390x", false, "0", false, false),
			Entry("s390x: VERIFY_P2P=0 and privileged=false, with existing SecurityContext", "s390x", false, "0", false, true),
			Entry("s390x with p2pdma: VERIFY_P2P=0 and privileged=false, with existing SecurityContext", "s390x", false, "0", false, true),
		)

		DescribeTable("apply p2pDMA on amd64", func(p2pDMA bool, existingEnv []corev1.EnvVar, expectedEnv []corev1.EnvVar) {
			initContainer := corev1.Container{
				Name: "app",
				Env:  existingEnv,
			}
			ApplyVerifyP2P(&initContainer, "amd64", p2pDMA)
			Expect(initContainer.Env).To(BeEquivalentTo(expectedEnv))
		},
			Entry("p2pDMA=false - empty env", false, []corev1.EnvVar{}, []corev1.EnvVar{{Name: "VERIFY_P2P", Value: "0"}}),
			Entry("p2pDMA=true - empty env", true, []corev1.EnvVar{}, []corev1.EnvVar{{Name: "VERIFY_P2P", Value: "1"}}),
			Entry("p2pDMA=false - replace env", false,
				[]corev1.EnvVar{{Name: "VERIFY_P2P", Value: "1"}}, []corev1.EnvVar{{Name: "VERIFY_P2P", Value: "0"}}),
			Entry("p2pDMA=false - other env", false, []corev1.EnvVar{{Name: "foo", Value: "bar"}},
				[]corev1.EnvVar{{Name: "foo", Value: "bar"}, {Name: "VERIFY_P2P", Value: "0"}}),
		)
	})

	Context("config/metrics path and name", func() {
		It("set if defined", func() {
			expectedConfigName := "config.json"
			expectedConfigPath := "/etc/ibm/spyre"
			expectedMetricsPath := "/tmp/data"
			devicePlugin := newDaemonset("device-plugin")
			metricsExporter := newDaemonset("metricsExporter")
			cpSpec := &spyrev1alpha1.SpyreClusterPolicySpec{
				MetricsExporter: spyrev1alpha1.MetricsExporterSpec{
					MetricsPath: expectedMetricsPath,
					Enabled:     true,
				},
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					DeviceConfigSpec: spyrev1alpha1.DeviceConfigSpec{
						ConfigName: expectedConfigName,
						ConfigPath: expectedConfigPath,
					},
				},
			}
			By("checking device plugin")
			TransformDevicePlugin(devicePlugin, cpSpec, DefaultArchitecture, zapcore.InfoLevel, false)
			devicePluginEnvs := make(map[string]string)
			for _, env := range devicePlugin.Spec.Template.Spec.Containers[0].Env {
				devicePluginEnvs[env.Name] = env.Value
			}
			val, found := devicePluginEnvs[spyreconst.DeviceConfigOutputPathKey]
			Expect(found).To(BeTrue())
			Expect(val).To(Equal(expectedConfigPath))
			val, found = devicePluginEnvs[spyreconst.DeviceConfigFileNameKey]
			Expect(found).To(BeTrue())
			Expect(val).To(Equal(expectedConfigName))
			val, found = devicePluginEnvs[spyreconst.MetricsExportKey]
			Expect(found).To(BeTrue())
			Expect(val).To(Equal("true"))
			By("checking metrics exporter")
			TransformMetricsExporter(metricsExporter, cpSpec, DefaultArchitecture)
			exporterEnvs := make(map[string]string)
			for _, env := range metricsExporter.Spec.Template.Spec.Containers[0].Env {
				exporterEnvs[env.Name] = env.Value
			}
			val, found = exporterEnvs[spyreconst.DeviceConfigFileNameKey]
			Expect(found).To(BeTrue())
			Expect(val).To(Equal(expectedConfigName))
			val, found = exporterEnvs[spyreconst.MetricsContainerPathKey]
			Expect(found).To(BeTrue())
			Expect(val).To(Equal(expectedMetricsPath))
		})
	})

	Context("health-checker integration", func() {
		originalSocket := "health-check.sock"

		DescribeTable("check health socket", func(enabled bool, expectedValue string) {
			devicePlugin := newDaemonset("device-plugin")
			SetContainerEnv(&devicePlugin.Spec.Template.Spec.Containers[0],
				spyreconst.SpyreHealthSocketEnvKey, originalSocket)
			cpSpec := &spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					DeviceConfigSpec: spyrev1alpha1.DeviceConfigSpec{},
				},
				HealthChecker: spyrev1alpha1.HealthCheckerSpec{
					Enabled: enabled,
				},
			}
			TransformDevicePlugin(devicePlugin, cpSpec, DefaultArchitecture, zapcore.InfoLevel, false)
			found := false
			for _, env := range devicePlugin.Spec.Template.Spec.Containers[0].Env {
				if env.Name == spyreconst.SpyreHealthSocketEnvKey {
					Expect(env.Value).To(BeEquivalentTo(expectedValue))
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		},
			Entry("enabled health checker", true, originalSocket),
			Entry("disabled health checker", false, ""),
		)
	})

	Context("card management", func() {
		cardMgmtCpSpec := spyrev1alpha1.SpyreClusterPolicySpec{
			CardManagement: spyrev1alpha1.CardManagementSpec{
				Enabled: true,
			},
		}
		cardMgmtCp := spyrev1alpha1.SpyreClusterPolicy{
			Spec: cardMgmtCpSpec,
		}
		It("disabled", func() {
			devicePlugin := newDaemonset("device-plugin")
			cpSpec := &spyrev1alpha1.SpyreClusterPolicySpec{}
			TransformDevicePlugin(devicePlugin, cpSpec, DefaultArchitecture, zapcore.InfoLevel, false)
			found := false
			for _, env := range devicePlugin.Spec.Template.Spec.Containers[0].Env {
				if env.Name == spyreconst.CardManagementEnabledKey {
					found = true
					break
				}
			}
			Expect(found).To(BeFalse())
		})
		It("enabled", func() {
			devicePlugin := newDaemonset("device-plugin")
			TransformDevicePlugin(devicePlugin, &cardMgmtCpSpec, DefaultArchitecture, zapcore.InfoLevel, false)
			found := false
			for _, env := range devicePlugin.Spec.Template.Spec.Containers[0].Env {
				if env.Name == spyreconst.CardManagementEnabledKey {
					Expect(env.Value).To(Equal(spyreconst.ModeEnabledValue))
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})
		It("mount pvc", func() {
			ds := newDaemonset("test-daemonset")
			ds.Spec.Template.Spec.NodeSelector = map[string]string{
				"kubernetes.io/hostname": "test-node",
			}
			err := TransformCardManagement(ds, &cardMgmtCp, nil, true) // tentative
			Expect(err).To(BeNil())
			volumes := ds.Spec.Template.Spec.Volumes
			Expect(volumes).To(HaveLen(1))
			Expect(volumes[0].Name).To(BeEquivalentTo("cardmgmt"))
			Expect(volumes[0].VolumeSource.PersistentVolumeClaim).NotTo(BeNil())
			Expect(volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(BeEquivalentTo(spyreconst.CardManagementClaimName))
			volumeMounts := ds.Spec.Template.Spec.Containers[0].VolumeMounts
			Expect(volumeMounts).To(HaveLen(1))
			Expect(volumeMounts[0].Name).To(BeEquivalentTo("cardmgmt"))
			Expect(volumeMounts[0].MountPath).To(BeEquivalentTo("/cardmgmt"))
		})
		It("empty pvc", func() {
			ds := newDaemonset("test-daemonset")
			ds.Spec.Template.Spec.NodeSelector = map[string]string{
				"kubernetes.io/hostname": "test-node",
			}
			err := TransformCardManagement(ds, &cardMgmtCp, nil, false) // tentative
			Expect(err).To(BeNil())
			Expect(ds.Spec.Template.Spec.Volumes).To(HaveLen(0))
			Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts).To(HaveLen(0))
		})

		Context("spyreFilter regex", func() {
			DescribeTable("filter nodes by regex pattern",
				func(nodeName string, filterPattern string, hasFilter bool, shouldSchedule bool, expectError bool) {
					ds := newDaemonset("test-cardmgmt")
					ds.Spec.Template.Spec.NodeSelector = map[string]string{
						"kubernetes.io/hostname": nodeName,
					}

					config := &spyrev1alpha1.CardManagementConfig{}
					if hasFilter {
						config.SpyreFilter = &filterPattern
					}

					cardMgmtCp := &spyrev1alpha1.SpyreClusterPolicy{
						Spec: spyrev1alpha1.SpyreClusterPolicySpec{
							CardManagement: spyrev1alpha1.CardManagementSpec{
								Enabled: true,
								Config:  config,
							},
						},
					}

					err := TransformCardManagement(ds, cardMgmtCp, nil, false)

					if expectError {
						Expect(err).NotTo(BeNil())
						return
					}

					Expect(err).To(BeNil())

					if shouldSchedule {
						// Pod should be allowed to schedule - no blocking label
						if ds.Spec.Template.Spec.NodeSelector != nil {
							_, hasBlockingLabel := ds.Spec.Template.Spec.NodeSelector["spyre.ibm.com/card-management-disabled"]
							Expect(hasBlockingLabel).To(BeFalse(), "blocking label should not be present")
						}
					} else {
						// Pod should be blocked from scheduling
						Expect(ds.Spec.Template.Spec.NodeSelector).NotTo(BeNil())
						val, hasBlockingLabel := ds.Spec.Template.Spec.NodeSelector["spyre.ibm.com/card-management-disabled"]
						Expect(hasBlockingLabel).To(BeTrue(), "blocking label should be present")
						Expect(val).To(Equal("true"))
					}
				},
				Entry("nil filter (default) - should match all nodes", "worker-node-1", "", false, true, false),
				Entry("dot pattern (default) - should match all nodes", "worker-node-1", ".", true, true, false),
				Entry("dot pattern - different node name", "any-node-name", ".", true, true, false),
				Entry("single node match - matching node", "node1", "node1", true, true, false),
				Entry("single node match - non-matching node", "node2", "node1", true, false, false),
				Entry("or expression - first node matches", "node1", "node1|node2", true, true, false),
				Entry("or expression - second node matches", "node2", "node1|node2", true, true, false),
				Entry("or expression - third node matches", "node3", "node1|node2|node3", true, true, false),
				Entry("or expression - non-matching node", "node4", "node1|node2|node3", true, false, false),
				Entry("partial match in node name", "worker-node1-spyre", "node1", true, true, false),
				Entry("invalid regex pattern - should return error", "worker-node-1", "[invalid(regex", true, false, true),
			)

			It("should remove blocking affinity when filter changes to include the node", func() {
				nodeName := "node2"
				ds := newDaemonset("test-cardmgmt")
				ds.Spec.Template.Spec.NodeSelector = map[string]string{
					"kubernetes.io/hostname": nodeName,
				}

				// First, apply a filter that excludes node2
				excludeFilter := "node1"
				config := &spyrev1alpha1.CardManagementConfig{
					SpyreFilter: &excludeFilter,
				}
				cardMgmtCp := &spyrev1alpha1.SpyreClusterPolicy{
					Spec: spyrev1alpha1.SpyreClusterPolicySpec{
						CardManagement: spyrev1alpha1.CardManagementSpec{
							Enabled: true,
							Config:  config,
						},
					},
				}

				// Transform with exclusion filter
				err := TransformCardManagement(ds, cardMgmtCp, nil, false)
				Expect(err).To(BeNil())

				// Verify blocking label is set
				Expect(ds.Spec.Template.Spec.NodeSelector).NotTo(BeNil())
				val, hasBlockingLabel := ds.Spec.Template.Spec.NodeSelector["spyre.ibm.com/card-management-disabled"]
				Expect(hasBlockingLabel).To(BeTrue())
				Expect(val).To(Equal("true"))

				// Now, change the filter to include node2
				includeFilter := "node1|node2"
				config.SpyreFilter = &includeFilter

				// Transform again with inclusion filter
				err = TransformCardManagement(ds, cardMgmtCp, nil, false)
				Expect(err).To(BeNil())

				// Verify blocking label is removed
				if ds.Spec.Template.Spec.NodeSelector != nil {
					_, hasBlockingLabel := ds.Spec.Template.Spec.NodeSelector["spyre.ibm.com/card-management-disabled"]
					Expect(hasBlockingLabel).To(BeFalse(), "blocking label should be removed")
				}
			})
		})
	})

	Context("hardware mount", func() {
		DescribeTable("architecture-specific mount", func(nodeArchitecture string, pseudoMode bool, expectedMount bool) {
			devicePlugin := newDaemonset("device-plugin")
			cpSpec := &spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					DeploymentConfig: ValidDeploymentConfig("device-plugin"),
				},
			}
			if pseudoMode {
				cpSpec.ExperimentalMode = []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PseudoDeviceMode}
			}
			err := TransformDevicePlugin(devicePlugin, cpSpec, nodeArchitecture, zapcore.InfoLevel, false)
			Expect(err).To(BeNil())
			mnts, found := HwHostPathMounts[nodeArchitecture]
			if !pseudoMode {
				Expect(found).To(Equal(expectedMount))
				if expectedMount {
					Expect(len(mnts)).To(BeNumerically(">", 0))
				}
			}

			mountsContains(devicePlugin.Spec.Template.Spec.Containers[0].VolumeMounts, mnts, expectedMount)
			volumesContains(devicePlugin.Spec.Template.Spec.Volumes, mnts, expectedMount)
		},
			Entry("unsupported type with pseudo mode", "unsupported", false, false),
			Entry("unsupported type with devices", "unsupported", false, false),
			Entry("amd64 with pseudo mode", "amd64", true, false),
			Entry("amd64 with devices", "amd64", false, true),
			Entry("s390x with pseudo mode", "s390x", true, false),
			Entry("s390x with devices", "s390x", false, true),
		)
	})

	Context("port", func() {
		Specify("can override port", func() {
			ds := newDaemonset("test-apply-port")
			ds.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{Name: "dummyKey", Value: "dummyVal"},
				{
					Name:  spyreconst.ExporterPortKey,
					Value: "8082",
				},
			}
			ds.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
				{Name: "dummyPort"},
				{
					Name:          spyreconst.MonitorPortName,
					ContainerPort: 8082,
					Protocol:      corev1.ProtocolTCP,
				},
			}
			ApplyPort(&(ds.Spec.Template.Spec.Containers[0]), 8083, spyreconst.ExporterPortKey, spyreconst.MonitorPortName)
			Expect(ds.Spec.Template.Spec.Containers[0].Env[1].Value).To(BeEquivalentTo("8083"))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[1].ContainerPort).To(BeEquivalentTo(int32(8083)))
		})

		Specify("can apply new port", func() {
			ds := newDaemonset("test-apply-port")
			ApplyPort(&(ds.Spec.Template.Spec.Containers[0]), 8083, spyreconst.ExporterPortKey, spyreconst.MonitorPortName)
			Expect(ds.Spec.Template.Spec.Containers[0].Env[0].Value).To(BeEquivalentTo("8083"))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[0].Name).To(BeEquivalentTo(spyreconst.MonitorPortName))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(BeEquivalentTo(int32(8083)))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[0].Protocol).To(BeEquivalentTo(corev1.ProtocolTCP))
		})

		Specify("can new port with some irrelevant values", func() {
			ds := newDaemonset("test-apply-port")
			ds.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{Name: "dummyKey", Value: "dummyVal"},
			}
			ds.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
				{Name: "dummyPort"},
			}
			ApplyPort(&(ds.Spec.Template.Spec.Containers[0]), 8083, spyreconst.ExporterPortKey, spyreconst.MonitorPortName)
			Expect(ds.Spec.Template.Spec.Containers[0].Env[1].Value).To(BeEquivalentTo("8083"))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[1].Name).To(BeEquivalentTo(spyreconst.MonitorPortName))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[1].ContainerPort).To(BeEquivalentTo(int32(8083)))
			Expect(ds.Spec.Template.Spec.Containers[0].Ports[1].Protocol).To(BeEquivalentTo(corev1.ProtocolTCP))
		})

		Specify("can override port by TransformMetricsExporterService", func() {
			newPort := int32(8083)
			spyrepol := &spyrev1alpha1.SpyreClusterPolicy{
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					MetricsExporter: spyrev1alpha1.MetricsExporterSpec{
						Port: &newPort,
					},
				},
			}
			svc := &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       spyreconst.MonitorPortName,
							Port:       int32(8082),
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromString(spyreconst.MonitorPortName),
						},
					},
				},
			}
			TransformMetricsExporterService(svc, spyrepol)
			Expect(len(svc.Spec.Ports)).To(Equal(1))
			Expect(svc.Spec.Ports[0].Port).To(BeEquivalentTo(newPort))
		})
	})

	DescribeTable("ApplyExperimentalModes",
		func(existingEnv map[string]string, enabledModes []spyrev1alpha1.SpyreClusterPolicyExperimentalMode, expectedEnvLen int) {
			containerEnv := []corev1.EnvVar{}
			for key, value := range existingEnv {
				containerEnv = append(containerEnv, corev1.EnvVar{Name: key, Value: value})
			}
			daemonSet := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-daemonset",
					Namespace: "default",
					Labels: map[string]string{
						"app": "dummy-daemonset",
					},
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "dummy-daemonset",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": "dummy-daemonset",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "dummy-daemonset-container",
									Image: "dummy-daemonset:v1",
									Env:   containerEnv,
								},
								//
								{
									Name:  "dummy-sidecar",
									Image: "dummy-sidecar:v1",
								},
							},
						},
					},
				},
			}
			ApplyExperimentalModes(&(daemonSet.Spec.Template.Spec.Containers[0]), enabledModes)
			container := daemonSet.Spec.Template.Spec.Containers[0]
			Expect(len(container.Env)).To(Equal(expectedEnvLen))
			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}
			experimentalKeys := make(map[string]bool)
			// should set environment variables related to experimental modes
			for _, mode := range enabledModes {
				Expect(len(mode.EnvKey())).NotTo(BeEquivalentTo(""))
				val, found := envMap[mode.EnvKey()]
				Expect(found).To(BeTrue())
				Expect(val).To(Equal(spyreconst.ModeEnabledValue))
				experimentalKeys[mode.EnvKey()] = true
			}
			// should not modify irrelevant environment variables
			for key, expected := range existingEnv {
				if experimentalKeys[key] {
					continue
				}
				val, found := envMap[key]
				Expect(found).To(BeTrue())
				Expect(val).To(Equal(expected))
			}

		},
		Entry("Default", map[string]string{}, []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{}, 0),
		Entry("No mode set with existing environment variables",
			map[string]string{"Key": "Value"},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{}, 1),
		Entry("perDeviceAllocation",
			map[string]string{},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode}, 1),
		Entry("perDeviceAllocation with existing environment variables",
			map[string]string{"Key": "Value"},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode}, 2),
		Entry("pseudoDevice",
			map[string]string{},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PseudoDeviceMode}, 1),
		Entry("pseudoDevice with existing environment variables",
			map[string]string{"Key": "Value"},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PseudoDeviceMode}, 2),
		Entry("perDeviceAllocation + pseudoDevice",
			map[string]string{},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode, spyrev1alpha1.PseudoDeviceMode}, 2),
		Entry("perDeviceAllocation + pseudoDevice with existing environment variables",
			map[string]string{"Key": "Value"},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode, spyrev1alpha1.PseudoDeviceMode}, 3),
		Entry("perDeviceAllocation + pseudoDevice with environment variables to be overridden",
			map[string]string{spyrev1alpha1.PerDeviceAllocationMode.EnvKey(): "0", spyrev1alpha1.PseudoDeviceMode.EnvKey(): "0", "Key": "Value"},
			[]spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode, spyrev1alpha1.PseudoDeviceMode}, 3),
	)

	It("TransformMetricsExporterService - new port", func() {
		newPort := int32(8083)
		spyrepol := &spyrev1alpha1.SpyreClusterPolicy{
			Spec: spyrev1alpha1.SpyreClusterPolicySpec{
				MetricsExporter: spyrev1alpha1.MetricsExporterSpec{
					Port: &newPort,
				},
			},
		}
		svc := &corev1.Service{
			Spec: corev1.ServiceSpec{},
		}
		TransformMetricsExporterService(svc, spyrepol)
		Expect(len(svc.Spec.Ports)).To(Equal(1))
		Expect(svc.Spec.Ports[0].Name).To(BeEquivalentTo(spyreconst.MonitorPortName))
		Expect(svc.Spec.Ports[0].Port).To(BeEquivalentTo(newPort))
		Expect(svc.Spec.Ports[0].Protocol).To(BeEquivalentTo(corev1.ProtocolTCP))
		Expect(svc.Spec.Ports[0].TargetPort).To(BeEquivalentTo(intstr.FromString(spyreconst.MonitorPortName)))
	})

	DescribeTable("TransformDeployment", func(replicas *int32, pullPolicy corev1.PullPolicy, expectedReplica int32) {
		oneReplica := int32(1)
		deploy := newDeployment("test-deploy", oneReplica)
		spyrepol := &spyrev1alpha1.SpyreClusterPolicy{
			Spec: spyrev1alpha1.SpyreClusterPolicySpec{
				PodValidator: spyrev1alpha1.PodValidatorSpec{
					Replicas: replicas,
					DeploymentConfig: spyrev1alpha1.DeploymentConfig{
						ImagePullPolicy: string(pullPolicy),
					},
				},
			},
		}
		TransformDeployment(deploy, &spyrepol.Spec.PodValidator.DeploymentConfig, spyrepol.Spec.PodValidator.Replicas)
		Expect(deploy.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(BeEquivalentTo(pullPolicy))
		Expect(*deploy.Spec.Replicas).To(BeEquivalentTo(expectedReplica))
	},
		Entry("default values", nil, corev1.PullIfNotPresent, int32(1)),
		Entry("change pull policy", nil, corev1.PullAlways, int32(1)),
		Entry("change replica", &twoReplicas, corev1.PullIfNotPresent, int32(2)),
	)

	Context("topology config map", func() {
		ctx := context.Background()
		cmName := "spyre-topology"

		DescribeTable("check config map and container mount", func(existMnts []corev1.VolumeMount, existVolumes []corev1.Volume) {
			ds := newDaemonset("device-pluin")
			ds.Spec.Template.Spec.Volumes = existVolumes
			ds.Spec.Template.Spec.Containers[0].VolumeMounts = existMnts
			MountTopologyConfig(&ds.Spec.Template, cmName)
			mnts := ds.Spec.Template.Spec.Containers[0].VolumeMounts
			volumes := ds.Spec.Template.Spec.Volumes
			var foundPath, foundConfigMap string
			mntCount := 0
			for _, mnt := range mnts {
				if mnt.Name == cmName {
					foundPath = mnt.MountPath
					mntCount += 1
				}
			}
			volumeCount := 0
			for _, volume := range volumes {
				if volume.Name == cmName {
					foundConfigMap = volume.ConfigMap.Name
					volumeCount += 1
				}
			}
			Expect(mntCount).To(Equal(1))
			Expect(foundPath).To(BeEquivalentTo(spyreconst.DefaultTopologyFolder))
			Expect(volumeCount).To(Equal(1))
			Expect(foundConfigMap).To(BeEquivalentTo(cmName))
		},
			Entry("empty, must mount", []corev1.VolumeMount{}, []corev1.Volume{}),
			Entry("with volume exist, must replace", []corev1.VolumeMount{},
				[]corev1.Volume{{Name: cmName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}),
			Entry("with volume mount exist, must replace", []corev1.VolumeMount{{Name: cmName, MountPath: ""}},
				[]corev1.Volume{}),
		)

		Specify("TransformSenlibConfigTemplate works when metric exporter is enabled", func() {
			senlibConfigMapPath := filepath.Join(AssetsPath, "state-core-components", "spyre-device-plugin", "0400_senlib_template.yaml")
			runtimeObj, gvk, err := DecodeFromFile(StateScheme, senlibConfigMapPath)
			Expect(err).To(BeNil())
			defaultObj, err := NewDefaultObject(ctx, gvk.Kind, OpNs, runtimeObj)
			Expect(err).To(BeNil())
			controlledObj, err := NewConfigMap(defaultObj, runtimeObj, OpNs)
			Expect(err).To(BeNil())
			configMap, ok := controlledObj.GetObject().(*corev1.ConfigMap)
			Expect(ok).To(BeTrue())
			var originalSenlibConfig SenlibConfig
			err = json.Unmarshal([]byte(configMap.Data[spyreconst.DefaultSenlibConfigFilename]), &originalSenlibConfig)
			Expect(err).To(BeNil())
			Expect(originalSenlibConfig.Metric.General.Enable).To(BeFalse())
			By("enable metric exporter")
			_, err = TransformSenlibConfigTemplate(true, configMap, "")
			Expect(err).To(BeNil())
			enabledTemplate := configMap.Data[spyreconst.DefaultSenlibConfigFilename]
			var enabledSenlibConfig SenlibConfig
			err = json.Unmarshal([]byte(enabledTemplate), &enabledSenlibConfig)
			Expect(err).To(BeNil())
			Expect(enabledSenlibConfig.Metric.General.Enable).To(BeTrue())
			By("disable metric exporter")
			_, err = TransformSenlibConfigTemplate(false, configMap, "")
			Expect(err).To(BeNil())
			disabledTemplate := configMap.Data[spyreconst.DefaultSenlibConfigFilename]
			var disabledSenlibConfig SenlibConfig
			err = json.Unmarshal([]byte(disabledTemplate), &disabledSenlibConfig)
			Expect(err).To(BeNil())
			Expect(disabledSenlibConfig.Metric.General.Enable).To(BeFalse())
		},
		)
	})
})

// volumesContains checks host path volumes.
// mnts contains all mounts if expected.
// if not expect, mnts should not exist.
func volumesContains(volumes []corev1.Volume, mnts []HostPathMount, expected bool) {
	existingVolumes := make(map[string]string, 0)
	for _, volume := range volumes {
		existingVolumes[volume.Name] = volume.HostPath.Path
	}
	for _, mnt := range mnts {
		path, found := existingVolumes[mnt.GetName()]
		Expect(found).To(Equal(expected))
		if expected {
			Expect(path).To(BeEquivalentTo(mnt.GetHostPath()))
		}
	}
}

// mountsContains checks container path volume mounts.
// mnts contains all mounts if expected.
// if not expect, mnts should not exist.
func mountsContains(volumeMounts []corev1.VolumeMount, mnts []HostPathMount, expected bool) {
	existingMnts := make(map[string]string, 0)
	for _, mnt := range volumeMounts {
		existingMnts[mnt.Name] = mnt.MountPath
	}
	if mnts == nil {
		return
	}
	for _, mnt := range mnts {
		path, found := existingMnts[mnt.GetName()]
		Expect(found).To(Equal(expected))
		if expected {
			Expect(path).To(BeEquivalentTo(mnt.GetContainPath()))
		}
	}
}
