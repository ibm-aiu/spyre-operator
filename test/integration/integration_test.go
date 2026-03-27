/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	testutils "github.com/ibm-aiu/spyre-operator/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	corev1 "k8s.io/api/core/v1"
	k8sErrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type entry struct {
	ScriptName string
	Doom       bool
}

var discoClient *discovery.DiscoveryClient
var amd64arch bool
var ppc64le bool
var nodeFilter []string
var pfWrkmap map[string]string
var vfWrkmap map[string]string
var vfPerDevWorker1, pf1Worker1, pf1Worker3, vf1Worker3, vf1Worker1 testutils.PodTemplateData
var spyreWorkers, workers []string
var spyreWorker1, worker2, spyreWorker3 string

var _ = Describe("integration test", Label("integration", "cardmgmt"), Ordered, ContinueOnFailure, func() {

	ctx := context.Background()
	format.MaxLength = 100000

	BeforeAll(func() {
		var err error
		discoClient, err = discovery.NewDiscoveryClientForConfig(config)
		Expect(err).To(BeNil())
		amd64arch, err = testutils.IsAmd64Arch(ctx, k8sClientset)
		Expect(err).To(BeNil())
		ppc64le, err = testutils.IsPpc64LeArch(ctx, k8sClientset)
		Expect(err).To(BeNil())
		nodeFilter = strings.Split(*itConfig.CardManagement.Config.SpyreFilter, "|")
	})

	Context("Spyre operator must be able to enable card management with VF on x86_64", Ordered, func() {
		var clusterPolicy = &spyrev1alpha1.SpyreClusterPolicy{}

		BeforeAll(func() {
			if !amd64arch {
				Skip("tests skipped due to cluster is not amd64")
			}
			By("wait for the Spyre worker node state to be populated")
			testutils.WaitForSpyreWorkerNodeStatePopulated(ctx, spyreV2Client, k8sClientset)
			pfWrkmap = testutils.RunnerWorkerMap(ctx, spyreV2Client, k8sClientset, "pf", nodeFilter)
			Expect(len(pfWrkmap)).To(BeNumerically(">=", 1))
			vfWrkmap = testutils.RunnerWorkerMap(ctx, spyreV2Client, k8sClientset, "vf", nodeFilter)
			Expect(len(vfWrkmap)).To(BeNumerically(">=", 1))
			renewSpyreAppsNamespace(ctx)
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: testutils.ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
			Expect(err).To(BeNil())

			By("update general config in ClusterPolicy for the test")
			clusterPolicy.Spec.PodValidator.Enabled = true
			testutils.EnableInitContainer(clusterPolicy, itConfig, spyrev1alpha1.ExecuteAlways)
		})
		It("should be able to enable card management", func() {
			By("update card management to be Enabled")
			if clusterPolicy.Spec.CardManagement.Enabled {
				Skip("Skip test due to card management is already enabled")
			}
			clusterPolicy.Spec.CardManagement.Enabled = true
			clusterPolicy.Spec.CardManagement.ImagePullPolicy = "Always"
			testutils.UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)

		})
		It("should have cardmgmt deployment on x86_64", func() {
			Eventually(func(g Gomega) {
				daemonsets, err := k8sClientset.AppsV1().DaemonSets("spyre-operator").List(ctx, metav1.ListOptions{
					LabelSelector: "app=cardmgmt",
				})
				g.Expect(err).To(BeNil())
				Expect(len(daemonsets.Items)).To(BeNumerically(">=", 1))
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

			By("one spyre card management pod should be created")
			pods, err := k8sClientset.CoreV1().Pods("spyre-operator").List(ctx, metav1.ListOptions{
				LabelSelector: "app=cardmgmt",
			})
			Expect(err).To(BeNil())
			Expect(len(pods.Items)).To(BeNumerically("==", 1))
		})

		It("should have cardmgmt-svc service on x86_64", func() {
			Eventually(func(g Gomega) {
				_, err := k8sClientset.CoreV1().Services("spyre-operator").Get(ctx, "cardmgmt-svc", metav1.GetOptions{})
				g.Expect(err).To(BeNil())

			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("deployment of VF Pods", Label("vf"), Ordered, func() {
		BeforeAll(func() {
			if ppc64le {
				Skip("tests skipped due to cluster is ppc64le")
			}
		})
		BeforeEach(func() {
			renewSpyreAppsNamespace(ctx)
		})

		It("Can handle one Spyre VF request", func() {
			var spyrens spyrev1alpha1.SpyreNodeState

			pod1data := testutils.PodTemplateData{
				Name:             "pod1",
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
			}
			pod1 := createPodFromTemplateAndWait(ctx, pod1data, nodeFilter, testutils.PodTemplate, "Running")

			By("check SpyreNodeState has allocated device for pod")
			nodeName := pod1.Spec.NodeName
			Expect(nodeName).NotTo(BeNil(), "stdout: %s", nodeName)
			Eventually(func(g Gomega) {
				obj, err := testutils.GetResource(ctx, dynClient, "", nodeName, "spyrenodestates.v1alpha1.spyre.ibm.com")
				g.Expect(err).To(BeNil())
				runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &spyrens)
				len := testutils.NumDeviceSpyrensForPod(pod1data.Name, "spyre-apps", spyrens.Status)
				g.Expect(len).To(BeEquivalentTo(1))
			}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("Can handle Pod with multiple containers", func() {
			By("Deploy the Pod with multi containers")
			testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps",
				filepath.Join("..", "manifest", "workloads", "multicontainer-pod.yaml"))
			By("Check Pod in Running state")
			Eventually(func(g Gomega) {
				mutipod, err := k8sClientset.CoreV1().Pods("spyre-apps").Get(ctx, "multipod-spyre", metav1.GetOptions{})
				g.Expect(err).To(BeNil())
				g.Expect(mutipod.Status.Phase).To(BeEquivalentTo("Running"))
			})
		})

		// Skip small toy test. Original image is obsolete.
		PIt("run small-toy.py on spyre VF", func() {

			By("create small toy config map")
			smallToyCM := entry{
				ScriptName: "small-toy.py",
				Doom:       true,
			}
			smallToyCmYaml := testutils.YamlFromTemplate(testutils.WorkloadConfigMapTemplate, smallToyCM)
			_, err := testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps", smallToyCmYaml)
			Expect(err).To(BeNil())
			defer os.Remove(smallToyCmYaml)

			By("create small toy pod")
			smallToyPodData := testutils.PodTemplateData{
				Name:             "small-toy",
				Image:            itConfig.WorkloadImage,
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
				FlexDevice:       "VF",
			}
			if len(nodeFilter) > 0 {
				smallToyPodData.NodeSelectorNode = nodeFilter[0]
			}
			smallToyYaml := testutils.YamlFromTemplate(testutils.WorkloadPodTemplate, smallToyPodData)
			defer os.Remove(smallToyYaml)
			_, err = testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps", smallToyYaml)
			Expect(err).To(BeNil())
			By("wait small toy to run successfully")
			Eventually(func(g Gomega) {
				pod, err := k8sClientset.CoreV1().Pods("spyre-apps").Get(ctx, smallToyPodData.Name, metav1.GetOptions{})
				g.Expect(err).To(BeNil())
				log, err := testutils.GetPodLog(ctx, k8sClientset, "app", *pod)
				g.Expect(err).To(BeNil())
				if strings.Contains(log, "FAILED") {
					Fail(fmt.Sprintf("%s workload log: %s", smallToyPodData.Name, log))
				}
				g.Expect(pod.Status.Phase).To(BeEquivalentTo("Succeeded"))
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})

	})

	Context("scheduler with card management enabled", Ordered, func() {
		var clusterPolicy = &spyrev1alpha1.SpyreClusterPolicy{}
		BeforeAll(func() {
			if ppc64le {
				Skip("tests skipped due to cluster is ppc64le")
			}

			// Need three worker nodes to run this test.
			workers = testutils.GetWorkerNodeNames(ctx, k8sClientset)
			Expect(len(workers)).To(BeNumerically(">=", 3))

			// Two of the workers need to have Spyre devices.
			spyreWorkers = testutils.GetSpyreWorkerNodeNames(ctx, k8sClientset)
			Expect(len(spyreWorkers)).To(BeNumerically(">=", 2))
			spyreWorker1 = spyreWorkers[0]                                                       // spyre worker-1
			spyreWorker3 = spyreWorkers[1]                                                       // spyre worker-3
			worker2 = testutils.NodeDifference(workers, []string{spyreWorker1, spyreWorker3})[0] // worker-2

			// Detect available Spyre device on worker-1
			avVFSpyreAddrs, found := testutils.GetAvailableVFSpyreInterface(ctx, k8sClientset, spyreV2Client, []string{spyreWorker1})
			Expect(found).To(BeTrue())

			// cases needs runtime variables
			vfPerDevWorker1 = testutils.PodTemplateData{
				Name:             "vf-aa-worker-1",
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_vf_" + avVFSpyreAddrs[0],
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
			}

			pf1Worker1 = testutils.PodTemplateData{
				Name:             "pf1-worker-1",
				NodeSelectorNode: spyreWorker1,
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
			}

			pf1Worker3 = testutils.PodTemplateData{
				Name:             "pf1-worker-3",
				NodeSelectorNode: spyreWorker3,
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
			}
			vf1Worker1 = testutils.PodTemplateData{
				Name:             "vf1-worker-1",
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
				NodeSelectorNode: spyreWorker1,
			}
			vf1Worker3 = testutils.PodTemplateData{
				Name:             "vf1-worker-3",
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
				NodeSelectorNode: spyreWorker3,
			}
		})

		BeforeEach(func() {
			renewSpyreAppsNamespace(ctx)
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: testutils.ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
			Expect(err).To(BeNil())
			testutils.EnableInitContainer(clusterPolicy, itConfig, spyrev1alpha1.ExecuteAlways)
		})
		// Following test temporarily skipped since it requires 2 nodes. Current test environment only has 1 node.
		PIt("verify Pod deployment running when spyreFilter: worker-1", func() {
			testutils.EnabledCardmgmtForWorkers(ctx, clusterPolicy, spyreV2Client, k8sClientset, spyreWorker1)
			createPodsFromTemplateListAndDelete(ctx, append(testutils.CardmgmtEnableWorker1TestRunning, vfPerDevWorker1, pf1Worker3, vf1Worker3), []string{}, testutils.CardmgmtTestPodTemplate, "Running")
		})

		It("verify Pod deployment pending when spyreFilter: worker-1", func() {
			testutils.EnabledCardmgmtForWorkers(ctx, clusterPolicy, spyreV2Client, k8sClientset, spyreWorker1)
			createPodsFromTemplateListAndDelete(ctx, append(testutils.CardmgmtEnableWorker1TestPending, pf1Worker1), []string{}, testutils.CardmgmtTestPodTemplate, "Pending")
		})

		It("verify Pods deployment running when spyreFilter: .", func() {
			testutils.EnabledCardmgmtForWorkers(ctx, clusterPolicy, spyreV2Client, k8sClientset, ".")
			createPodsFromTemplateListAndDelete(ctx, append(testutils.CardmgmtEnableAllNodesTestRunning, vfPerDevWorker1, vf1Worker1, vf1Worker3), []string{}, testutils.CardmgmtTestPodTemplate, "Running")
		})

		It("verify Pods deployment pending when spyreFilter: .", func() {
			testutils.EnabledCardmgmtForWorkers(ctx, clusterPolicy, spyreV2Client, k8sClientset, ".")
			createPodsFromTemplateListAndDelete(ctx, append(testutils.CardmgmtEnableAllNodesTestPending, pf1Worker1, pf1Worker3), []string{}, testutils.CardmgmtTestPodTemplate, "Pending")
		})

		It("verify Pods deployment running when spyreFilter: worker-2", func() {
			testutils.EnabledCardmgmtForWorkers(ctx, clusterPolicy, spyreV2Client, k8sClientset, worker2)
			createPodsFromTemplateListAndDelete(ctx, append(testutils.CardmgmtEnableWorker2TestRunning, vfPerDevWorker1, pf1Worker1, pf1Worker3, vf1Worker1, vf1Worker3), []string{}, testutils.CardmgmtTestPodTemplate, "Running")
		})
	})
})

var _ = Describe("integration test", Label("integration"), Ordered, ContinueOnFailure, func() {
	ctx := context.Background()
	BeforeAll(func() {
		var err error
		discoClient, err = discovery.NewDiscoveryClientForConfig(config)
		Expect(err).To(BeNil())
		amd64arch, err = testutils.IsAmd64Arch(ctx, k8sClientset)
		Expect(err).To(BeNil())
		ppc64le, err = testutils.IsPpc64LeArch(ctx, k8sClientset)
		Expect(err).To(BeNil())
		nodeFilter = strings.Split(*itConfig.CardManagement.Config.SpyreFilter, "|")
	})
	Context("Spyre can disable card management on x86_64", Ordered, func() {
		BeforeAll(func() {
			if !amd64arch {
				Skip("tests skipped due to cluster is not amd64")
			}
		})

		It("should be able to disable card management", func() {

			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: testutils.ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
			Expect(err).To(BeNil())
			if !clusterPolicy.Spec.CardManagement.Enabled {
				Skip("Skip test due to card management is already disabled")
			}
			clusterPolicy.Spec.CardManagement.Enabled = false
			testutils.UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeNames), spyrev1alpha1.Ready)

			By("Card management Deployment must NOT be found")
			Eventually(func(g Gomega) {
				_, err := k8sClientset.AppsV1().Deployments("spyre-operator").Get(ctx, "spyre-card-management", metav1.GetOptions{})
				g.Expect(k8sErrs.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

			By("Card management pod must NOT be found")
			Eventually(func(g Gomega) {
				pods, err := k8sClientset.CoreV1().Pods("spyre-operator").List(ctx, metav1.ListOptions{
					LabelSelector: "app=cardmgmt",
				})
				g.Expect(err).To(BeNil())
				g.Expect(pods.Items).To(BeEmpty())
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

			By("PF runner pod must NOT be found")
			Eventually(func(g Gomega) {
				for pfpodname := range pfWrkmap {
					_, err := k8sClientset.CoreV1().Pods("spyre-operator").Get(ctx, pfpodname, metav1.GetOptions{})
					g.Expect(k8sErrs.IsNotFound(err)).To(BeTrue())
				}
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

			By("VF runner pod must NOT be found")
			Eventually(func(g Gomega) {
				for vfpodname := range vfWrkmap {
					_, err := k8sClientset.CoreV1().Pods("spyre-operator").Get(ctx, vfpodname, metav1.GetOptions{})
					g.Expect(k8sErrs.IsNotFound(err)).To(BeTrue())
				}
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("deployment of VF Pods when cardmgmt is disabled", Label("vf"), Ordered, func() {
		BeforeAll(func() {
			if ppc64le {
				Skip("tests skipped due to cluster is ppc64le")
			}
			renewSpyreAppsNamespace(ctx)
		})
		It("User Pod request 1 VF resource", func() {
			pod1data := testutils.PodTemplateData{
				Name:             "pod2",
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
			}
			pod1Yaml := testutils.YamlFromTemplate(testutils.PodTemplate, pod1data)
			defer os.Remove(pod1Yaml)

			By("check pod deployment")
			_, err := testutils.CreateResourceFromYaml(
				ctx, dynClient, discoClient, "spyre-apps", pod1Yaml)
			Expect(err).To(BeNil(), "expect no error but get error in pod: %s", printFile(pod1Yaml))
			Eventually(func(g Gomega) {
				pod, err := k8sClientset.CoreV1().Pods("spyre-apps").Get(ctx, pod1data.Name, metav1.GetOptions{})
				g.Expect(err).To(BeNil())
				g.Expect(pod.Status.Phase).To(BeEquivalentTo("Running"))
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})
		It("Can handle Pod with multiple containers", func() {
			By("Deploy the Pod with multi containers")
			testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps",
				filepath.Join("..", "manifest", "workloads", "multicontainer-pod.yaml"))
			By("Check Pod in Running state")
			Eventually(func(g Gomega) {
				mutipod, err := k8sClientset.CoreV1().Pods("spyre-apps").Get(ctx, "multipod-spyre", metav1.GetOptions{})
				g.Expect(err).To(BeNil())
				g.Expect(mutipod.Status.Phase).To(BeEquivalentTo("Running"))
			})
		})
	})

	Context("deployment of PF Pods when cardmgmt is disabled", Ordered, func() {
		var spyrens spyrev1alpha1.SpyreNodeState
		var avif []string
		var pod11data, pod2data testutils.PodTemplateData

		BeforeEach(func() {
			renewSpyreAppsNamespace(ctx)
		})

		It("can handle two Spyre PF request", func() {

			pod2data = testutils.PodTemplateData{
				Name:             "pod2",
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "2",
			}
			avif, _ = testutils.GetAvailableSpyreInterface(ctx, k8sClientset, spyreV2Client, []string{})
			Expect(len(avif)).To(BeNumerically(">=", 2), "At least need 1 Spyre PCIs but got: %d", len(avif))
			pod2 := createPodFromTemplateAndWait(ctx, pod2data, nodeFilter, testutils.PodTemplate, "Running")

			By("check SpyreNodeState has allocated device for pod")
			nodeName := pod2.Spec.NodeName
			Expect(nodeName).NotTo(BeNil(), "stdout: %s", nodeName)
			Eventually(func(g Gomega) {
				obj, err := testutils.GetResource(ctx, dynClient, "", nodeName, "spyrenodestates.v1alpha1.spyre.ibm.com")
				g.Expect(err).To(BeNil())
				runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &spyrens)
				len := testutils.NumDeviceSpyrensForPod(pod2data.Name, "spyre-apps", spyrens.Status)
				g.Expect(len).To(BeNumerically("==", 2))
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

			testutils.DeletePod(ctx, k8sClientset, pod2)

			By("check SpyreNodeState has de-allocated device for pod")
			Eventually(func(g Gomega) {
				obj, err := testutils.GetResource(ctx, dynClient, "", nodeName, "spyrenodestates.v1alpha1.spyre.ibm.com")
				g.Expect(err).To(BeNil())
				runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &spyrens)
				len := testutils.NumDeviceSpyrensForPod(pod2data.Name, "spyre-apps", spyrens.Status)
				g.Expect(len).To(BeNumerically("==", 0))
			}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("can handle one per-device Spyre request", func() {

			avif, found := testutils.GetAvailableSpyreInterface(ctx, k8sClientset, spyreV2Client, []string{})
			Expect(found).To(BeTrue(), "At least need 1 Spyre PCIs but got: %d", len(avif))
			pod11data = testutils.PodTemplateData{
				Name:             "pod11",
				ResourceName:     fmt.Sprintf("ibm.com/spyre_pf_%s", avif[0]),
				ResourceQuantity: "1",
			}
			pod11 := createPodFromTemplateAndWait(ctx, pod11data, nodeFilter, testutils.PodTemplate, "Running")

			By("check PCIDEVICE_IBM_COM_AIU_PF")
			podcmd := []string{"bash", "-lc", "env"}
			out, err := testutils.ExecCommand(ctx, config, k8sClientset, "spyre-apps", pod11data.Name, podcmd)
			Expect(err).To(BeNil())
			Expect(out).Should(ContainSubstring("PCIDEVICE_IBM_COM_AIU_PF"))

			By("check validity of SpyreNodeState")
			nodeName := pod11.Spec.NodeName
			Expect(nodeName).NotTo(BeNil(), "stdout: %s", nodeName)
			Eventually(func(g Gomega) {
				obj, err := testutils.GetResource(ctx, dynClient, "", nodeName, "spyrenodestates.v1alpha1.spyre.ibm.com")
				g.Expect(err).To(BeNil())
				runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &spyrens)
				len := testutils.NumDeviceSpyrensForPod(pod11data.Name, "spyre-apps", spyrens.Status)
				g.Expect(len).To(BeNumerically("==", 1))
			})

			testutils.DeletePod(ctx, k8sClientset, pod11)

			Eventually(func(g Gomega) {
				obj, err := testutils.GetResource(ctx, dynClient, "", nodeName, "spyrenodestates.v1alpha1.spyre.ibm.com")
				g.Expect(err).To(BeNil())
				runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &spyrens)
				len := testutils.NumDeviceSpyrensForPod(pod11data.Name, "spyre-apps", spyrens.Status)
				g.Expect(len).To(BeNumerically("==", 0))
			}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("can handle one PF per-device Spyre request and then two PF Spyre request", func() {
			avif, _ := testutils.GetAvailableSpyreInterface(ctx, k8sClientset, spyreV2Client, []string{})
			Expect(len(avif)).To(BeNumerically(">=", 3), "At least 3 Spyre PCIs needed but got: %d", len(avif))
			pod11 := createPodFromTemplateAndWait(ctx, pod11data, nodeFilter, testutils.PodTemplate, "Running")

			By("check validity of SpyreNodeState")
			nodeName := pod11.Spec.NodeName
			Expect(nodeName).NotTo(BeNil(), "stdout: %s", nodeName)
			Eventually(func(g Gomega) {
				obj, err := testutils.GetResource(ctx, dynClient, "", nodeName, "spyrenodestates.v1alpha1.spyre.ibm.com")
				g.Expect(err).To(BeNil())
				runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &spyrens)
				len := testutils.NumDeviceSpyrensForPod(pod11data.Name, "spyre-apps", spyrens.Status)
				g.Expect(len).To(BeNumerically("==", 1))
			})

			By("allocation of two Spyres")
			pod2 := createPodFromTemplateAndWait(ctx, pod2data, nodeFilter, testutils.PodTemplate, "Running")
			testutils.DeletePod(ctx, k8sClientset, pod11)
			testutils.DeletePod(ctx, k8sClientset, pod2)
		})

		// Skip small toy test. Original image is obsolete.
		PIt("run small-toy.py with 2 spyre PFs", Label("extended"), func() {
			By("create small toy config map")
			smallToyCM := entry{
				ScriptName: "small-toy.py",
				Doom:       false,
			}
			smallToyCmYaml := testutils.YamlFromTemplate(testutils.WorkloadConfigMapTemplate, smallToyCM)
			_, err := testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps", smallToyCmYaml)
			Expect(err).To(BeNil())
			defer os.Remove(smallToyCmYaml)

			By("create small toy pod")
			smallToyPodData := testutils.PodTemplateData{
				Name:             "small-toy",
				Image:            itConfig.WorkloadImage,
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "2",
				FlexDevice:       "PF",
			}
			if len(nodeFilter) > 0 {
				smallToyPodData.NodeSelectorNode = nodeFilter[0]
			}
			smallToyYaml := testutils.YamlFromTemplate(testutils.WorkloadPodTemplate, smallToyPodData)
			_, err = testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps", smallToyYaml)
			Expect(err).To(BeNil())
			By("wait small toy to run successfully")
			Eventually(func(g Gomega) {
				pod, err := k8sClientset.CoreV1().Pods("spyre-apps").Get(ctx, smallToyPodData.Name, metav1.GetOptions{})
				g.Expect(err).To(BeNil())
				log, err := testutils.GetPodLog(ctx, k8sClientset, "app", *pod)
				g.Expect(err).To(BeNil())
				if strings.Contains(log, "FAILED") {
					Fail(fmt.Sprintf("%s workload log: %s", smallToyPodData.Name, log))
				}
				g.Expect(pod.Status.Phase).To(BeEquivalentTo("Succeeded"))
			}).WithTimeout(720 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("scheduler with card management disabled", Ordered, func() {

		BeforeAll(func() {
			if ppc64le {
				Skip("tests skipped due to cluster is ppc64le")
			}
			renewSpyreAppsNamespace(ctx)
		})
		// Since we only have a single node, skipping this.
		PIt("verify Pod deployment", func() {
			// Need three worker nodes to run this test.
			workers = testutils.GetWorkerNodeNames(ctx, k8sClientset)
			Expect(len(workers)).To(BeNumerically(">=", 3))

			// Two of the workers need to have Spyre devices.
			spyreWorkers = testutils.GetSpyreWorkerNodeNames(ctx, k8sClientset)
			Expect(len(spyreWorkers)).To(BeNumerically(">=", 2))
			spyreWorker1 = spyreWorkers[0]                                                       // spyre worker-1
			spyreWorker3 = spyreWorkers[1]                                                       // spyre worker-3
			worker2 = testutils.NodeDifference(workers, []string{spyreWorker1, spyreWorker3})[0] // worker-2

			// Detect available Spyre device on worker-1
			avVFSpyreAddrs, found := testutils.GetAvailableVFSpyreInterface(ctx, k8sClientset, spyreV2Client, []string{spyreWorker1})

			Expect(found).To(BeTrue())
			vfPerDevWorker1 = testutils.PodTemplateData{
				Name:             "vf-aa-worker-1",
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_vf_" + avVFSpyreAddrs[0],
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
			}
			pf1Worker1 = testutils.PodTemplateData{
				Name:             "pf1-worker-1",
				NodeSelectorNode: spyreWorker1,
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
			}

			pf1Worker3 = testutils.PodTemplateData{
				Name:             "pf1-worker-3",
				NodeSelectorNode: spyreWorker3,
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
			}
			vf1Worker1 = testutils.PodTemplateData{
				Name:             "vf1-worker-1",
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
				NodeSelectorNode: spyreWorker1,
			}
			vf1Worker3 = testutils.PodTemplateData{
				Name:             "vf1-worker-3",
				Image:            testutils.Ubi9MicroTestImage,
				ResourceName:     "ibm.com/spyre_vf",
				ResourceQuantity: "1",
				SidecarName:      "sidecar",
				NodeSelectorNode: spyreWorker3,
			}
			createPodsFromTemplateListAndDelete(ctx, append(testutils.CardmgmtDisabledTestRunning, vfPerDevWorker1, pf1Worker1, pf1Worker3, vf1Worker1, vf1Worker3), []string{}, testutils.CardmgmtTestPodTemplate, "Running")
		})
	})

	Context("PF metrics exporter", Ordered, func() {
		BeforeAll(func() {
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: testutils.ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
			Expect(err).To(BeNil())
			Expect(clusterPolicy.Spec.MetricsExporter.Enabled).To(BeTrue(), "Metric export must be enabled")
			if !clusterPolicy.Spec.MetricsExporter.Enabled {
				Skip("Skip test due to metrics exporter is disabled")
			}
		})
		BeforeEach(func() {
			renewSpyreAppsNamespace(ctx)
		})

		It("can get metrics", func() {
			promhttpApiEndpoint := "https://prometheus-k8s.openshift-monitoring.svc.cluster.local:9091/api/v1/query?"
			token := config.BearerToken
			By("run a PF workload Pod")
			metricsPodData := testutils.PodTemplateData{
				Name:             "metrics-test",
				ResourceName:     "ibm.com/spyre_pf",
				ResourceQuantity: "1",
			}
			createPodFromTemplateAndWait(ctx, metricsPodData, nodeFilter, testutils.PodTemplate, "Running")

			By("create a curl Pod")
			curlPodData := testutils.PodTemplateData{
				Name:  "curl-pod",
				Image: "quay.io/curl/curl:latest",
			}
			createPodFromTemplateAndWait(ctx, curlPodData, nodeFilter, testutils.CurlPodTemplate, "Running")

			By("query spyre_allocation")
			curlcmd := []string{
				"sh", "-c",
				fmt.Sprintf("curl -q -k -s -H \"Authorization: Bearer %s\" %s%s", token, promhttpApiEndpoint, "query=spyre_allocation"),
			}
			Eventually(func(g Gomega) {
				out, err := testutils.ExecCommand(ctx, config, k8sClientset, "spyre-apps", curlPodData.Name, curlcmd)
				g.Expect(err).To(BeNil())
				g.Expect(out).Should(And(
					ContainSubstring(`"status":"success"`),
					ContainSubstring(`"exported_pod":"metrics-test"`),
					ContainSubstring(`"__name__":"spyre_allocation"`),
				))
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

			By("query metric spyre_info_pci_cfg")
			curlcmd = []string{
				"sh", "-c",
				fmt.Sprintf("curl -q -k -s -H \"Authorization: Bearer %s\" %s%s", token, promhttpApiEndpoint, "query=spyre_info_pci_cfg"),
			}
			Eventually(func(g Gomega) {
				out, err := testutils.ExecCommand(ctx, config, k8sClientset, "spyre-apps", curlPodData.Name, curlcmd)
				g.Expect(err).To(BeNil())
				g.Expect(out).Should(And(
					ContainSubstring(`"status":"success"`),
					ContainSubstring(`"__name__":"spyre_info_pci_cfg"`),
				))
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("PF health checker", Ordered, func() {
		BeforeAll(func() {
			clusterPolicy := &spyrev1alpha1.SpyreClusterPolicy{}
			err := spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: testutils.ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
			Expect(err).To(BeNil())
			Expect(clusterPolicy.Spec.HealthChecker.Enabled).To(BeTrue(), "Health checker must be enabled")
			if !clusterPolicy.Spec.HealthChecker.Enabled {
				Skip("Skip test due to health checker is disabled")
			}
		})
		BeforeEach(func() {
			renewSpyreAppsNamespace(ctx)
		})

		It("All devices must be reported as healthy", func() {
			Eventually(func(g Gomega) {
				spyreNodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
				err := spyreV2Client.List(ctx, spyreNodeStateList)
				Expect(err).To(BeNil())
				g.Expect(len(spyreNodeStateList.Items)).To(BeNumerically(">", 0))
				for _, spyrens := range spyreNodeStateList.Items {
					hasDevice := len(spyrens.Spec.SpyreInterfaces) > 0 || len(spyrens.Spec.SpyreSSAInterfaces) > 0
					if hasDevice {
						g.Expect(spyrens.Status).NotTo(BeNil())
						g.Expect(spyrens.Status.Conditions).To(HaveLen(1))
						g.Expect(spyrens.Status.Conditions[0].Type).To(BeEquivalentTo("DeviceHealthy"))
						g.Expect(spyrens.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
						g.Expect(spyrens.Status.UnhealthyDevices).To(BeEmpty())
					}
				}
			}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
		})
	})
})

func printFile(path string) string {
	content, err := os.ReadFile(path)
	Expect(err).To(BeNil())
	return string(content)
}

func createPodFromTemplateAndWait(ctx context.Context, tplData testutils.PodTemplateData, nodeFilter []string, tpl string, phase string) *corev1.Pod {
	var pod *corev1.Pod
	if len(nodeFilter) > 0 {
		tplData.NodeSelectorNode = nodeFilter[0]
	}
	yaml := testutils.YamlFromTemplate(tpl, tplData)
	By(fmt.Sprintf("verify Pod to be %s:\n%s", phase, printFile(yaml)))
	_, err := testutils.CreateResourceFromYaml(ctx, dynClient, discoClient, "spyre-apps", yaml)
	Expect(err).To(BeNil(), "expect no error but get error in pod: %s", printFile(yaml))
	Eventually(func(g Gomega) {
		pod, err = k8sClientset.CoreV1().Pods("spyre-apps").Get(ctx, tplData.Name, metav1.GetOptions{})
		g.Expect(err).To(BeNil())
		g.Expect(pod.Status.Phase).To(BeEquivalentTo(phase))
	}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	return pod
}

func createPodsFromTemplateListAndDelete(ctx context.Context, tplDataList []testutils.PodTemplateData, nodeFilter []string, tpl string, phase string) {
	for _, testdata := range tplDataList {
		pod := createPodFromTemplateAndWait(ctx, testdata, []string{}, testutils.CardmgmtTestPodTemplate, phase)
		// testutils.DeletePod(ctx, k8sClientset, pod)
		err := k8sClientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		Expect(err).To(BeNil())
	}
}

func renewSpyreAppsNamespace(ctx context.Context) {
	By("Delete spyre-apps project")
	Eventually(func(g Gomega) {
		err := testutils.DeleteNamespace(ctx, k8sClientset, "spyre-apps")
		g.Expect(k8sErrs.IsNotFound(err)).To(BeTrue())
	}).WithTimeout(240 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

	By("Create spyre-apps project")
	Eventually(func(g Gomega) {
		err := testutils.CreateNamespace(ctx, k8sClientset, "spyre-apps")
		if err != nil {
			g.Expect(k8sErrs.IsAlreadyExists(err)).To(BeTrue())
		}
	}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

	By("Create read-cm-role.yaml")
	_, err := testutils.CreateResourceFromYaml(
		ctx, dynClient, discoClient, "spyre-apps",
		filepath.Join("..", "manifest", "rbac", "read-cm-role.yaml"))
	Expect(err).To(BeNil(), "Created read-cm-role")

	By("Create read-cm-rb.yaml")
	_, err = testutils.CreateResourceFromYaml(
		ctx, dynClient, discoClient, "spyre-apps",
		filepath.Join("..", "manifest", "rbac", "read-cm-rb.yaml"))
	Expect(err).To(BeNil(), "Created read-cm-rb")
}
