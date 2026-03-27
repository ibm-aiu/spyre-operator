/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
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
	"strings"

	"github.com/hashicorp/go-multierror"
	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	spyreconst "github.com/ibm-aiu/spyre-operator/const"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	componentToName = map[spyrev1alpha1.Component]string{
		spyrev1alpha1.ComponentCommonInit:      "common",
		spyrev1alpha1.ComponentDevicePlugin:    spyreconst.DevicePluginResourceName,
		spyrev1alpha1.ComponentCardManagement:  spyreconst.CardManagementResourceName,
		spyrev1alpha1.ComponentMetricsExporter: spyreconst.MonitorResourceName,
		spyrev1alpha1.ComponentScheduler:       spyreconst.SchedulerResourceName,
		spyrev1alpha1.ComponentPodValidator:    spyreconst.ValidatorResourceName,
		spyrev1alpha1.ComponentHealthChecker:   spyreconst.HealthCheckerResourceName,
	}
)

// DeploymentState comprises of a set of ControlledComponent in the same deployment state.
// The components in the same state are transformed and synced independently.
type DeploymentState struct {
	name       string
	components []*ControlledComponent
}

func NewDeploymentState(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme,
	statePath, namespace string) (*DeploymentState, error) {
	name := filepath.Base(statePath)
	componentPaths, err := os.ReadDir(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list component files in %s: %w", statePath, err)
	}
	state := &DeploymentState{
		name: name,
	}
	components := []*ControlledComponent{}
	for _, componentEntry := range componentPaths {
		componentName := componentEntry.Name()
		componentPath := filepath.Join(statePath, componentName)
		if componentName == spyreconst.CardManagementResourceName {
			if err := generatePerNodeCardMgmtManifest(ctx, k8sClient, scheme, componentPath); err != nil {
				return nil, fmt.Errorf("failed to generate cardmgmt DaemonSets: %w", err)
			}
		}
		component, err := NewControlledComponent(ctx, k8sClient, scheme, componentPath, namespace, componentName)
		if err != nil {
			return nil, fmt.Errorf("failed to init controlled component from %s, %w", componentPath, err)
		}
		components = append(components, component)
	}
	state.components = components
	return state, nil
}

func generatePerNodeCardMgmtManifest(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, componentPath string) error {
	const (
		tmplSuffix = ".tmpl"
		tempSuffix = ".tmp"
	)

	files, err := filePathWalkDir(componentPath, []string{".yaml", ".yaml.tmpl"})
	if err != nil {
		return fmt.Errorf("failed to generate per-node card-management manifests: %w", err)
	}

	// prepare node list
	nodeList := &corev1.NodeList{}
	err = k8sClient.List(ctx, nodeList, client.MatchingLabels{"node-role.kubernetes.io/worker": ""})
	if err != nil {
		return fmt.Errorf("failed to enumerate worker nodes: %w", err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file, tmplSuffix) {
			continue
		}

		// prepare template object
		runtimeObj, _, err := decodeFromFile(scheme, file)
		if err != nil {
			return fmt.Errorf("failed to decode from file %s: %w", file, err)
		}
		ds, ok := runtimeObj.(*appsv1.DaemonSet)
		if !ok {
			return fmt.Errorf("failed to assert DaemonSet object")
		}

		ext := filepath.Ext(file)             // ext must be ".tmpl"
		tmplFile := file[:len(file)-len(ext)] // tmplFile ends with ".yaml", not ".tmpl"
		ext = filepath.Ext(tmplFile)
		baseFile := file[:len(tmplFile)-len(ext)] // baseFile ends with "xxxx_daemonset", without ".yaml"
		dir := filepath.Dir(file)
		var multiErr error

		// cleanup existing daemonset files (except for ".tmpl" file)
		m, _ := filepath.Glob(baseFile + "*")
		for _, f := range m {
			if !strings.HasSuffix(f, ".tmpl") {
				os.Remove(f) // nolint: errcheck
			}
		}

		// generate manifest files per node from template file
		newFiles := make([]string, 0, len(nodeList.Items))
		dsBaseName := ds.Name
		for _, node := range nodeList.Items {
			ds.Name = dsBaseName + "-" + node.Name
			if ds.Spec.Template.Spec.NodeSelector == nil {
				ds.Spec.Template.Spec.NodeSelector = make(map[string]string, 1)
			}
			ds.Spec.Template.Spec.NodeSelector["kubernetes.io/hostname"] = node.Name
			ext := filepath.Ext(tmplFile)
			newFile := tmplFile[:len(tmplFile)-len(ext)] + "_" + node.Name + ext
			if err := encodeToFile(ds, newFile+tempSuffix); err != nil {
				multiErr = multierror.Append(multiErr, err)
			}
			newFiles = append(newFiles, newFile)
		}

		if multiErr == nil {
			// rename generated files to final names
			for _, f := range newFiles {
				if err = os.Rename(f+tempSuffix, f); err != nil {
					multiErr = multierror.Append(multiErr, err)
				}
			}
		}

		// remove temporary files if exist
		m, _ = filepath.Glob(filepath.Join(dir, "/*.tmp"))
		for _, f := range m {
			if err := os.Remove(f); err != nil {
				multiErr = multierror.Append(multiErr, err)
			}
		}

		if multiErr != nil {
			return fmt.Errorf("failed to generate per-node cardmgmt daemonset: %w", multiErr)
		}

		break
	}
	return nil
}

