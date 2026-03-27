/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package state

import (
	"context"

	spyrev2 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ApplyDeployConfig      = applyDeployConfig
	ApplyPort              = applyPort
	MountTopologyConfig    = mountTopologyConfig
	DecodeFromFile         = decodeFromFile
	NewDefaultObject       = newDefaultObject
	ApplyExperimentalModes = applyExperimentalModes
	HwHostPathMounts       = hwHostPathMounts
	DeviceHostPathMounts   = deviceHostPathMounts
	ApplyExecutePolicy     = applyExecutePolicy
	ApplyVerifyP2P         = applyVerifyP2P
	SetContainerEnv        = setContainerEnv
)

func (s *ClusterState) ApplyLogLevel(ctx context.Context, clusterPolicy *spyrev2.SpyreClusterPolicy) error {
	return s.applyLogLevel(ctx, clusterPolicy)
}

func (s *DeploymentState) GetComponents() []*ControlledComponent {
	return s.components
}

func (c *ControlledComponent) GetObjects() []ControlledObject {
	return c.objects
}

func (c *ControlledComponent) GetSkipUpdate() bool {
	return c.skipUpdate
}

func (s *DeploymentState) Transform(clusterPolicy *spyrev2.SpyreClusterPolicy, cluster *ClusterState) error {
	return s.transform(clusterPolicy, cluster)
}

func (s *DeploymentState) IsDisabled(componentName string) (found bool, disabled bool) {
	for _, component := range s.components {
		if component.GetName() == componentName {
			found = true
			disabled = component.disabled
		}
	}
	return found, disabled
}

func (s *DeploymentState) GetObject(componentName string, objectID ControlledID) ControlledObject {
	for _, component := range s.components {
		if component.GetName() == componentName {
			for _, obj := range component.GetObjects() {
				if obj.GetID().String() == objectID.String() {
					return obj
				}
			}
		}
	}
	return nil
}

func (c *ControlledComponent) DeleteAll(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues("Component", c.name, "Target Namespace", c.namespace)
	return c.deleteAll(ctx, logger)
}

func (c *ControlledComponent) ExportSetName(name string) {
	c.name = name
}

func (c *ControlledComponent) ExportSetClient(client client.Client) {
	c.client = client
}

func (c *ControlledComponent) SetObjects(objects []ControlledObject) {
	c.objects = objects
}

func (ds *DaemonSet) GetSpec() corev1.PodTemplateSpec {
	return ds.spec
}

func (p *HostPathMount) GetName() string {
	return p.name
}

func (p *HostPathMount) GetHostPath() string {
	return p.hostPath
}

func (p *HostPathMount) GetContainPath() string {
	return p.containerPath
}
