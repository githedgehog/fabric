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

// RackPosition defines the geopraphical position of the rack in a datacenter
type RackPosition struct {
	Location string `json:"location,omitempty"`
	Aisle    string `json:"aisle,omitempty"`
	Row      string `json:"row,omitempty"`
}

// RackSpec defines the properties of a rack which we are modelling
type RackSpec struct {
	NumServers       uint32       `json:"numServers,omitempty"`
	HasControlNode   bool         `json:"hasControlNode,omitempty"`
	HasConsoleServer bool         `json:"hasConsoleServer,omitempty"`
	Position         RackPosition `json:"position,omitempty"`
}

// RackStatus defines the observed state of Rack
type RackStatus struct {
	// TODO: add port status fields
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Rack is the Schema for the racks API
type Rack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RackSpec   `json:"spec,omitempty"`
	Status RackStatus `json:"status,omitempty"`
}

const KindRack = "Rack"

//+kubebuilder:object:root=true

// RackList contains a list of Rack
type RackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rack{}, &RackList{})
}
