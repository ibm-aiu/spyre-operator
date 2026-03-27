/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"github.com/ibm-aiu/spyre-operator/internal/labeler"
)

// NodeLabelerReconciler runs the Spyre node labeler when nodes or the SpyreClusterPolicy
// are created or updated. Triggered by Node events (e.g. NFD labels a node after a card is
// plugged in) and by SpyreClusterPolicy events (e.g. pseudoDeviceMode is enabled in CRC/e2e).
// Labeling is decoupled from SpyreClusterPolicy reconcile: operator sets ibm.com/spyre.present
// based on NFD detection or pseudoDeviceMode only.
type NodeLabelerReconciler struct {
	client.Client
}

// Reconcile labels nodes when triggered by a Node event or a SpyreClusterPolicy spec change.
// When req.Name is set the trigger was a specific Node — only that node is fetched and updated,
// avoiding a full cluster-wide list. When req.Name is empty (policy event) all nodes are scanned
// because pseudoDeviceMode may have changed and every node potentially needs relabeling.
func (r *NodeLabelerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("node-labeler")
	pseudoMode := r.pseudoDeviceModeFromPolicy(ctx)
	var l labeler.Labeler
	if req.Name != "" {
		// Node event: update only the node that changed (cache-backed Get, not a cluster scan).
		if err := l.LabelSpyreNode(ctx, r.Client, pseudoMode, req.Name); err != nil {
			logger.Error(err, "failed to label node", "node", req.Name)
			return ctrl.Result{}, fmt.Errorf("failed to label node %s: %w", req.Name, err)
		}
	} else {
		// Policy event: pseudoDeviceMode may have changed — scan every node.
		if _, _, err := l.LabelSpyreNodes(ctx, r.Client, pseudoMode); err != nil {
			logger.Error(err, "node labeler failed")
			return ctrl.Result{}, fmt.Errorf("node labeler failed: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *NodeLabelerReconciler) pseudoDeviceModeFromPolicy(ctx context.Context) bool {
	list := &spyrev1alpha1.SpyreClusterPolicyList{}
	if err := r.Client.List(ctx, list); err != nil {
		log.FromContext(ctx).WithName("node-labeler").Error(err, "failed to list SpyreClusterPolicies; defaulting pseudoMode=false")
		return false
	}
	for i := range list.Items {
		if list.Items[i].Spec.ExperimentalModeEnabled(spyrev1alpha1.PseudoDeviceMode) {
			return true
		}
	}
	return false
}

// SetupWithManager registers the node labeler controller with the manager.
// It watches Nodes (NFD card detection / hotplug) and SpyreClusterPolicy (e.g. pseudoDeviceMode
// toggled in CRC/e2e) so that ibm.com/spyre.present is applied whenever either changes.
// The labeler is idempotent: it is a no-op when node labels are already correct.
func (r *NodeLabelerReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	c, err := controller.New("node-labeler", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed to create node-labeler controller: %w", err)
	}
	// Pass the node name so Reconcile can do a targeted Get instead of listing all nodes.
	nodeMapFn := func(_ context.Context, n *corev1.Node) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: n.GetName()}}}
	}
	// TypedLabelChangedPredicate: fires on create and when node labels change (e.g. NFD adds/removes PCI label).
	// Filters out noise from node status heartbeats and annotation-only updates.
	if err := c.Watch(
		source.TypedKind(mgr.GetCache(), &corev1.Node{},
			handler.TypedEnqueueRequestsFromMapFunc(nodeMapFn),
			predicate.TypedLabelChangedPredicate[*corev1.Node]{})); err != nil {
		return fmt.Errorf("failed to add node watch: %w", err)
	}
	// Policy events collapse to a single empty request so Reconcile scans all nodes.
	// Only spec changes (generation bump) trigger relabeling; status updates and deletes are skipped.
	// Delete is excluded because the SpyreClusterPolicy finalizer already calls RemoveSpyreNodesLabels.
	policyMapFn := func(context.Context, *spyrev1alpha1.SpyreClusterPolicy) []reconcile.Request {
		return []reconcile.Request{{}}
	}
	policyPredicate := predicate.TypedFuncs[*spyrev1alpha1.SpyreClusterPolicy]{
		// CreateFunc: trigger on first apply or controller restart when pseudoDeviceMode is already set.
		CreateFunc: func(event.TypedCreateEvent[*spyrev1alpha1.SpyreClusterPolicy]) bool {
			return true
		},
		// UpdateFunc: only pseudoDeviceMode affects ibm.com/spyre.present; ignore all other spec changes.
		UpdateFunc: func(e event.TypedUpdateEvent[*spyrev1alpha1.SpyreClusterPolicy]) bool {
			oldPseudo := e.ObjectOld.Spec.ExperimentalModeEnabled(spyrev1alpha1.PseudoDeviceMode)
			newPseudo := e.ObjectNew.Spec.ExperimentalModeEnabled(spyrev1alpha1.PseudoDeviceMode)
			return oldPseudo != newPseudo
		},
		// DeleteFunc: skip — the SpyreClusterPolicy finalizer calls RemoveSpyreNodesLabels.
		DeleteFunc: func(event.TypedDeleteEvent[*spyrev1alpha1.SpyreClusterPolicy]) bool {
			return false
		},
	}
	if err := c.Watch(
		source.TypedKind(mgr.GetCache(), &spyrev1alpha1.SpyreClusterPolicy{},
			handler.TypedEnqueueRequestsFromMapFunc(policyMapFn),
			policyPredicate)); err != nil {
		return fmt.Errorf("failed to add SpyreClusterPolicy watch: %w", err)
	}
	return nil
}
