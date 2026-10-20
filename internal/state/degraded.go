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

	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// degradedNodeConditionType is the type of the node condition which reports the degraded node
	degradedNodeConditionType = corev1.NodeConditionType("ibm.com/SpyreCardManagementDegraded")
	degradedNodeReason        = "CardManagementUnschedulable"
	degradedRecoveredReason   = "CardManagementRecovered"
)

// markNodeDegraded taints the node with NoSchedule (card management pods tolerate it to be able to recover)
// and reports the degraded condition on the node. It is idempotent.
func markNodeDegraded(ctx context.Context, k8sClient client.Client, degraded DegradedNode) error {
	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: degraded.Name}, node); err != nil {
		return fmt.Errorf("failed to get node %s: %w", degraded.Name, err)
	}
	if !hasDegradedTaint(node) {
		patch := client.MergeFrom(node.DeepCopy())
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    spyreconst.CardManagementDegradedTaintKey,
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		})
		if err := k8sClient.Patch(ctx, node, patch); err != nil {
			return fmt.Errorf("failed to taint node %s: %w", degraded.Name, err)
		}
	}
	return setDegradedCondition(ctx, k8sClient, node, corev1.ConditionTrue, degradedNodeReason, degraded.Reason)
}

// clearNodeDegraded removes the taint and resets the degraded condition on the node. It is idempotent.
func clearNodeDegraded(ctx context.Context, k8sClient client.Client, nodeName string) error {
	node := &corev1.Node{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}
	if hasDegradedTaint(node) {
		patch := client.MergeFrom(node.DeepCopy())
		taints := make([]corev1.Taint, 0, len(node.Spec.Taints))
		for _, t := range node.Spec.Taints {
			if t.Key != spyreconst.CardManagementDegradedTaintKey {
				taints = append(taints, t)
			}
		}
		node.Spec.Taints = taints
		if err := k8sClient.Patch(ctx, node, patch); err != nil {
			return fmt.Errorf("failed to remove taint from node %s: %w", nodeName, err)
		}
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == degradedNodeConditionType && cond.Status == corev1.ConditionTrue {
			return setDegradedCondition(ctx, k8sClient, node, corev1.ConditionFalse, degradedRecoveredReason, "")
		}
	}
	return nil
}

func hasDegradedTaint(node *corev1.Node) bool {
	for _, t := range node.Spec.Taints {
		if t.Key == spyreconst.CardManagementDegradedTaintKey {
			return true
		}
	}
	return false
}

// setDegradedCondition patches the degraded condition on the node status. Transition time is kept if status is unchanged.
func setDegradedCondition(ctx context.Context, k8sClient client.Client, node *corev1.Node,
	status corev1.ConditionStatus, reason, message string) error {
	now := metav1.Now()
	cond := corev1.NodeCondition{
		Type:               degradedNodeConditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastHeartbeatTime:  now,
		LastTransitionTime: now,
	}
	for _, existing := range node.Status.Conditions {
		if existing.Type != degradedNodeConditionType {
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
		if existing.Type != degradedNodeConditionType {
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
