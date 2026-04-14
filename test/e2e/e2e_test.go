/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

/*
This e2e expects
- E2E_KUBECONFIG is set to the existing openshift-local cluster (which has crc node in a ready state).
- TEST_CONFIG is set to test image configuration file
- All images are available. (If using local registry, all images must be pushed)
- NodeFeatureDiscovery operator and instance which allows ibm.com namespace (set .spec.extraLabelNs) must be installed.
*/
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/hashicorp/go-multierror"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/controllers/spyrepod"
	. "github.com/ibm-aiu/spyre-operator/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	spyrePf = "ibm.com/spyre_pf"
	tier0Pf = "ibm.com/spyre_pf_tier0"
	spyreVf = "ibm.com/spyre_vf" // Used for SSA (isolated VFs) on s390x
)

const (
	requireNoDeviceSkipMessage = "Skipping: require no device"
)

var allDeviceList, specificDeviceList []string
var specificPf, specificPf2 string

var singleNumOfSpyre = int64(1)
var twoNumOfSpyre = int64(2)

var AllocationMetricLabels = func(namespace, name, nodeName string) string {
	return fmt.Sprintf(`spyre_allocation{namespace="%s",node="%s",pod="%s",spyre="[0-9a-fA-F:.]+"}`, namespace, nodeName, name)
}

var TempMetricLabels = func(namespace, name, nodeName string) string {
	return fmt.Sprintf(`spyre_usage_temperature_celsius{namespace="%s",node="%s",pod="%s",probe="0",spyre="[0-9a-fA-F:.]+"}`, namespace, nodeName, name)
}

