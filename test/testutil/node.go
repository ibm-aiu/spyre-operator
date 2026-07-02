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
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	workerNodeLabel      = "node-role.kubernetes.io/worker"
	spyreWorkerNodeLabel = spyreconst.CommonSpyreLabelKey
	hostnameLabel        = "kubernetes.io/hostname"
)

func GetWorkerNodeNames(ctx context.Context, k8sClientset *kubernetes.Clientset) []string {
	nodeNames := make([]string, 0)
	nodeList, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	Expect(err).To(BeNil())
	Expect(nodeList).ToNot(BeNil())
	Expect(nodeList.Items).ToNot(BeEmpty())
	for _, node := range nodeList.Items {
		// in a crc (openshift local) cluster the node is
		// both a master and worker
		if _, ok := node.Labels[workerNodeLabel]; ok {
			if !IsNodeReady(node) {
				_, err = fmt.Fprintf(GinkgoWriter, "Skip adding %s on GetWorkerNodeNames: NotReady\n", node.Name)
				Expect(err).To(BeNil())
				continue
			}
			nodeNames = append(nodeNames, node.Name)
		}
	}
	return nodeNames
}
func isCRC(ctx context.Context, k8sClientset *kubernetes.Clientset) bool {
	nodesNames := GetWorkerNodeNames(ctx, k8sClientset)
	if len(nodesNames) == 1 && nodesNames[0] == "crc" {
		return true
	}
	return false
}

func CleanUpNode(ctx context.Context, k8sClientset *kubernetes.Clientset, nodeName string) {
	node, err := k8sClientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	Expect(err).To(BeNil())
	newLabel := make(map[string]string)
	for key, value := range node.Labels {
		if _, found := ExpectedNodeLabelsWithPseudoDevice[key]; !found {
			// add only keys those are not in label keys set by the pseudo mode.
			newLabel[key] = value
		}
	}
	node.Labels = newLabel
	_, err = k8sClientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	Expect(err).To(BeNil())
}

func GetSpyreWorkerNodeNames(ctx context.Context, k8sClientset *kubernetes.Clientset) []string {
	labelSelector := fmt.Sprintf("%s,%s", spyreWorkerNodeLabel, workerNodeLabel)
	nodeList, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	Expect(err).To(BeNil())
	Expect(nodeList).ToNot(BeNil())
	Expect(nodeList.Items).ToNot(BeEmpty())

	// Pods that request ibm.com/spyre_pf can only run where that resource is
	// allocatable, so filter to nodes whose device plugin is available.
	pfResource := v1.ResourceName(spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName)
	nodeNames := make([]string, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		presentValue := node.Labels[spyreWorkerNodeLabel]
		pfQuantity, hasPF := node.Status.Allocatable[pfResource]
		serving := hasPF && !pfQuantity.IsZero()

		_, err = fmt.Fprintf(GinkgoWriter,
			"GetSpyreWorkerNodeNames: node=%s %s=%q allocatable[%s]=%s serving=%t\n",
			node.Name, spyreWorkerNodeLabel, presentValue, pfResource, pfQuantity.String(), serving)
		Expect(err).To(BeNil())

		if serving {
			nodeNames = append(nodeNames, node.Name)
		}
	}
	sort.Strings(nodeNames)

	_, err = fmt.Fprintf(GinkgoWriter,
		"GetSpyreWorkerNodeNames: selected %d spyre worker(s) advertising %s (sorted) = %v\n",
		len(nodeNames), pfResource, nodeNames)
	Expect(err).To(BeNil())

	return nodeNames
}

// ExpectNodeServesSpyrePF fails the running spec, with an actionable message, unless
// the node selected by kubernetes.io/hostname=<hostname> advertises allocatable
// ibm.com/spyre_pf > 0. It looks the node up by the hostname label (the same key pods
// use in nodeSelector) rather than by object name, since the two can differ across
// clusters. It retries briefly so a slightly-late device-plugin registration does not
// cause a false failure. Use it to validate an explicitly chosen target node up front
// instead of discovering a misconfigured node via a 120s pod "Pending" timeout.
func ExpectNodeServesSpyrePF(ctx context.Context, k8sClientset *kubernetes.Clientset, hostname string) {
	pfResource := v1.ResourceName(spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName)
	selector := fmt.Sprintf("%s=%s", hostnameLabel, hostname)

	Eventually(func(g Gomega) {
		nodeList, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector})
		g.Expect(err).To(BeNil())
		g.Expect(nodeList.Items).To(HaveLen(1), "expected exactly one node matching %s", selector)

		node := nodeList.Items[0]
		pfQuantity, hasPF := node.Status.Allocatable[pfResource]
		serving := hasPF && !pfQuantity.IsZero()

		_, err = fmt.Fprintf(GinkgoWriter,
			"ExpectNodeServesSpyrePF: hostname=%s node=%s allocatable[%s]=%s serving=%t\n",
			hostname, node.Name, pfResource, pfQuantity.String(), serving)
		g.Expect(err).To(BeNil())

		g.Expect(serving).To(BeTrue(),
			"node %q (hostname %s) does not advertise allocatable %s (got %q); ensure the "+
				"Spyre device plugin is running and serving on this node, or point "+
				"nodeName/NODE_NAME at a node that actually has Spyre cards",
			node.Name, hostname, pfResource, pfQuantity.String())
	}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
}

func IsNodeReady(node v1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			return cond.Status == v1.ConditionTrue
		}
	}
	return false
}
