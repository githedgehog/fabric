// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"go.githedgehog.com/fabric/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SwitchBootSpec defines the desired state of SwitchBoot.
type SwitchBootSpec struct {
	// Serial is the normalized serial number of the switch to identify it during the bootstrap
	Serial string `json:"serial,omitempty"`
	// MAC is the normalized MAC address of the switch management port to identify it during the bootstrap
	MAC string `json:"mac,omitempty"`
	// NOSType is the NOS type the switch should be installed with
	NOSType meta.NOSType `json:"nosType,omitempty"`
	// AgentVersion is the version of the agent the switch should be installed with
	AgentVersion string `json:"agentVersion,omitempty"`
}

// +kubebuilder:validation:Enum=onie;cumulus-ztp
// SwitchBootAttemptType is how the switch attempted to boot
type SwitchBootAttemptType string

const (
	// SwitchBootAttemptTypeONIE is an ONIE NOS install
	SwitchBootAttemptTypeONIE SwitchBootAttemptType = "onie"
	// SwitchBootAttemptTypeCumulusZTP is a Cumulus ZTP script request
	SwitchBootAttemptTypeCumulusZTP SwitchBootAttemptType = "cumulus-ztp"
)

// +kubebuilder:validation:Enum=served;agent-not-cached;error
// SwitchBootAttemptResult is what the switch got for its boot attempt
type SwitchBootAttemptResult string

const (
	// SwitchBootAttemptResultServed means the NOS installer or ZTP script was sent completely, it doesn't mean the
	// switch installed it successfully
	SwitchBootAttemptResultServed SwitchBootAttemptResult = "served"
	// SwitchBootAttemptResultAgentNotCached means the switch was asked to retry later as the agent version isn't
	// pre-cached yet
	SwitchBootAttemptResultAgentNotCached SwitchBootAttemptResult = "agent-not-cached"
	// SwitchBootAttemptResultError means serving failed, the boot server logs have the details
	SwitchBootAttemptResultError SwitchBootAttemptResult = "error"
)

// SwitchBootAttempt is a boot attempt of the switch, i.e. a request to the boot server matching it
type SwitchBootAttempt struct {
	// Time is when the request was received, precise enough to order the attempts
	Time kmetav1.MicroTime `json:"time"`
	// Type is how the switch attempted to boot
	Type SwitchBootAttemptType `json:"type"`
	// Result is what the switch got
	Result SwitchBootAttemptResult `json:"result"`
	// Remote is the IP address the request came from
	Remote string `json:"remote,omitempty"`
}

// SwitchBootStatus defines the observed state of SwitchBoot. It's written by the boot server only, as a whole.
type SwitchBootStatus struct {
	// LastAttempt is the last boot attempt of the switch
	LastAttempt *SwitchBootAttempt `json:"lastAttempt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hhint;hhfcint,shortName=swboot
// +kubebuilder:printcolumn:name="Serial",type=string,JSONPath=`.spec.serial`,priority=0
// +kubebuilder:printcolumn:name="MAC",type=string,JSONPath=`.spec.mac`,priority=0
// +kubebuilder:printcolumn:name="NOS",type=string,JSONPath=`.spec.nosType`,priority=0
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentVersion`,priority=0
// +kubebuilder:printcolumn:name="Attempt",type=string,JSONPath=`.status.lastAttempt.type`,priority=0
// +kubebuilder:printcolumn:name="Result",type=string,JSONPath=`.status.lastAttempt.result`,priority=0
// +kubebuilder:printcolumn:name="Attempted",type=date,JSONPath=`.status.lastAttempt.time`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// SwitchBoot is the Schema for the switchboots API. It's managed by the fabric controller for each switch and holds
// everything needed to bootstrap it (ONIE NOS install or ZTP): how to identify the switch and what to install on it.
type SwitchBoot struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchBootSpec   `json:"spec,omitempty"`
	Status SwitchBootStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SwitchBootList contains a list of SwitchBoot.
type SwitchBootList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []SwitchBoot `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &SwitchBoot{}, &SwitchBootList{})

		return nil
	})
}
