/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	secv1 "github.com/openshift/api/security/v1"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	OpNs       = "spyre-operator"
	K8sClient  client.Client
	testEnv    *envtest.Environment
	Cfg        *rest.Config
	AssetsPath = filepath.Join("..", "..", "assets")

	// StateScheme, StateClient are commonly used for all unit tests except state_controller
	StateScheme *runtime.Scheme
	StateClient client.Client
	Cluster     = &ClusterState{
		OperatorNamespace: OpNs,
		k8sClient:         StateClient,
		logLevel:          zapcore.InfoLevel,
	}
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "State Suite")
}

var _ = BeforeSuite(func() {
	ctx := context.Background()
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	var err error
	os.Setenv("OPERATOR_NAMESPACE", OpNs)
	os.Setenv("OPERATOR_ASSETS_PATH", AssetsPath)
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "test", "crd", "external"),
		},
		ErrorIfCRDPathMissing: true,
	}
	Cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(Cfg).NotTo(BeNil())
	testScheme := runtime.NewScheme()
	err = spyrev1alpha1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = corev1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = secv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = appsv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = nfdv1alpha1.AddToScheme(testScheme)
	Expect(err).To(BeNil())
	K8sClient, err = client.New(Cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(K8sClient).NotTo(BeNil())
	By("creating operator namespace")
	ns := &corev1.Namespace{}
	ns.Name = OpNs
	err = K8sClient.Create(ctx, ns)
	Expect(err).To(BeNil())
	By("initializing state scheme")
	StateScheme = runtime.NewScheme()
	clientgoscheme.AddToScheme(StateScheme)
	spyrev1alpha1.AddToScheme(StateScheme)
	secv1.AddToScheme(StateScheme)
	err = InitializeScheme(StateScheme)
	Expect(err).NotTo(HaveOccurred())
	StateClient, err = client.New(Cfg, client.Options{Scheme: StateScheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Eventually(func(g Gomega) {
		err := testEnv.Stop()
		g.Expect(err).NotTo(HaveOccurred())
	}).WithTimeout(60 * time.Second).WithPolling(1000 * time.Millisecond).Should(Succeed())
})

func ValidDeploymentConfig(imageName string) spyrev1alpha1.DeploymentConfig {
	return spyrev1alpha1.DeploymentConfig{
		Image:      imageName,
		Repository: "some-repo",
		Version:    "some-version",
	}
}
