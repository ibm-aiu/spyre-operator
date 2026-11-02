/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// degradedKeyPrefix is the prefix of the taint key and the node condition type put by the operator
	degradedKeyPrefix = "ibm.com/"

	degradedNodeReason      = "ComponentDegraded"
	degradedRecoveredReason = "ComponentRecovered"
)

// isDegradedTaintKey returns true if the key is a degraded taint key of any component
func isDegradedTaintKey(key string) bool {
	return strings.HasPrefix(key, degradedKeyPrefix) && strings.HasSuffix(key, "-degraded")
}

// degradedTaintKey returns the key of the NoSchedule taint put by the operator on a node
// where the component cannot run permanently. The key is per component so that the components
// do not restore the node degraded by the other component.
// e.g., spyre-card-management -> ibm.com/spyre-card-management-degraded
func degradedTaintKey(component string) string {
	return degradedKeyPrefix + component + "-degraded"
}

// degradedConditionType returns the type of the node condition which reports the degraded node for the component
// e.g., spyre-card-management -> ibm.com/SpyreCardManagementDegraded
func degradedConditionType(component string) corev1.NodeConditionType {
	var sb strings.Builder
	for _, word := range strings.Split(component, "-") {
		if word == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}
	return corev1.NodeConditionType(degradedKeyPrefix + sb.String() + "Degraded")
}

// ensureNodeTaint adds the NoSchedule taint of the component to the node if it is absent and returns the node.
// The DaemonSet pods of the same or the earlier components tolerate the taint to be able to recover
// while the later components cannot be deployed on the node.
func ensureNodeTaint(ctx context.Context, k8sClient client.Client, component, nodeName string) (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}
	taintKey := degradedTaintKey(component)
	if !hasTaint(node, taintKey) {
		patch := client.MergeFrom(node.DeepCopy())
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    taintKey,
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		})
		if err := k8sClient.Patch(ctx, node, patch); err != nil {
			return nil, fmt.Errorf("failed to taint node %s: %w", nodeName, err)
		}
	}
	return node, nil
}

// markNodeFailing holds the node from deploying the later components by the taint of the component
// while the failure is not confirmed as degraded yet. The degraded condition is not reported. It is idempotent.
func markNodeFailing(ctx context.Context, k8sClient client.Client, component, nodeName string) error {
	_, err := ensureNodeTaint(ctx, k8sClient, component, nodeName)
	return err
}

// markNodeDegraded taints the node with NoSchedule and reports the degraded condition on the node.
// It is idempotent.
func markNodeDegraded(ctx context.Context, k8sClient client.Client, component string, degraded DegradedNode) error {
	node, err := ensureNodeTaint(ctx, k8sClient, component, degraded.Name)
	if err != nil {
		return err
	}
	return setDegradedCondition(ctx, k8sClient, node, degradedConditionType(component),
		corev1.ConditionTrue, degradedNodeReason, degraded.Reason)
}

// clearNodeDegraded removes the taint and resets the degraded condition of the component on the node.
// It is idempotent.
func clearNodeDegraded(ctx context.Context, k8sClient client.Client, component, nodeName string) error {
	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}
	taintKey := degradedTaintKey(component)
	if hasTaint(node, taintKey) {
		patch := client.MergeFrom(node.DeepCopy())
		taints := make([]corev1.Taint, 0, len(node.Spec.Taints))
		for _, t := range node.Spec.Taints {
			if t.Key != taintKey {
				taints = append(taints, t)
			}
		}
		node.Spec.Taints = taints
		if err := k8sClient.Patch(ctx, node, patch); err != nil {
			return fmt.Errorf("failed to remove taint from node %s: %w", nodeName, err)
		}
	}
	condType := degradedConditionType(component)
	for _, cond := range node.Status.Conditions {
		if cond.Type == condType && cond.Status == corev1.ConditionTrue {
			return setDegradedCondition(ctx, k8sClient, node, condType, corev1.ConditionFalse, degradedRecoveredReason, "")
		}
	}
	return nil
}

// hasDegradedMark returns true if the node has the degraded taint or the true degraded condition
func hasDegradedMark(node *corev1.Node, taintKey string, condType corev1.NodeConditionType) bool {
	if hasTaint(node, taintKey) {
		return true
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == condType && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func hasTaint(node *corev1.Node, key string) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

// setDegradedCondition patches the degraded condition on the node status. Transition time is kept if status is unchanged.
func setDegradedCondition(ctx context.Context, k8sClient client.Client, node *corev1.Node,
	condType corev1.NodeConditionType, status corev1.ConditionStatus, reason, message string) error {
	now := metav1.Now()
	cond := corev1.NodeCondition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
	}
	for _, existing := range node.Status.Conditions {
		if existing.Type != condType {
			continue
		}
		if existing.Status == status && existing.Message == message {
			return nil // nothing changed
		}
		if existing.Status == status {
			cond.LastTransitionTime = existing.LastTransitionTime
		}
	}
	patch := client.StrategicMergeFrom(node.DeepCopy())
	conds := make([]corev1.NodeCondition, 0, len(node.Status.Conditions)+1)
	for _, existing := range node.Status.Conditions {
		if existing.Type != condType {
			conds = append(conds, existing)
		}
	}
	conds = append(conds, cond)
	node.Status.Conditions = conds
	if err := k8sClient.Status().Patch(ctx, node, patch); err != nil {
		return fmt.Errorf("failed to set degraded condition on node %s: %w", node.Name, err)
	}
	return nil
}

// DegradedMessage summarizes the degraded nodes in a message
func DegradedMessage(nodes []DegradedNode) string {
	msgs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		msgs = append(msgs, fmt.Sprintf("%s (%s)", n.Name, n.Reason))
	}
	sort.Strings(msgs)
	return fmt.Sprintf("degraded nodes: %s", strings.Join(msgs, "; "))
}
