/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package controllers_test

import (
	"context"
	"path/filepath"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/controllers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var _ = Describe("NodeLabelerReconciler", func() {
	Context("with API server", Ordered, func() {
		var ctx context.Context
		var k8sClient client.Client
		var testEnv *envtest.Environment
		var reconciler *controllers.NodeLabelerReconciler

		const nodeName = "labeler-ctrl-test-node"

		BeforeAll(func() {
			ctx = context.Background()
			testEnv = &envtest.Environment{
				CRDDirectoryPaths: []string{
					filepath.Join("..", "config", "crd", "bases"),
				},
				ErrorIfCRDPathMissing: true,
			}
			cfg, err := testEnv.Start()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			err = spyrev1alpha1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())

			k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			reconciler = &controllers.NodeLabelerReconciler{Client: k8sClient}
		})

		AfterAll(func() {
			Eventually(func(g Gomega) {
				g.Expect(testEnv.Stop()).To(Succeed())
			}).WithTimeout(60 * time.Second).WithPolling(time.Second).Should(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}})
			list := &spyrev1alpha1.SpyreClusterPolicyList{}
			_ = k8sClient.List(ctx, list)
			for i := range list.Items {
				_ = k8sClient.Delete(ctx, &list.Items[i])
			}
		})

		createNode := func(labels map[string]string) {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: labels}}
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
		}

		reconcile := func() {
			_, err := reconciler.Reconcile(ctx, ctrl.Request{})
			Expect(err).NotTo(HaveOccurred())
		}

		getNodeLabel := func(key string) (string, bool) {
			node := &corev1.Node{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, node)).To(Succeed())
			val, found := node.Labels[key]
			return val, found
		}

		It("does not set spyre.present when no NFD labels and no policy", func() {
			createNode(map[string]string{"kubernetes.io/arch": "amd64"})
			reconcile()
			_, found := getNodeLabel(spyreconst.CommonSpyreLabelKey)
			Expect(found).To(BeFalse())
		})

		It("sets spyre.present when NFD PCI labels are present and no policy", func() {
			createNode(map[string]string{
				"kubernetes.io/arch":                          "amd64",
				"feature.node.kubernetes.io/pci-1014.present": "true",
			})
			reconcile()
			val, found := getNodeLabel(spyreconst.CommonSpyreLabelKey)
			Expect(found).To(BeTrue())
			Expect(val).To(Equal("true"))
		})

		It("sets spyre.present when pseudoDeviceMode is enabled in policy (CRC/e2e case)", func() {
			createNode(map[string]string{"kubernetes.io/arch": "amd64"})
			// Create policy with pseudo mode enabled
			cp := &spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "spyreclusterpolicy"},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					ExperimentalMode: []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{
						spyrev1alpha1.PseudoDeviceMode,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cp)).To(Succeed())
			reconcile()
			val, found := getNodeLabel(spyreconst.CommonSpyreLabelKey)
			Expect(found).To(BeTrue())
			Expect(val).To(Equal("true"))
		})

		It("removes spyre.present when pseudoDeviceMode is disabled and no NFD labels", func() {
			// Start with pseudo mode on → label applied
			createNode(map[string]string{"kubernetes.io/arch": "amd64"})
			cp := &spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "spyreclusterpolicy"},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					ExperimentalMode: []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{
						spyrev1alpha1.PseudoDeviceMode,
					},
				},
			}
			Expect(k8sClient.Create(ctx, cp)).To(Succeed())
			reconcile()
			_, found := getNodeLabel(spyreconst.CommonSpyreLabelKey)
			Expect(found).To(BeTrue())

			// Disable pseudo mode
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "spyreclusterpolicy"}, cp)).To(Succeed())
			cp.Spec.ExperimentalMode = []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{}
			Expect(k8sClient.Update(ctx, cp)).To(Succeed())
			reconcile()
			val, found := getNodeLabel(spyreconst.CommonSpyreLabelKey)
			// label still present but set to "false"
			if found {
				Expect(val).To(Equal("false"))
			}
		})
	})
})
