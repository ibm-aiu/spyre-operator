/*
 * +-------------------------------------------------------------------+
 * | Copyright (c) 2025, 2026 IBM Corp.                                |
 * | SPDX-License-Identifier: Apache-2.0                               |
 * +-------------------------------------------------------------------+
 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
//	  "pod": {"namespace":"myapp", "name": "mypod"}
//	  "pool": "spyre_pf"
//	}
//
// ```
type Allocation struct {
	DeviceList   []string `json:"devices,omitempty"`
	Pod          *Pod     `json:"pod,omitempty"`
	ResourcePool string   `json:"pool,omitempty"`
}

// Reservation contains a pair of reserved device list and its requester.
// Spyre Scheduler creates a Reservation, and Spyre Device Plugin removes it
// at the time of allocation.
//
// ```
//
//		{
//	        "spyre_pf": {
//	            "deviceSets": [["000a", "0009"], ["001f"]],
//	            "podsUnderScheduling": [
//	                {"namespace": "myapp", "name": "app1"},
//	                {"namespace": "myapp", "name": "app2"}]
//	        },
//	        "spyre_pf_003d": {
//	            "deviceSets": [["003d"]],
//	            "podsUnderScheduling": [{"namespace": "myapp", "name": "app3"}]
//	        }
//		}
//
// ```
type Reservation struct {
	PodsUnderScheduling []Pod      `json:"podsUnderScheduling,omitempty"`
	DeviceSets          [][]string `json:"deviceSets,omitempty"`
}

type Pod struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
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
