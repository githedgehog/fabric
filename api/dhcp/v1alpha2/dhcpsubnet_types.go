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

const (
	ManagementSubnet = "management"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DHCPSubnetSpec defines the desired state of DHCPSubnet
type DHCPSubnetSpec struct {
	// Full VPC subnet name (including VPC name), such as "vpc-0/default"
	Subnet string `json:"subnet"`
	// CIDR block to use for VPC subnet, such as "10.10.10.0/24"
	CIDRBlock string `json:"cidrBlock"`
	// Gateway, such as 10.10.10.1
	Gateway string `json:"gateway"`
	// Start IP from the CIDRBlock to allocate IPs, such as 10.10.10.10
	StartIP string `json:"startIP"`
	// End IP from the CIDRBlock to allocate IPs, such as 10.10.10.99
	EndIP string `json:"endIP"`
	// VRF name to identify specific VPC (will be added to DHCP packets by DHCP relay in suboption 151), such as "VrfVvpc-1" as it's named on switch
	VRF string `json:"vrf"`
	// VLAN ID to identify specific subnet within the VPC, such as "Vlan1000" as it's named on switch
	CircuitID string `json:"circuitID"`
	// PXEURL (optional) to identify the pxe server to use to boot hosts connected to this segment such as http://10.10.10.99/bootfilename or tftp://10.10.10.99/bootfilename, http query strings are not supported
	PXEURL string `json:"pxeURL"`
	// DNSservers (optional) to configure Domain Name Servers for this particular segment such as: 10.10.10.1, 10.10.10.2
	DNSServers []string `json:"dnsServers"`
	// TimeServers (optional) NTP server addresses to configure for time servers for this particular segment such as: 10.10.10.1, 10.10.10.2
	TimeServers []string `json:"timeServers"`
	// InterfaceMTU (optional) is the MTU setting that the dhcp server will send to the clients. It is dependent on the client to honor this option.
	// +kubebuilder:validation:Minimum: 96
	// +kubebuilder:validation:Maximum: 9036
	InterfaceMTU uint16 `json:"interfaceMTU"`
	// DefaultURL (optional) is the option 114 "default-url" to be sent to the clients
	DefaultURL string `json:"defaultURL"`
}

// DHCPSubnetStatus defines the observed state of DHCPSubnet
type DHCPSubnetStatus struct {
	// Allocated is a map of allocated IPs with expiry time and hostname from DHCP requests
	Allocated map[string]DHCPAllocated `json:"allocated,omitempty"`
}

// DHCPAllocated is a single allocated IP with expiry time and hostname from DHCP requests, it's effectively a DHCP lease
type DHCPAllocated struct {
	// Allocated IP address
	IP string `json:"ip"`
	// Expiry time of the lease
	Expiry metav1.Time `json:"expiry"`
	// Hostname from DHCP request
	Hostname string `json:"hostname"`
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
// +kubebuilder:printcolumn:name="DNSServers",type=string,JSONPath=`.spec.dnsServers`,priority=1
// +kubebuilder:printcolumn:name="TimeServers",type=string,JSONPath=`.spec.timeServers`,priority=1
// +kubebuilder:printcolumn:name="InterfaceMTU",type=integer,JSONPath=`.spec.interfaceMTU`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=0
// DHCPSubnet is the configuration (spec) for the Hedgehog DHCP server and storage for the leases (status). It's
// primary internal API group, but it makes allocated IPs / leases information available to the end user through API.
// Not intended to be modified by the user.
type DHCPSubnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired state of the DHCPSubnet
	Spec DHCPSubnetSpec `json:"spec,omitempty"`
	// Status is the observed state of the DHCPSubnet
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
