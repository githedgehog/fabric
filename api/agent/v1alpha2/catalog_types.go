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

// CatalogSpec defines the desired state of Catalog
type CatalogSpec struct {
	// Global

	// ConnectionSystemIDs stores connection name -> ID, globally unique for the fabric
	ConnectionIDs map[string]uint32 `json:"connectionIDs,omitempty"`
	// VPCVNIs stores VPC name -> VPC VNI, globally unique for the fabric
	VPCVNIs map[string]uint32 `json:"vpcVNIs,omitempty"`
	// VPCSubnetVNIs stores VPC name -> subnet name -> VPC Subnet VNI, globally unique for the fabric
	VPCSubnetVNIs map[string]map[string]uint32 `json:"vpcSubnetVNIs,omitempty"`

	// Per redundancy group (or switch if no redundancy group)

	// IRBVLANs stores VPC name -> IRB VLAN ID, unique per redundancy group (or switch)
	IRBVLANs map[string]uint16 `json:"irbVLANs,omitempty"`
	// PortChannelIDs stores Connection name -> PortChannel ID, unique per redundancy group (or switch)
	PortChannelIDs map[string]uint16 `json:"portChannelIDs,omitempty"`

	// Per switch

	// LoopbackWorkaroundLinks stores loopback workaround "request" name (vpc@<vpc-peering> or ext@<external-peering>) -> loopback link name (<port1--port2>), unique per switch
	LooopbackWorkaroundLinks map[string]string `json:"loopbackWorkaroundLinks,omitempty"`
	// LoopbackWorkaroundVLANs stores loopback workaround "request" -> VLAN ID, unique per switch
	LoopbackWorkaroundVLANs map[string]uint16 `json:"loopbackWorkaroundVLANs,omitempty"`
	// ExternalIDs stores external name -> ID, unique per switch
	ExternalIDs map[string]uint16 `json:"externalIDs,omitempty"`
	// SubnetIDs stores subnet -> ID, unique per switch
	SubnetIDs map[string]uint32 `json:"subnetIDs,omitempty"`
}

// CatalogStatus defines the observed state of Catalog
type CatalogStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Catalog is the Schema for the catalogs API
type Catalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CatalogSpec   `json:"spec,omitempty"`
	Status CatalogStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CatalogList contains a list of Catalog
type CatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Catalog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Catalog{}, &CatalogList{})
}

func (c *CatalogSpec) GetVPCSubnetVNI(vpc, subnet string) (uint32, bool) {
	if c.VPCSubnetVNIs[vpc] == nil {
		return 0, false
	}

	vni, exists := c.VPCSubnetVNIs[vpc][subnet]

	return vni, exists
}
