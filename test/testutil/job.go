/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	SmallToyJobName       = "aiu-small-toy"
	SmallToyJobLabel      = "app=aiu-small-toy"
	SmallToyContainerName = "small-toy"
)

// GetJobPod returns the single Pod created by a Job.
func GetJobPod(ctx context.Context, k8sClientset *kubernetes.Clientset, g Gomega, namespace, label string) corev1.Pod {
	pods := GetPodsWithLabels(ctx, k8sClientset, g, namespace, label, "")
	g.Expect(pods).To(HaveLen(1), "expect exactly one Pod for label %s in namespace %s", label, namespace)
	return pods[0]
}

// DumpJobPodLog writes the log of a Job's Pod to the Ginkgo output; best-effort.
func DumpJobPodLog(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, label, container string) {
	pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil || len(pods.Items) == 0 {
		By(fmt.Sprintf("no Pod found for label %s to get log from", label))
		return
	}
	for _, pod := range pods.Items {
		log, err := GetPodLog(ctx, k8sClientset, container, pod)
		if err != nil {
			By(fmt.Sprintf("could not get log of %s/%s: %v", pod.Namespace, pod.Name, err))
			continue
		}
		By(fmt.Sprintf("log of %s/%s:\n%s", pod.Namespace, pod.Name, log))
	}
}

// waitForJob polls a Job until check passes, aborting as soon as it fails.
// backoffLimit is 0, so a failed Pod is terminal.
func waitForJob(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, jobName, label, container string,
	timeout time.Duration, check func(g Gomega, job *batchv1.Job)) {
	Eventually(func(g Gomega) {
		job, err := k8sClientset.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
		g.Expect(err).To(BeNil())
		if job.Status.Failed > 0 {
			DumpJobPodLog(ctx, k8sClientset, namespace, label, container)
			StopTrying(fmt.Sprintf("job %s/%s failed: %s", namespace, jobName, jobFailureMessage(job))).Now()
		}
		check(g, job)
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())
}

// jobFailureMessage summarizes why a Job reported a failure.
func jobFailureMessage(job *batchv1.Job) string {
	messages := make([]string, 0, len(job.Status.Conditions))
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			messages = append(messages, fmt.Sprintf("%s: %s", condition.Reason, condition.Message))
		}
	}
	if len(messages) == 0 {
		return fmt.Sprintf("%d failed Pod(s)", job.Status.Failed)
	}
	return strings.Join(messages, "; ")
}

// WaitForJobPodRunning waits until the Pod of a Job has started, and returns it.
// A completed Pod also satisfies this, so callers must tolerate one.
func WaitForJobPodRunning(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, jobName, label,
	container string, timeout time.Duration) corev1.Pod {
	var jobPod corev1.Pod
	By(fmt.Sprintf("waiting for the Pod of job %s/%s to start", namespace, jobName))
	waitForJob(ctx, k8sClientset, namespace, jobName, label, container, timeout,
		func(g Gomega, _ *batchv1.Job) {
			jobPod = GetJobPod(ctx, k8sClientset, g, namespace, label)
			printMessageIfPodNotRunning(jobPod)
			g.Expect(jobPod.Status.Phase).To(BeElementOf(corev1.PodRunning, corev1.PodSucceeded))
		})
	return jobPod
}

// WaitForJobSucceeded waits until a Job reports a successful completion.
func WaitForJobSucceeded(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, jobName, label,
	container string, timeout time.Duration) {
	By(fmt.Sprintf("waiting for job %s/%s to complete successfully", namespace, jobName))
	waitForJob(ctx, k8sClientset, namespace, jobName, label, container, timeout,
		func(g Gomega, job *batchv1.Job) {
			g.Expect(job.Status.Succeeded).To(BeNumerically(">=", 1), "job has not completed successfully yet")
		})
}

// DeleteJob deletes a Job and waits until its Pods are gone.
func DeleteJob(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace, jobName, label string) {
	By(fmt.Sprintf("deleting job %s/%s", namespace, jobName))
	policy := metav1.DeletePropagationForeground
	err := k8sClientset.BatchV1().Jobs(namespace).Delete(ctx, jobName, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !apiErrors.IsNotFound(err) {
		Expect(err).To(BeNil())
	}
	Eventually(func(g Gomega) {
		pods, err := k8sClientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: label})
		g.Expect(err).To(BeNil())
		g.Expect(pods.Items).To(BeEmpty())
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
}
