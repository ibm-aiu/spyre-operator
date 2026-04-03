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
	"path/filepath"
	"slices"

	"github.com/hashicorp/go-multierror"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StateController is composed of the following states, synced orderly:
// ClusterState -> InitState -> SpyreNodeState -> CoreComponentState -> PluginComponentState.
type StateController struct {
	*ClusterState
	*SpyreNodeStateState
	InitState            *DeploymentState
	CoreComponentState   *DeploymentState
	PluginComponentState *DeploymentState
}

var (
	supportedArchitectures = []string{"amd64", "ppc64le", "s390x"}
)

// NewStateController initializes StateController with ClusterState, SpyreNodeState, and DeploymentState
func NewStateController(ctx context.Context, cfg *rest.Config, scheme *runtime.Scheme) (*StateController, error) {
	if err := InitializeScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to init scheme: %w", err)
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to init client: %w", err)
	}
	clusterState, err := NewClusterState(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to init ClusterState: %w", err)
	}
	spyreNodeState := NewSpyreNodeStateState(k8sClient, scheme)

	// The OPERATOR_ASSETS_PATH environment variable needs to be set only
	// when debugging or running the executable on the local workstation
	// and should be set to this value: ${WORKSPACE_ROOT}/assets
	assetsPath := os.Getenv("OPERATOR_ASSETS_PATH")
	if assetsPath == "" {
		assetsPath = "/opt/spyre-operator"
	}
	initStatePath := filepath.Join(assetsPath, "state-init")
	coreComponentStatePath := filepath.Join(assetsPath, "state-core-components")
	pluginComponentStatePath := filepath.Join(assetsPath, "state-plugin-components")
	initState, err := NewDeploymentState(ctx, k8sClient, scheme, initStatePath, clusterState.OperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to init deployment state for init components: %w", err)
	}
	coreComponentsState, err := NewDeploymentState(ctx, k8sClient, scheme,
		coreComponentStatePath, clusterState.OperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to init deployment state for core components: %w", err)
	}
	pluginComponentState, err := NewDeploymentState(ctx, k8sClient, scheme,
		pluginComponentStatePath, clusterState.OperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to init deployment state for plugin components: %w", err)
	}
	return &StateController{
		ClusterState:         clusterState,
		SpyreNodeStateState:  spyreNodeState,
		InitState:            initState,
		CoreComponentState:   coreComponentsState,
		PluginComponentState: pluginComponentState,
	}, nil
}

// Sync synchronizes ClusterState, SpyreNodeStateState, and DeploymentState based on SpyreClusterPolicy
// returns overallStatus, message, err
func (c *StateController) Sync(ctx context.Context,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) (spyrev1alpha1.State, string, error) {
	logger := log.FromContext(ctx)
	logger.Info(fmt.Sprintf("synchronizing %s", clusterPolicy.Name))
	if err := c.ClusterState.Sync(ctx, clusterPolicy); err != nil {
		message := fmt.Sprintf("failed to sync cluster state: %v", err)
		return spyrev1alpha1.NotReady, message, errors.New(message)
	}
	initSuccess, initMessage, err := c.InitState.TransformAndSync(ctx, clusterPolicy, c.ClusterState)
	if err != nil {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", initMessage), fmt.Errorf("failed to sync init components: %w", err)
	}
	if !initSuccess {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", initMessage), nil
	}
	if !c.ClusterState.hasNFD {
		return spyrev1alpha1.NoNFD, "no node feature discovery labels", nil
	}
	if c.ClusterState.nodeArchitecture == "" {
		return spyrev1alpha1.NoSpyreNodes, "no Spyre nodes found", nil
	}
	if err := c.SpyreNodeStateState.UpdateSpyreNodeStates(ctx, clusterPolicy); err != nil {
		message := fmt.Sprintf("failed to update SpyreNodeState: %v", err)
		return spyrev1alpha1.NotReady, message, errors.New(message)
	}
	if !c.ClusterState.PseudoDeviceMode.Load() {
		if !slices.Contains(supportedArchitectures, c.ClusterState.nodeArchitecture) {
			message := fmt.Sprintf("%s unsupported. supported architectures: %v", c.ClusterState.nodeArchitecture, supportedArchitectures) //nolint:lll
			return spyrev1alpha1.NoSpyreNodes, message, errors.New(message)
		}
	}
	coreSuccess, coreMessages, err := c.CoreComponentState.TransformAndSync(ctx, clusterPolicy, c.ClusterState)
	if err != nil {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", coreMessages), fmt.Errorf("failed to sync core components: %w", err)
	}
	optionalSuccess, optionalMessages, err := c.PluginComponentState.TransformAndSync(ctx, clusterPolicy, c.ClusterState)
	if err != nil {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", optionalMessages), fmt.Errorf("failed to sync plugin components: %w", err)
	}
	if coreSuccess && optionalSuccess {
		return spyrev1alpha1.Ready, "", nil
	}
	return spyrev1alpha1.NotReady, fmt.Sprintf("%v", append(coreMessages, optionalMessages...)), nil
}

// Clear calls cleaning up cluster and states
func (c *StateController) Clear(ctx context.Context) error {
	var multiErr error
	if err := c.ClusterState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.InitState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.CoreComponentState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.PluginComponentState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if multiErr != nil {
		return fmt.Errorf("failed to clear cluster or some states: %w", multiErr)
	}
	return nil
}

// Remove assets which owner (SpyreClusterPolicy) not found
// return sum of deleted objects from all states
func (c *StateController) RemoveZombieAssets(ctx context.Context) int {
	sum := 0
	sum += c.PluginComponentState.RemoveZombieAssets(ctx)
	sum += c.CoreComponentState.RemoveZombieAssets(ctx)
	sum += c.InitState.RemoveZombieAssets(ctx)
	return sum
}
