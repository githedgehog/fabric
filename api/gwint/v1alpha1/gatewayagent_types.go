// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type VPCInfoData struct {
	gwapi.VPCInfoSpec   `json:",inline"`
	gwapi.VPCInfoStatus `json:",inline"`
}

type GatewaySpecAnn struct {
	gwapi.GatewaySpec `json:",inline"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

type PeeringSpecAnn struct {
	gwapi.PeeringSpec `json:",inline"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

type GatewayGroupInfo struct {
	// TODO inline gateway group config when it's added
	Members []GatewayGroupMember `json:"members,omitempty"`
}

type GatewayGroupMember struct {
	Name     string `json:"name"`
	Priority uint32 `json:"priority"`
	VTEPIP   string `json:"vtepIP"`
}

// GatewayAgentSpec defines the desired state of GatewayAgent.
type GatewayAgentSpec struct {
	// AgentVersion is the desired version of the gateway agent to trigger generation changes on controller upgrades
	AgentVersion string                      `json:"agentVersion,omitempty"`
	Gateway      GatewaySpecAnn              `json:"gateway,omitempty"`
	VPCs         map[string]VPCInfoData      `json:"vpcs,omitempty"`
	Peerings     map[string]PeeringSpecAnn   `json:"peerings,omitempty"`
	Groups       map[string]GatewayGroupInfo `json:"groups,omitempty"`
	Communities  map[string]string           `json:"communities,omitempty"`
	Config       GatewayAgentSpecConfig      `json:"config,omitempty"`
}

type GatewayAgentSpecConfig struct {
	// FabricBFD defines if fabric-facing links should be configured with BFD
	FabricBFD bool `json:"fabricBFD,omitempty"`
}

// GatewayAgentStatus defines the observed state of GatewayAgent.
type GatewayAgentStatus struct {
	// AgentVersion is the version of the gateway agent
	AgentVersion string `json:"agentVersion,omitempty"`
	// Time of the last successful configuration application
	LastAppliedTime kmetav1.Time `json:"lastAppliedTime,omitempty"`
	// Generation of the last successful configuration application
	LastAppliedGen int64 `json:"lastAppliedGen,omitempty"`
	// Time of the last heartbeat from the agent
	LastHeartbeat kmetav1.Time `json:"lastHeartbeat,omitempty"`
	// State represents collected data from the dataplane API that includes FRR as well
	State GatewayState `json:"state,omitempty"`
}

// GatewayState represents collected data from the dataplane API that includes FRR as well
type GatewayState struct {
	// LastCollectedTime is the time of the last successful collection of data from the dataplane API
	LastCollectedTime kmetav1.Time `json:"lastCollectedTime,omitempty"`
	// Dataplane is the status of the dataplane
	Dataplane DataplaneStatus `json:"dataplane,omitempty"`
	// FRR is the status of the FRR daemon
	FRR FRRStatus `json:"frr,omitempty"`
	// VPCs is the status of the VPCs where key is the vpc (vpcinfo) name
	VPCs map[string]VPCStatus `json:"vpcs,omitempty"`
	// Peerings is the status of the VPCs peerings where key is VPC1->VPC2 and data is for one direction only
	Peerings map[string]PeeringStatus `json:"peerings,omitempty"`
	// BGP is BGP status
	BGP BGPStatus `json:"bgp,omitempty"`
	// Interfaces state, keyed by the spec interface name
	Interfaces map[string]GatewayStateInterface `json:"interfaces,omitempty"`
	// Transceivers state, keyed by the spec interface name
	Transceivers map[string]GatewayStateTransceiver `json:"transceivers,omitempty"`
}

// The gateway state types below mirror the switch ones in the agent API (same JSON names and units), but are kept
// separate so that switch-side changes don't alter the GatewayAgent CRD.

// +kubebuilder:validation:Enum=up;down;testing
type InterfaceAdminStatus string

const (
	InterfaceAdminStatusUp      InterfaceAdminStatus = "up"
	InterfaceAdminStatusDown    InterfaceAdminStatus = "down"
	InterfaceAdminStatusTesting InterfaceAdminStatus = "testing"
)

// +kubebuilder:validation:Enum=up;down;testing;unknown;dormant;notPresent;lowerLayerDown
type InterfaceOperStatus string

const (
	InterfaceOperStatusUp             InterfaceOperStatus = "up"
	InterfaceOperStatusDown           InterfaceOperStatus = "down"
	InterfaceOperStatusTesting        InterfaceOperStatus = "testing"
	InterfaceOperStatusUnknown        InterfaceOperStatus = "unknown"
	InterfaceOperStatusDormant        InterfaceOperStatus = "dormant"
	InterfaceOperStatusNotPresent     InterfaceOperStatus = "notPresent"
	InterfaceOperStatusLowerLayerDown InterfaceOperStatus = "lowerLayerDown"
)

type GatewayStateInterface struct {
	AdminStatus InterfaceAdminStatus `json:"admin,omitempty"`
	OperStatus  InterfaceOperStatus  `json:"oper,omitempty"`
	MAC         string               `json:"mac,omitempty"`
	// When the oper status last changed
	LastChange *kmetav1.Time `json:"change,omitempty"`
	// Link speed, e.g. 2.5G, 25G, 100G
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?[MGT]$`
	Speed         string `json:"speed,omitempty"`
	AutoNegotiate bool   `json:"auto,omitempty"`
	// FEC mode, e.g. rs, fc, disabled
	FEC           string                         `json:"fec,omitempty"`
	Counters      *GatewayStateInterfaceCounters `json:"counters,omitempty"`
	LLDPNeighbors []GatewayStateLLDPNeighbor     `json:"lldpNeighbors,omitempty"`
	// Number of link flaps
	// +kubebuilder:validation:Format=int64
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	LinkFlapCount *uint32 `json:"linkFlapCount,omitempty"`
}

// GatewayStateInterfaceCounters are in bits, not bytes, like the switch ones
type GatewayStateInterfaceCounters struct {
	InBits           uint64  `json:"inb,omitempty"`
	InBitsPerSecond  float64 `json:"inbps,omitempty"`
	InDiscards       uint64  `json:"ind,omitempty"`
	InErrors         uint64  `json:"ine,omitempty"`
	InPktsPerSecond  float64 `json:"inpps,omitempty"`
	OutBits          uint64  `json:"outb,omitempty"`
	OutBitsPerSecond float64 `json:"outbps,omitempty"`
	OutDiscards      uint64  `json:"outd,omitempty"`
	OutErrors        uint64  `json:"oute,omitempty"`
	OutPktsPerSecond float64 `json:"outpps,omitempty"`
	// When the counters were last reset, e.g. on dataplane start
	LastClear *kmetav1.Time `json:"clear,omitempty"`
}

type GatewayStateTransceiver struct {
	// Human-readable module type, e.g. "OSFP112 800GBASE-DR8" or "SFP+ 10GBASE-LR"
	Description string `json:"descr,omitempty"`
	// Cable class, e.g. DAC or FIBER
	CableClass string `json:"cable,omitempty"`
	// OpenConfig form factor, e.g. SFP_PLUS, OSFP112
	FormFactor string `json:"formFactor,omitempty"`
	// OpenConfig connector type, e.g. LC_CONNECTOR, MPO_2x12_CONNECTOR, NO_SEPARABLE_CONNECTOR (DACs)
	ConnectorType string `json:"connType,omitempty"`
	// PRESENT or NOT_PRESENT
	Present string `json:"present,omitempty"`
	// Cable length in meters
	CableLength float64 `json:"length,omitempty"`
	// active, inactive or disabled
	OperStatus string `json:"oper,omitempty"`
	// Module temperature in Celsius
	Temperature float64 `json:"temp,omitempty"`
	// Module supply voltage in volts
	Voltage      float64 `json:"voltage,omitempty"`
	SerialNumber string  `json:"serial,omitempty"`
	Vendor       string  `json:"vendor,omitempty"`
	VendorPart   string  `json:"vendorPart,omitempty"`
	VendorOUI    string  `json:"vendorOUI,omitempty"`
	VendorRev    string  `json:"vendorRev,omitempty"`
	Firmware     string  `json:"firmware,omitempty"`
	// CMIS module state, e.g. Ready; CMIS modules only
	CMISStatus string `json:"cmis,omitempty"`
	CMISRev    string `json:"cmisRev,omitempty"`
	// Active CMIS application selection code
	CMISApp uint8 `json:"cmisApp,omitempty"`
	// Per-lane state, keyed by the lane index starting from 0
	Channels map[string]GatewayStateTransceiverChannel `json:"channels,omitempty"`
}

type GatewayStateTransceiverChannel struct {
	// Input (rx) power in dBm
	In *float64 `json:"in,omitempty"`
	// Output (tx) power in dBm
	Out *float64 `json:"out,omitempty"`
	// Laser bias current in mA
	Bias float64 `json:"bias,omitempty"`
}

type GatewayStateLLDPNeighbor struct {
	ChassisID         string `json:"chassis,omitempty"`
	SystemName        string `json:"sysName,omitempty"`
	SystemDescription string `json:"sysDescr,omitempty"`
	PortID            string `json:"portID,omitempty"`
	PortDescription   string `json:"portDescr,omitempty"`

	// TTL advertised by the neighbor, seconds
	TTL uint16 `json:"ttl,omitempty"`
	// When the neighbor entry was last updated
	LastUpdate *kmetav1.Time `json:"updated,omitempty"`

	// LLDP-MED inventory

	Manufacturer string `json:"manuf,omitempty"`
	Model        string `json:"model,omitempty"`
	SerialNumber string `json:"serial,omitempty"`

	// Derived (best-effort) values

	// MAC of the neighbor port: the LLDP source MAC, or the port ID if it's a MAC address
	MAC string `json:"mac,omitempty"`
	// Human-readable neighbor port name: the port ID, or the port description if the port ID is a MAC address
	Port string `json:"port,omitempty"`
}

// DataplaneStatus represents the status of the dataplane
type DataplaneStatus struct {
	Version string `json:"version,omitempty"`
}

// FRRStatus represents the status of the FRR daemon
type FRRStatus struct {
	// LastAppliedGen is the generation of the last successful application of a configuration to the FRR
	LastAppliedGen int64 `json:"lastAppliedGen,omitempty"`
}

type VPCStatus struct {
	// Packets is the number of packets sent on the vpc
	Packets uint64 `json:"p,omitempty"`
	// Bytes is the number of bytes sent on the vpc
	Bytes uint64 `json:"b,omitempty"`
	// Drops is the number of packets dropped on the vpc
	Drops uint64 `json:"d,omitempty"`
}

// PeeringStatus represents the status of a peering between a pair of VPCs in one direction
type PeeringStatus struct {
	// Packets is the number of packets sent on the peering
	Packets uint64 `json:"p,omitempty"`
	// Bytes is the number of bytes sent on the peering
	Bytes uint64 `json:"b,omitempty"`
	// Drops is the number of packets dropped on the peering
	Drops uint64 `json:"d,omitempty"`
	// BytesPerSecond is the number of bytes sent per second on the peering
	BytesPerSecond float64 `json:"bps,omitempty"`
	// PktsPerSecond is the number of packets sent per second on the peering
	PktsPerSecond float64 `json:"pps,omitempty"`
}

// BGPStatus represents BGP status across VRFs, derived from BMP/FRR.
type BGPStatus struct {
	// VRFs keyed by VRF name (e.g. "default", "vrfVvpc-1")
	VRFs map[string]BGPVRFStatus `json:"vrfs,omitempty"`
}

type BGPVRFStatus struct {
	// Neighbors keyed by an ip address string
	Neighbors map[string]BGPNeighborStatus `json:"neighbors,omitempty"`
}

// BGPNeighborSessionState represents the BGP FSM state for a neighbor.
// +kubebuilder:validation:Enum=unset;idle;connect;active;open;established
type BGPNeighborSessionState string

const (
	BGPStateUnset       BGPNeighborSessionState = "unset"
	BGPStateIdle        BGPNeighborSessionState = "idle"
	BGPStateConnect     BGPNeighborSessionState = "connect"
	BGPStateActive      BGPNeighborSessionState = "active"
	BGPStateOpen        BGPNeighborSessionState = "open"
	BGPStateEstablished BGPNeighborSessionState = "established"
)

type BGPNeighborStatus struct {
	Enabled bool `json:"enabled,omitempty"`
	// +kubebuilder:validation:Format=int64
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	LocalAS uint32 `json:"localAS,omitempty"`
	// +kubebuilder:validation:Format=int64
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	PeerAS         uint32 `json:"peerAS,omitempty"`
	RemoteRouterID string `json:"remoteRouterID,omitempty"`

	SessionState           BGPNeighborSessionState `json:"sessionState,omitempty"`
	ConnectionsDropped     uint64                  `json:"connectionsDropped,omitempty"`
	EstablishedTransitions uint64                  `json:"establishedTransitions,omitempty"`
	LastResetReason        string                  `json:"lastResetReason,omitempty"`

	Messages            BGPMessages         `json:"messages,omitempty"`
	IPv4UnicastPrefixes BGPNeighborPrefixes `json:"ipv4UnicastPrefixes,omitempty"`
	IPv6UnicastPrefixes BGPNeighborPrefixes `json:"ipv6UnicastPrefixes,omitempty"`
	L2VPNEVPNPrefixes   BGPNeighborPrefixes `json:"l2VPNEVPNPrefixes,omitempty"`
}

type BGPMessages struct {
	Received BGPMessageCounters `json:"received,omitempty"`
	Sent     BGPMessageCounters `json:"sent,omitempty"`
}

type BGPMessageCounters struct {
	Capability   uint64 `json:"capability,omitempty"`
	Keepalive    uint64 `json:"keepalive,omitempty"`
	Notification uint64 `json:"notification,omitempty"`
	Open         uint64 `json:"open,omitempty"`
	RouteRefresh uint64 `json:"routeRefresh,omitempty"`
	Update       uint64 `json:"update,omitempty"`
}

type BGPNeighborPrefixes struct {
	Received          uint32 `json:"received,omitempty"`
	ReceivedPrePolicy uint32 `json:"receivedPrePolicy,omitempty"`
	Sent              uint32 `json:"sent,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hhint;hhfcint,shortName=gwag
// +kubebuilder:printcolumn:name="Topo",type=string,JSONPath=`.metadata.annotations.fabric\.githedgehog\.com/topology`,priority=0
// +kubebuilder:printcolumn:name="Driver",type=string,JSONPath=`.spec.gateway.driver`,priority=0
// +kubebuilder:printcolumn:name="Workers",type=integer,JSONPath=`.spec.gateway.workers`,priority=1
// +kubebuilder:printcolumn:name="Heartbeat",type=date,JSONPath=`.status.lastHeartbeat`,priority=0
// +kubebuilder:printcolumn:name="Applied",type=date,JSONPath=`.status.lastAppliedTime`,priority=0
// +kubebuilder:printcolumn:name="AppliedG",type=integer,JSONPath=`.status.lastAppliedGen`,priority=0
// +kubebuilder:printcolumn:name="CurrentG",type=integer,JSONPath=`.metadata.generation`,priority=0
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.state.dataplane.version`,priority=0
// +kubebuilder:printcolumn:name="VTEPIP",type=string,JSONPath=`.spec.gateway.vtepIP`,priority=1
// +kubebuilder:printcolumn:name="ProtoIP",type=string,JSONPath=`.spec.gateway.protocolIP`,priority=2
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// GatewayAgent is the Schema for the gatewayagents API.
type GatewayAgent struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// +structType=atomic
	Spec GatewayAgentSpec `json:"spec,omitempty"`

	// +structType=atomic
	Status GatewayAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayAgentList contains a list of GatewayAgent.
type GatewayAgentList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []GatewayAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &GatewayAgent{}, &GatewayAgentList{})

		return nil
	})
}
