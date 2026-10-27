/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resourceSliceDeviceAttributes are the device attributes printed by DumpResourceSlices;
// the DeviceClass selectors of the DRA driver match on productId and managed.
var resourceSliceDeviceAttributes = []string{"productId", "pciAddress", "isPf", "managed", "deviceState"}

// deviceAttributeString formats a DRA device attribute value, or "<unset>" if absent.
func deviceAttributeString(attrs map[resourcev1.QualifiedName]resourcev1.DeviceAttribute, name string) string {
	attr, ok := attrs[resourcev1.QualifiedName(name)]
	switch {
	case !ok:
		return "<unset>"
	case attr.StringValue != nil:
		return *attr.StringValue
	case attr.BoolValue != nil:
		return strconv.FormatBool(*attr.BoolValue)
	case attr.IntValue != nil:
		return strconv.FormatInt(*attr.IntValue, 10)
	case attr.VersionValue != nil:
		return *attr.VersionValue
	}
	return "<unset>"
}

// DumpResourceSlices reports the name, driver, node, pool, and every device of every
// ResourceSlice in the cluster, with the attributes the DeviceClasses select on; best-effort.
func DumpResourceSlices(ctx context.Context, k8sClientset *kubernetes.Clientset) {
	list, err := k8sClientset.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		By(fmt.Sprintf("failed to list ResourceSlices: %v", err))
		return
	}
	By(fmt.Sprintf("ResourceSlices (%d items):", len(list.Items)))
	for _, rs := range list.Items {
		nodeName := "<cluster-scoped>"
		if rs.Spec.NodeName != nil {
			nodeName = *rs.Spec.NodeName
		}
		By(fmt.Sprintf("  ResourceSlice %s: driver=%s, node=%s, pool=%s (generation=%d, slices=%d), devices=%d",
			rs.Name, rs.Spec.Driver, nodeName, rs.Spec.Pool.Name, rs.Spec.Pool.Generation,
			rs.Spec.Pool.ResourceSliceCount, len(rs.Spec.Devices)))
		for _, d := range rs.Spec.Devices {
			attrs := make([]string, 0, len(resourceSliceDeviceAttributes))
			for _, name := range resourceSliceDeviceAttributes {
				attrs = append(attrs, fmt.Sprintf("%s=%s", name, deviceAttributeString(d.Attributes, name)))
			}
			By(fmt.Sprintf("    device %s: %s", d.Name, strings.Join(attrs, ", ")))
		}
	}
}

// DumpResourceClaims reports the device requests and allocation state of every
// ResourceClaim in namespace; best-effort.
func DumpResourceClaims(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) {
	list, err := k8sClientset.ResourceV1().ResourceClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		By(fmt.Sprintf("failed to list ResourceClaims in %s: %v", namespace, err))
		return
	}
	By(fmt.Sprintf("ResourceClaims in %s (%d items):", namespace, len(list.Items)))
	for _, rc := range list.Items {
		allocatedDevices := 0
		if rc.Status.Allocation != nil {
			allocatedDevices = len(rc.Status.Allocation.Devices.Results)
		}
		By(fmt.Sprintf("  ResourceClaim %s: requests=%d, allocated=%t, allocatedDevices=%d, reservedFor=%d",
			rc.Name, len(rc.Spec.Devices.Requests), rc.Status.Allocation != nil,
			allocatedDevices, len(rc.Status.ReservedFor)))
	}
}

const (
	// terminalRestartCount is the restart count at which a crash loop is treated as unrecoverable.
	terminalRestartCount = 3
	// terminalPollCount is how many consecutive polls a failure must persist before giving up.
	terminalPollCount = 3
	// diagnosticLogTailLines is the number of log lines dumped per failed container.
	diagnosticLogTailLines = 20
)

// terminalWaitReasons are waiting reasons a container never recovers on its own.
var terminalWaitReasons = []string{
	"ImagePullBackOff", "ErrImagePull", "InvalidImageName",
	"CreateContainerError", "CreateContainerConfigError",
}

