/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package controllers

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	spyrelabel "github.com/ibm-aiu/spyre-operator/internal/labeler"
	spyrestate "github.com/ibm-aiu/spyre-operator/internal/state"
	spyreerr "github.com/ibm-aiu/spyre-operator/pkg/errors"
)

const (
	staticRequeueAfter          = time.Second * 5
	SpyreClusterPolicyFinalizer = "finalizers.spyreclusterpolicies.spyre.ibm.com"
	nfdOSTreeVersionLabelKey    = "feature.node.kubernetes.io/system-os_release.OSTREE_VERSION"
)

var _ reconcile.Reconciler = &SpyreClusterPolicyReconciler{}

// SpyreClusterPolicyReconciler reconciles a SpyreClusterPolicyobject
type SpyreClusterPolicyReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ExporterPort         string
	ExporterPollingInSec string
	stateController      *spyrestate.StateController
}

// +kubebuilder:rbac:groups=spyre.ibm.com,resources=spyrenodestates;spyrenodestates/status;spyrenodestates/finalizers;spyreclusterpolicies;spyreclusterpolicies/status;spyreclusterpolicies/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=proxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces;serviceaccounts;pods;pods/log;services;services/finalizers;endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumes;persistentvolumeclaims;events;configmaps;secrets;nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;replicasets;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrule,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=image.openshift.io,resources=imagestreams,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=privileged;spyre-operator-nonroot,verbs=use
// +kubebuilder:rbac:groups=nfd.openshift.io,resources=nodefeaturerules,verbs=get;list;watch;create;delete;update;patch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=volumeattachments,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *SpyreClusterPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("starting SpyreClusterPolicy reconciler", "req", req.NamespacedName)

	// Fetch the SpyreClusterPolicyinstance
	instance := &spyrev1alpha1.SpyreClusterPolicy{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if apierrors.IsNotFound(err) {
		// Request object not found, could have been deleted after reconcile request.
		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
		// Return and don't requeue
		logger.Info("reconciliation finished without doing anything: SpyreClusterPolicy resource was not found")
		return ctrl.Result{}, nil
	}
	if err != nil {
		// Error reading the object - requeue the request.
		logger.Error(err, "reconciliation failed: unable to get SpyreClusterPolicy")
		return ctrl.Result{Requeue: true}, fmt.Errorf("error getting cluster policy: %w", err)
	}

	// Add finalizer to instance
	if !controllerutil.ContainsFinalizer(instance, SpyreClusterPolicyFinalizer) {
		controllerutil.AddFinalizer(instance, SpyreClusterPolicyFinalizer)
		err = r.Update(ctx, instance)
		if err != nil {
			return ctrl.Result{Requeue: true}, fmt.Errorf("failed to update cluster policy: %w", err)
		}
	}

	// Remove finalizer on delete (must be done before any other returns)
	isDeleted := instance.GetDeletionTimestamp() != nil
	if isDeleted {
		if controllerutil.ContainsFinalizer(instance, SpyreClusterPolicyFinalizer) {
			controllerutil.RemoveFinalizer(instance, SpyreClusterPolicyFinalizer)
			err := r.Client.Update(ctx, instance)
			if err != nil {
				return ctrl.Result{Requeue: true}, fmt.Errorf("failed to update cluster policy finalizers: %w", err)
			}
			// WARNING: expect only single spyreclusterpolicy in the cluster
			// This line will be called only once and not blocking the removal of finalizer.
			err = r.stateController.Clear(ctx)
			if err != nil {
				logger.Info(fmt.Sprintf("something is wrong during state controller's cleanup: %v", err))
			}
		}
		return ctrl.Result{}, nil
	}

	overallStatus, message, err := r.stateController.Sync(ctx, instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to synchronize state based on SpyreClusterPolicy: %w", err)
	}
	result := processOverallStatus(ctx, overallStatus, message)

	// status changes
	if instance.Status.Message != message || instance.Status.State != overallStatus {
		logger.Info(fmt.Sprintf("updating SpyreClusterPolicy's status to %s", overallStatus))
		if err := r.updateCRState(ctx, req.NamespacedName, overallStatus, message); err != nil {
			// failed to update, requeue if processed result has no requeue
			spyreerr.LogErrUpdate(logger, err)
			noRequeueResult := !result.Requeue && result.RequeueAfter == 0
			if noRequeueResult {
				return ctrl.Result{Requeue: true}, nil
			}
		}
	}
	return result, nil
}

