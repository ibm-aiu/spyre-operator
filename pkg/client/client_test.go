/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package client_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/ibm-aiu/spyre-operator/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("general CRUD", Ordered, func() {
	var spyreClient *SpyreClient
	var err error
	ctx := context.Background()

	BeforeEach(func() {
		spyreClient, err = NewClient(ctx, Cfg)
		Expect(err).To(BeNil())
		Expect(spyreClient).NotTo((BeNil()))
	})
	AfterAll(func() {
		err = spyreClient.DeleteAll(ctx)
		Expect(err).To(BeNil())
	})

	Context("SpyreNodeState", Ordered, func() {
		testSpyreNodeState := "testnodestate"

		It("can create a new SpyreNodeState resource", func() {
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSpyreNodeState},
			}
			nodeState, err = spyreClient.Create(ctx, nodeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
		})

		It("can get a SpyreNodeState resource", func() {
			nodeState, err := spyreClient.Get(ctx, testSpyreNodeState)
			Expect(err).To(BeNil())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
		})

		It("can update a SpyreNodeState resource's spec", func() {
			nodeState, err := spyreClient.Get(ctx, testSpyreNodeState)
			Expect(err).To(BeNil())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
			newNodeName := "newNodeName"
			nodeState.Spec.NodeName = newNodeName
			nodeState, err = spyreClient.Update(ctx, nodeState, false)
			Expect(err).To(BeNil())
			Expect(nodeState.Spec.NodeName).Should(Equal(newNodeName))
		})

		It("can update a SpyreNodeState resource's status", func() {
			nodeState, err := spyreClient.Get(ctx, testSpyreNodeState)
			Expect(err).To(BeNil())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
			nodeState.Status.AllocationList = []spyrev1alpha1.Allocation{
				{DeviceList: []string{"00:99"}},
			}
			_, err = spyreClient.UpdateStatus(ctx, nodeState, false)
			Expect(err).To(BeNil())
			nodeState, err = spyreClient.Get(ctx, testSpyreNodeState)
			Expect(err).To(BeNil())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
			Expect(nodeState.Status.AllocationList[0].DeviceList).Should(Equal([]string{"00:99"}))
		})

		It("can delete a SpyreNodeState resource", func() {
			delOpts := &client.DeleteOptions{}
			err = spyreClient.Delete(ctx, testSpyreNodeState, delOpts)
			Expect(err).To(BeNil())
			listOpts := &client.ListOptions{}
			nodeStateList, err := spyreClient.List(ctx, listOpts)
			Expect(err).To(BeNil())
			Expect(len(nodeStateList.Items)).Should(Equal(0))
		})

		It("can list all SpyreNodeState", func() {
			By("creating two SpyreNodeStates")
			nodeList := []string{"node1", "node2"}
			for _, node := range nodeList {
				s := &spyrev1alpha1.SpyreNodeState{
					ObjectMeta: metav1.ObjectMeta{
						Name: node,
					},
					Spec: spyrev1alpha1.SpyreNodeStateSpec{
						NodeName: node,
						SpyreInterfaces: []spyrev1alpha1.SpyreInterface{
							{PciAddress: "00:01", NumVfs: 1},
						},
					},
					Status: spyrev1alpha1.SpyreNodeStateStatus{},
				}
				_, err = spyreClient.Create(ctx, s)
				Expect(err).To(BeNil())
			}
			By("listing two SpyreNodeStates")
			opts := &client.ListOptions{}
			nodeStateList, err := spyreClient.List(ctx, opts)
			Expect(err).To(BeNil())
			Expect(len(nodeStateList.Items)).Should(Equal(len(nodeList)))
			By("deleting all SpyreNodeStates")
			err = spyreClient.DeleteAll(ctx)
			Expect(err).To(BeNil())
			nodeStateList, err = spyreClient.List(ctx, opts)
			Expect(err).To(BeNil())
			Expect(len(nodeStateList.Items)).Should(Equal(0))
		})

		It("can retry on conflict when update spec/status", func() {
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSpyreNodeState},
			}
			nodeState, err = spyreClient.Create(ctx, nodeState)
			Expect(err).NotTo(HaveOccurred())
			// Update spec
			nodeState.Spec.Pcitopo = "new topo"
			nodeState.ResourceVersion = "99"
			_, err = spyreClient.Update(ctx, nodeState, false)
			Expect(err).To(HaveOccurred())
			_, err = spyreClient.Update(ctx, nodeState, true)
			Expect(err).NotTo(HaveOccurred())
			// Update status
			nodeState.Status.AllocationList = []spyrev1alpha1.Allocation{{DeviceList: []string{"00"}}}
			_, err = spyreClient.UpdateStatus(ctx, nodeState, false)
			Expect(err).To(HaveOccurred())
			_, err = spyreClient.UpdateStatus(ctx, nodeState, true)
			Expect(err).To(BeNil())
			// Clean up
			err = spyreClient.Delete(ctx, testSpyreNodeState, &client.DeleteOptions{})
			Expect(err).To(BeNil())
		})

		It("can create SpyreNodeState with SpyreSSAInterfaces", func() {
			nodeState := &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSpyreNodeState},
				Spec: spyrev1alpha1.SpyreNodeStateSpec{
					SpyreSSAInterfaces: []spyrev1alpha1.SpyreSSAInterface{
						{PciAddress: "0001:00:00.0", Health: spyrev1alpha1.SpyreHealthy},
					},
				},
			}
			nodeState, err = spyreClient.Create(ctx, nodeState)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
			Expect(len(nodeState.Spec.SpyreSSAInterfaces)).Should(Equal(1))
			Expect(nodeState.Spec.SpyreSSAInterfaces[0].PciAddress).Should(Equal("0001:00:00.0"))
		})

		It("can update SpyreNodeState with SpyreSSAInterfaces", func() {
			nodeState, err := spyreClient.Get(ctx, testSpyreNodeState)
			Expect(err).To(BeNil())
			Expect(nodeState.Name).Should(Equal(testSpyreNodeState))
			nodeState.Spec.SpyreSSAInterfaces = []spyrev1alpha1.SpyreSSAInterface{
				{PciAddress: "0002:00:00.0", Health: spyrev1alpha1.SpyreHealthy},
				{PciAddress: "0003:00:00.0", Health: spyrev1alpha1.SpyreUnhealthy},
			}
			nodeState, err = spyreClient.Update(ctx, nodeState, false)
			Expect(err).To(BeNil())
			Expect(len(nodeState.Spec.SpyreSSAInterfaces)).Should(Equal(2))
			Expect(nodeState.Spec.SpyreSSAInterfaces[0].PciAddress).Should(Equal("0002:00:00.0"))
			Expect(nodeState.Spec.SpyreSSAInterfaces[1].Health).Should(Equal(spyrev1alpha1.SpyreUnhealthy))
		})

	})

	Context("SpyreClusterPolicy", Ordered, func() {
		testSpyreClusterPolicy := "testpolicy"

		It("can create a SpyreClusterPolicy resource", func() {
			scp := spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSpyreClusterPolicy,
				},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					ExperimentalMode: []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode},
				},
			}
			_, err := spyreClient.CreateSpyreClusterPolicy(ctx, &scp)
			Expect(err).To(BeNil())
		})

		It("can get a SpyreClusterPolicy resource", func() {
			scp := spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSpyreClusterPolicy,
				},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					ExperimentalMode: []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode},
				},
			}
			result, err := spyreClient.GetSpyreClusterPolicy(ctx, testSpyreClusterPolicy)
			Expect(err).To(BeNil())
			Expect(result.Name).Should(Equal(scp.Name))
			Expect(result.Spec.ExperimentalMode).Should(ContainElement(spyrev1alpha1.PerDeviceAllocationMode))
		})

		It("can update status of SpyreClusterPolicy", func() {
			p, err := spyreClient.GetSpyreClusterPolicy(ctx, testSpyreClusterPolicy)
			Expect(err).To(BeNil())
			p.Status.State = spyrev1alpha1.NotReady
			p, err = spyreClient.UpdateSpyreClusterPolicyStatus(ctx, p, false)
			Expect(err).To(BeNil())
			Expect(p.Status.State).Should(Equal(spyrev1alpha1.NotReady))
			p.Status.State = spyrev1alpha1.Ready
			p, err = spyreClient.UpdateSpyreClusterPolicyStatus(ctx, p, false)
			Expect(err).To(BeNil())
			Expect(p.Status.State).Should(Equal(spyrev1alpha1.Ready))
		})

		It("can delete a SpyreClusterPolicy resource", func() {
			opts := &client.DeleteOptions{}
			err = spyreClient.DeleteSpyreClusterPolicy(ctx, testSpyreClusterPolicy, opts)
			Expect(err).To(BeNil())
		})

		It("can retry on conflict when update status", func() {
			scp := &spyrev1alpha1.SpyreClusterPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSpyreClusterPolicy,
				},
				Spec: spyrev1alpha1.SpyreClusterPolicySpec{
					ExperimentalMode: []spyrev1alpha1.SpyreClusterPolicyExperimentalMode{spyrev1alpha1.PerDeviceAllocationMode},
				},
			}
			_, err := spyreClient.CreateSpyreClusterPolicy(ctx, scp)
			Expect(err).To(BeNil())
			// Update status
			scp.Status.State = spyrev1alpha1.Ready
			scp.ResourceVersion = "99"
			_, err = spyreClient.UpdateSpyreClusterPolicyStatus(ctx, scp, false)
			Expect(err).To(HaveOccurred())
			_, err = spyreClient.UpdateSpyreClusterPolicyStatus(ctx, scp, true)
			Expect(err).NotTo(HaveOccurred())
			// Clean up
			err = spyreClient.Delete(ctx, testSpyreClusterPolicy, &client.DeleteOptions{})
			Expect(err).To(BeNil())
		})
	})
})
