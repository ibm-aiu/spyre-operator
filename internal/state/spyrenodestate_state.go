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
	"strings"

	"github.com/go-logr/logr"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	spyreNodeStateListErr = "failed to list SpyreNodeState: %w"
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
		return fmt.Errorf(spyreNodeStateListErr, err)
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
		return nil, nil, fmt.Errorf(spyreNodeStateListErr, err)
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
	} else if len(nodeState.OwnerReferences) == 0 {
		if err := controllerutil.SetControllerReference(clusterPolicy, &nodeState, s.scheme); err != nil {
			return fmt.Errorf("failed to set controller reference for SpyreNodeState: %w", err)
		}
		if err := s.k8sClient.Update(ctx, &nodeState); err != nil {
			return fmt.Errorf("failed to update SpyreNodeState: %w", err)
		}
	}
	return nil
}

// CheckActiveDevicePluginWorkloads checks if there are any active device plugin workloads
// by examining SpyreNodeState allocations. Returns an error with details if active workloads are found.
func (s *SpyreNodeStateState) CheckActiveDevicePluginWorkloads(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("checking for active device plugin workloads")

	nodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
	if err := s.k8sClient.List(ctx, nodeStateList, []client.ListOption{}...); err != nil {
		return fmt.Errorf(spyreNodeStateListErr, err)
	}

	var activePods []string
	for _, nodeState := range nodeStateList.Items {
		for _, allocation := range nodeState.Status.AllocationList {
			if s.isActivePod(ctx, logger, allocation) {
				podName := allocation.Pod.Name
				podNamespace := allocation.Pod.Namespace
				activePods = append(activePods, fmt.Sprintf("%s/%s", podNamespace, podName))
			}
		}
	}

	if len(activePods) > 0 {
		return fmt.Errorf("cannot enable DRA driver: %d active device plugin workload(s) still running: %s. "+
			"Please terminate all pods using Spyre devices via device plugin before enabling DRA",
			len(activePods), strings.Join(activePods, ", "))
	}

	return nil
}

// isActivePod returns true if the allocation has devices and a pod reference that exists
func (s *SpyreNodeStateState) isActivePod(ctx context.Context, logger logr.Logger,
	allocation spyrev1alpha1.Allocation) bool {
	// Check if allocation has devices and a pod reference
	if len(allocation.DeviceList) > 0 && allocation.Pod != nil {
		podName := allocation.Pod.Name
		podNamespace := allocation.Pod.Namespace
		if podName == "" || podNamespace == "" {
			return false
		}

		// Verify the pod still exists
		pod := &corev1.Pod{}
		key := client.ObjectKey{Namespace: podNamespace, Name: podName}
		err := s.k8sClient.Get(ctx, key, pod)
		if err == nil {
			// A Pod that merely reuses the recorded name is a different Pod, and
			// the allocation it belongs to is stale rather than active.
			if allocation.Pod.UID != "" && pod.UID != allocation.Pod.UID {
				logger.V(1).Info("allocation refers to an earlier generation of the pod",
					"pod", fmt.Sprintf("%s/%s", podNamespace, podName),
					"allocatedUID", allocation.Pod.UID, "currentUID", pod.UID)
				return false
			}
			return true
		} else if !apierrors.IsNotFound(err) {
			// Error other than not found - log but continue checking
			logger.V(1).Info("error checking pod existence", "pod", fmt.Sprintf("%s/%s", podNamespace, podName), "error", err)
		}
	}
	return false
}

// DeleteAllSpyreNodeStates deletes all SpyreNodeState resources in the cluster.
// This should only be called when DRA is enabled and no active device plugin workloads exist.
func (s *SpyreNodeStateState) DeleteAllSpyreNodeStates(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("deleting all SpyreNodeState resources for DRA mode")

	nodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
	if err := s.k8sClient.List(ctx, nodeStateList, []client.ListOption{}...); err != nil {
		return fmt.Errorf(spyreNodeStateListErr, err)
	}

	for _, nodeState := range nodeStateList.Items {
		logger.V(1).Info("deleting SpyreNodeState", "name", nodeState.Name)
		if err := s.k8sClient.Delete(ctx, &nodeState); err != nil {
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete SpyreNodeState %s: %w", nodeState.Name, err)
			}
			// Already deleted, continue
		}
	}

	return nil
}
