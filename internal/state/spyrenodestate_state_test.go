/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"
	"time"

	. "github.com/ibm-aiu/spyre-operator/internal/state"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SpyreNodeState State", Ordered, func() {
	ctx := context.Background()
	cpName := "spyrenodestate-test-policy"
	BeforeAll(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName}}
		err := K8sClient.Create(ctx, cp)
		Expect(err).To(BeNil())
	})

	AfterAll(func() {
		By("deleting spyreclusterpolicy")
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName}}
		err := K8sClient.Delete(ctx, cp)
		Expect(err).To(BeNil())
		By("deleting spyrenodestate")
		nsList := spyrev1alpha1.SpyreNodeStateList{}
		err = K8sClient.List(ctx, &nsList, &client.ListOptions{})
		Expect(err).To(BeNil())
		for _, ns := range nsList.Items {
			err = K8sClient.Delete(ctx, &ns)
			Expect(err).To(BeNil())
		}
		By("waiting until deletion complete")
		Eventually(func(g Gomega) {
			cp := &spyrev1alpha1.SpyreClusterPolicy{}
			err = K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
			nsList := spyrev1alpha1.SpyreNodeStateList{}
			err := K8sClient.List(ctx, &nsList, &client.ListOptions{})
			Expect(err).To(BeNil())
			g.Expect(nsList.Items).To(HaveLen(0))
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})

	It("can create SpyreNodeState", func() {
		nodeNames := []string{"node1", "node2"}
		By("creating Node resources")
		for _, n := range nodeNames {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: n,
				},
			}
			err := K8sClient.Get(ctx, client.ObjectKey{Name: n}, node)
			if err != nil {
				opt := &client.CreateOptions{}
				err = K8sClient.Create(ctx, node, opt)
				Expect(err).To(BeNil())
			}
		}
		spyreNodeState := NewSpyreNodeStateState(StateClient, StateScheme)
		cp := &spyrev1alpha1.SpyreClusterPolicy{}
		err := K8sClient.Get(ctx, client.ObjectKey{Name: cpName}, cp)
		Expect(err).To(BeNil())
		Expect(cp.Name).To(Equal(cpName))
		err = spyreNodeState.UpdateSpyreNodeStates(ctx, cp)
		Expect(err).To(BeNil())
		Eventually(func(g Gomega) {
			nsList := &spyrev1alpha1.SpyreNodeStateList{}
			err := K8sClient.List(ctx, nsList, &client.ListOptions{})
			g.Expect(err).To(BeNil())
			g.Expect(len(nsList.Items)).Should(BeNumerically("==", 2))
			for _, nodeState := range nsList.Items {
				g.Expect(nodeState.Name).Should(BeElementOf(nodeNames))
				owners := nodeState.ObjectMeta.OwnerReferences
				Expect(owners).To(HaveLen(1))
				Expect(owners[0].Name).To(BeEquivalentTo(cp.Name))
				Expect(owners[0].UID).To(BeEquivalentTo(cp.UID))
			}
		}).WithTimeout(20 * time.Second).WithPolling(5000 * time.Millisecond).Should(Succeed())
	})
})
