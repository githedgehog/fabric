/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:validation:Enum=control;service;management;compute
type ServerRole string

const (
	ServerRoleControlNode    ServerRole = "control"
	ServerRoleServiceNode    ServerRole = "service"
	ServerRoleManagementNode ServerRole = "management"
	// Compute Nodes only have IP addresses/VLAN info in the overlay network
	// What we describe here is only the physical wiring for these.
	// Config in this object is not applicable to compute nodes
	ServerRoleComputeNode ServerRole = "compute"
)

// ServerSpec defines the desired state of Server
type ServerSpec struct {
	Profile     string      `json:"profile,omitempty"`
	Location    Location    `json:"location,omitempty"`
	LocationSig LocationSig `json:"locationSig,omitempty"`
}

// ServerStatus defines the observed state of Server
type ServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Server is the Schema for the servers API
type Server struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerSpec   `json:"spec,omitempty"`
	Status ServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerList contains a list of Server
type ServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Server `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Server{}, &ServerList{})
}
