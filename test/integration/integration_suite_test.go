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
	"testing"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"github.com/ibm-aiu/spyre-operator/test/testutil"
	testutils "github.com/ibm-aiu/spyre-operator/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
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
var dynClient *dynamic.DynamicClient
var scheme = runtime.NewScheme()
var spyreV2Client client.Client
var itConfig testutils.TestConfig
var config *rest.Config
var nodeNames []string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spyre Operator Integration test Suite")
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx := context.Background()
	var err error
	itConfig = testutils.LoadTestConfig()
	kubeconfig, ok := os.LookupEnv(testutils.KubeConfigFilePathKey)

	Expect(ok).To(BeTrue(), "%s must be set", testutils.KubeConfigFilePathKey)
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).To(BeNil())

	// further configure the client
	config.Timeout = 90 * time.Second
	config.Burst = 100
	config.QPS = 50.0
	// suppresses warning for good reasons
	// config.WarningHandler = rest.NoWarnings{}

	// instantiate the client
	k8sClientset, err = kubernetes.NewForConfig(config)
	Expect(err).To(BeNil())
	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = spyrev1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	err = nfdv1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	spyreV2Client, err = client.New(config, client.Options{Scheme: scheme})
	Expect(err).To(BeNil())
	dynClient, err = dynamic.NewForConfig(config)
	Expect(err).To(BeNil())
	err = spyrev1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())
	nodeNames = testutils.GetWorkerNodeNames(ctx, k8sClientset)
	Expect(len(nodeNames)).Should(BeNumerically(">=", 1))

	By("uninstalling the operator if already installed")
	testutils.UninstallOperator(ctx, k8sClientset, dynClient, spyreV2Client, itConfig.HasDevice, nodeNames)

	By("installing operator")
	testutils.InstallOperator(ctx, itConfig, k8sClientset, dynClient)

	By("deploying cluster policy")
	enabledModes := []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{
		spyrev1alpha1.PerDeviceAllocationMode, spyrev1alpha1.TopologyAwareAllocationMode, spyrev1alpha1.ReservationMode}
	testutils.DeployClusterPolicy(ctx, itConfig, k8sClientset, spyreV2Client,
		enabledModes, "", len(nodeNames))

	By("force second scheduler Pod to pull image")
	schedImage := fmt.Sprintf("%s/%s:%s",
		itConfig.Scheduler.Repository, itConfig.Scheduler.Image, itConfig.Scheduler.Version)
	testutil.ForcePullLatestSchedulerImage(ctx, k8sClientset, schedImage)
})

var _ = AfterSuite(func(ctx SpecContext) {
	renewSpyreAppsNamespace(ctx)
	testutils.UninstallOperator(ctx, k8sClientset, dynClient, spyreV2Client, itConfig.HasDevice, nodeNames)
}, NodeTimeout(time.Minute*3))
