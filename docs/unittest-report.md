# Unit Tests

Test item | Case description | File location
---|---|---
ClusterState/applyLogLevel|default to debug to info to debug|internal/state/cluster_state_test.go
ClusterState/applyLogLevel|default to debug to info to error|internal/state/cluster_state_test.go
ClusterState/applyLogLevel|default to debug to info|internal/state/cluster_state_test.go
ClusterState/applyLogLevel|default to debug|internal/state/cluster_state_test.go
ControlledComponent|can transform ConfigMap so that user can customize senlib template name by SpyreClusterPolicy|internal/state/controlled_component_test.go
ControlledComponent|can transform pod validator by SpyreClusterPolicy|internal/state/controlled_component_test.go
ControlledComponent/spyre-card-management|can apply num of spyre cards at transform|internal/state/controlled_component_test.go
ControlledComponent/spyre-card-management|can delete Pods with spyrecardmanager label at disabling cardmgmt|internal/state/controlled_component_test.go
ControlledComponent/spyre-card-management|can skip update|internal/state/controlled_component_test.go
ControlledComponent/spyre-device-plugin|can clear node state on clear|internal/state/controlled_component_test.go
ControlledComponent/spyre-device-plugin|can transform and sync|internal/state/controlled_component_test.go
ControlledObject|can apply and revert experimental modes|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|disableVF-customSpyreFilter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|disableVF-emptySpyreFilter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|disableVF-nilSpyreFilter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|enableVF-customSpyreFilter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|enableVF-emptySpyreFilter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|enableVF-nilSpyreFilter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|set both runner image with filter|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|set PF runner image|internal/state/controlled_object_test.go
ControlledObject/card management/aiucardmgmt.ini|set VF runner image|internal/state/controlled_object_test.go
DeploymentState/all states|can set skipUpdate to components|internal/state/deployment_state_test.go
DeploymentState/state-core-components|transforms device plugin deployments by SpyreClusterPolicy|internal/state/deployment_state_test.go
DeploymentState/state-init/Transform|only health checker enabled|internal/state/deployment_state_test.go
DeploymentState/state-init/Transform|only validator enabled|internal/state/deployment_state_test.go
DeploymentState/state-init/Transform|validator/health checker both disabled|internal/state/deployment_state_test.go
DeploymentState/state-init/Transform|validator/health checker both enabled|internal/state/deployment_state_test.go
DeploymentState/state-plugin-components/Transform|exporter disabled/scheduler disabled|internal/state/deployment_state_test.go
DeploymentState/state-plugin-components/Transform|exporter disabled/scheduler enabled|internal/state/deployment_state_test.go
DeploymentState/state-plugin-components/Transform|exporter enabled/scheduler disabled|internal/state/deployment_state_test.go
DeploymentState/state-plugin-components/Transform|exporter enabled/scheduler enabled|internal/state/deployment_state_test.go
general CRUD/SpyreClusterPolicy|can create a SpyreClusterPolicy resource|pkg/client/client_test.go
general CRUD/SpyreClusterPolicy|can delete a SpyreClusterPolicy resource|pkg/client/client_test.go
general CRUD/SpyreClusterPolicy|can get a SpyreClusterPolicy resource|pkg/client/client_test.go
general CRUD/SpyreClusterPolicy|can retry on conflict when update status|pkg/client/client_test.go
general CRUD/SpyreClusterPolicy|can update status of SpyreClusterPolicy|pkg/client/client_test.go
general CRUD/SpyreNodeState|can create a new SpyreNodeState resource|pkg/client/client_test.go
general CRUD/SpyreNodeState|can create SpyreNodeState with SpyreSSAInterfaces|pkg/client/client_test.go
general CRUD/SpyreNodeState|can delete a SpyreNodeState resource|pkg/client/client_test.go
general CRUD/SpyreNodeState|can get a SpyreNodeState resource|pkg/client/client_test.go
general CRUD/SpyreNodeState|can list all SpyreNodeState|pkg/client/client_test.go
general CRUD/SpyreNodeState|can retry on conflict when update spec/status|pkg/client/client_test.go
general CRUD/SpyreNodeState|can update a SpyreNodeState resource's spec|pkg/client/client_test.go
general CRUD/SpyreNodeState|can update a SpyreNodeState resource's status|pkg/client/client_test.go
general CRUD/SpyreNodeState|can update SpyreNodeState with SpyreSSAInterfaces|pkg/client/client_test.go
Labeler/GetClusterSpyreLabelInfo|has NFD + spyre label → returns arch|internal/labeler/labeler_test.go
Labeler/GetClusterSpyreLabelInfo|has NFD but no spyre common label → no arch|internal/labeler/labeler_test.go
Labeler/GetClusterSpyreLabelInfo|no NFD labels|internal/labeler/labeler_test.go
Labeler/HasCommonSpyreLabel|empty|internal/labeler/labeler_test.go
Labeler/HasCommonSpyreLabel|invalid key|internal/labeler/labeler_test.go
Labeler/HasCommonSpyreLabel|invalid value|internal/labeler/labeler_test.go
Labeler/HasCommonSpyreLabel|valid|internal/labeler/labeler_test.go
Labeler/HasNFDLabels|empty|internal/labeler/labeler_test.go
Labeler/HasNFDLabels|invalid|internal/labeler/labeler_test.go
Labeler/HasNFDLabels|valid|internal/labeler/labeler_test.go
Labeler/HasSpyreDeviceLabels|empty|internal/labeler/labeler_test.go
Labeler/HasSpyreDeviceLabels|invalid key|internal/labeler/labeler_test.go
Labeler/HasSpyreDeviceLabels|invalid value|internal/labeler/labeler_test.go
Labeler/HasSpyreDeviceLabels|valid pci-06e7_1014.present|internal/labeler/labeler_test.go
Labeler/HasSpyreDeviceLabels|valid pci-1014|internal/labeler/labeler_test.go
Labeler/LabelSpyreNodes|amd64|internal/labeler/labeler_test.go
Labeler/LabelSpyreNodes|non-amd64|internal/labeler/labeler_test.go
Labeler/LabelSpyreNodes|pseudoMode with an NFD label|internal/labeler/labeler_test.go
Labeler/LabelSpyreNodes|pseudoMode|internal/labeler/labeler_test.go
Labeler/UpdateCommonSpyreLabel|Existing pseudo-Spyre: hasNFD/hasSpyreCommonLabel|internal/labeler/labeler_test.go
Labeler/UpdateCommonSpyreLabel|Existing Spyre node: hasNFD/hasSpyre/hasSpyreCommonLabel|internal/labeler/labeler_test.go
Labeler/UpdateCommonSpyreLabel|New pseudo-Spyre: hasNFD|internal/labeler/labeler_test.go
Labeler/UpdateCommonSpyreLabel|New Spyre node: hasNFD/hasSpyre|internal/labeler/labeler_test.go
Labeler/UpdateCommonSpyreLabel|Non-Spyre node with previous set: hasNFD/hasSpyreCommonLabel|internal/labeler/labeler_test.go
Labeler/UpdateCommonSpyreLabel|Non-Spyre node: hasNFD|internal/labeler/labeler_test.go
Labeler/UpdateDeviceCountProductName|existing count labels: no capacity|internal/labeler/labeler_test.go
Labeler/UpdateDeviceCountProductName|existing match count labels: one capacity|internal/labeler/labeler_test.go
Labeler/UpdateDeviceCountProductName|existing unmatch count labels: one capacity|internal/labeler/labeler_test.go
Labeler/UpdateDeviceCountProductName|no labels: has one capacity|internal/labeler/labeler_test.go
Labeler/UpdateDeviceCountProductName|no labels: has zero capacity|internal/labeler/labeler_test.go
Labeler/UpdateDeviceCountProductName|no labels: no capacity|internal/labeler/labeler_test.go
NodeLabelerReconciler/with API server|does not set spyre.present when no NFD labels and no policy|controllers/node_labeler_controller_test.go
NodeLabelerReconciler/with API server|removes spyre.present when pseudoDeviceMode is disabled and no NFD labels|controllers/node_labeler_controller_test.go
NodeLabelerReconciler/with API server|sets spyre.present when NFD PCI labels are present and no policy|controllers/node_labeler_controller_test.go
NodeLabelerReconciler/with API server|sets spyre.present when pseudoDeviceMode is enabled in policy (CRC/e2e case)|controllers/node_labeler_controller_test.go
SpyreclusterpolicyController/api|can unmarshal an example file with skip components|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/api|can unmarshal an example file|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/accepts loglevel|accept debug|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/accepts loglevel|accept error|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/accepts loglevel|accept info|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/accepts loglevel|deny Debug|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/accepts loglevel|deny DEBUG|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/nodeUpdateNeedsReconcile|no label change → no reconcile|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/nodeUpdateNeedsReconcile|OS tree version changed → reconcile|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/nodeUpdateNeedsReconcile|spyre.present added (NodeLabelerReconciler labeled node) → reconcile|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/nodeUpdateNeedsReconcile|spyre.present removed (pseudoMode disabled, no NFD) → reconcile|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/nodeUpdateNeedsReconcile|unrelated label change → no reconcile|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/process status|no NFD|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/process status|no Spyre nodes|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/process status|not ready|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/reconciliation/process status|ready|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/yaml|can deploy example file|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/yaml|can deploy minimum spec|controllers/spyreclusterpolicy_controller_test.go
SpyreclusterpolicyController/with API server/yaml|must deny invalid spec (no device plugin)|controllers/spyreclusterpolicy_controller_test.go
SpyreNodeState State|can create SpyreNodeState|internal/state/spyrenodestate_state_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|both in both|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|non-spyre in both|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|non-spyre in lim|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|non-spyre in req|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|nothing|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre (pf + devId) in req|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre (vf) in req|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre in both|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre in lim and non-spyre in req|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre in lim|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre in req and non-spyre in lim|controllers/spyrepod/spyrepod_test.go
Spyrepod/GetSpyreResourceName and IsSpyrePod|spyre in req|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsPerDevice|irrelevant|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsPerDevice|per-device|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsPerDevice|pf|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsPerDevice|tier|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsTopologyAware|irrelevant|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsTopologyAware|per-device|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsTopologyAware|pf|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsTopologyAware|tier0|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsTopologyAware|tier1|controllers/spyrepod/spyrepod_test.go
Spyrepod/IsTopologyAware|tier2|controllers/spyrepod/spyrepod_test.go
Spyrepod/PCI address handling|numeric + char address|controllers/spyrepod/spyrepod_test.go
Spyrepod/PCI address handling|numeric address|controllers/spyrepod/spyrepod_test.go
StateController|init values are not nil|internal/state/state_controller_test.go
StateController/TransformAndSync|can update owner UUID|internal/state/state_controller_test.go
StateController/TransformAndSync/zombie asset|can remove zombie asset|internal/state/state_controller_test.go
StateController/TransformAndSync/zombie asset|must not delete assets with owner|internal/state/state_controller_test.go
Test Topology/topology v2|can read pcitopo v2 file containing PF devices|pkg/types/pcitopov2/pcitopo_test.go
Test Topology/topology v2|can read pcitopo v2 file containing VF devices|pkg/types/pcitopov2/pcitopo_test.go
Transform|TransformMetricsExporterService - new port|internal/state/transform_test.go
Transform/applyDeployConfig|args|internal/state/transform_test.go
Transform/applyDeployConfig|both request and limit|internal/state/transform_test.go
Transform/applyDeployConfig|envs|internal/state/transform_test.go
Transform/applyDeployConfig|image pull policy|internal/state/transform_test.go
Transform/applyDeployConfig|no image|internal/state/transform_test.go
Transform/applyDeployConfig|node selector|internal/state/transform_test.go
Transform/applyDeployConfig|resource limit|internal/state/transform_test.go
Transform/applyDeployConfig|resource request|internal/state/transform_test.go
Transform/applyDeployConfig|some image with sha|internal/state/transform_test.go
Transform/applyDeployConfig|some image|internal/state/transform_test.go
Transform/applyDeployConfig|wrong image combination - no repository|internal/state/transform_test.go
Transform/applyDeployConfig|wrong image combination - no version|internal/state/transform_test.go
Transform/ApplyExperimentalModes|Default|internal/state/transform_test.go
Transform/ApplyExperimentalModes|No mode set with existing environment variables|internal/state/transform_test.go
Transform/ApplyExperimentalModes|perDeviceAllocation + pseudoDevice with environment variables to be overridden|internal/state/transform_test.go
Transform/ApplyExperimentalModes|perDeviceAllocation + pseudoDevice with existing environment variables|internal/state/transform_test.go
Transform/ApplyExperimentalModes|perDeviceAllocation + pseudoDevice|internal/state/transform_test.go
Transform/ApplyExperimentalModes|perDeviceAllocation with existing environment variables|internal/state/transform_test.go
Transform/ApplyExperimentalModes|perDeviceAllocation|internal/state/transform_test.go
Transform/ApplyExperimentalModes|pseudoDevice with existing environment variables|internal/state/transform_test.go
Transform/ApplyExperimentalModes|pseudoDevice|internal/state/transform_test.go
Transform/card management|disabled|internal/state/transform_test.go
Transform/card management|empty pvc|internal/state/transform_test.go
Transform/card management|enabled|internal/state/transform_test.go
Transform/card management|mount pvc|internal/state/transform_test.go
Transform/config/metrics path and name|set if defined|internal/state/transform_test.go
Transform/hardware mount/architecture-specific mount|amd64 with devices|internal/state/transform_test.go
Transform/hardware mount/architecture-specific mount|amd64 with pseudo mode|internal/state/transform_test.go
Transform/hardware mount/architecture-specific mount|s390x with devices|internal/state/transform_test.go
Transform/hardware mount/architecture-specific mount|s390x with pseudo mode|internal/state/transform_test.go
Transform/hardware mount/architecture-specific mount|unsupported type with devices|internal/state/transform_test.go
Transform/hardware mount/architecture-specific mount|unsupported type with pseudo mode|internal/state/transform_test.go
Transform/health-checker integration/check health socket|disabled health checker|internal/state/transform_test.go
Transform/health-checker integration/check health socket|enabled health checker|internal/state/transform_test.go
Transform/init container|init container can be loaded|internal/state/transform_test.go
Transform/init container/apply execute policy|Always - empty env|internal/state/transform_test.go
Transform/init container/apply execute policy|Always - other env|internal/state/transform_test.go
Transform/init container/apply execute policy|Always - replace env|internal/state/transform_test.go
Transform/init container/apply execute policy|IfNotPresent - empty env|internal/state/transform_test.go
Transform/init container/apply execute policy|IfNotPresent - other env|internal/state/transform_test.go
Transform/init container/apply execute policy|IfNotPresent - replace env|internal/state/transform_test.go
Transform/init container/apply execute policy|nil - empty env|internal/state/transform_test.go
Transform/init container/apply p2pDMA on amd64|p2pDMA=false - empty env|internal/state/transform_test.go
Transform/init container/apply p2pDMA on amd64|p2pDMA=false - other env|internal/state/transform_test.go
Transform/init container/apply p2pDMA on amd64|p2pDMA=false - replace env|internal/state/transform_test.go
Transform/init container/apply p2pDMA on amd64|p2pDMA=true - empty env|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|amd64 with p2pdma: VERIFY_P2P=1 and privileged=true, no existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|amd64 with p2pdma: VERIFY_P2P=1 and privileged=true, with existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|amd64 without p2pdma: VERIFY_P2P=0 and privileged=false, with existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|ppc64le with p2pdma: VERIFY_P2P=0 and privileged=false, with existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|ppc64le: VERIFY_P2P=0 and privileged=false, no existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|ppc64le: VERIFY_P2P=0 and privileged=false, with existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|s390x with p2pdma: VERIFY_P2P=0 and privileged=false, with existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|s390x: VERIFY_P2P=0 and privileged=false, no existing SecurityContext|internal/state/transform_test.go
Transform/init container/architecture-aware VERIFY_P2P and privileged settings|s390x: VERIFY_P2P=0 and privileged=false, with existing SecurityContext|internal/state/transform_test.go
Transform/init container/transform init container config|amd64: with init template, with image|internal/state/transform_test.go
Transform/init container/transform init container config|pseudo, with init template, no image|internal/state/transform_test.go
Transform/init container/transform init container config|pseudo, with init template, with image|internal/state/transform_test.go
Transform/init container/transform init container config|pseudo, without init template, no image|internal/state/transform_test.go
Transform/init container/transform init container config|pseudo, without init template, with image|internal/state/transform_test.go
Transform/init container/transform init container config|unsupported: with init template, with image|internal/state/transform_test.go
Transform/init container/transform init container config|with init template, no image|internal/state/transform_test.go
Transform/init container/transform init container config|without init template, no image|internal/state/transform_test.go
Transform/init container/transform init container config|without init template, with image|internal/state/transform_test.go
Transform/P2PDMA|P2PDMA=false|internal/state/transform_test.go
Transform/P2PDMA|P2PDMA=true|internal/state/transform_test.go
Transform/port|can apply new port|internal/state/transform_test.go
Transform/port|can new port with some irrelevant values|internal/state/transform_test.go
Transform/port|can override port by TransformMetricsExporterService|internal/state/transform_test.go
Transform/port|can override port|internal/state/transform_test.go
Transform/topology config map|TransformSenlibConfigTemplate works when metric exporter is enabled|internal/state/transform_test.go
Transform/topology config map/check config map and container mount|empty, must mount|internal/state/transform_test.go
Transform/topology config map/check config map and container mount|with volume exist, must replace|internal/state/transform_test.go
Transform/topology config map/check config map and container mount|with volume mount exist, must replace|internal/state/transform_test.go
Transform/TransformDeployment|change pull policy|internal/state/transform_test.go
Transform/TransformDeployment|change replica|internal/state/transform_test.go
Transform/TransformDeployment|default values|internal/state/transform_test.go