// DumpPodStatus reports the phase and container states of every Pod in namespace,
// with the log tail of each failed container; best-effort.
func DumpPodStatus(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) {
	pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		By(fmt.Sprintf("failed to list Pods in %s: %v", namespace, err))
		return
	}
	for _, pod := range pods.Items {
		By(fmt.Sprintf("Pod %s/%s is %s (node: %s, scheduler: %s)",
			pod.Namespace, pod.Name, pod.Status.Phase, pod.Spec.NodeName, pod.Spec.SchedulerName))
		for _, cond := range pod.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				By(fmt.Sprintf("  Condition %s is %s: reason=%s message=%s",
					cond.Type, cond.Status, cond.Reason, cond.Message))
			}
		}
		for _, status := range pod.Status.InitContainerStatuses {
			dumpContainerStatus(ctx, k8sClientset, pod, "init container", status)
		}
		for _, status := range pod.Status.ContainerStatuses {
			dumpContainerStatus(ctx, k8sClientset, pod, "container", status)
		}
	}
}

// DumpDaemonSets reports the status, node selectors, tolerations, and container resources of every DaemonSet in namespace.
func DumpDaemonSets(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) {
	daemonSets, err := k8sClientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		By(fmt.Sprintf("failed to list DaemonSets in %s: %v", namespace, err))
		return
	}
	for _, ds := range daemonSets.Items {
		By(fmt.Sprintf("DaemonSet %s/%s: desired=%d, current=%d, ready=%d, available=%d, unavailable=%d, misscheduled=%d",
			ds.Namespace, ds.Name,
			ds.Status.DesiredNumberScheduled, ds.Status.CurrentNumberScheduled,
			ds.Status.NumberReady, ds.Status.NumberAvailable,
			ds.Status.NumberUnavailable, ds.Status.NumberMisscheduled))
		By(fmt.Sprintf("  NodeSelector: %v, SchedulerName: %s",
			ds.Spec.Template.Spec.NodeSelector, ds.Spec.Template.Spec.SchedulerName))
		for _, c := range ds.Spec.Template.Spec.Containers {
			By(fmt.Sprintf("  Container %s: image=%s, requests=%v, limits=%v",
				c.Name, c.Image, c.Resources.Requests, c.Resources.Limits))
		}
	}
}

// DumpPodEvents reports recent events in namespace to aid failure diagnosis.
func DumpPodEvents(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) {
	events, err := k8sClientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		By(fmt.Sprintf("failed to list Events in %s: %v", namespace, err))
		return
	}
	By(fmt.Sprintf("Events in %s (%d events):", namespace, len(events.Items)))
	for _, event := range events.Items {
		By(fmt.Sprintf("  [%s] %s %s/%s: %s (reason: %s, count: %d)",
			event.Type, event.InvolvedObject.Kind, event.InvolvedObject.Namespace, event.InvolvedObject.Name,
			event.Message, event.Reason, event.Count))
	}
}

// DumpSpyreNodeStates reports the spec and status of all SpyreNodeState CRs.
func DumpSpyreNodeStates(ctx context.Context, spyreV2Client client.Client) {
	var nodeStates spyrev1alpha1.SpyreNodeStateList
	if err := spyreV2Client.List(ctx, &nodeStates); err != nil {
		By(fmt.Sprintf("failed to list SpyreNodeStates: %v", err))
		return
	}
	By(fmt.Sprintf("SpyreNodeStates (%d items):", len(nodeStates.Items)))
	for _, ns := range nodeStates.Items {
		By(fmt.Sprintf("SpyreNodeState %s:", ns.Name))
		By(fmt.Sprintf("  Spec SpyreInterfaces (%d):", len(ns.Spec.SpyreInterfaces)))
		for _, iface := range ns.Spec.SpyreInterfaces {
			By(fmt.Sprintf("    PCI: %s, Health: %s, VFs: %v", iface.PciAddress, iface.Health, iface.Vfs))
		}
		status := ns.Status.DeepCopy()
		status.NormalizeReservations()
		By(fmt.Sprintf("  Status Allocations (%d):", len(status.AllocationList)))
		for _, alloc := range status.AllocationList {
			podDesc := "<none>"
			if alloc.Pod != nil {
				podDesc = fmt.Sprintf("%s/%s (UID: %s)", alloc.Pod.Namespace, alloc.Pod.Name, alloc.Pod.UID)
			}
			By(fmt.Sprintf("    Pool: %s, Pod: %s, Devices: %v", alloc.ResourcePool, podDesc, alloc.DeviceList))
		}
		By(fmt.Sprintf("  Status Reservations (%d pools, reserved devices: %v):", len(status.Reservations), status.ReservedDevices()))
		for pool, res := range status.Reservations {
			for _, entry := range res.Entries {
				By(fmt.Sprintf("    Pool: %s, Pod: %s/%s (UID: %s), Devices: %v, ReservedAt: %v",
					pool, entry.Pod.Namespace, entry.Pod.Name, entry.Pod.UID, entry.DeviceList, entry.ReservedAt))
			}
		}
	}
}

