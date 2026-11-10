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
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
// by examining SpyreNodeState allocations and reservations.
// Returns an error with details if active workloads are found.
func (s *SpyreNodeStateState) CheckActiveDevicePluginWorkloads(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("checking for active device plugin workloads")

	nodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
	if err := s.k8sClient.List(ctx, nodeStateList, []client.ListOption{}...); err != nil {
		return fmt.Errorf(spyreNodeStateListErr, err)
	}

	var activePods, reservedPods []string
	for _, nodeState := range nodeStateList.Items {
		activePods = append(activePods, s.activeAllocationPods(ctx, logger, nodeState)...)
		reservedPods = append(reservedPods, s.activeReservationPods(ctx, logger, nodeState)...)
	}

	if len(activePods) > 0 || len(reservedPods) > 0 {
		return fmt.Errorf("cannot enable DRA driver: %d active device plugin workload(s) still running: %s, "+
			"%d pod(s) holding a device reservation: %s. "+
			"Please terminate all pods using or reserving Spyre devices via device plugin before enabling DRA",
			len(activePods), strings.Join(activePods, ", "),
			len(reservedPods), strings.Join(reservedPods, ", "))
	}

	return nil
}

// activeAllocationPods returns the Pods ("namespace/name") of the allocations of the SpyreNodeState which are still active.
func (s *SpyreNodeStateState) activeAllocationPods(ctx context.Context, logger logr.Logger,
	nodeState spyrev1alpha1.SpyreNodeState) []string {
	var pods []string
	for _, allocation := range nodeState.Status.AllocationList {
		isActive, err := s.allocationIsActive(ctx, logger, allocation)
		if err != nil {
			// Fail closed: if we cannot tell whether the Pod is still using the
			// devices, assume it is, so that DRA is not enabled underneath it.
			logger.Info("failed to check allocation, treating it as active",
				"namespace", allocation.Pod.Namespace, "name", allocation.Pod.Name, "error", err)
			isActive = true
		}
		if isActive {
			pods = append(pods, fmt.Sprintf("%s/%s", allocation.Pod.Namespace, allocation.Pod.Name))
		}
	}
	return pods
}

// activeReservationPods returns the Pods ("namespace/name") holding a device reservation of the SpyreNodeState
// which are still active, or a placeholder for each reservation of an unknown owner.
func (s *SpyreNodeStateState) activeReservationPods(ctx context.Context, logger logr.Logger,
	nodeState spyrev1alpha1.SpyreNodeState) []string {
	var pods []string
	// Reconcile the reservations written by an older version before inspecting them.
	nodeState.Status.NormalizeReservations()
	for _, entries := range nodeState.Status.ReservationEntries() {
		for _, entry := range entries {
			pod := entry.Pod
			if pod.Name == "" && pod.Namespace == "" {
				// Fail closed: the devices are reserved by an unknown owner (adopted from an older writer).
				pods = append(pods, fmt.Sprintf("<unknown pod on %s>", nodeState.Name))
				continue
			}
			isActive, err := s.podIsActive(ctx, logger, pod)
			if err != nil {
				// Fail closed, the same as for the allocations.
				logger.Info("failed to check reservation, treating it as active",
					"namespace", pod.Namespace, "name", pod.Name, "error", err)
				isActive = true
			}
			if isActive {
				pods = append(pods, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			}
		}
	}
	return pods
}

// CheckActiveDRAResourceClaims returns an error if any ResourceClaim still has a Spyre device allocated by the DRA driver.
// It must pass before switching from the DRA driver to the device plugin, otherwise the devices would be
// handed out by the device plugin while they are still used through the claims.
func (s *SpyreNodeStateState) CheckActiveDRAResourceClaims(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.V(1).Info("checking for allocated Spyre ResourceClaims")

	claimList := &resourcev1.ResourceClaimList{}
	if err := s.k8sClient.List(ctx, claimList); err != nil {
		if meta.IsNoMatchError(err) {
			// the DRA API is not served, so there cannot be any claim
			return nil
		}
		// Fail closed: if the claims cannot be listed, assume some are allocated.
		return fmt.Errorf("failed to list ResourceClaims: %w", err)
	}

	var activeClaims []string
	for _, claim := range claimList.Items {
		if claim.Status.Allocation == nil {
			continue
		}
		for _, result := range claim.Status.Allocation.Devices.Results {
			if result.Driver == spyreconst.DRADriverName {
				activeClaims = append(activeClaims, fmt.Sprintf("%s/%s", claim.Namespace, claim.Name))
				break
			}
		}
	}

	if len(activeClaims) > 0 {
		return fmt.Errorf("cannot disable DRA driver: %d ResourceClaim(s) still have Spyre devices allocated: %s. "+
			"Please delete the workloads and ResourceClaims using Spyre devices via DRA before switching to device plugin",
			len(activeClaims), strings.Join(activeClaims, ", "))
	}
	return nil
}

// allocationIsActive returns true if the allocation has devices and a pod reference that exists
func (s *SpyreNodeStateState) allocationIsActive(ctx context.Context, logger logr.Logger,
	allocation spyrev1alpha1.Allocation) (bool, error) {
	// Check if allocation has devices and a pod reference
	if len(allocation.DeviceList) > 0 && allocation.Pod != nil {
		return s.podIsActive(ctx, logger, *allocation.Pod)
	}
	return false, nil
}

// podIsActive returns true if the Pod referenced by an allocation or a reservation still exists.
func (s *SpyreNodeStateState) podIsActive(ctx context.Context, logger logr.Logger,
	ref spyrev1alpha1.Pod) (bool, error) {
	podName := ref.Name
	podNamespace := ref.Namespace
	if podName == "" || podNamespace == "" {
		return false, nil
	}

	// Verify the pod still exists
	pod := &corev1.Pod{}
	key := client.ObjectKey{Namespace: podNamespace, Name: podName}
	err := s.k8sClient.Get(ctx, key, pod)
	switch {
	case err == nil:
		// This is the only path that returns true: the Pod exists and its
		// UID matches the reference, or the reference has no UID recorded
		// (written by an older version; accepted for backward compatibility).
		if pod.UID == ref.UID || ref.UID == "" {
			return true, nil
		}
		logger.V(1).Info("ignore stale reference",
			"pod", fmt.Sprintf("%s/%s", podNamespace, podName),
			"referencedUID", ref.UID, "currentUID", pod.UID)
	case apierrors.IsNotFound(err):
		logger.V(1).Info("referenced Pod no longer exists",
			"pod", fmt.Sprintf("%s/%s", podNamespace, podName))
	default:
		return false, fmt.Errorf("failed to get pod %s/%s: %w", podNamespace, podName, err)
	}
	return false, nil
}

// SpyreNodeStatesExist returns true if any SpyreNodeState resource exists in the cluster.
func (s *SpyreNodeStateState) SpyreNodeStatesExist(ctx context.Context) (bool, error) {
	nodeStateList := &spyrev1alpha1.SpyreNodeStateList{}
	if err := s.k8sClient.List(ctx, nodeStateList, client.Limit(1)); err != nil {
		return false, fmt.Errorf(spyreNodeStateListErr, err)
	}
	return len(nodeStateList.Items) > 0, nil
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
