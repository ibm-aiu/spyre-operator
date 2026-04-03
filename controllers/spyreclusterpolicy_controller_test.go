/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package controllers_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"github.com/ibm-aiu/spyre-operator/controllers"
	"github.com/ibm-aiu/spyre-operator/test/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	nfdv1alpha1 "github.com/openshift/cluster-nfd-operator/api/v1alpha1"
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8Yaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var _ = Describe("SpyreclusterpolicyController", func() {
	var logBuf bytes.Buffer
	var k8sClient client.Client
	var testEnv *envtest.Environment
	var dynClient *dynamic.DynamicClient
	var discoClient *discovery.DiscoveryClient

	Context("api", func() {
		It("can unmarshal an example file", func() {
			data, err := os.ReadFile(filepath.Join("..", "config", "samples", "spyre_v1alpha1_spyreclusterpolicy.yaml"))
			Expect(err).To(BeNil())
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			dec := k8Yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 1000)
			err = dec.Decode(&cp)
			Expect(err).To(BeNil())
			Expect(cp.Name).To(Equal("spyreclusterpolicy"))
			Expect(cp.Spec.DevicePlugin.Image).To(Equal("spyre-device-plugin"))
			Expect(cp.Spec.MetricsExporter.Image).To(Equal("spyre-exporter"))
		})

		It("can unmarshal an example file with skip components", func() {
			data, err := os.ReadFile(filepath.Join("..", "config", "samples", "spyre_v1alpha1_spyreclusterpolicy_skip_components.yaml"))
			Expect(err).To(BeNil())
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			dec := k8Yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 1000)
			err = dec.Decode(&cp)
			Expect(err).To(BeNil())
			Expect(cp.Name).To(Equal("spyreclusterpolicy"))
			Expect(cp.Spec.SkipUpdateComponents).To(HaveLen(6))
		})
	})

	Context("with API server", Ordered, func() {
		var ctx context.Context
		var cfg *rest.Config
		var err error
		cpName := "spyreclusterpolicy"

		BeforeAll(func() {
			GinkgoWriter.TeeTo(&logBuf)
			ctx = context.Background()
			var err error
			os.Setenv("OPERATOR_NAMESPACE", controllers.OpNs)
			testEnv = &envtest.Environment{
				CRDDirectoryPaths: []string{
					filepath.Join("..", "config", "crd", "bases"),
					filepath.Join("..", "test", "crd", "external"),
				},
				ErrorIfCRDPathMissing: true,
			}
			cfg, err = testEnv.Start()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			err = scheme.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			err = spyrev1alpha1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			err = promv1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			err = nfdv1alpha1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient).NotTo(BeNil())
			dynClient, err = dynamic.NewForConfig(cfg)
			Expect(err).To(BeNil())
			discoClient, err = discovery.NewDiscoveryClientForConfig(cfg)
			Expect(err).To(BeNil())
			ns := &corev1.Namespace{}
			ns.Name = controllers.OpNs
			err = k8sClient.Create(ctx, ns)
			Expect(err).To(BeNil())
		})

		AfterAll(func() {
			Eventually(func(g Gomega) {
				err := testEnv.Stop()
				g.Expect(err).NotTo(HaveOccurred())
			}).WithTimeout(60 * time.Second).WithPolling(1000 * time.Millisecond).Should(Succeed())
		})

		Context("yaml", func() {
			It("can deploy example file", func() {
				_, err = testutil.CreateResourceFromYaml(
					ctx, dynClient, discoClient, metav1.NamespaceAll,
					filepath.Join("..", "config", "samples", "spyre_v1alpha1_spyreclusterpolicy.yaml"))
				Expect(err).To(BeNil())
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: cpName}}
				err = k8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
			})

			It("can deploy minimum spec", func() {
				_, err = testutil.CreateResourceFromYaml(
					ctx, dynClient, discoClient, metav1.NamespaceAll,
					filepath.Join("..", "config", "samples", "spyre_v1alpha1_spyreclusterpolicy_minimum.yaml"))
				Expect(err).To(BeNil())
				cp := &spyrev1alpha1.SpyreClusterPolicy{}
				err = k8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
				Expect(err).To(BeNil())
				By("checking default values")
				Expect(cp.Spec.Scheduler.DeploymentConfig.Image).To(BeEmpty())
				Expect(cp.Spec.PodValidator.Enabled).To(BeFalse())
				Expect(cp.Spec.CardManagement.Enabled).To(BeFalse())
				Expect(cp.Spec.MetricsExporter.Enabled).To(BeFalse())
				cp = &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: cpName}}
				err = k8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
			})

			It("must deny invalid spec (no device plugin)", func() {
				_, err = testutil.CreateResourceFromYaml(
					ctx, dynClient, discoClient, metav1.NamespaceAll,
					filepath.Join("..", "config", "samples", "spyre_v1alpha1_spyreclusterpolicy_invalid.yaml"))
				Expect(err).NotTo(BeNil())
			})
		})

		Describe("reconciliation", Ordered, func() {

			BeforeEach(func() {
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: cpName}}
				err = k8sClient.Create(ctx, cp)
				Expect(err).To(BeNil())
				time.Sleep(1000 * time.Millisecond)
			})

			AfterEach(func() {
				By("deleting spyreclusterpolicy")
				cp := &spyrev1alpha1.SpyreClusterPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: cpName}}
				err = k8sClient.Delete(ctx, cp)
				Expect(err).To(BeNil())
				By("deleting spyrenodestate")
				nsList := spyrev1alpha1.SpyreNodeStateList{}
				err := k8sClient.List(ctx, &nsList, &client.ListOptions{})
				Expect(err).To(BeNil())
				for _, ns := range nsList.Items {
					err = k8sClient.Delete(ctx, &ns)
					Expect(err).To(BeNil())
				}
				By("waiting until deletion complete")
				Eventually(func(g Gomega) {
					cp := &spyrev1alpha1.SpyreClusterPolicy{}
					err = k8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
					nsList := spyrev1alpha1.SpyreNodeStateList{}
					err := k8sClient.List(ctx, &nsList, &client.ListOptions{})
					Expect(err).To(BeNil())
					g.Expect(nsList.Items).To(HaveLen(0))
				}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
			})

			DescribeTable("accepts loglevel",
				func(level string, shouldAccept bool) {
					cp := &spyrev1alpha1.SpyreClusterPolicy{}
					err = k8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
					Expect(err).To(BeNil())
					cp.Spec.LogLevel = &level
					err = k8sClient.Update(ctx, cp)
					if shouldAccept {
						Expect(err).To(BeNil())
					} else {
						Expect(err.(*apiErrors.StatusError).ErrStatus.Reason).Should(Equal(metav1.StatusReasonInvalid))
					}
				},
				Entry("accept \"debug\"", "debug", true),
				Entry("deny \"Debug\"", "Debug", false),
				Entry("deny \"DEBUG\"", "DEBUG", false),
				Entry("accept \"info\"", "info", true),
				Entry("accept \"error\"", "error", true),
			)

			DescribeTable("nodeUpdateNeedsReconcile",
				func(oldLabels, newLabels map[string]string, wantUpdate, wantOSTree, wantSpyre bool) {
					gotUpdate, gotOSTree, gotSpyre := controllers.NodeUpdateNeedsReconcile(oldLabels, newLabels)
					Expect(gotUpdate).To(Equal(wantUpdate), "needsUpdate")
					Expect(gotOSTree).To(Equal(wantOSTree), "osTreeLabelChanged")
					Expect(gotSpyre).To(Equal(wantSpyre), "spyreCommonLabelChanged")
				},
				Entry("no label change → no reconcile",
					map[string]string{"ibm.com/spyre.present": "true"},
					map[string]string{"ibm.com/spyre.present": "true"},
					false, false, false),
				Entry("spyre.present added (NodeLabelerReconciler labeled node) → reconcile",
					map[string]string{},
					map[string]string{"ibm.com/spyre.present": "true"},
					true, false, true),
				Entry("spyre.present removed (pseudoMode disabled, no NFD) → reconcile",
					map[string]string{"ibm.com/spyre.present": "true"},
					map[string]string{},
					true, false, true),
				Entry("OS tree version changed → reconcile",
					map[string]string{"feature.node.kubernetes.io/system-os_release.OSTREE_VERSION": "v1"},
					map[string]string{"feature.node.kubernetes.io/system-os_release.OSTREE_VERSION": "v2"},
					true, true, false),
				Entry("unrelated label change → no reconcile",
					map[string]string{"kubernetes.io/hostname": "node-a"},
					map[string]string{"kubernetes.io/hostname": "node-b"},
					false, false, false),
			)

			DescribeTable("process status", func(overallStatus spyrev1alpha1.State, expectedRequeue bool) {
				result := controllers.ProcessOverallStatus(ctx, overallStatus, "")
				if expectedRequeue {
					Expect(result.RequeueAfter).To(BeNumerically(">", 0))
				} else {
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeEquivalentTo(0))
				}
			},
				Entry("ready", spyrev1alpha1.Ready, false),
				Entry("not ready", spyrev1alpha1.NotReady, true),
				Entry("no Spyre nodes", spyrev1alpha1.NoSpyreNodes, false),
				Entry("no NFD", spyrev1alpha1.NoNFD, false),
			)
		})
	})
})
