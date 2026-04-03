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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCase struct {
	Prefix        string
	TestNamespace string
	ResourceName  string
	Quantity      int64
	NodeName      string
}

type TestStep struct {
	prefix        string
	namespace     string
	step          int
	deploymentMap map[string][]*appsv1.Deployment
	k8sClientset  *kubernetes.Clientset
	spyreV2Client client.Client
	nodeName      string
}

type ResourceRequest struct {
	ResourceName string
	Quantity     int64
}

type MixedResourceTestCase struct {
	Prefix        string
	TestNamespace string
	Requests      map[ResourceRequest]int
	v1.RestartPolicy
	NodeName string
}

func NewTestStep(prefix, namespace string, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, nodeName string) *TestStep {
	return &TestStep{
		prefix:        prefix,
		namespace:     namespace,
		deploymentMap: make(map[string][]*appsv1.Deployment),
		k8sClientset:  k8sClientset,
		spyreV2Client: spyreV2Client,
		nodeName:      nodeName,
	}
}

func (t *TestStep) Deploy(ctx context.Context, resourceName string, quantity int, n int, expectedPodPhase map[v1.PodPhase]int, expectedAllocatedDevice int) string {
	key := fmt.Sprintf("%s-s%d", t.prefix, t.step)
	tc := TestCase{
		Prefix:        key,
		TestNamespace: t.namespace,
		ResourceName:  resourceName,
		Quantity:      int64(quantity),
		NodeName:      t.nodeName,
	}
	deploys := tc.TestNSpyrePfDeployment(ctx, t.k8sClientset, t.spyreV2Client, n, expectedPodPhase, expectedAllocatedDevice)
	t.deploymentMap[key] = deploys
	t.step += 1
	return key
}

func (t *TestStep) Delete(ctx context.Context, key string, expectedPodPhase map[v1.PodPhase]int, expectedAllocatedDevice int) {
	testDeploys, found := t.deploymentMap[key]
	Expect(found).To(BeTrue())
	podMap := make(map[string]string)
	for _, deploy := range testDeploys {
		pod := GetPodFromDeploymentWithoutTrial(ctx, t.k8sClientset, deploy)
		podMap[pod.Name] = pod.Namespace
	}
	By("deleting deployments")
	deleteDeployments(ctx, t.k8sClientset, testDeploys)

	// get all the pods in the namespace
	podList, err := t.k8sClientset.CoreV1().Pods(t.namespace).List(ctx, metav1.ListOptions{})
	Expect(err).To(BeNil())
	pods := []*v1.Pod{}
	for _, pod := range podList.Items {
		// exclude from the pods to check the pods of the deployment
		// deployed earlier
		if _, ok := podMap[pod.Name]; !ok {
			pods = append(pods, pod.DeepCopy())
		}
	}
	checkPodPhases(ctx, t.k8sClientset, pods, expectedPodPhase)
	By("checking SpyreNodeState after deployment deletion")
	checkSpyreNodeStateWithN(ctx, t.spyreV2Client, t.nodeName, expectedAllocatedDevice)
}