func (r *SpyreClusterPolicyReconciler) addWatchNewSpyreNode(ctx context.Context, c controller.Controller, mgr manager.Manager) error {
	logger := log.FromContext(ctx).WithName("addWatchNewSpyreNode")
	// Define a mapping from the Node object in the event to one or more
	// Spyre objects to Reconcile
	mapFn := func(ctx context.Context, a *corev1.Node) []reconcile.Request {
		// find all the Spyre devices to trigger their reconciliation
		opts := []client.ListOption{} // Namespace = "" to list across all namespaces.
		list := &spyrev1alpha1.SpyreClusterPolicyList{}

		err := mgr.GetClient().List(ctx, list, opts...)
		if err != nil {
			logger.Error(err, "unable to list SpyreClusterPolicies")
			return []reconcile.Request{}
		}

		cpToRec := []reconcile.Request{}

		for _, cp := range list.Items {
			cpToRec = append(cpToRec, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      cp.ObjectMeta.GetName(),
				Namespace: metav1.NamespaceAll,
			}})
		}
		return cpToRec
	}
	p := predicate.TypedFuncs[*corev1.Node]{
		// CreateFunc: trigger when a Node is created while PseudoDeviceMode is already set, or when
		// the controller restarts and a node already carries ibm.com/spyre.present (state refresh).
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Node]) bool {
			labels := e.Object.GetLabels()

			return r.stateController.PseudoDeviceMode.Load() || spyrelabel.HasSpyreDeviceLabels(labels)
		},
		// UpdateFunc: trigger on OS tree version change (DriverToolkit DaemonSet rebuild) and when
		// NodeLabelerReconciler sets/clears ibm.com/spyre.present so policy state stays in sync.
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
			needsUpdate, osTreeLabelChanged, spyreCommonLabelChanged := nodeUpdateNeedsReconcile(
				e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels())
			if needsUpdate {
				logger.Info("reconcile triggered: node label changed",
					"name", e.ObjectNew.GetName(),
					"osTreeLabelChanged", osTreeLabelChanged,
					"spyreCommonLabelChanged", spyreCommonLabelChanged,
				)
			}
			return needsUpdate
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Node]) bool {
			// if an RHCOS GPU node is deleted, trigger a
			// reconciliation to ensure that there is no dangling
			// OpenShift Driver-Toolkit (RHCOS version-specific)
			// DaemonSet.
			// NB: we cannot know here if the DriverToolkit is
			// enabled.
			labels := e.Object.GetLabels()
			_, hasOSTreeLabel := labels[nfdOSTreeVersionLabelKey]
			hasSpyre := r.stateController.PseudoDeviceMode.Load() || spyrelabel.HasCommonSpyreLabel(labels)
			return hasSpyre && hasOSTreeLabel
		},
	}

	err := c.Watch(
		source.TypedKind(mgr.GetCache(), &corev1.Node{}, handler.TypedEnqueueRequestsFromMapFunc(mapFn), p))
	if err != nil {
		return fmt.Errorf("failed to add watch for node: %w", err)
	}
	return nil
}

