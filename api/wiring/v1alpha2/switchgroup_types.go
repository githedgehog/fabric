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

// SwitchGroupSpec defines the desired state of SwitchGroup
type SwitchGroupSpec struct{}

// SwitchGroupStatus defines the observed state of SwitchGroup
type SwitchGroupStatus struct{}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;wiring;fabric,shortName=sg
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// SwitchGroup is the marker API object to group switches together, switch can belong to multiple groups
type SwitchGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the SwitchGroup
	Spec SwitchGroupSpec `json:"spec,omitempty"`
	// Status is the observed state of the SwitchGroup
	Status SwitchGroupStatus `json:"status,omitempty"`
}

const KindSwitchGroup = "SwitchGroup"

//+kubebuilder:object:root=true

// SwitchGroupList contains a list of SwitchGroup
type SwitchGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SwitchGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SwitchGroup{}, &SwitchGroupList{})
}
