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
	supportedArchitectures = []string{archAMD64, archPPC64LE, archS390X}
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
	// A component failing on a node taints the node to hold the components of the later states.
	// The components of the same and earlier states tolerate the taint to keep running.
	states := []*DeploymentState{initState, coreComponentsState, pluginComponentState}
	for i, state := range states {
		var tolerated []string
		for _, later := range states[i:] {
			tolerated = append(tolerated, later.componentNames()...)
		}
		state.setHoldTolerated(tolerated)
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
	initialReady, initialMessage, err := c.syncInitialState(ctx, clusterPolicy)
	// Check if initial state is ready, if not, return immediately
	if initialReady != spyrev1alpha1.Ready {
		return initialReady, initialMessage, err
	}
	// SpyreNodeState is only needed for device plugin mode, not for DRA mode
	if clusterPolicy.Spec.DevicePlugin.Mode == spyrev1alpha1.DevicePluginModeDRA {
		// When DRA is enabled, check for active device plugin workloads
		if err := c.CheckActiveDevicePluginWorkloads(ctx); err != nil {
			message := fmt.Sprintf("migration to DRA blocked: %v", err)
			return spyrev1alpha1.NotReady, message, errors.New(message)
		}
		// All device plugin workloads are gone, safe to delete SpyreNodeState resources
		if err := c.DeleteAllSpyreNodeStates(ctx); err != nil {
			message := fmt.Sprintf("failed to delete SpyreNodeState resources: %v", err)
			return spyrev1alpha1.NotReady, message, errors.New(message)
		}
	} else {
		// Device plugin mode: update SpyreNodeState as usual
		if err := c.UpdateSpyreNodeStates(ctx, clusterPolicy); err != nil {
			message := fmt.Sprintf("failed to update SpyreNodeState: %v", err)
			return spyrev1alpha1.NotReady, message, errors.New(message)
		}
	}
	if !c.PseudoDeviceMode.Load() {
		if !slices.Contains(supportedArchitectures, c.nodeArchitecture) {
			message := fmt.Sprintf("%s unsupported. supported architectures: %v", c.nodeArchitecture, supportedArchitectures) //nolint:lll
			return spyrev1alpha1.NoSpyreNodes, message, errors.New(message)
		}
	}
	return c.syncCoreAndPluginState(ctx, clusterPolicy)
}

// syncInitialState syncs the initial state of the cluster.
func (c *StateController) syncInitialState(ctx context.Context,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) (spyrev1alpha1.State, string, error) {
	initSuccess, initMessage, err := c.InitState.TransformAndSync(ctx, clusterPolicy, c.ClusterState)
	if err != nil {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", initMessage), fmt.Errorf("failed to sync init components: %w", err)
	}
	if !initSuccess {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", initMessage), nil
	}
	if !c.hasNFD {
		return spyrev1alpha1.NoNFD, "no node feature discovery labels", nil
	}
	if c.nodeArchitecture == "" {
		return spyrev1alpha1.NoSpyreNodes, "no Spyre nodes found", nil
	}
	return spyrev1alpha1.Ready, "", nil
}

// syncCoreAndPluginState syncs the core and plugin components of the cluster.
func (c *StateController) syncCoreAndPluginState(ctx context.Context,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) (spyrev1alpha1.State, string, error) {
	coreSuccess, coreMessages, err := c.CoreComponentState.TransformAndSync(ctx, clusterPolicy, c.ClusterState)
	if err != nil {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", coreMessages), fmt.Errorf("failed to sync core components: %w", err)
	}
	// A cluster whose previous state is Ready (or Degraded) keeps syncing the plugin components even if a component
	// is not ready, so that the existing components keep running. The cluster is then reported as Degraded
	// instead of NotReady, e.g., a core component is failing on some nodes (held by the taint), or a DaemonSet
	// is initializing for a new node.
	// Otherwise (e.g., the first deployment), the plugin components wait for the core components, the same as
	// the core components wait for the init ones, so that they are not deployed on the node of a failing component,
	// and the cluster is NotReady until it becomes Ready.
	running := clusterPolicy.Status.State == spyrev1alpha1.Ready || clusterPolicy.Status.State == spyrev1alpha1.Degraded
	if !coreSuccess && !running {
		state, message := overallState(false, false, false, coreMessages, c.heldNodes(), nil)
		return state, message, nil
	}
	optionalSuccess, optionalMessages, err := c.PluginComponentState.TransformAndSync(ctx, clusterPolicy, c.ClusterState)
	if err != nil {
		return spyrev1alpha1.NotReady, fmt.Sprintf("%v", optionalMessages), fmt.Errorf("failed to sync plugin components: %w", err)
	}
	degradedNodes := slices.Concat(c.CoreComponentState.DegradedNodes(), c.PluginComponentState.DegradedNodes())
	state, message := overallState(running, coreSuccess, optionalSuccess,
		slices.Concat(coreMessages, optionalMessages), c.heldNodes(), degradedNodes)
	return state, message, nil
}

// overallState decides the state and the message of the core and plugin components.
// running is true if the previous state of the cluster is Ready or Degraded. A running cluster is Degraded,
// not NotReady, when some components are not ready, so that the components which are running keep serving.
// heldNodes are the nodes held by the taint of a failing component, either confirmed as degraded or not yet,
// and degradedNodes are the nodes confirmed as degraded.
func overallState(running, coreSuccess, optionalSuccess bool, messages []string,
	heldNodes, degradedNodes []DegradedNode) (spyrev1alpha1.State, string) {
	if coreSuccess && optionalSuccess {
		if len(degradedNodes) > 0 {
			// healthy nodes keep serving while degraded nodes are tainted and reported.
			return spyrev1alpha1.Degraded, DegradedMessage(degradedNodes)
		}
		return spyrev1alpha1.Ready, ""
	}
	messages = slices.Clone(messages)
	if len(heldNodes) > 0 {
		messages = append(messages, DegradedMessage(heldNodes))
	}
	state := spyrev1alpha1.NotReady
	if running {
		state = spyrev1alpha1.Degraded
	}
	return state, fmt.Sprintf("%v", messages)
}

// heldNodes returns the nodes held by the taint of a failing component in the core and plugin states,
// either confirmed as degraded or not yet.
func (c *StateController) heldNodes() []DegradedNode {
	return slices.Concat(
		c.CoreComponentState.DegradedNodes(), c.PluginComponentState.DegradedNodes(),
		c.CoreComponentState.FailingNodes(), c.PluginComponentState.FailingNodes())
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
