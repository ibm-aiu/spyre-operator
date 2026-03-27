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
	"slices"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSpyreNodeState(ctx context.Context, spyreV2Client client.Client, nodeName string) (spyrev1alpha1.SpyreNodeState, error) {
	namespacedName := types.NamespacedName{
		Name: nodeName,
	}
	var nodeState spyrev1alpha1.SpyreNodeState
	err := spyreV2Client.Get(ctx, namespacedName, &nodeState)
	if err != nil {
		return nodeState, fmt.Errorf("failed to get spyre node state for node: '%s': %w", nodeName, err)
	}
	return nodeState, nil
}

func checkSpyreNodeState(ctx context.Context, spyreV2Client client.Client, expectedAllocation map[string]bool, nodeName string) {
	Eventually(func(g Gomega) {
		g.Expect(nodeName).ToNot(BeEmpty())
		nodeState, err := GetSpyreNodeState(ctx, spyreV2Client, nodeName)
		g.Expect(err).To(BeNil())
		totalAllocation := 0
		for _, allocated := range expectedAllocation {
			if allocated {
				totalAllocation += 1
			}
		}
		if totalAllocation > 0 {
			g.Expect(len(nodeState.Status.AllocationList)).To(Equal(1))
			g.Expect(len(nodeState.Status.AllocationList[0].DeviceList)).To(BeEquivalentTo(totalAllocation))
			for _, dev := range nodeState.Status.AllocationList[0].DeviceList {
				allocated, found := expectedAllocation[dev]
				g.Expect(found).To(BeTrue())
				g.Expect(allocated).To(BeTrue())
			}
		} else {
			g.Expect(len(nodeState.Status.AllocationList)).To(Equal(0))
		}
	}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
}

func checkSpyreNodeStateWithN(ctx context.Context, spyreV2Client client.Client, nodeName string, expectedNumber int) []spyrev1alpha1.Allocation {
	Eventually(func(g Gomega) {
		g.Expect(nodeName).ToNot(BeEmpty())
		nodeState, err := GetSpyreNodeState(ctx, spyreV2Client, nodeName)
		g.Expect(err).To(BeNil())
		if expectedNumber == 0 {
			g.Expect(len(nodeState.Status.AllocationList)).To(Equal(0))
		}
		actualAllocated := 0
		allocationCache := make(map[string]bool)
		for _, allocation := range nodeState.Status.AllocationList {
			actualAllocated += len(allocation.DeviceList)
			for _, dev := range allocation.DeviceList {
				_, found := allocationCache[dev]
				g.Expect(found).To(BeFalse())
				allocationCache[dev] = true
			}
		}
		g.Expect(actualAllocated).To(Equal(expectedNumber))
	}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	nodeState, err := GetSpyreNodeState(ctx, spyreV2Client, nodeName)
	Expect(err).To(BeNil())
	return nodeState.Status.AllocationList
}

func isAvailable(nodeState *spyrev1alpha1.SpyreNodeState, d string) bool {
	if len(d) == 0 { // this happens when no peer2 device exists.
		return false
	}
	for _, a := range nodeState.Status.AllocationList {
		if slices.Contains(a.DeviceList, d) {
			return false
		}
	}
	for _, r := range nodeState.Status.Reservations {
		for _, ds := range r.DeviceSets {
			if slices.Contains(ds, d) {
				return false
			}
		}
	}
	return true
}
func GetAvailableVFSpyreInterface(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, nodes []string) ([]string, bool) {
	var availableIfs []string
	if len(nodes) == 0 {
		nodes = GetWorkerNodeNames(ctx, k8sClientset)
	}
	for _, node := range nodes {
		ns, err := GetSpyreNodeState(ctx, spyreV2Client, node)
		Expect(err).To(BeNil())
		for _, spyreIf := range ns.Spec.SpyreInterfaces {
			for _, spyreVf := range spyreIf.Vfs {
				if isAvailable(&ns, spyreVf) {
					pciResName := strings.ReplaceAll(spyreIf.PciAddress, ":", "_")
					availableIfs = append(availableIfs, pciResName)
				}
			}
		}
	}
	if len(availableIfs) == 0 {
		return availableIfs, false
	}
	return availableIfs, true
}
func GetAvailableSpyreInterface(ctx context.Context, k8sClientset *kubernetes.Clientset, spyreV2Client client.Client, nodes []string) ([]string, bool) {
	var availableIfs []string
	if len(nodes) == 0 {
		nodes = GetWorkerNodeNames(ctx, k8sClientset)
	}
	for _, node := range nodes {
		ns, err := GetSpyreNodeState(ctx, spyreV2Client, node)
		Expect(err).To(BeNil())
		for _, spyreIf := range ns.Spec.SpyreInterfaces {
			if isAvailable(&ns, spyreIf.PciAddress) {
				pciResName := strings.ReplaceAll(spyreIf.PciAddress, ":", "_")
				availableIfs = append(availableIfs, pciResName)
			}
		}
	}
	if len(availableIfs) == 0 {
		return availableIfs, false
	}
	return availableIfs, true
}
func WaitForSpyreWorkerNodeStatePopulated(ctx context.Context, spyreV2Client client.Client, k8sClientset *kubernetes.Clientset) {
	spyreWorkerNodes := GetSpyreWorkerNodeNames(ctx, k8sClientset)
	Eventually(func(g Gomega) {
		_, foundIf := GetAvailableSpyreInterface(ctx, k8sClientset, spyreV2Client, spyreWorkerNodes)
		g.Expect(foundIf).To(BeTrue())
		_, foundVf := GetAvailableVFSpyreInterface(ctx, k8sClientset, spyreV2Client, spyreWorkerNodes)
		g.Expect(foundVf).To(BeTrue())
	}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
}

func NodeDifference(allWorkers []string, nodesToRemove []string) []string {
	removeMap := make(map[string]bool)
	for _, v := range nodesToRemove {
		removeMap[v] = true
	}

	result := []string{}
	for _, v := range allWorkers {
		if !removeMap[v] {
			result = append(result, v)
		}
	}
	return result
}
