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

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
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
	var (
		degradedConditionType = DegradedConditionType(spyreconst.CardManagementResourceName)
		taintKey              = DegradedTaintKey(spyreconst.CardManagementResourceName)
		ctx                   = context.Background()
		pfName                = corev1.ResourceName(spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName)
		counter               = 0
		created               []client.Object
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

	waiting := func(reason string) corev1.ContainerState {
		return corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: "failure message"}}
	}
	terminated := func(exitCode int32) *corev1.ContainerStateTerminated {
		return &corev1.ContainerStateTerminated{ExitCode: exitCode, Reason: "Error"}
	}

	// createFailingPod creates a pod of the DaemonSet scheduled on the node with the container status,
	// not ready since failingFor
	createFailingPod := func(ds *appsv1.DaemonSet, node string, status corev1.ContainerStatus, failingFor time.Duration) {
		isController := true
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "failing-" + node, Namespace: OpNs, Labels: ds.Spec.Template.Labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.Name, UID: ds.UID, Controller: &isController}},
			},
			Spec: corev1.PodSpec{
				NodeName:   node,
				Containers: []corev1.Container{{Name: "app", Image: "image"}},
			},
		}
		Expect(StateClient.Create(ctx, pod)).To(Succeed())
		track(pod)
		status.Name = "app"
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type: corev1.ContainersReady, Status: corev1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-failingFor)),
			}},
			ContainerStatuses: []corev1.ContainerStatus{status},
		}
		Expect(StateClient.Status().Update(ctx, pod)).To(Succeed())
	}

	Context("Container failure", func() {
		DescribeTable("Degraded",
			func(status corev1.ContainerStatus, failingFor time.Duration, expectedReason string, expectedFailing bool) {
				node := uniqueNode()
				createNode(node, 2)
				// any DaemonSet (not only card management) is degradable
				ds := newDaemonSet(node)
				ds.Name = "plain-ds-" + node
				Expect(StateClient.Create(ctx, ds)).To(Succeed())
				track(ds)
				createFailingPod(ds, node, status, failingFor)
				defaultObj, err := NewDefaultObject(ctx, "DaemonSet", OpNs, ds)
				Expect(err).To(BeNil())
				plain, err := NewDaemonSet(defaultObj, ds, OpNs)
				Expect(err).To(BeNil())
				Expect(plain).To(BeAssignableToTypeOf(&DaemonSet{}))
				degradable, ok := plain.(DegradableObject)
				Expect(ok).To(BeTrue())
				failing := degradable.Failing(ctx, StateClient)
				if expectedFailing {
					Expect(failing).To(HaveLen(1))
					Expect(failing[0].Name).To(Equal(node))
				} else {
					Expect(failing).To(BeEmpty())
				}
				degraded := degradable.Degraded(ctx, StateClient)
				if expectedReason != "" {
					Expect(degraded).To(HaveLen(1))
					Expect(degraded[0].Name).To(Equal(node))
					Expect(degraded[0].Reason).To(ContainSubstring(expectedReason))
				} else {
					Expect(degraded).To(BeEmpty())
				}
			},
			Entry("CreateContainerConfigError over the grace period",
				corev1.ContainerStatus{State: waiting("CreateContainerConfigError")}, time.Hour, "CreateContainerConfigError", true),
			Entry("CreateContainerError over the grace period",
				corev1.ContainerStatus{State: waiting("CreateContainerError")}, time.Hour, "CreateContainerError", true),
			Entry("CreateContainerConfigError but recent (transient)",
				corev1.ContainerStatus{State: waiting("CreateContainerConfigError")}, time.Second, "", true),
			Entry("not a creation failure (image is being pulled)",
				corev1.ContainerStatus{State: waiting("ContainerCreating")}, time.Hour, "", false),
			Entry("exit 1 repeatedly at runtime (CrashLoopBackOff)",
				corev1.ContainerStatus{State: waiting("CrashLoopBackOff"), RestartCount: 5,
					LastTerminationState: corev1.ContainerState{Terminated: terminated(1)}}, time.Hour, "exit code 1", true),
			Entry("exit 1 repeatedly at runtime (just terminated)",
				corev1.ContainerStatus{State: corev1.ContainerState{Terminated: terminated(1)}, RestartCount: 5},
				time.Hour, "exit code 1", true),
			Entry("exit 1 at runtime but few restarts (transient)",
				corev1.ContainerStatus{State: waiting("CrashLoopBackOff"), RestartCount: 1,
					LastTerminationState: corev1.ContainerState{Terminated: terminated(1)}}, time.Hour, "", true),
			Entry("exit 1 repeatedly at runtime is degraded instantly without waiting for the grace period",
				corev1.ContainerStatus{State: waiting("CrashLoopBackOff"), RestartCount: 3,
					LastTerminationState: corev1.ContainerState{Terminated: terminated(1)}}, time.Second, "exit code 1", true),
			Entry("restarted with exit 0 (completed normally)",
				corev1.ContainerStatus{State: waiting("CrashLoopBackOff"), RestartCount: 5,
					LastTerminationState: corev1.ContainerState{Terminated: terminated(0)}}, time.Hour, "", false),
		)
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
				HaveField("Key", taintKey),
				HaveField("Effect", corev1.TaintEffectNoSchedule))))
			Expect(a.Status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionTrue))))
			Expect(getNode(nodeB).Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))

			By("recovering when node A DaemonSet becomes ready")
			setStatus(nodeA, true)
			ready, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(c.DegradedNodes()).To(BeEmpty())
			a = getNode(nodeA)
			Expect(a.Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))
			Expect(a.Status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionFalse))))
		})

		It("holds the node by the taint only while the container failure is not confirmed", func() {
			nodeA, nodeB := uniqueNode(), uniqueNode()
			createNode(nodeA, 2)
			createNode(nodeB, 2)
			c := newComponent(nodeA, nodeB)
			_, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			createFailingPod(getDaemonSet(nodeA), nodeA, corev1.ContainerStatus{
				State: waiting("CrashLoopBackOff"), RestartCount: 1,
				LastTerminationState: corev1.ContainerState{Terminated: terminated(1)}}, time.Second)
			setStatus(nodeA, false)
			setStatus(nodeB, true)

			By("holding node A without reporting it as degraded")
			ready, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeFalse())
			Expect(c.DegradedNodes()).To(BeEmpty())
			a := getNode(nodeA)
			Expect(a.Spec.Taints).To(ContainElement(SatisfyAll(
				HaveField("Key", taintKey), HaveField("Effect", corev1.TaintEffectNoSchedule))))
			Expect(a.Status.Conditions).NotTo(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionTrue))))
			Expect(getNode(nodeB).Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))

			By("releasing node A when the DaemonSet becomes ready")
			setStatus(nodeA, true)
			ready, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(getNode(nodeA).Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))
		})

		It("removes the taint and the condition of failing and degraded nodes on Clear", func() {
			nodeA, nodeB := uniqueNode(), uniqueNode()
			createNode(nodeA, 2)
			createNode(nodeB, 0)
			c := newComponent(nodeA, nodeB)
			_, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			// node A is failing (taint only), node B is degraded (taint and condition)
			createFailingPod(getDaemonSet(nodeA), nodeA, corev1.ContainerStatus{
				State: waiting("CrashLoopBackOff"), RestartCount: 1,
				LastTerminationState: corev1.ContainerState{Terminated: terminated(1)}}, time.Second)
			createCardMgmtPod(nodeB, 2, time.Hour)
			setStatus(nodeA, false)
			setStatus(nodeB, false)
			_, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(getNode(nodeA).Spec.Taints).To(ContainElement(HaveField("Key", taintKey)))
			b := getNode(nodeB)
			Expect(b.Spec.Taints).To(ContainElement(HaveField("Key", taintKey)))
			Expect(b.Status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionTrue))))

			By("cleaning up as the SpyreClusterPolicy is deleted")
			Expect(c.Clear(ctx)).To(Succeed())
			Expect(getNode(nodeA).Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))
			b = getNode(nodeB)
			Expect(b.Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))
			Expect(b.Status.Conditions).NotTo(ContainElement(SatisfyAll(
				HaveField("Type", degradedConditionType), HaveField("Status", corev1.ConditionTrue))))
		})

		It("does not stay pending when the only node is held by the taint of an earlier component", func() {
			node := uniqueNode()
			n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: node,
				Labels: map[string]string{"kubernetes.io/hostname": node}},
				Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
					Key: DegradedTaintKey(spyreconst.DevicePluginResourceName), Value: "true", Effect: corev1.TaintEffectNoSchedule}}}}
			Expect(StateClient.Create(ctx, n)).To(Succeed())
			track(n)

			// the DaemonSet of a later component, which does not tolerate the taint of the device plugin
			ds := newDaemonSet(node)
			ds.Name = spyreconst.MonitorResourceName + "-" + node
			defaultObj, err := NewDefaultObject(ctx, "DaemonSet", OpNs, ds)
			Expect(err).To(BeNil())
			obj, err := NewDaemonSet(defaultObj, ds, OpNs)
			Expect(err).To(BeNil())
			c := &ControlledComponent{}
			c.ExportSetName(spyreconst.MonitorResourceName)
			c.ExportSetNamespace(OpNs)
			c.ExportSetClient(StateClient)
			c.SetObjects([]ControlledObject{obj})
			_, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())
			track(ds)

			// no Pod is created on the node, as the DaemonSet controller would do
			created := &appsv1.DaemonSet{}
			Expect(StateClient.Get(ctx, client.ObjectKeyFromObject(ds), created)).To(Succeed())
			created.Status = appsv1.DaemonSetStatus{ObservedGeneration: created.Generation}
			Expect(StateClient.Status().Update(ctx, created)).To(Succeed())

			ready, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(c.DegradedNodes()).To(BeEmpty())

			By("staying pending once the node is not held by a taint any more")
			n = getNode(node)
			n.Spec.Taints = nil
			Expect(StateClient.Update(ctx, n)).To(Succeed())
			ready, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeFalse())
		})

		It("is not blocked by the service without endpoint when every pod of the DaemonSet is degraded", func() {
			node := uniqueNode()
			createNode(node, 2)
			dsObj := newControlledDaemonSet(node)
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "degraded-test-svc-" + node, Namespace: OpNs},
				Spec: corev1.ServiceSpec{
					Selector: newDaemonSet(node).Spec.Template.Labels,
					Ports:    []corev1.ServicePort{{Name: "http", Port: 8080}},
				},
			}
			svcDefault, err := NewDefaultObject(ctx, "Service", OpNs, svc)
			Expect(err).To(BeNil())
			svcObj, err := NewService(svcDefault, svc, OpNs)
			Expect(err).To(BeNil())
			track(svc)
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "degraded-test-after-svc", Namespace: OpNs}}
			track(sa)
			saDefault, err := NewDefaultObject(ctx, "ServiceAccount", OpNs, sa)
			Expect(err).To(BeNil())
			after, err := NewServiceAccount(saDefault, sa, OpNs)
			Expect(err).To(BeNil())
			c := &ControlledComponent{}
			c.ExportSetName(spyreconst.CardManagementResourceName)
			c.ExportSetNamespace(OpNs)
			c.ExportSetClient(StateClient)
			// DaemonSet -> Service -> the step after the Service (e.g., ServiceMonitor)
			c.SetObjects([]ControlledObject{dsObj, svcObj, after})
			track(newDaemonSet(node))
			_, _, err = c.Sync(ctx)
			Expect(err).To(BeNil())

			// the only pod is desired and crashing, so the service has no endpoint
			createFailingPod(getDaemonSet(node), node, corev1.ContainerStatus{
				State: waiting("CrashLoopBackOff"), RestartCount: 5,
				LastTerminationState: corev1.ContainerState{Terminated: terminated(1)}}, time.Hour)
			setStatus(node, false)

			ready, _, err := c.Sync(ctx)
			Expect(err).To(BeNil())
			Expect(ready).To(BeTrue())
			Expect(c.DegradedNodes()).To(HaveLen(1))
			Expect(StateClient.Get(ctx, client.ObjectKey{Name: sa.Name, Namespace: OpNs}, &corev1.ServiceAccount{})).To(Succeed())
			Expect(getNode(node).Spec.Taints).To(ContainElement(HaveField("Key", taintKey)))
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
			Expect(getNode(nodeA).Spec.Taints).NotTo(ContainElement(HaveField("Key", taintKey)))
		})
	})

	Context("Service of DaemonSets", func() {
		// newService creates the service selecting the pods of the DaemonSet of the node, and returns its controlled object
		newService := func(node string) ControlledObject {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "degraded-test-svc-" + node, Namespace: OpNs},
				Spec: corev1.ServiceSpec{
					Selector: newDaemonSet(node).Spec.Template.Labels,
					Ports:    []corev1.ServicePort{{Name: "http", Port: 8080}},
				},
			}
			defaultObj, err := NewDefaultObject(ctx, "Service", OpNs, svc)
			Expect(err).To(BeNil())
			obj, err := NewService(defaultObj, svc, OpNs)
			Expect(err).To(BeNil())
			Expect(StateClient.Create(ctx, svc)).To(Succeed())
			track(svc)
			return obj
		}

		setDesired := func(node string, desired int32) {
			ds := getDaemonSet(node)
			ds.Status = appsv1.DaemonSetStatus{DesiredNumberScheduled: desired, ObservedGeneration: ds.Generation}
			Expect(StateClient.Status().Update(ctx, ds)).To(Succeed())
		}

		It("is ready without endpoint when the DaemonSet has no desired pod", func() {
			node := uniqueNode()
			createDaemonSet(node)
			svc := newService(node)

			By("not ready while the DaemonSet is not observed by the controller yet")
			Expect(svc.Ready(ctx, StateClient)).To(BeFalse())

			By("ready as no pod is desired, e.g., the nodes are held by the taint of a failing component")
			setDesired(node, 0)
			Expect(svc.Ready(ctx, StateClient)).To(BeTrue())

			By("not ready when a pod is desired but there is no endpoint")
			setDesired(node, 1)
			Expect(svc.Ready(ctx, StateClient)).To(BeFalse())
		})

		It("is not ready without endpoint when no DaemonSet is selected", func() {
			Expect(newService(uniqueNode()).Ready(ctx, StateClient)).To(BeFalse())
		})
	})

	DescribeTable("Overall state",
		func(running, coreSuccess, optionalSuccess bool, held, degraded []DegradedNode,
			expectedState spyrev1alpha1.State, expectedMessage string) {
			state, message := OverallState(running, coreSuccess, optionalSuccess, []string{"msg"}, held, degraded)
			Expect(state).To(Equal(expectedState))
			Expect(message).To(ContainSubstring(expectedMessage))
		},
		Entry("everything is ready", true, true, true, nil, nil, spyrev1alpha1.Ready, ""),
		Entry("degraded nodes with the others serving", false, true, true,
			[]DegradedNode{{Name: "a", Reason: "r"}}, []DegradedNode{{Name: "a", Reason: "r"}},
			spyrev1alpha1.Degraded, "degraded nodes: a (r)"),
		Entry("Ready to failing: Degraded, not NotReady", true, false, true,
			[]DegradedNode{{Name: "a", Reason: "r"}}, nil, spyrev1alpha1.Degraded, "a (r)"),
		Entry("Degraded to failing on another node stays Degraded", true, false, false,
			[]DegradedNode{{Name: "b", Reason: "r"}}, []DegradedNode{{Name: "a", Reason: "r"}},
			spyrev1alpha1.Degraded, "b (r)"),
		Entry("first deployment with a failing node: NotReady", false, false, false,
			[]DegradedNode{{Name: "a", Reason: "r"}}, nil, spyrev1alpha1.NotReady, "a (r)"),
		Entry("Ready to a new node initializing (no failing node): Degraded", true, false, true,
			nil, nil, spyrev1alpha1.Degraded, "msg"),
		Entry("not running and not ready without a held node: NotReady", false, false, true, nil, nil,
			spyrev1alpha1.NotReady, "msg"),
	)

	It("summarizes degraded nodes in a message", func() {
		msg := DegradedMessage([]DegradedNode{{Name: "node-b", Reason: "r2"}, {Name: "node-a", Reason: "r1"}})
		Expect(msg).To(Equal("degraded nodes: node-a (r1); node-b (r2)"))
	})
})
