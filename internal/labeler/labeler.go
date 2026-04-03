/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package labeler

import (
	"context"
	"fmt"
	"strings"

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	nfdLabelPrefix        = "feature.node.kubernetes.io/"
	commonSpyreLabelValue = "true"
	architectureLabel     = "kubernetes.io/arch"
	productName           = "IBM_Spyre"
)

var spyreNodeLabels = map[string]string{
	"feature.node.kubernetes.io/pci-1014.present":      "true",
	"feature.node.kubernetes.io/pci-06e7_1014.present": "true",
}

var fullPfResourceName = spyreconst.ResourcePrefix + "/" + spyreconst.PfResourceName

type Labeler struct{}

// LabelSpyreNodes labels nodes with Spyre common label
// it return clusterHasNFDLabels (bool), nodeArchitecture (string), error
func (l Labeler) LabelSpyreNodes(ctx context.Context, k8sClient client.Client,
	pseudoDeviceMode bool) (bool, string, error) {
	// fetch all nodes
	opts := []client.ListOption{}
	list := &corev1.NodeList{}
	err := k8sClient.List(ctx, list, opts...)
	if err != nil {
		return false, "", fmt.Errorf("unable to list nodes to check labels, err %s", err.Error())
	}
	for _, node := range list.Items {
		if err := l.addLabelsForNode(ctx, k8sClient, pseudoDeviceMode, node); err != nil {
			return false, "", err
		}
	}
	hasNFD, nodeArchitecture := l.getInfoFromLabels(*list)
	return hasNFD, nodeArchitecture, nil
}

// LabelSpyreNode labels a single node by name. Used when a Node event triggers reconcile to avoid
// listing all nodes — the cache-backed Get is a local lookup rather than a scan of all nodes.
func (l Labeler) LabelSpyreNode(ctx context.Context, k8sClient client.Client,
	pseudoDeviceMode bool, nodeName string) error {
	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}
	return l.addLabelsForNode(ctx, k8sClient, pseudoDeviceMode, *node)
}

// GetClusterSpyreLabelInfo returns hasNFD and node architecture from cluster node labels without applying any labels.
// Use this when sync logic needs cluster label state but should not trigger node labeling (e.g. SpyreClusterPolicy reconcile).
func (l Labeler) GetClusterSpyreLabelInfo(ctx context.Context, k8sClient client.Client) (bool, string, error) {
	list := &corev1.NodeList{}
	if err := k8sClient.List(ctx, list, []client.ListOption{}...); err != nil {
		return false, "", fmt.Errorf("unable to list nodes: %w", err)
	}
	hasNFD, nodeArchitecture := l.getInfoFromLabels(*list)
	return hasNFD, nodeArchitecture, nil
}

// getInfoFromLabels returns hasNFD and node architecture from node labels
func (Labeler) getInfoFromLabels(list corev1.NodeList) (bool, string) {
	hasNFD := false
	nodeArchitecture := ""
	for _, node := range list.Items {
		labels := node.GetLabels()
		if !hasNFD {
			hasNFD = hasNFDLabels(labels)
		}
		if HasCommonSpyreLabel(labels) && nodeArchitecture == "" {
			if arch, found := labels[architectureLabel]; found {
				nodeArchitecture = arch
			}
		}
	}
	return hasNFD, nodeArchitecture
}

func (Labeler) addLabelsForNode(ctx context.Context, k8sClient client.Client,
	pseudoDeviceMode bool, node corev1.Node) error {
	// get node labels
	labels := node.GetLabels()
	capacity := node.Status.Capacity
	commonLabelChanged := updateCommonSpyreLabel(pseudoDeviceMode, labels)
	deviceCountLabelChanged := updateDeviceCountProductName(capacity, labels)
	if commonLabelChanged || deviceCountLabelChanged {
		// update node labels
		if err := updateNodeLabels(ctx, k8sClient, node, labels); err != nil {
			return err
		}
	}
	return nil
}

// RemoveSpyreNodesLabels removes labels from LabelSpyreNodes call
func (l Labeler) RemoveSpyreNodesLabels(ctx context.Context, k8sClient client.Client) error {
	// fetch all nodes
	opts := []client.ListOption{}
	list := &corev1.NodeList{}
	err := k8sClient.List(ctx, list, opts...)
	if err != nil {
		return fmt.Errorf("unable to list nodes to check labels: %w", err)
	}
	var anyError error
	for _, node := range list.Items {
		if err = l.removeLabelsForNode(ctx, k8sClient, node); err != nil {
			anyError = err
		}
	}
	return anyError
}

