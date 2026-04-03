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
	"slices"
	"strings"
	"time"

	spyrev2 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// construct list of key:vale of pf runner pod and node name
func RunnerWorkerMap(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, deviceType string, nodeFilter []string) map[string]string {
	wpfmap := make(map[string]string)
	nodes := GetWorkerNodeNames(ctx, k8sClientset)
	for _, node := range nodes {
		if slices.Contains(nodeFilter, node) {
			ns, err := GetSpyreNodeState(ctx, spyreV2Client, node)
			Expect(err).To(BeNil())
			for _, spyreIf := range ns.Spec.SpyreInterfaces {
				pciResName := strings.ReplaceAll(spyreIf.PciAddress, ":", "-")
				pfresName := strings.Join([]string{deviceType, pciResName, node}, "-")
				wpfmap[pfresName] = node
			}
		}
	}
	return wpfmap
}
func VerifyRunner(ctx context.Context, k8sClientset *kubernetes.Clientset, namespace string, runnerWrkmap map[string]string) {
	By("runner pods are created")
	Eventually(func(g Gomega) {
		for podname, nodename := range runnerWrkmap {
			pod, err := k8sClientset.CoreV1().Pods(namespace).Get(ctx, podname, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
			g.Expect(pod.Spec.NodeName).To(BeEquivalentTo(nodename))
		}
	}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

	By("runner pods must be running")
	Eventually(func(g Gomega) {
		for podname := range runnerWrkmap {
			pod, err := k8sClientset.CoreV1().Pods(namespace).Get(ctx, podname, metav1.GetOptions{})
			g.Expect(err).To(BeNil())
			g.Expect(pod.Status.Phase).To(BeEquivalentTo("Running"))
		}
	}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
}

func IsAmd64Arch(ctx context.Context, k8sClientset *kubernetes.Clientset) (bool, error) {
	nodes, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("fail to list Nodes: %w", err)
	}
	for _, node := range nodes.Items {
		arch := node.Status.NodeInfo.Architecture
		if arch == "amd64" {
			return true, nil
		}
	}
	return false, nil
}

func IsPpc64LeArch(ctx context.Context, k8sClientset *kubernetes.Clientset) (bool, error) {
	nodes, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("fail to list Nodes: %w", err)
	}
	for _, node := range nodes.Items {
		arch := node.Status.NodeInfo.Architecture
		if arch == "ppc64le" {
			return true, nil
		}
	}
	return false, nil
}

func EnabledCardmgmtForWorkers(ctx context.Context, clusterPolicy *spyrev2.SpyreClusterPolicy, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset, nodeFilter string) {
	nodeList, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	Expect(err).To(BeNil())
	By(fmt.Sprintf("Enabled cardmgmt for %s", nodeFilter))
	clusterPolicy.Spec.CardManagement.Enabled = false
	UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeList.Items), spyrev2.Ready)
	err = spyreV2Client.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceAll, Name: ClusterPolicyName}, clusterPolicy, &client.GetOptions{})
	Expect(err).To(BeNil())
	*clusterPolicy.Spec.CardManagement.Config.SpyreFilter = nodeFilter
	clusterPolicy.Spec.CardManagement.Enabled = true
	UpdateClusterPolicy(ctx, spyreV2Client, k8sClientset, clusterPolicy, len(nodeList.Items), spyrev2.Ready)
}

type PodTemplateData struct {
	Name             string
	Image            string
	ResourceName     string
	ResourceQuantity string
	NodeSelectorNode string
	FlexDevice       string
	SidecarName      string
}

var (
	nospyre = PodTemplateData{
		Name:  "nospyre",
		Image: Ubi9MicroTestImage,
	}
	pf1 = PodTemplateData{
		Name:             "pf1",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_pf",
		ResourceQuantity: "1",
		SidecarName:      "sidecar",
	}
	vfTier02 = PodTemplateData{
		Name:             "vf-tier0-2",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf_tier0",
		ResourceQuantity: "2",
		SidecarName:      "sidecar",
	}
	vfTier14 = PodTemplateData{
		Name:             "vf-tier1-4",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf_tier1",
		ResourceQuantity: "4",
		SidecarName:      "sidecar",
	}
	vfTier24 = PodTemplateData{
		Name:             "vf-tier2-4",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf_tier2",
		ResourceQuantity: "4",
		SidecarName:      "sidecar",
	}
	vf1 = PodTemplateData{
		Name:             "vf1",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf",
		ResourceQuantity: "1",
		SidecarName:      "sidecar",
	}
	vf6 = PodTemplateData{
		Name:             "vf6",
		Image:            Ubi9MicroTestImage,
		ResourceName:     "ibm.com/spyre_vf",
		ResourceQuantity: "6",
		SidecarName:      "sidecar",
	}
)

// Cardmgmt stability test
// Enabled on worker 1
var CardmgmtEnableWorker1TestRunning = []PodTemplateData{nospyre, pf1, vfTier02, vfTier14, vfTier24, vf1, vf6}
var CardmgmtEnableWorker1TestPending = []PodTemplateData{}

// Enabled on worker 2
var CardmgmtEnableWorker2TestRunning = []PodTemplateData{nospyre, pf1, vfTier02, vfTier14, vfTier24, vf1, vf6}

// Enabled on all spyre workers
var CardmgmtEnableAllNodesTestRunning = []PodTemplateData{nospyre, vfTier02, vfTier14, vfTier24, vf1, vf6}
var CardmgmtEnableAllNodesTestPending = []PodTemplateData{pf1}

// Disabled
var CardmgmtDisabledTestRunning = []PodTemplateData{nospyre, pf1, vfTier02, vfTier14, vfTier24, vf1, vf6}
