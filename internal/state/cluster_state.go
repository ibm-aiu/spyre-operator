/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	"github.com/ibm-aiu/spyre-operator/internal/labeler"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ocpNamespaceMonitoringLabelKey   = "openshift.io/cluster-monitoring"
	ocpNamespaceMonitoringLabelValue = "true"
	// see bundle/manifests/spyre-operator.clusterserviceversion.yaml
	//     --> ClusterServiceVersion.metadata.annotations.operatorframework.io/suggested-namespace
	ocpSuggestedNamespace = "spyre-operator"
)

// ClusterState manages state related to cluster information such as operator namespace, log level
type ClusterState struct {
	// set on init
	OperatorNamespace string
	k8sClient         client.Client
	labeler.Labeler
	// set on sync
	logLevel               zapcore.Level
	logMu                  sync.RWMutex
	PseudoDeviceMode       atomic.Bool
	hasNFD                 bool
	nodeArchitecture       string
	topologyConfigMapExist bool
	cardMgmtPvcExist       bool
}

// NewClusterState initialize ClusterState with default log level, operator namespace, and openshift version
func NewClusterState(ctx context.Context, k8sClient client.Client) (*ClusterState, error) {
	operatorNamespace := os.Getenv("OPERATOR_NAMESPACE")
	if operatorNamespace == "" {
		err := errors.New("OPERATOR_NAMESPACE environment variable not set")
		return nil, err
	}
	clusterState := &ClusterState{
		k8sClient:         k8sClient,
		logLevel:          zapcore.InfoLevel,
		OperatorNamespace: operatorNamespace,
		PseudoDeviceMode:  atomic.Bool{},
	}
	if err := ocpEnsureNamespaceMonitoring(ctx, k8sClient, operatorNamespace); err != nil {
		return nil, fmt.Errorf("failure in ocpEnsureNamespaceMonitoring: %w", err)
	}
	return clusterState, nil
}

func (s *ClusterState) Sync(ctx context.Context, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	if err := s.applyLogLevel(ctx, clusterPolicy); err != nil {
		return fmt.Errorf("failed to apply log level: %w", err)
	}
	s.PseudoDeviceMode.Store(clusterPolicy.Spec.ExperimentalModeEnabled(spyrev1alpha1.PseudoDeviceMode))
	// Node labeling (ibm.com/spyre.present) is handled by NodeLabelerReconciler on node/policy events, not on policy reconcile.
	hasNFD, nodeArchitecture, err := s.Labeler.GetClusterSpyreLabelInfo(ctx, s.k8sClient)
	if err != nil {
		return fmt.Errorf("failed to get cluster Spyre label info: %w", err)
	}
	s.hasNFD = hasNFD
	s.nodeArchitecture = nodeArchitecture
	if err = s.setTopologyConfigMapFlag(ctx, clusterPolicy); err != nil {
		return fmt.Errorf("failed to set topology configmap flag: %w", err)
	}
	if err = s.setCardMgmtPvcFlag(ctx, clusterPolicy); err != nil {
		return fmt.Errorf("failed to set card management pvc flag: %w", err)
	}
	return nil
}

func (s *ClusterState) Clear(ctx context.Context) error {
	if s.PseudoDeviceMode.Load() {
		err := s.Labeler.RemoveSpyreNodesLabels(ctx, s.k8sClient)
		if err != nil {
			return fmt.Errorf("failed to remove Spyre labels: %w", err)
		}
	}

	return nil
}

func (s *ClusterState) GetLogLevel() zapcore.Level {
	s.logMu.RLock()
	defer s.logMu.RUnlock()
	return s.logLevel
}

func (s *ClusterState) SetLogLevel(l zapcore.Level) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	s.logLevel = l
}

func (s *ClusterState) applyLogLevel(ctx context.Context, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	logger := log.FromContext(ctx)
	if clusterPolicy.Spec.LogLevel == nil {
		s.SetLogLevel(zapcore.InfoLevel)
		return nil
	}
	l, err := zapcore.ParseLevel(*clusterPolicy.Spec.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to parse specified loglevel: %s", *clusterPolicy.Spec.LogLevel)
	}
	if s.GetLogLevel() != l {
		logger.Info("changing loglevel", "loglevel.current", s.logLevel.String())
		nextLevelInt := int(l)
		nextLevelStr := l.String()
		s.SetLogLevel(l)
		logger.V(-nextLevelInt).Info("changing loglevel", "loglevel.new", nextLevelStr)
	}
	return nil
}

