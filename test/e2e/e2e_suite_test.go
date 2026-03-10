/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package e2e_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"github.com/ibm-aiu/spyre-operator/test/testutil"
	testutils "github.com/ibm-aiu/spyre-operator/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	spyrev1alpha1 "github.ibm.com/ibm-aiu/spyre-operator/v2/api/v1alpha1"
	"github.ibm.com/ibm-aiu/spyre-operator/v2/internal/state"
	"github.ibm.com/ibm-aiu/spyre-operator/v2/test/testutil"
	testutils "github.ibm.com/ibm-aiu/spyre-operator/v2/test/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var k8sClientset *kubernetes.Clientset
var dynClient dynamic.Interface
var scheme = runtime.NewScheme()
var spyreV2Client client.Client
var nodeNames []string
var targetNodeName string
var nodeArchitecture string
var testConfig testutils.TestConfig

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spyre Operator End 2 End Suite")
}

var _ = BeforeSuite(func() {

	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx := context.Background()

	kubeconfig, ok := os.LookupEnv(testutils.KubeConfigFilePathKey)
	Expect(ok).To(BeTrue(), "%s must be set", testutils.KubeConfigFilePathKey)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).To(BeNil())

	// further configure the client
	config.Timeout = 90 * time.Second
	config.Burst = 100
	config.QPS = 50.0
	config.WarningHandler = rest.NoWarnings{}

	// instantiate the client
	k8sClientset, err = kubernetes.NewForConfig(config)
	Expect(err).To(BeNil())
	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = spyrev1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = nfdv1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = state.InitializeScheme(scheme)
	Expect(err).To(BeNil())
	spyreV2Client, err = client.New(config, client.Options{Scheme: scheme})
	Expect(err).To(BeNil())
	dynClient, err = dynamic.NewForConfig(config)
	Expect(err).To(BeNil())
	err = spyrev1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	nodeNames = testutils.GetWorkerNodeNames(ctx, k8sClientset)
	Expect(len(nodeNames)).Should(BeNumerically(">=", 1))

	testConfig = testutils.LoadTestConfig()
	if testConfig.NodeName != "" {
		for _, nodeName := range nodeNames {
			if nodeName == testConfig.NodeName {
				targetNodeName = nodeName
				break
			}
		}
	}
	if targetNodeName == "" {
		targetNodeName = nodeNames[0]
	}

	By("getting node architecture")
	targetNode, err := k8sClientset.CoreV1().Nodes().Get(ctx, targetNodeName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	var found bool
	nodeArchitecture, found = targetNode.Labels["kubernetes.io/arch"]
	Expect(found).To(BeTrue())

	setMsg := fmt.Sprintf("setting target node = %s (%s)", targetNodeName, nodeArchitecture)
	By(setMsg)

	By("uninstalling the operator if already installed")
	testutils.UninstallOperator(ctx, k8sClientset, dynClient, spyreV2Client, testConfig.HasDevice, nodeNames)

	By("installing operator")
	testutils.InstallOperator(ctx, testConfig, k8sClientset, dynClient)

	By("deploying cluster policy")
	enabledModes := DefaultModes()
	if testConfig.PseudoDeviceMode {
		enabledModes = append(enabledModes, spyrev1alpha1.PseudoDeviceMode)
	}
	testutils.DeployClusterPolicy(ctx, testConfig, k8sClientset, spyreV2Client,
		enabledModes, "", len(nodeNames))

	By("force second scheduler Pod to pull image")
	schedImage := fmt.Sprintf("%s/%s:%s",
		testConfig.Scheduler.Repository, testConfig.Scheduler.Image, testConfig.Scheduler.Version)
	testutil.ForcePullLatestSchedulerImage(ctx, k8sClientset, schedImage)
})

var _ = AfterSuite(func() {
	ctx := context.Background()
	testutils.UninstallOperator(ctx, k8sClientset, dynClient, spyreV2Client, testConfig.HasDevice, nodeNames)
})

func DefaultModes() []spyrev1alpha1.SpyreClusterPolicyExperimentalMode {
	return []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{
		spyrev1alpha1.PerDeviceAllocationMode, spyrev1alpha1.TopologyAwareAllocationMode, spyrev1alpha1.ReservationMode}
}
