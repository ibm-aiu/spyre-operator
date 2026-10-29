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

const (
	testDeviceID = "0001:00:00.0"
	testPodName  = "test-pod"
)

var _ = Describe("SpyreNodeState State", Ordered, func() {
	ctx := context.Background()
	cpName := "spyrenodestate-test-policy"
	BeforeAll(func() {
		cp := &spyrev1alpha1.SpyreClusterPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: cpName},
			Spec: spyrev1alpha1.SpyreClusterPolicySpec{
				DevicePlugin: spyrev1alpha1.DevicePluginSpec{
					Mode: spyrev1alpha1.DevicePluginModeClassic,
				},
			},
		}
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

	Describe("DRA Migration Safety", func() {
		var spyreNodeState *SpyreNodeStateState

		BeforeEach(func() {
			spyreNodeState = NewSpyreNodeStateState(StateClient, StateScheme)
		})

		It("should detect active device plugin workloads", func() {
			By("creating a SpyreNodeState")
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: spyrev1alpha1.SpyreNodeStateSpec{
					NodeName: "test-node",
				},
			}
			err := K8sClient.Create(ctx, nodeState)
			Expect(err).To(BeNil())

			By("updating SpyreNodeState status with active allocation")
			nodeState.Status = spyrev1alpha1.SpyreNodeStateStatus{
				AllocationList: []spyrev1alpha1.Allocation{
					{
						DeviceList: []string{testDeviceID},
						Pod: &spyrev1alpha1.Pod{
							Name:      testPodName,
							Namespace: "default",
						},
						ResourcePool: "spyre_pf",
					},
				},
			}
			err = K8sClient.Status().Update(ctx, nodeState)
			Expect(err).To(BeNil())

			By("creating the pod referenced in allocation")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "nginx",
						},
					},
				},
			}
			err = K8sClient.Create(ctx, pod)
			Expect(err).To(BeNil())

			By("waiting for pod to be created and available")
			Eventually(func(g Gomega) {
				createdPod := &corev1.Pod{}
				err := K8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: testPodName}, createdPod)
				g.Expect(err).To(BeNil())
			}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

			By("checking for active workloads - should find the pod")
			err = spyreNodeState.CheckActiveDevicePluginWorkloads(ctx)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring("active device plugin workload"))
			Expect(err.Error()).To(ContainSubstring("default/test-pod"))

			By("cleaning up the test pod and nodeState")
			K8sClient.Delete(ctx, pod)
			K8sClient.Delete(ctx, nodeState)
		})

		It("should not report error when no active workloads exist", func() {
			By("creating a SpyreNodeState with no allocations")
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-empty",
				},
				Spec: spyrev1alpha1.SpyreNodeStateSpec{
					NodeName: "test-node-empty",
				},
				Status: spyrev1alpha1.SpyreNodeStateStatus{
					AllocationList: []spyrev1alpha1.Allocation{},
				},
			}
			err := K8sClient.Create(ctx, nodeState)
			Expect(err).To(BeNil())

			By("checking for active workloads - should find none")
			err = spyreNodeState.CheckActiveDevicePluginWorkloads(ctx)
			Expect(err).To(BeNil())

			By("cleaning up the nodeState")
			K8sClient.Delete(ctx, nodeState)
		})

		It("should ignore stale allocations (pods that no longer exist)", func() {
			By("creating a SpyreNodeState with allocation to non-existent pod")
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-stale",
				},
				Spec: spyrev1alpha1.SpyreNodeStateSpec{
					NodeName: "test-node-stale",
				},
				Status: spyrev1alpha1.SpyreNodeStateStatus{
					AllocationList: []spyrev1alpha1.Allocation{
						{
							DeviceList: []string{testDeviceID},
							Pod: &spyrev1alpha1.Pod{
								Name:      "non-existent-pod",
								Namespace: "default",
							},
							ResourcePool: "spyre_pf",
						},
					},
				},
			}
			err := K8sClient.Create(ctx, nodeState)
			Expect(err).To(BeNil())

			By("checking for active workloads - should ignore stale allocation")
			err = spyreNodeState.CheckActiveDevicePluginWorkloads(ctx)
			Expect(err).To(BeNil())

			By("cleaning up the nodeState")
			K8sClient.Delete(ctx, nodeState)
		})

		It("should delete all SpyreNodeState resources", func() {
			By("creating multiple SpyreNodeState resources")
			nodeStates := []*spyrev1alpha1.SpyreNodeState{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node-1",
					},
					Spec: spyrev1alpha1.SpyreNodeStateSpec{
						NodeName: "test-node-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node-2",
					},
					Spec: spyrev1alpha1.SpyreNodeStateSpec{
						NodeName: "test-node-2",
					},
				},
			}
			for _, ns := range nodeStates {
				err := K8sClient.Create(ctx, ns)
				Expect(err).To(BeNil())
			}

			By("verifying SpyreNodeState resources exist")
			nsList := &spyrev1alpha1.SpyreNodeStateList{}
			err := K8sClient.List(ctx, nsList, &client.ListOptions{})
			Expect(err).To(BeNil())
			initialCount := len(nsList.Items)

			By("deleting all SpyreNodeState resources")
			err = spyreNodeState.DeleteAllSpyreNodeStates(ctx)
			Expect(err).To(BeNil())

			By("verifying all SpyreNodeState resources are deleted")
			Eventually(func(g Gomega) {
				nsList := &spyrev1alpha1.SpyreNodeStateList{}
				err := K8sClient.List(ctx, nsList, &client.ListOptions{})
				g.Expect(err).To(BeNil())
				// Should have fewer items (only the ones created in other tests remain)
				g.Expect(len(nsList.Items)).To(BeNumerically("<", initialCount))
			}).WithTimeout(10 * time.Second).WithPolling(1 * time.Second).Should(Succeed())
		})

		It("should handle allocation without pod reference", func() {
			By("creating a SpyreNodeState with allocation but no pod")
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-no-pod",
				},
				Spec: spyrev1alpha1.SpyreNodeStateSpec{
					NodeName: "test-node-no-pod",
				},
				Status: spyrev1alpha1.SpyreNodeStateStatus{
					AllocationList: []spyrev1alpha1.Allocation{
						{
							DeviceList:   []string{testDeviceID},
							Pod:          nil,
							ResourcePool: "spyre_pf",
						},
					},
				},
			}
			err := K8sClient.Create(ctx, nodeState)
			Expect(err).To(BeNil())

			By("checking for active workloads - should ignore allocation without pod")
			err = spyreNodeState.CheckActiveDevicePluginWorkloads(ctx)
			Expect(err).To(BeNil())

			By("cleaning up")
			K8sClient.Delete(ctx, nodeState)
		})
	})
})
