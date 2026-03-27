/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package testutil

import (
	"context"
	"fmt"

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
	node.ObjectMeta.Labels = newLabel
	_, err = k8sClientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	Expect(err).To(BeNil())
}

func GetSpyreWorkerNodeNames(ctx context.Context, k8sClientset *kubernetes.Clientset) []string {
	nodeNames := make([]string, 0)
	labelSelector := fmt.Sprintf("%s,%s", spyreWorkerNodeLabel, workerNodeLabel)
	nodeList, err := k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	Expect(err).To(BeNil())
	Expect(nodeList).ToNot(BeNil())
	Expect(nodeList.Items).ToNot(BeEmpty())
	for _, node := range nodeList.Items {
		nodeNames = append(nodeNames, node.Name)
	}
	return nodeNames
}

func IsNodeReady(node v1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			return cond.Status == v1.ConditionTrue
		}
	}
	return false
}
