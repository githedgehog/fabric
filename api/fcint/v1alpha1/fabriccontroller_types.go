// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FabricControllerName is the name of the only FabricController, it lives in the fabric controller's namespace
const FabricControllerName = "default"

const (
	// ConditionInitialized is true once a fabric controller version completed initialization
	ConditionInitialized = "Initialized"
	// ConditionRefreshing is true while stored objects are being refreshed with the current defaults
	ConditionRefreshing = "Refreshing"
)

// FabricControllerSpec defines the desired state of FabricController.
type FabricControllerSpec struct {
	// ForceUnlock unlocks the fabric controller regardless of the initialized version. It's a last resort for a
	// stuck refresh: objects that weren't refreshed don't carry the current defaults and labels
	ForceUnlock bool `json:"forceUnlock,omitempty"`
}

// FabricControllerStatus defines the observed state of FabricController.
type FabricControllerStatus struct {
	// InitializedVersion is the fabric controller version that completed initialization, including the refresh of
	// stored objects. The fabric controller is locked until it matches the running version
	InitializedVersion string `json:"initializedVersion,omitempty"`
	// Refresh is the progress of the last or current refresh of stored objects
	Refresh FabricControllerRefresh `json:"refresh,omitempty"`
	// Conditions of the fabric controller: Initialized and Refreshing
	// +listType=map
	// +listMapKey=type
	Conditions []kmetav1.Condition `json:"conditions,omitempty"`
}

// FabricControllerRefresh is the progress of a refresh of stored objects with the current defaults and labels
type FabricControllerRefresh struct {
	// Version is the fabric controller version running the refresh
	Version string `json:"version,omitempty"`
	// StartedAt is the time the refresh started
	StartedAt kmetav1.Time `json:"startedAt,omitempty"`
	// FinishedAt is the time the refresh finished
	FinishedAt kmetav1.Time `json:"finishedAt,omitempty"`
	// Passes is the number of passes over the stored objects so far
	Passes int `json:"passes,omitempty"`
	// Kinds is the progress per kind
	Kinds []FabricControllerRefreshKind `json:"kinds,omitempty"`
}

// FabricControllerRefreshKind is the refresh progress of a single kind
type FabricControllerRefreshKind struct {
	// Kind is the kind of the refreshed objects
	Kind string `json:"kind"`
	// Total is the number of objects of the kind
	Total int `json:"total"`
	// Updated is the number of objects updated with the current defaults
	Updated int `json:"updated,omitempty"`
	// Rejected is the number of objects whose update was rejected
	Rejected int `json:"rejected,omitempty"`
	// Stale is the number of objects that still don't carry the current defaults after retries
	Stale int `json:"stale,omitempty"`
	// Invalid is the number of objects that fail the validation of the current version, checked without looking up
	// other objects. They still get the current defaults, but any change to them is rejected until they're fixed
	Invalid int `json:"invalid,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog,shortName=fc
// +kubebuilder:printcolumn:name="Initialized",type=string,JSONPath=`.status.initializedVersion`,priority=0
// +kubebuilder:printcolumn:name="Refreshing",type=string,JSONPath=`.status.conditions[?(@.type=="Refreshing")].status`,priority=0
// +kubebuilder:printcolumn:name="Passes",type=integer,JSONPath=`.status.refresh.passes`,priority=1
// +kubebuilder:printcolumn:name="ForceUnlock",type=boolean,JSONPath=`.spec.forceUnlock`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// FabricController is the Schema for the fabriccontrollers API. It holds the state of the fabric controller itself:
// the version that completed initialization (including refreshing all stored objects with the current defaults) and
// the progress of that refresh. The fabric controller only reconciles and accepts user changes once it's initialized.
type FabricController struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FabricControllerSpec   `json:"spec,omitempty"`
	Status FabricControllerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FabricControllerList contains a list of FabricController.
type FabricControllerList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []FabricController `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &FabricController{}, &FabricControllerList{})

		return nil
	})
}
