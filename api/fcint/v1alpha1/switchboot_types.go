// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"strings"

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

// SwitchBootStatus defines the observed state of SwitchBoot.
type SwitchBootStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog,shortName=swboot
// +kubebuilder:printcolumn:name="Serial",type=string,JSONPath=`.spec.serial`,priority=0
// +kubebuilder:printcolumn:name="MAC",type=string,JSONPath=`.spec.mac`,priority=0
// +kubebuilder:printcolumn:name="NOS",type=string,JSONPath=`.spec.nosType`,priority=0
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentVersion`,priority=0
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
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

// NormalizeBootID normalizes a serial number or MAC address used to identify a switch during the bootstrap, so they
// match regardless of surrounding whitespace and case
func NormalizeBootID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