func (Labeler) removeLabelsForNode(ctx context.Context, k8sClient client.Client, node corev1.Node) error {
	logger := log.FromContext(ctx)
	labels := node.GetLabels()
	labelSizeBefore := len(labels)
	for label := range labels {
		if managedLabel(label) {
			delete(labels, label)
		}
	}
	labelSizeAfter := len(labels)
	if labelSizeAfter != labelSizeBefore {
		logger.Info(fmt.Sprintf("remove managed labels from node %s", node.Name))
		if updateErr := updateNodeLabels(ctx, k8sClient, node, labels); updateErr != nil {
			logger.Error(updateErr, fmt.Sprintf("failed to remove node %s's labels", node.Name))
			return updateErr
		}
	} else {
		logger.Info(fmt.Sprintf("node %s does not contain managed labels, skipped", node.Name))
	}
	return nil
}

// hasNFDLabels return true if node labels contain any NFD labels
func hasNFDLabels(labels map[string]string) bool {
	for key := range labels {
		if strings.HasPrefix(key, nfdLabelPrefix) {
			return true
		}
	}
	return false
}

// updateCommonSpyreLabel updates ibm.com/spyre.present based on NFD/pseudo (operator-managed, like custom NFD label).
// node-role.kubernetes.io/spyre is separate: admin-applied for MachineConfig, not set by operator.
func updateCommonSpyreLabel(pseudoDeviceMode bool, labels map[string]string) bool {
	expectedBoolValue := pseudoDeviceMode || HasSpyreDeviceLabels(labels)
	existingValue, found := labels[spyreconst.CommonSpyreLabelKey]

	expectedValue := spyreconst.FALSE
	if expectedBoolValue {
		expectedValue = spyreconst.TRUE
	}

	changed := (!found && expectedBoolValue) || (found && expectedValue != existingValue)
	if changed {
		labels[spyreconst.CommonSpyreLabelKey] = expectedValue
	}
	return changed
}

// updateDeviceCountProductName adds the device count and product name in the labels map based on the capacity map,
// assume the produce name must be always set together.
func updateDeviceCountProductName(capacity corev1.ResourceList, labels map[string]string) bool {
	expectedQuantity, capacityFound := capacity[corev1.ResourceName(fullPfResourceName)]
	expectedCountValue := ""
	if capacityFound {
		quantity, succeed := expectedQuantity.AsInt64()
		if succeed && quantity > 0 {
			expectedCountValue = expectedQuantity.ToDec().String()
		}
	}
	existingCountValue, found := labels[spyreconst.SpyreCountLabelKey]

	changed := (!found && capacityFound) || (found && expectedCountValue != existingCountValue)
	if changed {
		if expectedCountValue == "" {
			delete(labels, spyreconst.SpyreCountLabelKey)
			delete(labels, spyreconst.SpyreProductLabelKey)
		} else {
			labels[spyreconst.SpyreCountLabelKey] = expectedCountValue
			labels[spyreconst.SpyreProductLabelKey] = productName
		}
	}
	return changed
}

// HasSpyreDeviceLabels return true if node labels contain IBM Spyre labels
// "feature.node.kubernetes.io/pci-1014.present":      "true",
// "feature.node.kubernetes.io/pci-06e7_1014.present": "true",
func HasSpyreDeviceLabels(labels map[string]string) bool {
	for key, val := range labels {
		if _, ok := spyreNodeLabels[key]; ok {
			if spyreNodeLabels[key] == val {
				return true
			}
		}
	}
	return false
}

// HasCommonSpyreLabel returns true if the operator-managed ibm.com/spyre.present label is set (card detected).
func HasCommonSpyreLabel(labels map[string]string) bool {
	if _, ok := labels[spyreconst.CommonSpyreLabelKey]; ok {
		if labels[spyreconst.CommonSpyreLabelKey] == commonSpyreLabelValue {
			return true
		}
	}
	return false
}

func updateNodeLabels(ctx context.Context, k8sClient client.Client,
	node corev1.Node, labels map[string]string) error {
	node.SetLabels(labels)
	err := k8sClient.Update(ctx, &node)
	if err != nil {
		return fmt.Errorf("failed to update node labels: %w", err)
	}
	return nil
}

// managedLabel return true if the label is managed by the operator (ibm.com/spyre.* only; not node-role.kubernetes.io/spyre).
func managedLabel(label string) bool {
	return label == spyreconst.CommonSpyreLabelKey ||
		label == spyreconst.SpyreCountLabelKey || label == spyreconst.SpyreProductLabelKey
}
