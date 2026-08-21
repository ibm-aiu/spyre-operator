/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type SpyreInterface struct {
	PciAddress string `json:"pciAddress"`

	// +kubebuilder:default=healthy
	// +kubebuilder:validation:Enum=healthy;unhealthy
	Health SpyreHealth `json:"health,omitempty"`

	NumVfs int      `json:"numVfs,omitempty"`
	Vfs    []string `json:"vfs,omitempty"`
}

type SpyreSSAInterface struct {
	PciAddress string `json:"pciAddress"`

	// +kubebuilder:default=healthy
	// +kubebuilder:validation:Enum=healthy;unhealthy
	Health SpyreHealth `json:"health,omitempty"`
}

type SpyreInterfaces []SpyreInterface
type SpyreSSAInterfaces []SpyreSSAInterface

// SpyreHealth indicates Spyre
type SpyreHealth string

// Constants representing different Spyre device's health.
const (
	// SpyreHealthy indicates Spyre is healthy.
	SpyreHealthy SpyreHealth = "healthy"
	// SpyreUnhealthy indicates Spyre is unhealthy.
	SpyreUnhealthy SpyreHealth = "unhealthy"
)

// SpyreNodeStateSpec defines the desired state of SpyreNodeState
type SpyreNodeStateSpec struct {
	NodeName           string             `json:"nodeName"`
	SpyreInterfaces    SpyreInterfaces    `json:"spyreInterfaces,omitempty"`
	SpyreSSAInterfaces SpyreSSAInterfaces `json:"spyreSSAInterfaces,omitempty"`
	Pcitopo            string             `json:"pcitopo,omitempty"`
}

// UnhealthyDevice represents a device that is not in a healthy state
type UnhealthyDevice struct {
	// ID is the device identifier.
	ID string `json:"id"`
	// State is the current state of the device.
	State string `json:"state"`
}

// SpyreNodeStateStatus defines the observed state of SpyreNodeState
type SpyreNodeStateStatus struct {
	// Conditions represent the latest available observations of the SpyreNodeState's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// UnhealthyDevices is a list of devices that are not in a healthy state.
	// Only devices that need attention are reported here.
	// +optional
	UnhealthyDevices []UnhealthyDevice `json:"unhealthyDevices,omitempty"`
	// AllocationList is a list of allocated devices and their owner Pods.
	AllocationList []Allocation `json:"allocation,omitempty"`
	// Reservations is a map from resource name to Reservation.
	Reservations map[string]Reservation `json:"reservation,omitempty"`
}

// Allocation contains a pair of allocated device list and the
// consumer of the devices.
//
// Spyre Device Plugin adds Allocation.DeviceList at a time of allocation.
//
// ```
// {"devices": ["0000:00:0a", "0000:00:09"]}
// ```
//
// and the creation of a Pod triggers Spyre Pod Resource Watcher to append
// the Pod information to the Allocation entry.
//
// ```
//
//	{
//	  "devices": ["000a", "0009"],
//	  "pod": {"namespace":"myapp", "name": "mypod", "uid": "1c8f..."}
//	  "pool": "spyre_pf"
//	}
//
// ```
type Allocation struct {
	DeviceList   []string `json:"devices,omitempty"`
	Pod          *Pod     `json:"pod,omitempty"`
	ResourcePool string   `json:"pool,omitempty"`
}

// ReservationEntry binds one Pod to the exact set of devices reserved for it.
//
// Entries is the authoritative representation of a Reservation: because each
// entry carries both the owner Pod (including its UID) and that Pod's devices,
// consumers never have to guess which device set belongs to which Pod.
type ReservationEntry struct {
	// Pod is the owner of the reserved devices.
	Pod Pod `json:"pod"`
	// DeviceList is the set of devices reserved for Pod.
	DeviceList []string `json:"devices,omitempty"`
	// ReservedAt is the time the reservation was created. It is used to keep a
	// reservation alive for a grace period after its Pod disappears from the
	// API, so that a device is not handed out again while the previous consumer
	// is still tearing down.
	// +optional
	ReservedAt *metav1.Time `json:"reservedAt,omitempty"`
}

// Reservation records which devices are reserved for which Pods.
// Spyre Scheduler creates a Reservation, and Spyre Device Plugin removes it
// at the time of allocation.
//
// ```
//
//		{
//	        "spyre_pf": {
//	            "entries": [
//	                {"pod": {"namespace": "myapp", "name": "app1", "uid": "1c8f..."},
//	                 "devices": ["000a", "0009"], "reservedAt": "2026-08-21T01:23:45Z"},
//	                {"pod": {"namespace": "myapp", "name": "app2", "uid": "97ab..."},
//	                 "devices": ["001f"], "reservedAt": "2026-08-21T01:23:46Z"}],
//	            "deviceSets": [["000a", "0009"], ["001f"]],
//	            "podsUnderScheduling": [
//	                {"namespace": "myapp", "name": "app1", "uid": "1c8f..."},
//	                {"namespace": "myapp", "name": "app2", "uid": "97ab..."}]
//	        }
//		}
//
// ```
type Reservation struct {
	// PodsUnderScheduling lists the Pods holding a reservation.
	//
	// Deprecated: derived from Entries and kept only so that components which
	// have not yet been updated keep working during a rolling upgrade. Writers
	// must call SyncLegacy() before persisting. Will be removed in a future
	// release.
	PodsUnderScheduling []Pod `json:"podsUnderScheduling,omitempty"`
	// DeviceSets lists the reserved device sets, positionally unrelated to
	// PodsUnderScheduling.
	//
	// Deprecated: derived from Entries. See PodsUnderScheduling.
	DeviceSets [][]string `json:"deviceSets,omitempty"`

	// Entries is the source of truth: one entry per reserving Pod, binding that
	// Pod's identity to its devices.
	// +optional
	Entries []ReservationEntry `json:"entries,omitempty"`
}

type Pod struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	// UID distinguishes generations of a Pod that reuses a name, which happens
	// routinely with GitHub Actions Runner Controller runners. Matching on name
	// alone lets a successor Pod be mistaken for its predecessor.
	// +optional
	UID types.UID `json:"uid,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=spyrens,scope=Cluster

// SpyreNodeState is the Schema for the SpyreNodeState API
// +operator-sdk:csv:customresourcedefinitions:displayName="Spyre Node State"
type SpyreNodeState struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SpyreNodeStateSpec   `json:"spec,omitempty"`
	Status SpyreNodeStateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SpyreNodeStateList contains a list of SpyreNodeState
type SpyreNodeStateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SpyreNodeState `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SpyreNodeState{}, &SpyreNodeStateList{})
}
