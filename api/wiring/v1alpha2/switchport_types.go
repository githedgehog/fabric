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

// +kubebuilder:validation:Enum=leaf-server-l3;leaf-server-l2-untagged;leaf-server-l2-tagged;leaf-service;leaf-fabric;leaf-border;spine-fabric;spine-border
type SwitchPortRole string

const (
	SwitchPortRoleLeafServerRouted         SwitchPortRole = "leaf-server-l3"
	SwitchPortRoleLeafServerSwitched       SwitchPortRole = "leaf-server-l2-untagged"
	SwitchPortRoleLeafServerTaggedSwitched SwitchPortRole = "leaf-server-l2-tagged"
	SwitchPortRoleLeafService              SwitchPortRole = "leaf-service"
	SwitchPortRoleLeafUntaggedFabric       SwitchPortRole = "leaf-fabric"
	SwitchPortRoleLeafBorder               SwitchPortRole = "leaf-border"
	SwitchPortRoleSpineUntaggedFabric      SwitchPortRole = "spine-fabric"
	SwitchPortRoleSpineBorder              SwitchPortRole = "spine-border"
)

// +kubebuilder:validation:Enum=copper;optical
type CableType string

const (
	CableTypeCopperCable  CableType = "copper"
	CableTypeOpticalCable CableType = "optical"
)

// +kubebuilder:validation:Enum=untagged-l2;tagged-l2;routed
type InterfaceMode string

const (
	InterfaceModeUntaggedL2 InterfaceMode = "untagged-l2"
	InterfaceModeTaggedL2   InterfaceMode = "tagged-l2"
	InterfaceModeRouted     InterfaceMode = "routed"
)

// Neighbor represents the neighbor of a particular port
// which could be either be a Switch or Server
type Neighbor struct {
	Switch *NeighborInfo `json:"switch,omitempty"`
	Server *NeighborInfo `json:"server,omitempty"`
}

type NeighborInfo struct {
	Name string `json:"name,omitempty"`
	Port string `json:"port,omitempty"`
}

func (n Neighbor) Port() string {
	if n.Server != nil {
		return n.Server.Name
	}
	if n.Switch != nil {
		return n.Switch.Name
	}

	return ""
}

// Interfaces are pseudo ports ( vlan interfaces,subinterfaces). They
// always have a parent Port.
type Interface struct {
	Name       string        `json:"name,omitempty"`
	VLANs      []uint16      `json:"vlans,omitempty"`
	IPAddress  string        `json:"ipAddress,omitempty"`
	BGPEnabled bool          `json:"bgpEnabled,omitempty"`
	BFDEnabled bool          `json:"bfdEnabled,omitempty"`
	VRF        string        `json:"vrf,omitempty"`
	Mode       InterfaceMode `json:"mode,omitempty"`
	Bundle     string        `json:"bundle,omitempty"`
}

// ONIEConfig holds all the port configuration at installation/ONIE time.
// They are being consumed by the seeder (DAS BOOT).
type ONIEConfig struct {
	PortNum     uint16       `json:"portNum,omitempty"`
	PortName    string       `json:"portName,omitempty"`
	BootstrapIP string       `json:"bootstrapIP,omitempty"`
	VLAN        uint16       `json:"vlan,omitempty"`
	Routes      []ONIERoutes `json:"routes,omitempty"`
}

// ONIERoutes holds additional routing information to be applied in ONIE at installation/ONIE time.
// They are being consumed by the seeder (DAS BOOT).
type ONIERoutes struct {
	Destinations []string `json:"destinations,omitempty"`
	Gateway      string   `json:"gateway,omitempty"`
}

// SwitchPortSpec is the model used to represent a switch port
type SwitchPortSpec struct {
	Role          SwitchPortRole `json:"role,omitempty"`
	IsConnected   bool           `json:"isConnected,omitempty"`
	NOSPortNum    uint16         `json:"nosPortNum,omitempty"`
	NOSPortName   string         `json:"nosPortName,omitempty"`
	PortSpeed     string         `json:"portSpeed,omitempty"`
	ConnectorType string         `json:"connectorType,omitempty"`
	CableType     CableType      `json:"cableType,omitempty"`
	Neighbor      Neighbor       `json:"neighbor,omitempty"`
	ONIE          ONIEConfig     `json:"onie,omitempty"`
	Interfaces    []Interface    `json:"interfaces,omitempty"`
	AdminState    string         `json:"adminState,omitempty"`
	VRF           string         `json:"vrf,omitempty"`
}

// SwitchPortStatus defines the observed state of Port
type SwitchPortStatus struct {
	// TODO: add port status fields
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SwitchPort is the Schema for the ports API
//
// All ports should always have 2 labels defined: wiring.githedgehog.com/rack and wiring.githedgehog.com/switch. It
// represents names of the rack and switch it belongs to.
type SwitchPort struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchPortSpec   `json:"spec,omitempty"`
	Status SwitchPortStatus `json:"status,omitempty"`
}

const KindSwitchPort = "SwitchPort"

//+kubebuilder:object:root=true

// PortList contains a list of Port
type SwitchPortList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SwitchPort `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SwitchPort{}, &SwitchPortList{})
}

func (p *SwitchPort) GetSwitchName() string {
	return p.Labels[LabelSwitch]
}
