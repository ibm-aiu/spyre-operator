/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state_test

import (
	"context"
	"fmt"
	"time"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	. "github.com/ibm-aiu/spyre-operator/internal/state"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Degraded card management", Ordered, func() {
	const degradedConditionType = corev1.NodeConditionType("ibm.com/SpyreCardManagementDegraded")
	var (
		ctx     = context.Background()
		pfName  = corev1.ResourceName(spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName)
		counter = 0
		created []client.Object
	)

	// uniqueNode returns a unique node name for each test since envtest has no GC and shares the API server
	uniqueNode := func() string {
		counter++
		return fmt.Sprintf("degraded-test-node-%d", counter)
	}

	track := func(obj client.Object) {
		created = append(created, obj)
	}

	AfterEach(func() {
		for _, obj := range created {
			err := StateClient.Delete(ctx, obj, client.GracePeriodSeconds(0))
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		}
		created = nil
	})

	createNode := func(name string, allocatable int64) {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
		Expect(StateClient.Create(ctx, node)).To(Succeed())
		track(node)
		node.Status.Allocatable = corev1.ResourceList{pfName: *resource.NewQuantity(allocatable, resource.DecimalSI)}
		Expect(StateClient.Status().Update(ctx, node)).To(Succeed())
	}

	newDaemonSet := func(node string) *appsv1.DaemonSet {
		labels := map[string]string{"app": "cardmgmt-" + node}
		return &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: spyreconst.CardManagementResourceName + "-" + node, Namespace: OpNs},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						NodeSelector: map[string]string{"kubernetes.io/hostname": node},
						Containers:   []corev1.Container{{Name: "app", Image: "image"}},
					},
				},
			},
		}
	}

	newControlledDaemonSet := func(node string) ControlledObject {
		ds := newDaemonSet(node)
		defaultObj, err := NewDefaultObject(ctx, "DaemonSet", OpNs, ds)
		Expect(err).To(BeNil())
		obj, err := NewDaemonSet(defaultObj, ds, OpNs)
		Expect(err).To(BeNil())
		return obj
	}

	getDaemonSet := func(node string) *appsv1.DaemonSet {
		ds := &appsv1.DaemonSet{}
		Expect(StateClient.Get(ctx, client.ObjectKey{
			Name: spyreconst.CardManagementResourceName + "-" + node, Namespace: OpNs}, ds)).To(Succeed())
		return ds
	}

	// createCardMgmtPod creates a pod controlled by the (already created) DaemonSet of the node
	// requesting the devices, pending as unschedulable since unschedulableFor
	createCardMgmtPod := func(node string, requested int64, unschedulableFor time.Duration) *corev1.Pod {
		ds := getDaemonSet(node)
		isController := true
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cardmgmt-" + node, Namespace: OpNs, Labels: ds.Spec.Template.Labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: &isController}},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "image", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{pfName: *resource.NewQuantity(requested, resource.DecimalSI)}}}},
			},
		}
		Expect(StateClient.Create(ctx, pod)).To(Succeed())
		track(pod)
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-unschedulableFor)),
			}},
		}
		Expect(StateClient.Status().Update(ctx, pod)).To(Succeed())
		return pod
	}

	// createUserPod creates a running pod on the node which holds the devices
	createUserPod := func(node string, holding int64) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "user-" + node, Namespace: OpNs},
			Spec: corev1.PodSpec{
				NodeName: node,
				Containers: []corev1.Container{{Name: "app", Image: "image", Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{pfName: *resource.NewQuantity(holding, resource.DecimalSI)}}}},
			},
		}
		Expect(StateClient.Create(ctx, pod)).To(Succeed())
		track(pod)
		pod.Status.Phase = corev1.PodRunning
		Expect(StateClient.Status().Update(ctx, pod)).To(Succeed())
	}

	createDaemonSet := func(node string) {
		ds := newDaemonSet(node)
		Expect(StateClient.Create(ctx, ds)).To(Succeed())
		track(ds)
	}

	DescribeTable("Degraded",
		func(allocatable, requested, held int64, unschedulableFor time.Duration, expectDegraded bool) {
			node := uniqueNode()
			createNode(node, allocatable)
			createDaemonSet(node)
			createCardMgmtPod(node, requested, unschedulableFor)
			if held > 0 {
				createUserPod(node, held)
			}
			obj, ok := newControlledDaemonSet(node).(DegradableObject)
			Expect(ok).To(BeTrue())
			Expect(obj.NodeNames()).To(Equal([]string{node}))
			degraded := obj.Degraded(ctx, StateClient)
			if expectDegraded {
				Expect(degraded).To(HaveLen(1))
				Expect(degraded[0].Name).To(Equal(node))
				Expect(degraded[0].Reason).To(ContainSubstring("devices are not available"))
			} else {
				Expect(degraded).To(BeEmpty())
			}
		},
		Entry("all devices are reserved by other pods", int64(2), int64(2), int64(1), time.Hour, true),
		Entry("devices are not in the allocatable list", int64(0), int64(2), int64(0), time.Hour, true),
		Entry("devices are free but pending is recent (transient)", int64(2), int64(2), int64(0), time.Second, false),
		Entry("devices are free (pending for other reason)", int64(2), int64(2), int64(0), time.Hour, false),
	)

	It("does not regard a non-pending pod as degraded", func() {
		node := uniqueNode()
		createNode(node, 0)
		createDaemonSet(node)
		pod := createCardMgmtPod(node, 2, time.Hour)
		pod.Status.Phase = corev1.PodRunning
		Expect(StateClient.Status().Update(ctx, pod)).To(Succeed())
		obj := newControlledDaemonSet(node).(DegradableObject)
		Expect(obj.Degraded(ctx, StateClient)).To(BeEmpty())
	})

	Context("ControlledComponent", func() {
		// setStatus sets DaemonSet status to be ready or pending (desired but not ready)
		setStatus := func(node string, ready bool) {
			ds := getDaemonSet(node)
			n := int32(1)
			ds.Status = appsv1.DaemonSetStatus{DesiredNumberScheduled: n, CurrentNumberScheduled: n,
				UpdatedNumberScheduled: n, ObservedGeneration: ds.Generation}
			if ready {
				ds.Status.NumberReady = n
				ds.Status.NumberAvailable = n
			}
			Expect(StateClient.Status().Update(ctx, ds)).To(Succeed())
		}

		newComponent := func(nodeA, nodeB string) *ControlledComponent {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "degraded-test-last", Namespace: OpNs}}
			track(sa)
			defaultObj, err := NewDefaultObject(ctx, "ServiceAccount", OpNs, sa)
			Expect(err).To(BeNil())
			last, err := NewServiceAccount(defaultObj, sa, OpNs)
			Expect(err).To(BeNil())
			c := &ControlledComponent{}
			c.ExportSetName(spyreconst.CardManagementResourceName)
			c.ExportSetNamespace(OpNs)
			c.ExportSetClient(StateClient)
			// the pending DaemonSet (node A) comes before the healthy one (node B) and the last step
			c.SetObjects([]ControlledObject{newControlledDaemonSet(nodeA), newControlledDaemonSet(nodeB), last})
			for _, n := range []string{nodeA, nodeB} {
				ds := newDaemonSet(n)
				track(ds)
			}
			return c
		}

		getNode := func(name string) *corev1.Node {
			n := &corev1.Node{}
			Expect(StateClient.Get(ctx, client.ObjectKey{Name: name}, n)).To(Succeed())
			return n
		}

		It("does not block the healthy node and reports the node with permanent pending as degraded", func() {
			nodeA, nodeB := uniqueNode(), uniqueNode()
			createNode(nodeA, 0)
			createNode(nodeB, 2)
			c := newComponent(nodeA, nodeB)

			By("creating objects")
			_, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			createCardMgmtPod(nodeA, 2, time.Hour)
			setStatus(nodeA, false)
			setStatus(nodeB, true)

			By("syncing: node B DaemonSet and the last step are synced regardless of node A")
			ready, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(c.DegradedNodes()).To(HaveLen(1))
			Expect(c.DegradedNodes()[0].Name).To(Equal(nodeA))
			Expect(StateClient.Get(ctx, client.ObjectKey{Name: "degraded-test-last", Namespace: OpNs},
				&corev1.ServiceAccount{})).To(Succeed())

			By("checking NoSchedule taint and condition on node A only")
			a := getNode(nodeA)
			Expect(a.Spec.Taints).To(ContainElement(SatisfyAll(
				HaveField("Key", spyreconst.CardManagementDegradedTaintKey),
				HaveField("Effect", corev1.TaintEffectNoSchedule))))
			Expect(a.Status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionTrue))))
			Expect(getNode(nodeB).Spec.Taints).NotTo(ContainElement(HaveField("Key", spyreconst.CardManagementDegradedTaintKey)))

			By("recovering when node A DaemonSet becomes ready")
			setStatus(nodeA, true)
			ready, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(c.DegradedNodes()).To(BeEmpty())
			a = getNode(nodeA)
			Expect(a.Spec.Taints).NotTo(ContainElement(HaveField("Key", spyreconst.CardManagementDegradedTaintKey)))
			Expect(a.Status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionFalse))))
		})

		It("stays not ready while the pending is still transient", func() {
			nodeA, nodeB := uniqueNode(), uniqueNode()
			createNode(nodeA, 2)
			createNode(nodeB, 2)
			c := newComponent(nodeA, nodeB)
			_, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			createCardMgmtPod(nodeA, 2, time.Second)
			setStatus(nodeA, false)
			setStatus(nodeB, true)
			ready, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeFalse())
			Expect(c.DegradedNodes()).To(BeEmpty())
			Expect(getNode(nodeA).Spec.Taints).NotTo(ContainElement(HaveField("Key", spyreconst.CardManagementDegradedTaintKey)))
		})
	})

	It("summarizes degraded nodes in a message", func() {
		msg := DegradedMessage([]DegradedNode{{Name: "node-b", Reason: "r2"}, {Name: "node-a", Reason: "r1"}})
		Expect(msg).To(Equal("degraded nodes: node-a (r1); node-b (r2)"))
	})
})