func (s *DeploymentState) GetName() string {
	return s.name
}

// TransformAndSync transforms each components in state and syncs the cluster
// return success, error messages, and overall error
func (s *DeploymentState) TransformAndSync(ctx context.Context,
	clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) (bool, []string, error) {
	if err := s.transform(clusterPolicy, cluster); err != nil {
		msg := fmt.Sprintf("failed to transform: %v", err)
		return false, []string{msg}, errors.New(msg)
	}
	return s.sync(ctx)
}

func (s *DeploymentState) transform(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy, cluster *ClusterState) error {
	skipUpdateComponentNames := getSkipUpdateComponentNames(clusterPolicy.Spec.SkipUpdateComponents)
	s.setDisabled(clusterPolicy)
	for _, component := range s.components {
		skipUpdate := slices.Contains(skipUpdateComponentNames, component.name)
		component.SetSkipUpdate(skipUpdate)
		if err := component.Transform(clusterPolicy, cluster); err != nil {
			return fmt.Errorf("%s: %w", component.name, err)
		}
	}
	return nil
}

func (s *DeploymentState) setDisabled(clusterPolicy *spyrev1alpha1.SpyreClusterPolicy) {
	for _, component := range s.components {
		disabled := true
		switch component.name {
		case spyreconst.CardManagementResourceName:
			disabled = !clusterPolicy.Spec.CardManagement.Enabled
		case spyreconst.MonitorResourceName:
			disabled = !clusterPolicy.Spec.MetricsExporter.Enabled
		case spyreconst.ValidatorResourceName:
			disabled = !clusterPolicy.Spec.PodValidator.Enabled
		case spyreconst.HealthCheckerResourceName:
			disabled = !clusterPolicy.Spec.HealthChecker.Enabled
		case spyreconst.SchedulerResourceName:
			for _, mode := range clusterPolicy.Spec.ExperimentalMode {
				if mode == spyrev1alpha1.ReservationMode {
					disabled = false
					break
				}
			}
		default:
			disabled = false
		}
		component.SetDisable(disabled)
	}
}

func (s *DeploymentState) sync(ctx context.Context) (bool, []string, error) {
	messages := []string{}
	ready := true
	for _, component := range s.components {
		success, message, err := component.Sync(ctx)
		if !success {
			componentMessage := fmt.Sprintf("%s (%s)", component.name, message)
			messages = append(messages, componentMessage)
			ready = false
		}
		if err != nil {
			return false, messages, fmt.Errorf("%s: %w", component.name, err)
		}
	}
	return ready, messages, nil
}

// Clear propagates clear command to all components in the state.
func (s *DeploymentState) Clear(ctx context.Context) error {
	var multiErr error
	for _, component := range s.components {
		if err := component.Clear(ctx); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	if multiErr != nil {
		return fmt.Errorf("failed to clear some components: %w", multiErr)
	}
	return nil
}

// Remove assets which owner (SpyreClusterPolicy) not found
// return sum of deleted objects from all components
func (s *DeploymentState) RemoveZombieAssets(ctx context.Context) int {
	sum := 0
	for _, component := range s.components {
		deletedCount := component.RemoveZombieAssets(ctx)
		sum += deletedCount
	}
	return sum
}

func getSkipUpdateComponentNames(skipComponents []spyrev1alpha1.Component) []string {
	names := []string{}
	for _, component := range skipComponents {
		name := componentToName[component]
		names = append(names, name)
	}
	return names
}
