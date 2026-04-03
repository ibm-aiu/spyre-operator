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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
)

func (tc *TestCase) TestNSpyrePfDeployment(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, n int, expectedPodPhase map[v1.PodPhase]int, expectedAllocatedDevice int) []*appsv1.Deployment {
	// create expectedAllocation
	By("creating deployments")
	testDeployments := generateNDeployments(n, tc.Prefix, tc.TestNamespace, tc.ResourceName, tc.Quantity, false, tc.NodeName)
	deployDeployments(ctx, k8sClientset, tc.TestNamespace, testDeployments, expectedPodPhase)
	By("checking Spyre node state on each node")
	allocationList := checkSpyreNodeStateWithN(ctx, spyreV2Client, tc.NodeName, expectedAllocatedDevice)
	By("checking Senlib config")
	checkPodsLogFromAllocationList(ctx, k8sClientset, allocationList)
	return testDeployments
}

func (tc *TestCase) DeleteNDeployments(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, testDeploys []*appsv1.Deployment, expectedAllocatedDevice int) {
	By("deleting deployments")
	deleteDeployments(ctx, k8sClientset, testDeploys)
	By("checking Spyre node state after deployments deletion")
	checkSpyreNodeStateWithN(ctx, spyreV2Client, tc.NodeName, expectedAllocatedDevice)
}

func (tc *MixedResourceTestCase) TestDeploys(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, expectedPodPhase map[v1.PodPhase]int) []*appsv1.Deployment {
	By("building deployment")
	testDeploys := []*appsv1.Deployment{}
	requestIndex := 0
	for request, n := range tc.Requests {
		prefix := fmt.Sprintf("%s%d", tc.Prefix, requestIndex)
		deploys := generateNDeployments(n, prefix, tc.TestNamespace, request.ResourceName, request.Quantity, false, tc.NodeName)
		testDeploys = append(testDeploys, deploys...)
		requestIndex += 1
	}
	By("creating deployment")
	deployDeployments(ctx, k8sClientset, tc.TestNamespace, testDeploys, expectedPodPhase)
	return testDeploys
}

