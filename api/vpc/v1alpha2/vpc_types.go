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

// VPCSpec defines the desired state of VPC
type VPCSpec struct {
	Subnet string  `json:"subnet,omitempty"`
	DHCP   VPCDHCP `json:"dhcp,omitempty"`
}

type VPCDHCP struct {
	Enable bool          `json:"enable,omitempty"`
	Range  *VPCDHCPRange `json:"range,omitempty"`
}

type VPCDHCPRange struct {
	Start *string `json:"start,omitempty"`
	End   *string `json:"end,omitempty"`
}

// VPCStatus defines the observed state of VPC
type VPCStatus struct {
	VLAN uint16 `json:"vlan,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VPC is the Schema for the vpcs API
type VPC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCSpec   `json:"spec,omitempty"`
	Status VPCStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VPCList contains a list of VPC
type VPCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPC{}, &VPCList{})
}
