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
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
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
	// Card management holds Spyre devices in both modes, so it is deleted once, and kept deleted, while
	// the device plugin and the DRA driver are being swapped, before checking for the workloads of the old mode.
	switching, state, message, err := c.prepareModeSwitch(ctx, clusterPolicy)
	if state != "" {
		return state, message, err
	}
	// SpyreNodeState is only needed for device plugin mode, not for DRA mode,
	// where prepareModeSwitch has already cleaned them up.
	if clusterPolicy.Spec.DevicePlugin.Mode != spyrev1alpha1.DevicePluginModeDRA {
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
	return c.syncCoreAndPluginState(ctx, clusterPolicy, clusterRunning(clusterPolicy, switching))
}

// clusterRunning returns true if the cluster is serving, i.e., its previous state is Ready or Degraded.
// A cluster switching between the device plugin and the DRA driver is not serving the workloads
// (card management is deleted and the driver is replaced), so it is never running, and is reported as NotReady
// instead of Degraded until the switch completes.
func clusterRunning(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, switching bool) bool {
	if switching {
		return false
	}
	return clusterPolicy.Status.State == spyrev1alpha1.Ready || clusterPolicy.Status.State == spyrev1alpha1.Degraded
}

// prepareModeSwitch handles a pending switch between the device plugin and the DRA driver, i.e., the policy
// requests one mode while the driver of the other mode is still deployed.
// The old mode's resources are its driver, and, when switching to DRA, the SpyreNodeStates.
// While the switch is pending, card management is suspended (deleted, and waited until its Pods are gone),
// and the switch is blocked if the old mode still has workloads using Spyre devices
// (including the ones holding a device reservation). When switching to DRA, the SpyreNodeStates are
// deleted once no workload is left.
// It returns whether the switch is pending, and an empty state if the sync can proceed,
// otherwise the state, the message and the error to return.
func (c *StateController) prepareModeSwitch(ctx context.Context,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) (bool, spyrev1alpha1.State, string, error) {
	toDRA := clusterPolicy.Spec.DevicePlugin.Mode == spyrev1alpha1.DevicePluginModeDRA
	switching, err := c.isModeSwitchPending(ctx, toDRA)
	if err != nil {
		return false, spyrev1alpha1.NotReady, err.Error(), err
	}
	if !switching {
		c.PluginComponentState.setSuspended(nil)
		return false, "", "", nil
	}
	c.PluginComponentState.setSuspended([]string{spyreconst.CardManagementResourceName})
	gone, messages, err := c.PluginComponentState.syncSuspended(ctx)
	if err != nil {
		return true, spyrev1alpha1.NotReady, fmt.Sprintf("%v", messages),
			fmt.Errorf("failed to suspend card management: %w", err)
	}
	if !gone {
		return true, spyrev1alpha1.NotReady,
			fmt.Sprintf("switching device plugin mode, waiting for card management: %v", messages), nil
	}
	if err := c.releaseOldMode(ctx, toDRA); err != nil {
		return true, spyrev1alpha1.NotReady, err.Error(), err
	}
	return true, "", "", nil
}

// isModeSwitchPending returns true if the driver of the old mode is still deployed, or, when switching to DRA,
// if any SpyreNodeState still exists.
func (c *StateController) isModeSwitchPending(ctx context.Context, toDRA bool) (bool, error) {
	oldDriver := spyreconst.DRADriverResourceName
	if toDRA {
		oldDriver = spyreconst.DevicePluginResourceName
	}
	if oldComponent := c.CoreComponentState.component(oldDriver); oldComponent != nil {
		remaining, err := oldComponent.remainingResources(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to check whether %s is deployed: %w", oldDriver, err)
		}
		if len(remaining) > 0 {
			return true, nil
		}
	}
	if !toDRA {
		return false, nil
	}
	// The SpyreNodeStates of the device plugin must be cleaned up, even if its driver is already gone.
	exists, err := c.SpyreNodeStatesExist(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check whether SpyreNodeState exists: %w", err)
	}
	return exists, nil
}

// releaseOldMode fails if the old mode still has workloads using Spyre devices.
// When switching to DRA, it deletes the SpyreNodeStates once no workload is left.
func (c *StateController) releaseOldMode(ctx context.Context, toDRA bool) error {
	if !toDRA {
		// The ResourceClaims of card management are released by now, the remaining ones belong to the workloads.
		if err := c.CheckActiveDRAResourceClaims(ctx); err != nil {
			return fmt.Errorf("migration to device plugin blocked: %w", err)
		}
		return nil
	}
	// None of the SpyreNodeStates may still hold an allocation or a reservation before they are deleted.
	if err := c.CheckActiveDevicePluginWorkloads(ctx); err != nil {
		return fmt.Errorf("migration to DRA blocked: %w", err)
	}
	if err := c.DeleteAllSpyreNodeStates(ctx); err != nil {
		return fmt.Errorf("failed to delete SpyreNodeState resources: %w", err)
	}
	return nil
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
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, running bool) (spyrev1alpha1.State, string, error) {
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

// Clear calls cleaning up states and cluster in the reverse order of deployment:
// plugin -> core -> init -> cluster, so that the dependents (e.g., card management)
// are gone before what they depend on (e.g., DRA driver).
func (c *StateController) Clear(ctx context.Context) error {
	var multiErr error
	if err := c.PluginComponentState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.CoreComponentState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.InitState.Clear(ctx); err != nil {
		multiErr = multierror.Append(multiErr, err)
	}
	if err := c.ClusterState.Clear(ctx); err != nil {
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