func (tc *MixedResourceTestCase) DeleteRunningDeployAndExpectPendingRunIfExists(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, testDeploys []*appsv1.Deployment, expectedPodPhase map[v1.PodPhase]int) ([]*appsv1.Deployment, map[v1.PodPhase]int) {
	remainingDeploys := []*appsv1.Deployment{}
	deleted := false
	for _, testDeploy := range testDeploys {
		pod := GetPodFromDeploymentWithoutTrial(ctx, k8sClientset, testDeploy)
		if !deleted && pod.Status.Phase == v1.PodRunning {
			By("deleting running deployment")
			Eventually(func(g Gomega) {
				err := k8sClientset.AppsV1().Deployments(testDeploy.Namespace).Delete(ctx, testDeploy.Name, metav1.DeleteOptions{})
				if err == nil {
					_, err = k8sClientset.AppsV1().Deployments(testDeploy.Namespace).Get(ctx, testDeploy.Name, metav1.GetOptions{})
				}
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
			deleted = true
		} else {
			remainingDeploys = append(remainingDeploys, testDeploy)
		}
	}
	Expect(deleted).To(BeTrue())

	_, found := expectedPodPhase[v1.PodRunning]
	Expect(found).To(BeTrue())
	if count, found := expectedPodPhase[v1.PodPending]; found && count > 0 {
		expectedPodPhase[v1.PodPending] -= 1
		if expectedPodPhase[v1.PodPending] == 0 {
			delete(expectedPodPhase, v1.PodPending)
		}
	} else {
		expectedPodPhase[v1.PodRunning] -= 1
		if expectedPodPhase[v1.PodRunning] == 0 {
			delete(expectedPodPhase, v1.PodRunning)
		}
	}
	By(fmt.Sprintf("checking deployment phases %v", expectedPodPhase))
	CheckDeployPhases(ctx, k8sClientset, remainingDeploys, expectedPodPhase)
	return remainingDeploys, expectedPodPhase
}

func CheckDeployPhases(ctx context.Context, k8sClientset *kubernetes.Clientset, deploys []*appsv1.Deployment, expectedStateNum map[v1.PodPhase]int) {
	if len(expectedStateNum) > 0 {
		By(fmt.Sprintf("waiting for %v from %d deployments", expectedStateNum, len(deploys)))
		Eventually(func(g Gomega) {
			count := make(map[v1.PodPhase]int)
			for _, deploy := range deploys {
				pod := GetPodFromDeployment(ctx, g, k8sClientset, deploy)
				By(fmt.Sprintf("getting pod count: %s - %s", pod.Name, pod.Status.Phase))
				if len(pod.OwnerReferences) > 0 {
					g.Expect(pod.Status.Phase).NotTo(Equal(v1.PodFailed))
				} else {
					Expect(pod.Status.Phase).NotTo(Equal(v1.PodFailed))
				}
				if _, found := count[pod.Status.Phase]; !found {
					count[pod.Status.Phase] = 0
				}
				count[pod.Status.Phase] += 1
			}
			By(fmt.Sprintf("count: %v", count))
			for phase, num := range expectedStateNum {
				if num == 0 {
					continue
				}
				countNum, found := count[phase]
				g.Expect(found).To(BeTrue())
				g.Expect(countNum).To(Equal(num))
			}
		}).WithTimeout(10 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
}

func generateNDeployments(n int, prefix string, namespace string, resourceName string, quantity int64, metricExporterEnabled bool, nodeName string) []*appsv1.Deployment {
	deploys := []*appsv1.Deployment{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("%s-%d", prefix, i)
		if n == 1 {
			name = prefix
		}
		deploy := BuildDeployment(name, namespace, resourceName, quantity, metricExporterEnabled, nodeName)
		deploys = append(deploys, deploy)
	}
	Expect(len(deploys)).To(Equal(n))
	return deploys
}

func deployDeployments(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string, deployments []*appsv1.Deployment, expectedPodPhase map[v1.PodPhase]int) {
	for _, deploy := range deployments {
		Eventually(func(g Gomega) {
			_, err := k8sClientset.AppsV1().Deployments(namespace).Create(ctx, deploy, metav1.CreateOptions{})
			g.Expect(err).To(Succeed())
		}).WithTimeout(1 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
	expectedPods := 0
	for _, num := range expectedPodPhase {
		expectedPods += num
	}
	var existingDeploys []*appsv1.Deployment
	Eventually(func(g Gomega) {
		existingDeploys = getDeployList(ctx, k8sClientset, namespace)
	}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	CheckDeployPhases(ctx, k8sClientset, existingDeploys, expectedPodPhase)
}

func deleteDeployments(ctx context.Context, k8sClientset *kubernetes.Clientset, deployments []*appsv1.Deployment) {
	for _, deploy := range deployments {
		Eventually(func(g Gomega) {
			err := k8sClientset.AppsV1().Deployments(deploy.Namespace).Delete(ctx, deploy.Name, metav1.DeleteOptions{})
			if err == nil {
				_, err = k8sClientset.AppsV1().Deployments(deploy.Namespace).Get(ctx, deploy.Name, metav1.GetOptions{})
			}
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	}
}

func BuildDeployment(name, namespace string, resourceName string, quantity int64, metricExporterEnabled bool, nodeName string) *appsv1.Deployment {
	replicas := int32(1)
	resourceRequest := make(v1.ResourceList)
	resourceLimit := make(v1.ResourceList)
	resourceRequest[v1.ResourceName(resourceName)] = *resource.NewQuantity(quantity, resource.DecimalSI)
	resourceLimit[v1.ResourceName(resourceName)] = *resource.NewQuantity(quantity, resource.DecimalSI)
	annotations := make(map[string]string)
	if !metricExporterEnabled {
		annotations = monitoringDisableAnnotations
	}
	monitorVolume := v1.Volume{
		Name: monitorVolumeName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	monitorMount := v1.VolumeMount{
		Name:      monitorVolumeName,
		MountPath: "/data",
	}
	zeroGracePeriod := int64(0)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: v1.PodSpec{
					NodeSelector:  map[string]string{"kubernetes.io/hostname": nodeName},
					SchedulerName: spyreconst.SpyreSchedulerName,
					Containers: []v1.Container{
						{
							Name:            "app",
							Image:           containerTestImage,
							ImagePullPolicy: v1.PullIfNotPresent,
							Command: []string{
								"/bin/bash", "-c",
							},
							Args: []string{
								"cat /etc/aiu/senlib_config.json;tail -f /dev/null",
							},
							Resources: v1.ResourceRequirements{
								Requests: resourceRequest,
								Limits:   resourceLimit,
							},
							VolumeMounts: []v1.VolumeMount{monitorMount},
						},
					},
					Volumes:                       []v1.Volume{monitorVolume},
					TerminationGracePeriodSeconds: &zeroGracePeriod,
				},
			},
		},
	}
	return deployment
}

func GetPodFromDeploymentWithoutTrial(ctx context.Context, k8sClientset *kubernetes.Clientset, deploy *appsv1.Deployment) v1.Pod {
	labels := fmt.Sprintf("app=%s", deploy.Name)
	listOptions := metav1.ListOptions{
		LabelSelector: labels,
	}
	podList, err := k8sClientset.CoreV1().Pods(deploy.Namespace).List(ctx, listOptions)
	Expect(err).To(BeNil())
	Expect(len(podList.Items)).To(Equal(1))
	return podList.Items[0]
}

func GetPodFromDeployment(ctx context.Context, g Gomega, k8sClientset *kubernetes.Clientset, deploy *appsv1.Deployment) v1.Pod {
	labels := fmt.Sprintf("app=%s", deploy.Name)
	listOptions := metav1.ListOptions{
		LabelSelector: labels,
	}
	podList, err := k8sClientset.CoreV1().Pods(deploy.Namespace).List(ctx, listOptions)
	g.Expect(err).To(BeNil())
	g.Expect(len(podList.Items)).To(Equal(1),
		"if found more than one pods, potentially, at least one of them is failed to allocate by device plugin after scheduled")
	return podList.Items[0]
}

func getDeployList(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string) []*appsv1.Deployment {
	deployList, err := k8sClientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	Expect(err).To(BeNil())
	deploys := []*appsv1.Deployment{}
	for _, deployment := range deployList.Items {
		deploys = append(deploys, deployment.DeepCopy())
	}
	return deploys
}
