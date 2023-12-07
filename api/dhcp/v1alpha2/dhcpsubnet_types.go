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

// DHCPSubnetSpec defines the desired state of DHCPSubnet
type DHCPSubnetSpec struct {
	Subnet    string `json:"subnet"`    // e.g. vpc-0/default (vpc name + vpc subnet name)
	CIDRBlock string `json:"cidrBlock"` // e.g. 10.10.10.0/24
	Gateway   string `json:"gateway"`   // e.g. 10.10.10.1
	StartIP   string `json:"startIP"`   // e.g. 10.10.10.10
	EndIP     string `json:"endIP"`     // e.g. 10.10.10.99
	VRF       string `json:"vrf"`       // e.g. VrfVvpc-1 as it's named on switch
	CircuitID string `json:"circuitID"` // e.g. Vlan1000 as it's named on switch
}

// DHCPSubnetStatus defines the observed state of DHCPSubnet
type DHCPSubnetStatus struct {
	AllocatedIPs map[string]DHCPAllocatedIP `json:"allocatedIPs,omitempty"`
}

type DHCPAllocatedIP struct {
	Expiry   metav1.Time `json:"expiry"`
	MAC      string      `json:"mac"`
	Hostname string      `json:"hostname"` // from dhcp request
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog;fabric,shortName=dhcp
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnet`,priority=0
// +kubebuilder:printcolumn:name="CIDRBlock",type=string,JSONPath=`.spec.cidrBlock`,priority=0
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.spec.gateway`,priority=0
// +kubebuilder:printcolumn:name="StartIP",type=string,JSONPath=`.spec.startIP`,priority=0
// +kubebuilder:printcolumn:name="EndIP",type=string,JSONPath=`.spec.endIP`,priority=0
// +kubebuilder:printcolumn:name="VRF",type=string,JSONPath=`.spec.vrf`,priority=1
// +kubebuilder:printcolumn:name="CircuitID",type=string,JSONPath=`.spec.circuitID`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// DHCPSubnet is the Schema for the dhcpsubnets API
type DHCPSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DHCPSubnetSpec   `json:"spec,omitempty"`
	Status DHCPSubnetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DHCPSubnetList contains a list of DHCPSubnet
type DHCPSubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DHCPSubnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DHCPSubnet{}, &DHCPSubnetList{})
}
