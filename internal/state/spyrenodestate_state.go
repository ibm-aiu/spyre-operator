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

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SpyreNodeStateState manages SpyreNodeState resource
type SpyreNodeStateState struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
}

func NewSpyreNodeStateState(k8sClient client.Client, scheme *runtime.Scheme) *SpyreNodeStateState {
	return &SpyreNodeStateState{
		k8sClient: k8sClient,
		scheme:    scheme,
	}
}

func (s *SpyreNodeStateState) UpdateSpyreNodeStates(ctx context.Context, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("getting NodeList of the cluster and building a node map")
	nodeMap, nodeList, err := s.getNodeMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to get node map: %w", err)
	}
	logger.V(1).Info("getting SpyreNodeState resources")
	nodeStateMap, nodeStateList, err := s.getNodeStateMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to list SpyreNodeState: %w", err)
	}

	logger.V(1).Info("synchronizing Node and SpyreNodeState")
	for _, nodeState := range nodeStateList.Items {
		err := s.deleteUnboundSpyreNodeState(ctx, nodeMap, nodeStateMap, nodeState)
		if err != nil {
			return fmt.Errorf("failed to synchronize SpyreNodeState: %w", err)
		}
	}
	for _, node := range nodeList.Items {
		err := s.createSpyreNodeState(ctx, nodeStateMap, node, clusterPolicy)
		if err != nil {
			return fmt.Errorf("failed to create SpyreNodeState for node '%s': %w", node.Name, err)
		}
	}
	return nil
}

func (s *SpyreNodeStateState) getNodeMap(ctx context.Context) (map[string]corev1.Node, *corev1.NodeList, error) {
	nodeList := &corev1.NodeList{}
	nodeMap := make(map[string]corev1.Node)
	if err := s.k8sClient.List(ctx, nodeList); err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	for _, node := range nodeList.Items {
		nodeMap[node.Name] = node
	}
	return nodeMap, nodeList, nil
}

func (s *SpyreNodeStateState) getNodeStateMap(ctx context.Context) (map[string]spyrev1alpha1.SpyreNodeState, *spyrev1alpha1.SpyreNodeStateList, error) { //nolint:lll
	nodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
	nodeStateMap := make(map[string]spyrev1alpha1.SpyreNodeState)
	if err := s.k8sClient.List(ctx, nodeStateList, []client.ListOption{}...); err != nil {
		return nil, nil, fmt.Errorf("failed to list SpyreNodeState: %w", err)
	}
	return nodeStateMap, nodeStateList, nil
}

// deleteUnboundSpyreNodeState deletes an `SpyreNodeState` resource which is not bound with
// existing `Node` resource, and reflects the changes to `nodeStateMap` specified in
// the argument.
func (s *SpyreNodeStateState) deleteUnboundSpyreNodeState(ctx context.Context, nodeMap map[string]corev1.Node,
	nodeStateMap map[string]spyrev1alpha1.SpyreNodeState, nodeState spyrev1alpha1.SpyreNodeState) error {
	logger := log.FromContext(ctx)
	if _, ok := nodeMap[nodeState.Name]; ok {
		nodeStateMap[nodeState.Name] = nodeState
	} else {
		logger.Info(fmt.Sprintf("deleting SpyreNodeState resource for removed node: %v", nodeState.Name))
		if err := s.k8sClient.Delete(ctx, &nodeState); err != nil {
			logger.Error(err, "failed to delete SpyreNodeState")
			return fmt.Errorf("failed to delete SpyreNodeState: %w", err)
		}
	}
	return nil
}

// createSpyreNodeState creates an `SpyreNodeState` resource for a certain `Node` if missing.
// In addition, it adds an `ownerReference` element if missing so that
// it can be deleted when the `SpyreClusterPolicy` is deleted.
// If the ownerReference is missing, this function will add and update the existing one.
// The result reflects the changes to `nodeStateMap` specified in the argument.
func (s *SpyreNodeStateState) createSpyreNodeState(ctx context.Context,
	nodeStateMap map[string]spyrev1alpha1.SpyreNodeState, node corev1.Node, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	logger := log.FromContext(ctx)
	if nodeState, ok := nodeStateMap[node.Name]; !ok {
		logger.Info(fmt.Sprintf("creating SpyreNodeState resource for new node: %s", node.Name))
		nodeState := &spyrev1alpha1.SpyreNodeState{
			ObjectMeta: metav1.ObjectMeta{
				Name: node.Name,
			},
			Spec: spyrev1alpha1.SpyreNodeStateSpec{
				NodeName: node.Name,
			},
		}
		if err := controllerutil.SetControllerReference(clusterPolicy, nodeState, s.scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for SpyreNodeState: %w", err)
		}
		if err := s.k8sClient.Create(ctx, nodeState); err != nil {
			return fmt.Errorf("failed to create SpyreNodeState: %w", err)
		}
		nodeStateMap[node.Name] = *nodeState
	} else if len(nodeState.ObjectMeta.OwnerReferences) == 0 {
		if err := controllerutil.SetControllerReference(clusterPolicy, &nodeState, s.scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for SpyreNodeState: %w", err)
		}
		if err := s.k8sClient.Update(ctx, &nodeState); err != nil {
			return fmt.Errorf("failed to update SpyreNodeState: %w", err)
		}
	}
	return nil
}