// TerminalPodFailures describes the container failures in namespace that will not recover.
func TerminalPodFailures(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) []string {
	pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	failures := []string{}
	for _, pod := range pods.Items {
		// a Pod on its way out is about to be replaced, its failures are stale
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, status := range slices.Concat(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses) {
			if failure := containerTerminalFailure(status); failure != "" {
				failures = append(failures, fmt.Sprintf("%s/%s %s", pod.Namespace, pod.Name, failure))
			}
		}
	}
	return failures
}

func dumpContainerStatus(ctx context.Context, k8sClientset *kubernetes.Clientset, pod corev1.Pod,
	kind string, status corev1.ContainerStatus) {
	By(fmt.Sprintf("  %s %s %s, %d restarts", kind, status.Name, containerState(status), status.RestartCount))
	if containerState(status) == "running" || status.Ready {
		return
	}
	// a container waiting in backoff has no readable current log, fall back to its last attempt
	podLog, err := containerLogTail(ctx, k8sClientset, pod, status.Name, false)
	if err != nil || strings.TrimSpace(podLog) == "" {
		podLog, err = containerLogTail(ctx, k8sClientset, pod, status.Name, true)
	}
	if err != nil {
		By(fmt.Sprintf("  no log for %s: %v", status.Name, err))
		return
	}
	if strings.TrimSpace(podLog) == "" {
		By(fmt.Sprintf("  %s produced no log, its image was most likely never pulled", status.Name))
		return
	}
	By(fmt.Sprintf("  last %d log lines of %s:\n%s", diagnosticLogTailLines, status.Name, podLog))
}

// containerState summarizes a container state as a single line.
func containerState(status corev1.ContainerStatus) string {
	if waiting := status.State.Waiting; waiting != nil {
		return strings.TrimSpace(fmt.Sprintf("waiting on %s: %s", waiting.Reason, waiting.Message))
	}
	if terminated := status.State.Terminated; terminated != nil {
		return fmt.Sprintf("terminated with exit code %d (%s)", terminated.ExitCode, terminated.Reason)
	}
	if status.State.Running != nil {
		return "running"
	}
	return "in an unknown state"
}

// containerTerminalFailure describes a container that will not recover, or "" otherwise.
func containerTerminalFailure(status corev1.ContainerStatus) string {
	if waiting := status.State.Waiting; waiting != nil && slices.Contains(terminalWaitReasons, waiting.Reason) {
		return fmt.Sprintf("%s is %s", status.Name, containerState(status))
	}
	// a crash loop only counts once the restarts have piled up
	if status.RestartCount < terminalRestartCount {
		return ""
	}
	for _, terminated := range []*corev1.ContainerStateTerminated{status.State.Terminated, status.LastTerminationState.Terminated} {
		if terminated != nil && terminated.ExitCode != 0 {
			return fmt.Sprintf("%s exited with code %d after %d restarts", status.Name, terminated.ExitCode, status.RestartCount)
		}
	}
	return ""
}

func containerLogTail(ctx context.Context, k8sClientset *kubernetes.Clientset, pod corev1.Pod,
	container string, previous bool) (string, error) {
	tailLines := int64(diagnosticLogTailLines)
	req := k8sClientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: container, Previous: previous, TailLines: &tailLines})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get container '%s' log: %w", container, err)
	}
	defer stream.Close() //nolint:errcheck
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, stream); err != nil {
		return "", fmt.Errorf("failed to copy container '%s' log: %w", container, err)
	}
	return buf.String(), nil
}
