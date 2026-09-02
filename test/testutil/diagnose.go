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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// terminalRestartCount is the restart count at which a crash loop is treated as unrecoverable.
	terminalRestartCount = 3
	// terminalPollCount is how many consecutive polls a failure must persist before giving up.
	terminalPollCount = 3
	// diagnosticLogTailLines is the number of log lines dumped per failed container.
	diagnosticLogTailLines = 20
)

// terminalWaitReasons are waiting reasons a container never recovers from on its own.
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
		By(fmt.Sprintf("Pod %s/%s is %s", pod.Namespace, pod.Name, pod.Status.Phase))
		for _, status := range pod.Status.InitContainerStatuses {
			dumpContainerStatus(ctx, k8sClientset, pod, "init container", status)
		}
		for _, status := range pod.Status.ContainerStatuses {
			dumpContainerStatus(ctx, k8sClientset, pod, "container", status)
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
