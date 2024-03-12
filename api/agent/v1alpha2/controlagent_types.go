// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ControlAgentSpec defines the desired state of the ControlAgent and includes all relevant information to configure
// the control node. Not intended to be modified by the user.
type ControlAgentSpec struct {
	ControlVIP string            `json:"controlVIP,omitempty"`
	Version    AgentVersion      `json:"version,omitempty"`
	Networkd   map[string]string `json:"networkd,omitempty"`
	Hosts      map[string]string `json:"hosts,omitempty"`
}

// ControlAgentStatus defines the observed state of ControlAgent
type ControlAgentStatus struct {
	Conditions      []metav1.Condition `json:"conditions"`
	Version         string             `json:"version,omitempty"`
	LastHeartbeat   metav1.Time        `json:"lastHeartbeat,omitempty"`
	LastAttemptTime metav1.Time        `json:"lastAttemptTime,omitempty"`
	LastAttemptGen  int64              `json:"lastAttemptGen,omitempty"`
	LastAppliedTime metav1.Time        `json:"lastAppliedTime,omitempty"`
	LastAppliedGen  int64              `json:"lastAppliedGen,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=cag
// +kubebuilder:printcolumn:name="Heartbeat",type=date,JSONPath=`.status.lastHeartbeat`,priority=0
// +kubebuilder:printcolumn:name="Applied",type=date,JSONPath=`.status.lastAppliedTime`,priority=0
// +kubebuilder:printcolumn:name="AppliedG",type=string,JSONPath=`.status.lastAppliedGen`,priority=0
// +kubebuilder:printcolumn:name="CurrentG",type=string,JSONPath=`.metadata.generation`,priority=0
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,priority=0
// +kubebuilder:printcolumn:name="Attempt",type=date,JSONPath=`.status.lastAttemptTime`,priority=2
// +kubebuilder:printcolumn:name="AttemptG",type=string,JSONPath=`.status.lastAttemptGen`,priority=2
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// ControlAgent is the Schema for the controlagents API
type ControlAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlAgentSpec   `json:"spec,omitempty"`
	Status ControlAgentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ControlAgentList contains a list of ControlAgent
type ControlAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ControlAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ControlAgent{}, &ControlAgentList{})
}
