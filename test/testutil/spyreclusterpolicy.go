/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ClusterPolicy(testConfig TestConfig, modes []spyrev1alpha1.SpyreClusterPolicyExperimentalMode, loglevel string) *spyrev1alpha1.SpyreClusterPolicy {
	clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterPolicyName,
			Namespace: metav1.NamespaceAll,
		},
		Spec: spyrev1alpha1.SpyreClusterPolicySpec{
			ExperimentalMode: modes,
			DevicePlugin: spyrev1alpha1.DevicePluginSpec{
				DeploymentConfig: spyrev1alpha1.DeploymentConfig{
					Repository:      testConfig.DevicePlugin.Repository,
					Image:           testConfig.DevicePlugin.Image,
					Version:         testConfig.DevicePlugin.Version,
					ImagePullPolicy: testConfig.DevicePlugin.ImagePullPolicy,
				},
			},
			MetricsExporter: spyrev1alpha1.MetricsExporterSpec{
				Enabled: testConfig.Exporter.Enabled,
				DeploymentConfig: spyrev1alpha1.DeploymentConfig{
					Repository:      testConfig.Exporter.Repository,
					Image:           testConfig.Exporter.Image,
					Version:         testConfig.Exporter.Version,
					ImagePullPolicy: testConfig.Exporter.ImagePullPolicy,
				},
			},
			Scheduler: spyrev1alpha1.SchedulerSpec{
				DeploymentConfig: spyrev1alpha1.DeploymentConfig{
					Repository:      testConfig.Scheduler.Repository,
					Image:           testConfig.Scheduler.Image,
					Version:         testConfig.Scheduler.Version,
					ImagePullPolicy: testConfig.Scheduler.ImagePullPolicy,
				},
			},
			PodValidator: spyrev1alpha1.PodValidatorSpec{
				Enabled: testConfig.PodValidator.Enabled,
				DeploymentConfig: spyrev1alpha1.DeploymentConfig{
					Repository:      testConfig.PodValidator.Repository,
					Image:           testConfig.PodValidator.Image,
					Version:         testConfig.PodValidator.Version,
					ImagePullPolicy: testConfig.PodValidator.ImagePullPolicy,
				},
			},
			HealthChecker: spyrev1alpha1.HealthCheckerSpec{
				Enabled: testConfig.HealthChecker.Enabled,
				DeploymentConfig: spyrev1alpha1.DeploymentConfig{
					Repository:      testConfig.HealthChecker.Repository,
					Image:           testConfig.HealthChecker.Image,
					Version:         testConfig.HealthChecker.Version,
					ImagePullPolicy: testConfig.HealthChecker.ImagePullPolicy,
				},
			},
			CardManagement: spyrev1alpha1.CardManagementSpec{
				Enabled: testConfig.CardManagement.Enabled,
				DeploymentConfig: spyrev1alpha1.DeploymentConfig{
					Image:           testConfig.CardManagement.Image,
					Version:         testConfig.CardManagement.Version,
					Repository:      testConfig.CardManagement.Repository,
					ImagePullPolicy: testConfig.CardManagement.ImagePullPolicy,
				},
				Config: &spyrev1alpha1.CardManagementConfig{
					SpyreFilter:   testConfig.CardManagement.Config.SpyreFilter,
					PfRunnerImage: testConfig.CardManagement.Config.PfRunnerImage,
					VfRunnerImage: testConfig.CardManagement.Config.VfRunnerImage,
				},
			},
		},
	}
	if testConfig.DevicePluginInit.Enabled {
		EnableInitContainer(clusterPolicy, testConfig, testConfig.DevicePluginInit.ExecutePolicy)
	} else {
		DisableInitContainer(clusterPolicy)
	}
	if len(loglevel) > 0 {
		clusterPolicy.Spec.LogLevel = &loglevel
	}
	return clusterPolicy
}

