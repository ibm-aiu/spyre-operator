/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package client_test

import (
	"context"
	"errors"

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

	Context("MutateNodeStateStatus", Ordered, func() {
		const (
			testNode     = "mutatenode"
			resourceName = "spyre.ibm.com/spyre_pf"
		)
		owner := spyrev1alpha1.Pod{Name: "owner", Namespace: "app", UID: "00000000-0000-0000-0000-00000000000a"}

		BeforeAll(func() {
			// Own client: BeforeAll must not depend on where the enclosing
			// BeforeEach falls in the setup order.
			c, err := NewClient(ctx, Cfg)
			Expect(err).To(BeNil())
			_, err = c.Create(ctx, &spyrev1alpha1.SpyreNodeState{
				ObjectMeta: metav1.ObjectMeta{Name: testNode},
				Spec:       spyrev1alpha1.SpyreNodeStateSpec{NodeName: testNode},
			})
			Expect(err).To(BeNil())
		})
		AfterAll(func() {
			c, err := NewClient(ctx, Cfg)
			Expect(err).To(BeNil())
			Expect(c.Delete(ctx, testNode, &client.DeleteOptions{})).To(Succeed())
		})

		It("writes the mutated status back", func() {
			result, err := spyreClient.MutateNodeStateStatus(ctx, testNode,
				func(s *spyrev1alpha1.SpyreNodeState) error {
					s.Status.ReserveDevices(resourceName, owner, []string{"0000:1a:00.0"}, metav1.Now())
					return nil
				})
			Expect(err).To(BeNil())
			Expect(result.Status.Reservations[resourceName].Entries).Should(HaveLen(1))

			By("checking it reached the API server rather than only the returned copy")
			stored, err := spyreClient.GetSpyreNodeState(ctx, testNode)
			Expect(err).To(BeNil())
			entry, found := stored.Status.Reservations[resourceName].EntryForPod(owner)
			Expect(found).Should(BeTrue())
			Expect(entry.DeviceList).Should(ConsistOf("0000:1a:00.0"))
			Expect(entry.ReservedAt).ShouldNot(BeNil())

			By("checking the deprecated fields were kept in step for older readers")
			// Reading the deprecated fields is the point of this assertion: a reader
			// that has not been updated yet sees only these.
			legacy := stored.Status.Reservations[resourceName]
			Expect(legacy.PodsUnderScheduling).Should(HaveExactElements(owner))   //nolint:staticcheck // SA1019
			Expect(legacy.DeviceSets).Should(Equal([][]string{{"0000:1a:00.0"}})) //nolint:staticcheck // SA1019
		})

		It("returns the mutate error unwrapped", func() {
			// The scheduler surfaces this error verbatim to the user, so it must not
			// pick up any read-modify-write context on the way out.
			want := errors.New("device unavailable: 0000:1a:00.0")
			_, err := spyreClient.MutateNodeStateStatus(ctx, testNode,
				func(*spyrev1alpha1.SpyreNodeState) error { return want })
			Expect(err).Should(Equal(want))
		})

		It("succeeds without writing when the mutation turns out to be unnecessary", func() {
			before, err := spyreClient.GetSpyreNodeState(ctx, testNode)
			Expect(err).To(BeNil())

			result, err := spyreClient.MutateNodeStateStatus(ctx, testNode,
				func(*spyrev1alpha1.SpyreNodeState) error { return ErrNoStatusChange })
			Expect(err).To(BeNil())
			Expect(result.ResourceVersion).Should(Equal(before.ResourceVersion))
		})

		It("recomputes the change when another writer gets there first", func() {
			// Make the first attempt lose: mutate moves the object out from under
			// itself, so the Status().Update that follows takes a 409 and the whole
			// read-modify-write cycle has to run again against the new state.
			loser := spyrev1alpha1.Pod{Name: "loser", Namespace: "app", UID: "00000000-0000-0000-0000-00000000000b"}
			attempts := 0
			result, err := spyreClient.MutateNodeStateStatus(ctx, testNode,
				func(s *spyrev1alpha1.SpyreNodeState) error {
					attempts++
					if attempts == 1 {
						winner, err := spyreClient.GetSpyreNodeState(ctx, testNode)
						Expect(err).To(BeNil())
						winner.Status.AllocationList = []spyrev1alpha1.Allocation{{
							Pod:          &spyrev1alpha1.Pod{Name: "winner", Namespace: "app"},
							DeviceList:   []string{"0000:1c:00.0"},
							ResourcePool: resourceName,
						}}
						_, err = spyreClient.UpdateStatus(ctx, winner, false)
						Expect(err).To(BeNil())
					}
					s.Status.ReserveDevices(resourceName, loser, []string{"0000:1d:00.0"}, metav1.Now())
					return nil
				})
			Expect(err).To(BeNil())
			Expect(attempts).Should(Equal(2))

			// Recomputing is what keeps both changes: replaying a status built
			// against the stale object would have dropped the winner's allocation.
			Expect(result.Status.AllocatedDevices()).Should(ConsistOf("0000:1c:00.0"))
			Expect(result.Status.ReservedDevices()).Should(ConsistOf("0000:1a:00.0", "0000:1d:00.0"))
			Expect(result.Status.ReservationEntries()).Should(HaveKey(resourceName))
		})

		It("reports the node when the read-modify-write cycle itself fails", func() {
			_, err := spyreClient.MutateNodeStateStatus(ctx, "no-such-node",
				func(*spyrev1alpha1.SpyreNodeState) error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("no-such-node"))
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
