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

type LinkPort struct {
	Name string `json:"name,omitempty"`
}

type LinkPart struct {
	SwitchPort LinkPort `json:"switchPort,omitempty"`
	ServerPort LinkPort `json:"serverPort,omitempty"`
}

// +kubebuilder:validation:MaxItems=2
// +kubebuilder:validation:MinItems=2
type Link []LinkPart

type UnbundledConnection struct {
	Link Link `json:"link,omitempty"`
}

type ManagementConnection struct {
	// TODO: add management connection fields like bootstrap IP and vlan here or in a custom link part
	Link Link `json:"link,omitempty"`
}

type MCLAGConnection struct {
	Links []Link `json:"links,omitempty"`
}

type MCLAGDomainConnection struct {
	Links []Link `json:"links,omitempty"`
}

// ConnectionSpec defines the desired state of Connection
type ConnectionSpec struct {
	Unbundled   UnbundledConnection   `json:"unbundled,omitempty"`
	Management  ManagementConnection  `json:"management,omitempty"`
	MCLAG       MCLAGConnection       `json:"mclag,omitempty"`
	MCLAGDomain MCLAGDomainConnection `json:"mclagDomain,omitempty"`
}

// ConnectionStatus defines the observed state of Connection
type ConnectionStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Connection is the Schema for the connections API
type Connection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConnectionSpec   `json:"spec,omitempty"`
	Status ConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ConnectionList contains a list of Connection
type ConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Connection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Connection{}, &ConnectionList{})
}
