/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var Cfg *rest.Config
var testEnv *envtest.Environment
var Scheme *runtime.Scheme

const (
	TestNamespace = "test"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

var _ = BeforeSuite(func() {

	var err error

	By("bootstrapping test environment")
	crdPath := filepath.Join("..", "..", "config", "crd", "bases")
	_, err = os.Stat(crdPath)
	Expect(err).To(
		BeNil(),
		"%v not exist; spyre-operator must exists the same directory of the device plugin code",
		crdPath)
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}

	// cfg is defined in this file globally.
	Cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(Cfg).NotTo(BeNil())

	// create namespace "test"
	k8sClient, err := client.New(Cfg, client.Options{})
	Expect(err).NotTo(HaveOccurred())
	ns := &corev1.Namespace{}
	ns.Name = TestNamespace
	err = k8sClient.Create(context.Background(), ns)
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
