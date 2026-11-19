// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FabricControllerName is the name of the only FabricController, it lives in the fabric controller's namespace
const FabricControllerName = "default"

// ConditionReady is true once the running version completed initialization for the current spec, its reason is the
// state of the fabric controller and its message a short summary
const ConditionReady = "Ready"

const (
	ReasonInitializing = "Initializing" // initialization running or retrying after an error, see status.error
	ReasonFailed       = "Failed"       // objects the refresh couldn't fix, see status.problems
	ReasonMaintenance  = "Maintenance"  // see spec.maintenance
	ReasonInitialized  = "Initialized"  // the only reason Ready is true with
)

// Problem reasons, see FabricControllerRefreshKind for what they mean
const (
	ProblemRejected = "Rejected"
	ProblemStale    = "Stale"
	ProblemInvalid  = "Invalid"
)

// FabricControllerSpec defines the desired state of FabricController.
type FabricControllerSpec struct {
	// Maintenance is dangerous, it's for fixing the objects that keep the fabric controller locked: all changes are
	// admitted, validation errors only come back as warnings, while reconcilers stay stopped, so nothing is deployed.
	// Clearing it initializes the fabric controller again, which validates every object on its own only: a conflict
	// between objects or a deletion of an object still in use, made in maintenance, goes unnoticed and gets deployed
	Maintenance bool `json:"maintenance,omitempty"`
}

// FabricControllerStatus defines the observed state of FabricController.
type FabricControllerStatus struct {
	// Version is the running fabric controller version
	Version string `json:"version,omitempty"`
	// InitializedVersion is the fabric controller version that last completed initialization, including the refresh
	// of stored objects. The fabric controller is locked until it matches the running version and it's Ready
	InitializedVersion string `json:"initializedVersion,omitempty"`
	// Error is why the initialization failed, while it's retried
	Error string `json:"error,omitempty"`
	// Refresh is the progress of the last or current refresh of stored objects
	Refresh FabricControllerRefresh `json:"refresh,omitempty"`
	// Problems are the objects the last refresh couldn't fix. They keep the fabric controller locked, it initializes
	// again only on a spec change, e.g. leaving maintenance to fix them, or a restart. Only the first ones are listed,
	// refresh.kinds has the counts
	Problems []FabricControllerProblem `json:"problems,omitempty"`
	// Conditions of the fabric controller: Ready
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
	// Rejected is the number of objects whose update was rejected, e.g. by the CRD schema or missing permissions
	Rejected int `json:"rejected,omitempty"`
	// Stale is the number of objects that still don't carry the current defaults after retries, e.g. as the CRD
	// schema drops a defaulted field or something else keeps writing them back
	Stale int `json:"stale,omitempty"`
	// Invalid is the number of objects that fail the validation of the current version, checked without looking up
	// other objects. They still get the current defaults
	Invalid int `json:"invalid,omitempty"`
}

// FabricControllerProblem is an object the refresh couldn't fix
type FabricControllerProblem struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	// Reason is Rejected, Stale or Invalid
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hhint;hhfcint,shortName=fc
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`,priority=0
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,priority=0
// +kubebuilder:printcolumn:name="Initialized",type=string,JSONPath=`.status.initializedVersion`,priority=0
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`,priority=0
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=0
// +kubebuilder:printcolumn:name="Maintenance",type=boolean,JSONPath=`.spec.maintenance`,priority=0
// +kubebuilder:printcolumn:name="Passes",type=integer,JSONPath=`.status.refresh.passes`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
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

// IsReady is true if the version completed initialization for the current spec: every spec change, e.g. maintenance
// turned on and off, keeps the fabric controller locked until it's initialized again
func (fc *FabricController) IsReady(version string) bool {
	ready := kmeta.FindStatusCondition(fc.Status.Conditions, ConditionReady)

	return fc.Status.InitializedVersion == version && ready != nil && ready.Status == kmetav1.ConditionTrue &&
		ready.ObservedGeneration == fc.Generation
}