func DeployClusterPolicy(ctx context.Context, testConfig TestConfig, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client,
	modes []spyrev1alpha1.SpyreClusterPolicyExperimentalMode, loglevel string, nodeCount int) {
	clusterPolicy := ClusterPolicy(testConfig, modes, loglevel)
	By("creating cluster policy state")
	err := spyreV2Client.Create(ctx, clusterPolicy, &client.CreateOptions{})
	Expect(err).To(BeNil())
	WaitForSpyreClusterPolicyState(ctx, spyreV2Client, k8sClientset, nodeCount, spyrev1alpha1.Ready)
}

func UpdateClusterPolicy(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, nodeCount int, expectedState spyrev1alpha1.State) {
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return spyreV2Client.Update(ctx, clusterPolicy)
	})
	Expect(err).To(BeNil())
	By("updating cluster policy state to not ready")
	Eventually(func(g Gomega) {
		err := spyreV2Client.Get(ctx,
			client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
		g.Expect(err).To(BeNil())

		clusterPolicy.Status.State = spyrev1alpha1.NotReady
		err = spyreV2Client.Status().Update(ctx, clusterPolicy)
		g.Expect(err).To(BeNil())
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	WaitForSpyreClusterPolicyState(ctx, spyreV2Client, k8sClientset, nodeCount, expectedState)
}

func WaitForSpyreClusterPolicyState(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, nodeCount int, expectedState spyrev1alpha1.State) {
	defer CheckOperatorAssetsRunning(ctx, spyreV2Client, k8sClientset, nodeCount)
	By("polling for status update of the cluster policy")
	Eventually(func(g Gomega) {
		var clusterPolicy spyrev1alpha1.SpyreClusterPolicy
		err := spyreV2Client.Get(ctx,
			client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, &clusterPolicy)
		g.Expect(err).To(BeNil())
		// now make sure that the reconciliation has occurred.
		g.Expect(clusterPolicy.Status.State).To(Equal(expectedState))
	}).WithTimeout(20 * time.Minute).WithPolling(30 * time.Second).Should(Succeed())
}

func CheckOperatorAssetsRunning(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, nodeCount int) {
	var clusterPolicy spyrev1alpha1.SpyreClusterPolicy
	err := spyreV2Client.Get(ctx,
		client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, &clusterPolicy)
	Expect(err).To(BeNil())
	By("checking common resources")
	Eventually(func(g Gomega) {
		_, err := k8sClientset.CoreV1().ConfigMaps(OperatorNamespace).Get(ctx, "spyre-config", metav1.GetOptions{})
		g.Expect(err).To(BeNil())
		getObj := &nfdv1alpha1.NodeFeatureRule{}
		namespacedName := types.NamespacedName{Name: "spyre-node-feature-rule", Namespace: OperatorNamespace}
		err = spyreV2Client.Get(ctx, namespacedName, getObj)
		g.Expect(err).To(BeNil())
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	By("checking pod validator deployment")
	if clusterPolicy.Spec.PodValidator.Enabled {
		Eventually(func(g Gomega) {
			podValidatorPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, podValidatorLabel, "")
			if clusterPolicy.Spec.PodValidator.Replicas != nil {
				g.Expect(podValidatorPods).To(HaveLen(int(*clusterPolicy.Spec.PodValidator.Replicas)))
			} else {
				g.Expect(podValidatorPods).To(HaveLen(1))
			}
			for _, podValidatorPod := range podValidatorPods {
				printMessageIfPodNotRunning(podValidatorPod)
				g.Expect(podValidatorPod.Status.Phase).To(BeEquivalentTo(v1.PodRunning))
			}
			g.Expect(err).To(BeNil())
			_, err = k8sClientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, podValidatorName, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	} else {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.AppsV1().Deployments(OperatorNamespace).Get(ctx, podValidatorName, metav1.GetOptions{})
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
			_, err = k8sClientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, podValidatorName, metav1.GetOptions{})
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
	By("checking health checker")
	if clusterPolicy.Spec.HealthChecker.Enabled {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Get(ctx, spyreconst.HealthCheckerResourceName, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
			healthCheckerPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, healthCheckerLabel, "")
			if !isCRC(ctx, k8sClientset) {
				// For non crc clusters with homogeneous node types
				g.Expect(len(healthCheckerPods)).To(BeNumerically(">=", 1))
			} else {
				g.Expect(healthCheckerPods).To(HaveLen(nodeCount))
			}
			for _, healthCheckerPod := range healthCheckerPods {
				printMessageIfPodNotRunning(healthCheckerPod)
				g.Expect(healthCheckerPod.Status.Phase).To(BeEquivalentTo(v1.PodRunning))
			}
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	} else {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Get(ctx, spyreconst.HealthCheckerResourceName, metav1.GetOptions{})
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
	By("checking device plugin")
	Eventually(func(g Gomega) {
		_, err := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Get(ctx, spyreconst.DevicePluginResourceName, metav1.GetOptions{})
		g.Expect(err).To(BeNil())
		devicePluginPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, devicePluginLabel, "")
		if !isCRC(ctx, k8sClientset) && nodeCount > 0 {
			// For non crc clusters with homogeneous node types
			g.Expect(len(devicePluginPods)).To(BeNumerically(">=", 1))
		} else {
			g.Expect(devicePluginPods).To(HaveLen(nodeCount))
		}
		for _, devicePluginPod := range devicePluginPods {
			printMessageIfPodNotRunning(devicePluginPod)
			g.Expect(devicePluginPod.Status.Phase).To(BeEquivalentTo(v1.PodRunning))
			if clusterPolicy.Spec.DevicePlugin.InitContainer != nil {
				g.Expect(devicePluginPod.Spec.InitContainers).To(HaveLen(1))
				checkEnvExists(devicePluginPod.Spec.Containers[0].Env, spyreconst.IgnoreMetadataKey, "false")
				// Verify VERIFY_P2P and privileged settings
				By("checking VERIFY_P2P and privileged")
				checkEnvExists(devicePluginPod.Spec.InitContainers[0].Env, "VERIFY_P2P", "0")
				g.Expect(devicePluginPod.Spec.InitContainers[0].SecurityContext).NotTo(BeNil())
				g.Expect(devicePluginPod.Spec.InitContainers[0].SecurityContext.Privileged).NotTo(BeNil())
				g.Expect(*devicePluginPod.Spec.InitContainers[0].SecurityContext.Privileged).To(BeFalse())
			} else {
				g.Expect(devicePluginPod.Spec.InitContainers).To(HaveLen(0))
				checkEnvExists(devicePluginPod.Spec.Containers[0].Env, spyreconst.IgnoreMetadataKey, "true")
			}
		}
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	By("checking card management deployment")
	if clusterPolicy.Spec.CardManagement.Enabled {
		Eventually(func(g Gomega) {
			cardManagementPod := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, cardManagementLabel, "")
			if clusterPolicy.Spec.PodValidator.Replicas != nil {
				g.Expect(cardManagementPod).To(HaveLen(int(*clusterPolicy.Spec.PodValidator.Replicas)))
			} else {
				g.Expect(cardManagementPod).To(HaveLen(1))
			}
			for _, cardManagementPod := range cardManagementPod {
				printMessageIfPodNotRunning(cardManagementPod)
				g.Expect(cardManagementPod.Status.Phase).To(BeEquivalentTo(v1.PodRunning))
			}
			g.Expect(err).To(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	} else {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.AppsV1().Deployments(OperatorNamespace).Get(ctx, cardManagementName, metav1.GetOptions{})
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
	By("checking metric exporter")
	if clusterPolicy.Spec.MetricsExporter.Enabled {
		ds, err := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Get(ctx, spyreconst.MonitorResourceName, metav1.GetOptions{})
		Expect(err).To(BeNil())
		nodeSelector := ds.Spec.Template.Spec.NodeSelector
		Expect(nodeSelector).To(HaveLen(1))
		if isCRC(ctx, k8sClientset) {
			nodeName, found := nodeSelector["kubernetes.io/hostname"]
			Expect(found).To(BeTrue())
			Expect(nodeName).To(BeEquivalentTo(nodeName))
		}
		Eventually(func(g Gomega) {
			metricsExporterPods := GetPodsWithLabels(ctx, k8sClientset, g, OperatorNamespace, metricsExporterLabel, "")
			g.Expect(len(metricsExporterPods)).To(BeNumerically(">=", 1))
			for _, pod := range metricsExporterPods {
				printMessageIfPodNotRunning(pod)
				g.Expect(pod.Status.Phase).To(BeEquivalentTo(v1.PodRunning))
			}
			_, err := k8sClientset.CoreV1().Services(OperatorNamespace).Get(ctx, metricsExporterName, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		waitForEndpoint(ctx, k8sClientset, metricsExporterName, OperatorNamespace)
	} else {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.AppsV1().DaemonSets(OperatorNamespace).Get(ctx, metricsExporterName, metav1.GetOptions{})
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
}

func waitForEndpoint(ctx context.Context, k8sClientset *kubernetes.Clientset, serviceName, serviceNamespace string) {
	Eventually(func(g Gomega) {
		By("checking endpoint")
		endpoint, err := k8sClientset.CoreV1().Endpoints(serviceNamespace).Get(ctx, serviceName, metav1.GetOptions{})
		g.Expect(err).To(BeNil())
		By("checking endpoint subnets")
		g.Expect(len(endpoint.Subsets)).To(BeNumerically(">", 0))
		By("checking endpoint IPs")
		g.Expect(len(endpoint.Subsets[0].Addresses)).To(BeNumerically(">", 0), "")
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func UpdateModes(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, nodeCount int,
	enabledModes []spyrev1alpha1.SpyreClusterPolicyExperimentalMode, addPseudoDevice bool,
	expectedState spyrev1alpha1.State) {
	clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
	err := spyreV2Client.Get(ctx,
		client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
	Expect(err).To(BeNil())
	if addPseudoDevice {
		enabledModes = append(enabledModes, spyrev1alpha1.PseudoDeviceMode)
	}
	clusterPolicy.Spec.ExperimentalMode = enabledModes
	UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, nodeCount, expectedState)
}

func UpdateInitContainer(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, nodeCount int,
	testConfig TestConfig, executePolicy spyrev1alpha1.ExecutePolicy, initEnable bool, expectedState spyrev1alpha1.State) {
	clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
	err := spyreV2Client.Get(ctx,
		client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
	Expect(err).To(BeNil())
	if initEnable {
		EnableInitContainer(clusterPolicy, testConfig, executePolicy)
	} else {
		DisableInitContainer(clusterPolicy)
	}
	UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, nodeCount, expectedState)
}

func EnableInitContainer(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy,
	testConfig TestConfig, executePolicy spyrev1alpha1.ExecutePolicy) {
	clusterPolicy.Spec.DevicePlugin.InitContainer = &spyrev1alpha1.ExternalInitContainerSpec{
		DeploymentConfig: spyrev1alpha1.DeploymentConfig{
			Repository:      testConfig.DevicePluginInit.Repository,
			Image:           testConfig.DevicePluginInit.Image,
			Version:         testConfig.DevicePluginInit.Version,
			ImagePullPolicy: testConfig.DevicePluginInit.ImagePullPolicy,
		},
		ExecutePolicy: &executePolicy,
	}
}

func DisableInitContainer(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) {
	clusterPolicy.Spec.DevicePlugin.InitContainer = nil
}

func checkEnvExists(envs []v1.EnvVar, key, value string) {
	found := false
	for _, env := range envs {
		if env.Name == key {
			Expect(env.Value).To(Equal(value))
			found = true
			break
		}
	}
	Expect(found).To(BeTrue())
}