func (s *ClusterState) setTopologyConfigMapFlag(ctx context.Context, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	if clusterPolicy.Spec.DevicePlugin.TopologyConfigMapName == "" {
		// No topology config set. Nothing to do.
		return nil
	}
	obj := types.NamespacedName{
		Name:      clusterPolicy.Spec.DevicePlugin.TopologyConfigMapName,
		Namespace: s.OperatorNamespace,
	}
	cm := &corev1.ConfigMap{}
	err := s.k8sClient.Get(ctx, obj, cm)
	logger := log.FromContext(ctx)
	if err == nil {
		logger.Info(fmt.Sprintf("set topology config map %s found", clusterPolicy.Spec.DevicePlugin.TopologyConfigMapName))
		s.topologyConfigMapExist = true
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check topology config map: %w", err)
	}
	return nil
}

func (s *ClusterState) setCardMgmtPvcFlag(ctx context.Context, clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) error {
	logger := log.FromContext(ctx)
	if !clusterPolicy.Spec.CardManagement.Enabled {
		logger.Info("skipping cardmgmt PVC config", "cardmgmt.enabled", clusterPolicy.Spec.CardManagement.Enabled)
		return nil
	}
	obj := types.NamespacedName{
		Name:      spyreconst.CardManagementClaimName,
		Namespace: s.OperatorNamespace,
	}
	claim := &corev1.PersistentVolumeClaim{}
	err := s.k8sClient.Get(ctx, obj, claim)

	switch {
	case err == nil:
		logger.Info("cardmgmt PVC exists", "namespace", s.OperatorNamespace, "name", spyreconst.CardManagementClaimName)
		s.cardMgmtPvcExist = true
		return nil
	case apierrors.IsNotFound(err):
		logger.Info("cardmgmt PVC does not exist", "namespace", s.OperatorNamespace, "name", spyreconst.CardManagementClaimName)
		return nil
	default:
		return fmt.Errorf("failed to check pvc %s/%s: %w", s.OperatorNamespace, spyreconst.CardManagementClaimName, err)
	}
}

func ocpEnsureNamespaceMonitoring(ctx context.Context, k8sClient client.Client, operatorNamespace string) error {
	logger := log.FromContext(context.Background())

	if operatorNamespace != ocpSuggestedNamespace {
		// The Spyre Operator is not installed in the suggested
		// namespace, so the namespace may be shared with other
		// untrusted operators.  Do not enable namespace monitoring in
		// this case, as per OpenShift/Prometheus best practices.
		logger.Info("Spyre Operator not installed in the suggested namespace, skipping namespace monitoring verification",
			"namespace", operatorNamespace,
			"suggested namespace", ocpSuggestedNamespace)
		return nil
	}

	ns := &corev1.Namespace{}
	opts := client.ObjectKey{Name: operatorNamespace}
	err := k8sClient.Get(ctx, opts, ns)
	if err != nil {
		return fmt.Errorf("could not get Namespace %s from client: %w", operatorNamespace, err)
	}

	val, ok := ns.ObjectMeta.Labels[ocpNamespaceMonitoringLabelKey]
	if ok {
		// label already defined, do not change it
		var msg string
		if val == ocpNamespaceMonitoringLabelValue {
			msg = "OpenShift monitoring is enabled on operator namespace"
		} else {
			msg = "WARNING: OpenShift monitoring currently disabled on user request"
		}

		logger.V(1).Info(msg,
			"namespace", operatorNamespace,
			"label", ocpNamespaceMonitoringLabelKey,
			"value", val,
			"excepted value", ocpNamespaceMonitoringLabelValue)

		return nil
	}

	// label not defined, enable monitoring
	patch := client.MergeFrom(ns.DeepCopy())
	ns.ObjectMeta.Labels[ocpNamespaceMonitoringLabelKey] = ocpNamespaceMonitoringLabelValue
	err = k8sClient.Patch(ctx, ns, patch)
	if err != nil {
		logger.Error(err, "unable to label namespace for the Spyre Operator monitoring", "namespace", operatorNamespace)
		return fmt.Errorf("failed to label namespace for the Spyre Operator monitoring: %w", err)
	}
	return nil
}