var _ = Describe("e2e test", Label("e2e"), Ordered, func() {
	testNamespace := "default"
	ctx := context.Background()

	BeforeAll(func() {
		err := cleanupTestNamespace(ctx, k8sClientset, testNamespace)
		Expect(err).To(BeNil())

		Eventually(func(g Gomega) {
			allDeviceList = getDeviceList(ctx)
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		deviceListMsg := fmt.Sprintf("All devices: %v", allDeviceList)
		By(deviceListMsg)
		specificPf = spyrepod.SafePciAddress(spyrePf, allDeviceList[0])
		specificPf2 = spyrepod.SafePciAddress(spyrePf, allDeviceList[1])
		specificDeviceList = allDeviceList[0:1]
	})

	Context("SpyreNodeState", func() {
		It("can set vfs (pseudoDevice)", func() {
			// Skip this test on s390x as VFs are in SpyreSSAInterfaces, not under PF
			if nodeArchitecture == "s390x" {
				Skip("VF under PF test not applicable for s390x - VFs are in SpyreSSAInterfaces")
			}
			if testConfig.PseudoDeviceMode {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				Expect(err).To(BeNil())
				Expect(len(spyrens.Spec.SpyreInterfaces)).To(BeNumerically(">", 0))
				Expect(spyrens.Spec.SpyreInterfaces[0].NumVfs).To(Equal(2))
				Expect(len(spyrens.Spec.SpyreInterfaces[0].Vfs)).To(Equal(2))
			}
		})

		It("has topology", func() {
			if !testConfig.DevicePluginInit.Enabled {
				Skip("No init container, no topology expected")
			}
			Consistently(func(g Gomega) {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				g.Expect(err).To(BeNil())
				g.Expect(spyrens.Spec.Pcitopo).NotTo(BeEquivalentTo(""))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
		})
	})

	Context("Mode changes", func() {
		AfterEach(func() {
			By("reverting to default modes")
			enabledModes := DefaultModes()
			UpdateModes(ctx, spyreV2Client, k8sClientset, len(nodeNames), enabledModes, testConfig.PseudoDeviceMode, spyrev1alpha1.Ready)
		})

		It("can set NoSpyreNodes state", func() {
			if testConfig.HasDevice || !testConfig.PseudoDeviceMode {
				Skip(requireNoDeviceSkipMessage)
			}
			CleanUpNode(ctx, k8sClientset, targetNodeName)
			enabledModes := DefaultModes()
			UpdateModes(ctx, spyreV2Client, k8sClientset, 0, enabledModes, false, spyrev1alpha1.NoSpyreNodes)
		})

		It("can use basic mode", func() {
			enabledModes := []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{}
			UpdateModes(ctx, spyreV2Client, k8sClientset, len(nodeNames), enabledModes, testConfig.PseudoDeviceMode, spyrev1alpha1.Ready)
			By("gradually allocate 25 Spyres")
			commonAllocationTestCase(ctx, testNamespace, false)
		})
	})

	// TODO: move behind
	Context("Health checker", Ordered, func() {
		ctx := context.Background()
		pseudoPFDeviceCardNum := 8
		goodPFNum := 7
		var badCards []string

		BeforeAll(func() {
			if nodeArchitecture == "s390x" {
				badCards = []string{"0000:41:00.0", "0008:00:00.0"}
			} else {
				badCards = []string{"0000:41:00.0"}
			}
			By("enabling health checker in the cluster policy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.HealthChecker.Enabled = true
			debugLevel := "debug"
			clusterPolicy.Spec.LogLevel = &debugLevel
			clusterPolicy.Spec.HealthChecker.NodeSelector = map[string]string{"kubernetes.io/hostname": targetNodeName}
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})

		It("device plugin can update pseudo device health from health checker", func() {
			if !testConfig.PseudoDeviceMode {
				Skip("only available for pseudo device mode")
			}
			Eventually(func(g Gomega) {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				g.Expect(err).To(BeNil())
				g.Expect(spyrens.Spec.SpyreInterfaces).To(HaveLen(pseudoPFDeviceCardNum))
				for _, device := range spyrens.Spec.SpyreInterfaces {
					if slices.Contains(badCards, device.PciAddress) {
						g.Expect(device.Health).To(Equal(spyrev1alpha1.SpyreUnhealthy))
					} else {
						g.Expect(device.Health).To(Equal(spyrev1alpha1.SpyreHealthy))
					}
				}
				g.Expect(spyrens.Status.Conditions).To(HaveLen(1))
				Expect(spyrens.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))

				By("Checking UnhealthyDevices")
				numUnhealthy := len(badCards)
				Expect(spyrens.Status.UnhealthyDevices).To(HaveLen(numUnhealthy))
				// Check uniqueness
				foundUnhealthy := make(map[string]bool, 0)
				for _, unhealthyDevice := range spyrens.Status.UnhealthyDevices {
					if slices.Contains(badCards, unhealthyDevice.ID) {
						foundUnhealthy[unhealthyDevice.ID] = true
					}
				}
				Expect(foundUnhealthy).To(HaveLen(numUnhealthy))
			}).WithTimeout(300 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
		})

		It("can allocate good cards", func() {
			if !testConfig.PseudoDeviceMode {
				Skip("only available for pseudo device mode")
			}
			tc := TestCase{
				Prefix:        petname.Generate(2, "-") + "-single",
				TestNamespace: testNamespace,
				ResourceName:  spyrePf,
				Quantity:      int64(goodPFNum),
				NodeName:      targetNodeName,
			}
			goodCards := []string{}
			for _, card := range allDeviceList {
				if !slices.Contains(badCards, card) {
					goodCards = append(goodCards, card)
				}
			}
			tc.TestSinglePod(ctx, k8sClientset, spyreV2Client, goodCards, v1.PodRunning)
		})

		It("must not allocate bad cards", func() {
			if !testConfig.PseudoDeviceMode {
				Skip("only available for pseudo device mode")
			}
			tc := TestCase{
				Prefix:        petname.Generate(2, "-") + "-single",
				TestNamespace: testNamespace,
				ResourceName:  spyrePf,
				Quantity:      int64(goodPFNum),
				NodeName:      targetNodeName,
			}
			tc.TestSinglePod(ctx, k8sClientset, spyreV2Client, []string{}, v1.PodPending)
		})

		AfterAll(func() {
			By("disabling health checker in the cluster policy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.HealthChecker.Enabled = false
			infoLevel := "info"
			clusterPolicy.Spec.LogLevel = &infoLevel
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})
	})

	Context("Disable init container (no metadata topology file)", func() {
		BeforeAll(func() {
			UpdateInitContainer(ctx, spyreV2Client, k8sClientset, len(nodeNames),
				testConfig, spyrev1alpha1.ExecuteIfNotPresent, false, spyrev1alpha1.Ready)
		})

		It("can allocate in basic mode", func() {
			enabledModes := []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{}
			UpdateModes(ctx, spyreV2Client, k8sClientset, len(nodeNames), enabledModes, testConfig.PseudoDeviceMode, spyrev1alpha1.Ready)
			By("checking no topology in SpyreNodeState")
			Consistently(func(g Gomega) {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				g.Expect(err).To(BeNil())
				g.Expect(spyrens.Spec.Pcitopo).To(BeEquivalentTo(""))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
			By("gradually allocate 25 Spyres")
			commonAllocationTestCase(ctx, testNamespace, false)
			By("reverting to default modes")
			enabledModes = DefaultModes()
			UpdateModes(ctx, spyreV2Client, k8sClientset, len(nodeNames), enabledModes, testConfig.PseudoDeviceMode, spyrev1alpha1.Ready)
		})

		It("can allocate in default mode", func() {
			By("ensuring to default modes")
			enabledModes := DefaultModes()
			UpdateModes(ctx, spyreV2Client, k8sClientset, len(nodeNames), enabledModes, testConfig.PseudoDeviceMode, spyrev1alpha1.Ready)
			By("checking no topology in SpyreNodeState")
			Consistently(func(g Gomega) {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				g.Expect(err).To(BeNil())
				g.Expect(spyrens.Spec.Pcitopo).To(BeEquivalentTo(""))
				g.Expect(len(spyrens.Spec.SpyreInterfaces)).To(BeNumerically(">", 0))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
			By("gradually allocate 25 Spyres")
			commonAllocationTestCase(ctx, testNamespace, true)
		})

		AfterAll(func() {
			UpdateInitContainer(ctx, spyreV2Client, k8sClientset, len(nodeNames),
				testConfig, spyrev1alpha1.ExecuteIfNotPresent, testConfig.DevicePluginInit.Enabled, spyrev1alpha1.Ready)
		})
	})

	Context("Isolated VF (SSA) functionality (s390x only)", func() {
		BeforeAll(func() {
			By("disabling init container")
			UpdateInitContainer(ctx, spyreV2Client, k8sClientset, len(nodeNames),
				testConfig, spyrev1alpha1.ExecuteIfNotPresent, false, spyrev1alpha1.Ready)
		})

		BeforeEach(func() {
			// Only run SSA tests on s390x architecture
			if nodeArchitecture != "s390x" {
				Skip("SSA tests only run on s390x architecture")
			}
		})

		It("can validate SSA interfaces exist in SpyreNodeState", func() {
			if testConfig.PseudoDeviceMode {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				Expect(err).To(BeNil())
				Expect(len(spyrens.Spec.SpyreSSAInterfaces)).To(BeNumerically(">", 0))

				// Validate SSA interface structure
				for _, ssaInterface := range spyrens.Spec.SpyreSSAInterfaces {
					Expect(ssaInterface.PciAddress).To(Not(BeEmpty()))
					Expect(ssaInterface.Health).To(Equal(spyrev1alpha1.SpyreHealthy))
					// Validate PCI address format for s390x SSA interfaces
					Expect(ssaInterface.PciAddress).To(MatchRegexp(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9]$`))
				}
			}
		})

		It("can deploy pod requesting isolated VF resources", func() {
			if testConfig.PseudoDeviceMode {
				tc := TestCase{
					Prefix:        petname.Generate(2, "-") + "-ssa-single",
					TestNamespace: testNamespace,
					ResourceName:  spyreVf, // SSA uses spyre_vf resource name
					Quantity:      1,
					NodeName:      targetNodeName,
				}
				tc.TestSinglePod(ctx, k8sClientset, spyreV2Client, []string{}, v1.PodRunning)
			}
		})

		It("can deploy multiple pods requesting isolated VF resources", func() {
			if testConfig.PseudoDeviceMode {
				tc := TestCase{
					Prefix:        petname.Generate(2, "-") + "-ssa-multiple",
					TestNamespace: testNamespace,
					ResourceName:  spyreVf, // SSA uses spyre_vf resource name
					Quantity:      2,
					NodeName:      targetNodeName,
				}
				tc.TestSinglePod(ctx, k8sClientset, spyreV2Client, []string{}, v1.PodRunning)
			}
		})

		It("can handle mixed PF and isolated VF resource requests", func() {
			if testConfig.PseudoDeviceMode {
				requests := map[ResourceRequest]int{
					{ResourceName: spyrePf, Quantity: 8}: 1,
					{ResourceName: spyreVf, Quantity: 1}: 1, // SSA isolated VF
				}
				tc := MixedResourceTestCase{
					Prefix:        petname.Generate(2, "-") + "-ssa-mixed",
					TestNamespace: testNamespace,
					Requests:      requests,
					RestartPolicy: v1.RestartPolicyOnFailure,
					NodeName:      targetNodeName,
				}
				testPods := tc.TestDeployPods(ctx, k8sClientset, spyreV2Client, map[v1.PodPhase]int{v1.PodRunning: 2})
				Expect(len(testPods)).To(Equal(2))

				// Clean up the pods
				By("deleting test pods")
				for _, pod := range testPods {
					err := k8sClientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
					Expect(err).To(BeNil())
				}
			}
		})

		It("can validate SSA device allocation in pod status", func() {
			if testConfig.PseudoDeviceMode {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				Expect(err).To(BeNil())

				// Check if there are any SSA allocations
				if len(spyrens.Status.AllocationList) > 0 {
					for _, allocation := range spyrens.Status.AllocationList {
						if allocation.ResourcePool == "spyre_vf" {
							Expect(len(allocation.DeviceList)).To(BeNumerically(">", 0))
							// Validate device addresses are from SSA interfaces
							for _, device := range allocation.DeviceList {
								Expect(device).To(MatchRegexp(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-9]$`))
							}
						}
					}
				}
			}
		})

		It("can handle SSA interface health status changes", func() {
			if testConfig.PseudoDeviceMode {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				Expect(err).To(BeNil())

				// Validate all SSA interfaces are healthy
				for _, ssaInterface := range spyrens.Spec.SpyreSSAInterfaces {
					Expect(ssaInterface.Health).To(Equal(spyrev1alpha1.SpyreHealthy))
				}

				// Test that unhealthy SSA interfaces would be handled
				// (This is a validation test - actual health changes would be tested in integration tests)
				Expect(len(spyrens.Spec.SpyreSSAInterfaces)).To(BeNumerically(">", 0))
			}
		})

		AfterAll(func() {
			By("setting init container back")
			UpdateInitContainer(ctx, spyreV2Client, k8sClientset, len(nodeNames),
				testConfig, spyrev1alpha1.ExecuteIfNotPresent, testConfig.DevicePluginInit.Enabled, spyrev1alpha1.Ready)
		})
	})

	Context("Pod deployment", Ordered, func() {

		DescribeTable("deploy/delete single pod",
			func(resourceNameFunc func() string, quantityGet func() int, expectedAllocatedDevicesFuc func() []string, expectedPodPhase v1.PodPhase) {
				resourceName := resourceNameFunc()
				quantity := quantityGet()
				expectedAllocatedDevices := expectedAllocatedDevicesFuc()

				tc := TestCase{
					Prefix:        petname.Generate(2, "-") + "-single",
					TestNamespace: testNamespace,
					ResourceName:  resourceName,
					Quantity:      int64(quantity),
					NodeName:      targetNodeName,
				}
				tc.TestSinglePod(ctx, k8sClientset, spyreV2Client, expectedAllocatedDevices, expectedPodPhase)
			},
			Entry("no allocation - request all spyre_pf", func() string { return spyrePf }, getNumDevices, func() []string { return allDeviceList }, v1.PodRunning),
			Entry("no allocation - request specific pf", func() string { return specificPf }, func() int { return 1 }, func() []string { return specificDeviceList }, v1.PodRunning),
		)

		DescribeTable("check file preparation", func(resourceNameFunc func() string, expectedTopologyFile bool) {
			if !testConfig.DevicePluginInit.Enabled {
				expectedTopologyFile = false
			}
			resourceName := resourceNameFunc()
			var expectedPaths []string
			commonPaths := []string{
				"/etc/aiu/senlib_config.json",
				"/etc/aiu/resource_pool",
			}
			if expectedTopologyFile {
				expectedPaths = []string{"/etc/aiu/topo.json"}
			}
			expectedPaths = append(expectedPaths, commonPaths...)

			By("creating pod to list files")
			pod := BuildPodListFiles("test-config", "default", resourceName, expectedPaths, targetNodeName, true)
			_, err := k8sClientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			By("waiting for pod running")
			WaitForPodRunning(ctx, k8sClientset, pod)
			By("testing files must found")
			CheckPodListFilesLog(ctx, k8sClientset, pod, expectedTopologyFile, false)
			By("deleting pod")
			DeletePod(ctx, k8sClientset, pod)
		},
			Entry("must not have topology file when request spyre_pf", func() string { return spyrePf }, false),
			Entry("must not have topology file when specific pf", func() string { return specificPf }, false),
			Entry("must have topology file when request tier0 pf", func() string { return tier0Pf }, true),
		)

		DescribeTable("deploy/delete mixed resource pods (two types, full resource allocation)",
			func(requestsFunc func() map[ResourceRequest]int, requireTopo bool, restartPolicy v1.RestartPolicy, expectedPodPhase map[v1.PodPhase]int) {
				if !testConfig.DevicePluginInit.Enabled && requireTopo {
					Skip("Require Topology while init container is not provided")
				}
				requests := requestsFunc()
				tc := MixedResourceTestCase{
					Prefix:        petname.Generate(2, "-") + "-mixed",
					TestNamespace: testNamespace,
					Requests:      requests,
					RestartPolicy: restartPolicy,
					NodeName:      targetNodeName,
				}
				testPods := tc.TestDeployPods(ctx, k8sClientset, spyreV2Client, expectedPodPhase)
				for expectedPodPhase[v1.PodRunning] > 0 {
					testPods, expectedPodPhase = tc.DeleteRunningPodAndExpectPendingRunIfExists(ctx, k8sClientset, spyreV2Client, testPods, expectedPodPhase)
				}
			},
			Entry("spyre_pf, tier0", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: spyrePf, Quantity: 8}: 1, {ResourceName: tier0Pf, Quantity: 4}: 1}
			}, true, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 1, v1.PodPending: 1}),
			Entry("spyre_pf, per-device", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: spyrePf, Quantity: 8}: 1, {ResourceName: specificPf, Quantity: 1}: 1}
			}, false, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 1, v1.PodPending: 1}),
			Entry("two per-devices", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: specificPf, Quantity: 1}: 1, {ResourceName: specificPf2, Quantity: 1}: 1}
			}, false, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 2}),
			Entry("tier0 x 3", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: tier0Pf, Quantity: 4}: 3}
			}, true, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 2, v1.PodPending: 1}),
		)
		DescribeTable("deploy/delete mixed resource deployments",
			func(requestsFunc func() map[ResourceRequest]int, requireTopo bool, restartPolicy v1.RestartPolicy, expectedPodPhase map[v1.PodPhase]int) {
				if !testConfig.DevicePluginInit.Enabled && requireTopo {
					Skip("Require Topology while init container is not provided")
				}
				requests := requestsFunc()
				tc := MixedResourceTestCase{
					Prefix:        petname.Generate(2, "-") + "-mixed",
					TestNamespace: testNamespace,
					Requests:      requests,
					RestartPolicy: restartPolicy,
					NodeName:      targetNodeName,
				}
				testDeploys := tc.TestDeploys(ctx, k8sClientset, spyreV2Client, expectedPodPhase)
				for expectedPodPhase[v1.PodRunning] > 0 {
					testDeploys, expectedPodPhase = tc.DeleteRunningDeployAndExpectPendingRunIfExists(ctx, k8sClientset, spyreV2Client, testDeploys, expectedPodPhase)
				}
				WaitUntilNoPod(ctx, k8sClientset, spyreV2Client, testNamespace, targetNodeName)
			},
			Entry("spyre_pf, tier0", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: spyrePf, Quantity: 8}: 1, {ResourceName: tier0Pf, Quantity: 4}: 1}
			}, true, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 1, v1.PodPending: 1}),
			Entry("spyre_pf, per-device", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: spyrePf, Quantity: 8}: 1, {ResourceName: specificPf, Quantity: 1}: 1}
			}, false, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 1, v1.PodPending: 1}),
			Entry("two per-devices", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: specificPf, Quantity: 1}: 1, {ResourceName: specificPf2, Quantity: 1}: 1}
			}, false, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 2}),
			Entry("tier0 x 3", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: tier0Pf, Quantity: 4}: 3}
			}, true, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 2, v1.PodPending: 1}),
			Entry("tier0 x 5", func() map[ResourceRequest]int {
				return map[ResourceRequest]int{{ResourceName: tier0Pf, Quantity: 4}: 5}
			}, true, v1.RestartPolicyOnFailure, map[v1.PodPhase]int{v1.PodRunning: 2, v1.PodPending: 3}),
		)

		DescribeTable("deploy/delete multiple pods with the same resource requests in scale",
			func(resourceName string, quantity int, n int, expectedPodPhase map[v1.PodPhase]int) {
				if !testConfig.DevicePluginInit.Enabled && strings.Contains(resourceName, "tier") {
					Skip("Require Topology while init container is not provided")
				}
				tc := TestCase{
					Prefix:        petname.Generate(2, "-") + "-multipod",
					TestNamespace: testNamespace,
					ResourceName:  resourceName,
					Quantity:      int64(quantity),
					NodeName:      targetNodeName,
				}
				tc.TestNSpyrePfPod(ctx, k8sClientset, spyreV2Client, n, expectedPodPhase)
			},
			Entry("no allocation - request 8 spyre_pf", spyrePf, 1, 8, map[v1.PodPhase]int{v1.PodRunning: 8}),
			Entry("no allocation - request 9 spyre_pf", spyrePf, 1, 9, map[v1.PodPhase]int{v1.PodRunning: 8, v1.PodPending: 1}),
			Entry("no allocation - request 50 spyre_pf", spyrePf, 1, 50, map[v1.PodPhase]int{v1.PodRunning: 8, v1.PodPending: 42}),
			Entry("no allocation - request tier0 x 3", tier0Pf, 4, 3, map[v1.PodPhase]int{v1.PodRunning: 2, v1.PodPending: 1}),
		)
		DescribeTable("deploy/delete multiple deployments with the same resource requests in scale",
			func(resourceName string, quantity int, n int, expectedPodPhase map[v1.PodPhase]int) {
				if !testConfig.DevicePluginInit.Enabled && strings.Contains(resourceName, "tier") {
					Skip("Require Topology while init container is not provided")
				}
				tc := TestCase{
					Prefix:        petname.Generate(2, "-") + "-multideploy",
					TestNamespace: testNamespace,
					ResourceName:  resourceName,
					Quantity:      int64(quantity),
					NodeName:      targetNodeName,
				}
				expectedAllocatedDevice := expectedPodPhase[v1.PodRunning] * quantity
				deploys := tc.TestNSpyrePfDeployment(ctx, k8sClientset, spyreV2Client, n, expectedPodPhase, expectedAllocatedDevice)
				tc.DeleteNDeployments(ctx, k8sClientset, spyreV2Client, deploys, 0)
				WaitUntilNoPod(ctx, k8sClientset, spyreV2Client, testNamespace, targetNodeName)
			},
			Entry("no allocation - request 8 spyre_pf", spyrePf, 1, 8, map[v1.PodPhase]int{v1.PodRunning: 8}),
			Entry("no allocation - request 9 spyre_pf", spyrePf, 1, 9, map[v1.PodPhase]int{v1.PodRunning: 8, v1.PodPending: 1}),
			Entry("no allocation - request 50 spyre_pf", spyrePf, 1, 50, map[v1.PodPhase]int{v1.PodRunning: 8, v1.PodPending: 42}),
			Entry("no allocation - request tier0 x 10", tier0Pf, 4, 10, map[v1.PodPhase]int{v1.PodRunning: 2, v1.PodPending: 8}),
		)

		It("can have a pod running after pending when the existing pod is released", func() {
			ts := NewTestStep("pend-to-run", testNamespace, k8sClientset, spyreV2Client, targetNodeName)
			By("deploying first deployment spyre_pf: 1")
			step1 := ts.Deploy(ctx, spyrePf, 1, 1, map[v1.PodPhase]int{v1.PodRunning: 1}, 1)
			By("deploying second deployment spyre_pf: 7")
			step2 := ts.Deploy(ctx, spyrePf, 7, 1, map[v1.PodPhase]int{v1.PodRunning: 2}, 8)
			By("deploying third deployment spyre_pf: 1")
			step3 := ts.Deploy(ctx, spyrePf, 1, 1, map[v1.PodPhase]int{v1.PodRunning: 2, v1.PodPending: 1}, 8)
			By("deleting first deployment")
			ts.Delete(ctx, step1, map[v1.PodPhase]int{v1.PodRunning: 2}, 8)
			By("deleting second deployment")
			ts.Delete(ctx, step2, map[v1.PodPhase]int{v1.PodRunning: 1}, 1)
			By("deleting third deployment")
			ts.Delete(ctx, step3, map[v1.PodPhase]int{}, 0)
		})

		It("can gradually allocate 25 Spyres", func() {
			commonAllocationTestCase(ctx, testNamespace, true)
		})
	})

	Context("Metric exporter", Ordered, func() {
		ctx := context.Background()
		mockUserContainer := "app"
		getMetricsKeyFuncs := []func(namespace, name, nodeName string) string{
			AllocationMetricLabels, TempMetricLabels,
		}

		BeforeAll(func() {
			By("enabling metric exporter in the cluster policy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.MetricsExporter.Enabled = true
			clusterPolicy.Spec.MetricsExporter.NodeSelector = map[string]string{"kubernetes.io/hostname": targetNodeName}
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})

		DescribeTable("Get metrics", func(podName string, numOfSpyre int64) {
			exporterPort := GetExporterPort(ctx, k8sClientset)
			p := BuildMockUserPod(exporterPort, testConfig, podName, testNamespace, numOfSpyre, targetNodeName)
			By("creating pod")
			_, err := k8sClientset.CoreV1().Pods(p.Namespace).Create(ctx, p, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			By("waiting for pod to be running")
			WaitForPodRunning(ctx, k8sClientset, p)
			By("checking pod log")
			WaitForKeywordInPodLog(ctx, k8sClientset, p, mockUserContainer, getMetricsKeyFuncs, targetNodeName)
			By("deleting pod")
			err = k8sClientset.CoreV1().Pods(p.Namespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
		},
			Entry("single Spyre", "single-spyre-metric", singleNumOfSpyre),
			Entry("multi Spyre", "multi-spyre-metrics", twoNumOfSpyre),
		)

		It("pod metrics can be deleted", func() {
			By("deploying first pod")
			firstPodName := "first-pod"
			secondPodName := "second-pod"
			exporterPort := GetExporterPort(ctx, k8sClientset)
			firstPod := BuildMockUserPod(exporterPort, testConfig, firstPodName, testNamespace, singleNumOfSpyre, targetNodeName)
			secondPod := BuildMockUserPod(exporterPort, testConfig, secondPodName, testNamespace, singleNumOfSpyre, targetNodeName)
			By("creating and validating first pod's metrics")
			_, err := k8sClientset.CoreV1().Pods(firstPod.Namespace).Create(ctx, firstPod, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			WaitForPodRunning(ctx, k8sClientset, firstPod)
			WaitForKeywordInPodLog(ctx, k8sClientset, firstPod, mockUserContainer, getMetricsKeyFuncs, targetNodeName)
			By("deleting first pod")
			err = k8sClientset.CoreV1().Pods(firstPod.Namespace).Delete(ctx, firstPodName, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
			By("creating and validating second pod's metrics")
			_, err = k8sClientset.CoreV1().Pods(secondPod.Namespace).Create(ctx, secondPod, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			WaitForPodRunning(ctx, k8sClientset, secondPod)
			WaitForKeywordInPodLog(ctx, k8sClientset, secondPod, mockUserContainer, getMetricsKeyFuncs, targetNodeName)
			By("checking deleted pod's metrics should not found")
			bufStr, err := GetPodLog(ctx, k8sClientset, mockUserContainer, *secondPod)
			Expect(err).To(BeNil())
			keyword := AllocationMetricLabels(firstPod.Namespace, firstPodName, targetNodeName)
			index := strings.Index(bufStr, keyword)
			Expect(index).Should(BeNumerically("<", 0))
		})

		AfterAll(func() {
			By("disabling metric exporter in the cluster policy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.MetricsExporter.Enabled = false
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})
	})

	Context("Pod validator", Ordered, func() {
		BeforeAll(func() {
			By("enabling pod validator in the cluster policy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.PodValidator.Enabled = true
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})

		It("must allow valid pod creation", func() {
			podName := "mock-user"
			exporterPort := GetExporterPort(ctx, k8sClientset)
			p := BuildMockUserPod(exporterPort, testConfig, podName, testNamespace, singleNumOfSpyre, targetNodeName)
			By("creating pod")
			_, err := k8sClientset.CoreV1().Pods(p.Namespace).Create(ctx, p, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			By("waiting for pod to be running")
			WaitForPodRunning(ctx, k8sClientset, p)
			By("deleting pod")
			err = k8sClientset.CoreV1().Pods(p.Namespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
		})

		It("must deny invalid pod creation (wrong schedulerName)", func() {
			podName := "mock-user"
			exporterPort := GetExporterPort(ctx, k8sClientset)
			p := BuildMockUserPod(exporterPort, testConfig, podName, testNamespace, singleNumOfSpyre, targetNodeName)
			p.Spec.SchedulerName = ""
			By("creating pod")
			_, err := k8sClientset.CoreV1().Pods(p.Namespace).Create(ctx, p, metav1.CreateOptions{})
			Expect(err).NotTo(BeNil())
		})

		AfterAll(func() {
			By("disabling pod validator in the cluster policy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.PodValidator.Enabled = false
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})
	})

	// This set of tests should be executed last so that the stop / start
	// of the daemon sets does not affect the execution of other tests
	Context("Cluster Policy Updates", Ordered, func() {
		cmName := "test-topology"
		BeforeAll(func() {
			By("restarting controller")
			RestartOperator(Default, ctx, testConfig, k8sClientset)
		})
		AfterEach(func() {
			By("delete configmap")
			Eventually(func(g Gomega) {
				err := k8sClientset.CoreV1().ConfigMaps(OperatorNamespace).Delete(ctx, cmName, metav1.DeleteOptions{})
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
		})
		It("can change config path and file", func() {
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			resourceName := "ibm.com/spyre_pf"
			configPath := "/etc/ibm/spyre"
			configName := "senlib_config.json"
			metricsPath := "/tmp/data"
			expectedDevicePluginEnv := map[string]string{
				spyreconst.DeviceConfigOutputPathKey: configPath,
				spyreconst.DeviceConfigFileNameKey:   configName,
				spyreconst.MetricsContainerPathKey:   metricsPath,
			}
			expectedMetricsExporterEnv := map[string]string{
				spyreconst.DeviceConfigFileNameKey: configName,
				spyreconst.MetricsContainerPathKey: metricsPath,
			}
			expectedPaths := []string{
				"/etc/ibm/spyre/senlib_config.json",
				"/etc/ibm/spyre/resource_pool",
				// TODO: remove the following two lines after merge updated device plugin
				"/tmp/data/POD_NAME",
				"/tmp/data/POD_NAMESPACE",
				"/etc/ibm/spyre/POD_NAME",
				"/etc/ibm/spyre/POD_NAMESPACE",
			}
			By("changing config/metrics path in the cluster policy")
			clusterPolicy.Spec.DevicePlugin.ConfigPath = configPath
			clusterPolicy.Spec.DevicePlugin.ConfigName = configName
			clusterPolicy.Spec.MetricsExporter.MetricsPath = metricsPath
			clusterPolicy.Spec.MetricsExporter.Enabled = true
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
			By("waiting for device plugin's environment variable change")
			WaitForDevicePluginEnvUpdate(ctx, k8sClientset, OperatorNamespace, targetNodeName, expectedDevicePluginEnv)
			By("waiting for metrics exporter's environment variable change")
			WaitForMetricsExporterEnvUpdate(ctx, k8sClientset, OperatorNamespace, targetNodeName, expectedMetricsExporterEnv)
			By("creating pod to list files")
			pod := BuildPodListFiles("test-config", "default", resourceName, expectedPaths, targetNodeName, true)
			_, err = k8sClientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			By("waiting for pod running")
			WaitForPodRunning(ctx, k8sClientset, pod)
			By("testing files must found")
			CheckPodListFilesLog(ctx, k8sClientset, pod, false, true)
			By("deleting pod")
			DeletePod(ctx, k8sClientset, pod)
			By("reverting SpyreClusterPolicy")
			err = spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.DevicePlugin.ConfigName = ""
			clusterPolicy.Spec.DevicePlugin.ConfigPath = ""
			clusterPolicy.Spec.MetricsExporter.MetricsPath = ""
			clusterPolicy.Spec.MetricsExporter.Enabled = false
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
		})

		It("starts with INFO loglevel if loglevel is not specified", func() {
			Consistently(func(g Gomega) { //nolint:dupl
				opPod, err := k8sClientset.CoreV1().Pods(OperatorNamespace).List(ctx,
					metav1.ListOptions{LabelSelector: "control-plane=spyre-operator"})
				g.Expect(err).To(BeNil())
				g.Expect(len(opPod.Items)).Should(Equal(1))
				req := k8sClientset.CoreV1().Pods(OperatorNamespace).GetLogs(
					opPod.Items[0].Name, &v1.PodLogOptions{Container: "manager"})
				podLog, err := req.Stream(ctx)
				g.Expect(err).To(BeNil())
				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, podLog)
				g.Expect(err).To(BeNil())
				g.Expect(buf.String()).ShouldNot(ContainSubstring("DEBUG"))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
		})

		It("can dynamically change loglevel to DEBUG", func() {
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			debugLevel := "debug"
			clusterPolicy.Spec.LogLevel = &debugLevel
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
			Eventually(func(g Gomega) { //nolint:dupl
				opPod, err := k8sClientset.CoreV1().Pods(OperatorNamespace).List(ctx,
					metav1.ListOptions{LabelSelector: "control-plane=spyre-operator"})
				g.Expect(err).To(BeNil())
				g.Expect(len(opPod.Items)).Should(Equal(1))
				req := k8sClientset.CoreV1().Pods(OperatorNamespace).GetLogs(
					opPod.Items[0].Name, &v1.PodLogOptions{Container: "manager"})
				podLog, err := req.Stream(ctx)
				g.Expect(err).To(BeNil())
				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, podLog)
				g.Expect(err).To(BeNil())
				g.Expect(buf.String()).Should(ContainSubstring("DEBUG"))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
		})

		It("can use topology from configmap", func() {
			By("disabling init container")
			UpdateInitContainer(ctx, spyreV2Client, k8sClientset, len(nodeNames), testConfig, spyrev1alpha1.ExecuteIfNotPresent, false, spyrev1alpha1.Ready)
			By("creating configmap")
			cmName := "test-topology"
			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: OperatorNamespace,
				},
				Data: map[string]string{
					targetNodeName: "{\"num_devices\": 1, \"version\": 2.0, \"devices\": {\"0000:1a:00.0\":{\"name\":\"IBM Device 06a7 (rev 01)\",\"numanode\":0,\"linkspeed\":\"15.75 GB/s\",\"peers\":{}}}}",
				},
			}
			_, err := k8sClientset.CoreV1().ConfigMaps(OperatorNamespace).Create(ctx, cm, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			By("updating SpyreClusterPolicy")
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err = spyreV2Client.Get(ctx,
				client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy)
			Expect(err).To(BeNil())
			clusterPolicy.Spec.DevicePlugin.TopologyConfigMapName = cmName
			UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)
			By("checking topology mount")
			Eventually(func(g Gomega) {
				devicePluginPod := GetDevicePluginPod(ctx, k8sClientset, g, OperatorNamespace, targetNodeName)
				mnts := devicePluginPod.Spec.Containers[0].VolumeMounts
				volumes := devicePluginPod.Spec.Volumes
				foundMount := false
				foundVolume := false
				var foundPath, foundConfigMap string
				for _, mnt := range mnts {
					if mnt.Name == cmName {
						foundMount = true
						foundPath = mnt.MountPath
						break
					}
				}
				for _, volume := range volumes {
					if volume.Name == cmName {
						foundVolume = true
						foundConfigMap = volume.ConfigMap.Name
						break
					}
				}
				g.Expect(foundMount).To(Equal(true))
				// From this point, all assertions must be passed. Otherwise, exit with error.
				Expect(foundPath).To(BeEquivalentTo(spyreconst.DefaultTopologyFolder))
				Expect(foundVolume).To(Equal(true))
				Expect(foundConfigMap).To(BeEquivalentTo(cmName))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
			By("checking SpyreNodeState update")
			Eventually(func(g Gomega) {
				spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
				g.Expect(err).To(BeNil())
				g.Expect(spyrens.Spec.SpyreInterfaces).To(HaveLen(1))
				pcitopoMap, err := getPciTopoFromSpyreNodeState(ctx)
				g.Expect(err).To(BeNil())
				devices, ok := pcitopoMap["devices"]
				Expect(ok).To(BeTrue())
				devicesMap, ok := devices.(map[string]interface{})
				Expect(ok).To(BeTrue())
				g.Expect(devicesMap).To(HaveLen(1))
			}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
			By("setting init container back")
			UpdateInitContainer(ctx, spyreV2Client, k8sClientset, len(nodeNames), testConfig, spyrev1alpha1.ExecuteIfNotPresent, testConfig.DevicePluginInit.Enabled, spyrev1alpha1.Ready)
		})
	})

	AfterAll(func() {
		if len(nodeNames) > 0 {
			By("writing log and node state")
			WriteLogAndState(ctx, k8sClientset, spyreV2Client, Default, targetNodeName)
		}
		By("cleaning up")
		err := cleanupTestNamespace(ctx, k8sClientset, testNamespace)
		Expect(err).To(BeNil())
	})
})

func cleanupTestNamespace(ctx context.Context, c *kubernetes.Clientset, namespace string) error {
	By("cleaning up test namespace")
	var result *multierror.Error
	podList, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		By("deleting pods")
		for _, pod := range podList.Items {
			err := c.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
			if err != nil {
				GinkgoWriter.Printf("Error encountered while deleting pods %s from %s namespace, %v", pod.Name, namespace, err)
				result = multierror.Append(result, err)
			}
		}
	}
	deployList, err := c.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		By("deleting deployments")
		for _, deploy := range deployList.Items {
			err := c.AppsV1().Deployments(namespace).Delete(ctx, deploy.Name, metav1.DeleteOptions{})
			if err != nil {
				GinkgoWriter.Printf("Error encountered while deleting pods %s from %s namespace, %v", deploy.Name, namespace, err)
				result = multierror.Append(result, err)
			}
		}
	}
	err = result.ErrorOrNil()
	if err != nil {
		return fmt.Errorf("failed to cleanup test namespace '%s': %w", namespace, err)
	}
	return nil
}

func getPciTopoFromSpyreNodeState(ctx context.Context) (map[string]interface{}, error) {
	var pcitopoMap map[string]interface{}
	var err error
	Eventually(func(g Gomega) {
		var spyrens spyrev1alpha1.SpyreNodeState
		spyrens, err = GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
		g.Expect(err).To(BeNil())
		pcitopo := spyrens.Spec.Pcitopo
		err = json.Unmarshal([]byte(pcitopo), &pcitopoMap)
		g.Expect(err).To(BeNil())
		g.Expect(pcitopoMap).NotTo(BeNil())
	}).WithTimeout(150 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
	if err != nil {
		return nil, fmt.Errorf("failed to get PCI topology from nodestate: %w", err)
	}
	return pcitopoMap, nil
}

func getNumDevices() int {
	numDevices := len(allDeviceList)
	return numDevices
}

func getDeviceList(ctx context.Context) (deviceList []string) {
	pcitopoMap, err := getPciTopoFromSpyreNodeState(ctx)
	Expect(err).To(BeNil())
	devices, ok := pcitopoMap["devices"]
	Expect(ok).To(BeTrue())
	devicesMap, ok := devices.(map[string]interface{})
	Expect(ok).To(BeTrue())
	Expect(len(devicesMap)).To(BeNumerically(">=", 2))
	for device := range devicesMap {
		deviceList = append(deviceList, device)
	}
	return deviceList
}

func commonAllocationTestCase(ctx context.Context, testNamespace string, schedulerEnabled bool) {
	nPods := 25
	nSpyres := getNumDevices()

	By("creating Pods")
	pods := BuildPods(nPods, petname.Generate(2, "-"), testNamespace, "ibm.com/spyre_pf", 1, targetNodeName, schedulerEnabled)
	for _, pod := range pods {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			g.Expect(err).To(BeNil())
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}

	By("deleting pods, then asserting pod and allocation statuses")
	var runningPods *v1.PodList
	for nRemainingPods := nPods; nRemainingPods > 0; nRemainingPods -= nSpyres {
		Eventually(func(g Gomega) {
			// assert expected number of Pods are running
			// expected number should be equals to the number of Spyres unless the number of
			// remaining Pods is less than the number of Spyres.
			expected := nSpyres
			if nRemainingPods < nSpyres {
				expected = nRemainingPods
			}

			failedPods, err := k8sClientset.CoreV1().Pods(testNamespace).List(ctx,
				metav1.ListOptions{FieldSelector: "status.phase=Failed"})
			Expect(err).To(BeNil())
			Expect(failedPods.Items).To(HaveLen(0))
			runningPods, err = k8sClientset.CoreV1().Pods(testNamespace).List(ctx,
				metav1.ListOptions{FieldSelector: "status.phase=Running"})
			g.Expect(err).To(BeNil())
			g.Expect(len(runningPods.Items)).Should(BeNumerically("==", expected), fmt.Sprintf("%v\n", runningPods))
			// assert each Pod can read senlib_config.json
			for _, p := range runningPods.Items {
				senlibConfig := GetSenlibConfig(ctx, k8sClientset, p.Name, p.Namespace)
				g.Expect(len(senlibConfig.General.PciAddresses)).Should(Equal(1))
			}
			// asset allocation of SpyreNodeState is expected
			spyrens, err := GetSpyreNodeState(ctx, spyreV2Client, targetNodeName)
			g.Expect(err).To(BeNil())
			g.Expect(len(spyrens.Status.AllocationList)).To(Equal(len(runningPods.Items)))
		}).WithTimeout(180 * time.Second).WithPolling(10 * time.Second).Should(Succeed())

		// delete all of the running Pods
		for _, p := range runningPods.Items {
			err := k8sClientset.CoreV1().Pods(testNamespace).Delete(
				ctx, p.Name, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
		}
	}

	By("waiting for all pods to be deleted", func() {
		Eventually(func(g Gomega) {
			podList, err := k8sClientset.CoreV1().Pods(testNamespace).List(ctx, metav1.ListOptions{})
			Expect(err).To(BeNil())
			g.Expect(podList.Items).To(HaveLen(0))
		}).WithTimeout(60 * time.Second).WithPolling(10 * time.Second).Should(Succeed())
	})
}
