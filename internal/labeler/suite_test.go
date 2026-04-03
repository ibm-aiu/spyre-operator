/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package labeler

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv      *envtest.Environment
	K8sClient    client.Client
	K8sClientset *kubernetes.Clientset
	Cfg          *rest.Config
)

func TestLabelers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Labeler Suite")
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	var err error
	testEnv = &envtest.Environment{}
	Cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	testScheme := runtime.NewScheme()
	err = corev1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	K8sClient, err = client.New(Cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(K8sClient).NotTo(BeNil())
	K8sClientset, err = kubernetes.NewForConfig(Cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	Eventually(func(g Gomega) {
		err := testEnv.Stop()
		g.Expect(err).NotTo(HaveOccurred())
	}).WithTimeout(60 * time.Second).WithPolling(1000 * time.Millisecond).Should(Succeed())
})