// nodeUpdateNeedsReconcile returns whether a node update event should trigger a SpyreClusterPolicy
// reconcile, along with which conditions were true. Extracted for unit testability.
func nodeUpdateNeedsReconcile(oldLabels, newLabels map[string]string) (needsUpdate, osTreeLabelChanged, spyreCommonLabelChanged bool) {
	osTreeLabelChanged = oldLabels[nfdOSTreeVersionLabelKey] != newLabels[nfdOSTreeVersionLabelKey]
	spyreCommonLabelChanged = oldLabels[spyreconst.CommonSpyreLabelKey] != newLabels[spyreconst.CommonSpyreLabelKey]
	needsUpdate = osTreeLabelChanged || spyreCommonLabelChanged
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *SpyreClusterPolicyReconciler) SetupWithManager(ctx context.Context, cfg *rest.Config, mgr ctrl.Manager) error {
	stateController, err := spyrestate.NewStateController(ctx, cfg, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to init StateController: %w", err)
	}
	r.stateController = stateController

	// Create a new controller
	c, err := controller.New("spyre-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return fmt.Errorf("failed create spyre controller: %w", err)
	}

	// Watch for changes to primary resource SpyreClusterPolicy
	err = c.Watch(source.TypedKind(mgr.GetCache(), &spyrev1alpha1.SpyreClusterPolicy{}, &handler.TypedEnqueueRequestForObject[*spyrev1alpha1.SpyreClusterPolicy]{}))
	if err != nil {
		return fmt.Errorf("failed to add watch for spyre cluster policy: %w", err)
	}

	// Watch for changes to Node labels and requeue the owner SpyreClusterPolicy
	err = r.addWatchNewSpyreNode(ctx, c, mgr)
	if err != nil {
		return fmt.Errorf("failed to add watch for node label changes: %w", err)
	}

	// Watch for changes to secondary resource Daemonsets and requeue the owner SpyreClusterPolicy
	err = c.Watch(source.TypedKind(mgr.GetCache(), &appsv1.DaemonSet{}, handler.TypedEnqueueRequestForOwner[*appsv1.DaemonSet](
		mgr.GetScheme(), mgr.GetRESTMapper(), &spyrev1alpha1.SpyreClusterPolicy{}, handler.OnlyControllerOwner())))

	if err != nil {
		return fmt.Errorf("failed to add watch for daemonset: %w", err)
	}

	// Remove zombie assets at initializing state
	r.stateController.RemoveZombieAssets(ctx)

	return nil
}

func (r *SpyreClusterPolicyReconciler) updateCRState(ctx context.Context, namespacedName types.NamespacedName, state spyrev1alpha1.State, message string) error {
	// Fetch latest instance and update state to avoid version mismatch
	instance := &spyrev1alpha1.SpyreClusterPolicy{}
	err := r.Client.Get(ctx, namespacedName, instance)
	if err != nil {
		return fmt.Errorf("failed to get cluster policy: %w", err)
	}

	// Update the CR state
	instance.SetStatus(state, r.stateController.ClusterState.OperatorNamespace, message)
	err = r.Client.Status().Update(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to update spyre cluster policy status: %w", err)
	}
	return nil
}

func processOverallStatus(ctx context.Context,
	overallStatus spyrev1alpha1.State, message string) ctrl.Result {
	logger := log.FromContext(ctx)
	logger.Info("processing overall status")
	switch overallStatus {
	case spyrev1alpha1.NotReady:
		// if any state is not ready, requeue for reconciliation
		logger.Info("requeue reconciliation: Spyre is not ready", "message", message)
		return ctrl.Result{RequeueAfter: staticRequeueAfter}
	case spyrev1alpha1.NoSpyreNodes, spyrev1alpha1.NoNFD:
		// node is not ready, do not requeue
		logger.Info("reconciliation skip: node is not ready", "message", message)
		return ctrl.Result{}
	case spyrev1alpha1.Ready:
		logger.Info("reconciliation successfully finished: Spyre is ready")
		return ctrl.Result{}
	default:
		logger.Info(fmt.Sprintf("unsupported status: %s", overallStatus))
	}
	return ctrl.Result{}
}

func (r *SpyreClusterPolicyReconciler) GetLogLevel() zapcore.Level {
	if r.stateController != nil {
		return r.stateController.ClusterState.GetLogLevel()
	}
	return zapcore.InfoLevel
}
