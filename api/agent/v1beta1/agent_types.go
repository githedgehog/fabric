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

package v1beta1

import (
	"slices"
	"sort"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO do we need to create user inputable AgentAction CRD with version override, reinstall and reboot requests?

// AgentSpec defines the desired state of the Agent and includes all relevant information required to fully configure
// the switch and manage its lifecycle. It is not intended to be used directly by users.
type AgentSpec struct {
	Role                 wiringapi.SwitchRole                     `json:"role,omitempty"`
	Description          string                                   `json:"description,omitempty"`
	Config               AgentSpecConfig                          `json:"config,omitempty"`
	Alloy                meta.AlloyConfig                         `json:"alloy,omitempty"`
	Version              AgentVersion                             `json:"version,omitempty"`
	Users                []UserCreds                              `json:"users,omitempty"`
	Switch               wiringapi.SwitchSpec                     `json:"switch,omitempty"`
	SwitchProfile        *wiringapi.SwitchProfileSpec             `json:"switchProfile,omitempty"`
	Switches             map[string]wiringapi.SwitchSpec          `json:"switches,omitempty"`
	RedundancyGroupPeers []string                                 `json:"redundancyGroupPeers,omitempty"`
	Connections          map[string]wiringapi.ConnectionSpec      `json:"connections,omitempty"`
	VPCs                 map[string]vpcapi.VPCSpec                `json:"vpcs,omitempty"`
	VPCAttachments       map[string]vpcapi.VPCAttachmentSpec      `json:"vpcAttachments,omitempty"`
	VPCPeerings          map[string]vpcapi.VPCPeeringSpec         `json:"vpcPeers,omitempty"`
	IPv4Namespaces       map[string]vpcapi.IPv4NamespaceSpec      `json:"ipv4Namespaces,omitempty"`
	VLANNamespaces       map[string]wiringapi.VLANNamespaceSpec   `json:"vlanNamespaces,omitempty"`
	Externals            map[string]vpcapi.ExternalSpec           `json:"externals,omitempty"`
	ExternalAttachments  map[string]vpcapi.ExternalAttachmentSpec `json:"externalAttachments,omitempty"`
	ExternalPeerings     map[string]vpcapi.ExternalPeeringSpec    `json:"externalPeerings,omitempty"`
	ConfiguredVPCSubnets map[string]bool                          `json:"configuredVPCSubnets,omitempty"`
	AttachedVPCs         map[string]bool                          `json:"attachedVPCs,omitempty"`
	Reinstall            string                                   `json:"reinstall,omitempty"`  // set to InstallID to reinstall NOS
	Reboot               string                                   `json:"reboot,omitempty"`     // set to RunID to reboot
	PowerReset           string                                   `json:"powerReset,omitempty"` // set to RunID to power reset
	Catalog              CatalogSpec                              `json:"catalog,omitempty"`

	// TODO impl
	StatusUpdates []ApplyStatusUpdate `json:"statusUpdates,omitempty"`
}

type AgentSpecConfig struct {
	DeploymentID          string                        `json:"deploymentID,omitempty"`
	ControlVIP            string                        `json:"controlVIP,omitempty"`
	VPCPeeringDisabled    bool                          `json:"vpcPeeringDisabled,omitempty"`
	CollapsedCore         *AgentSpecConfigCollapsedCore `json:"collapsedCore,omitempty"`
	SpineLeaf             *AgentSpecConfigSpineLeaf     `json:"spineLeaf,omitempty"`
	BaseVPCCommunity      string                        `json:"baseVPCCommunity,omitempty"`
	VPCLoopbackSubnet     string                        `json:"vpcLoopbackSubnet,omitempty"`
	FabricMTU             uint16                        `json:"fabricMTU,omitempty"`
	ServerFacingMTUOffset uint16                        `json:"serverFacingMTUOffset,omitempty"`
	ESLAGMACBase          string                        `json:"eslagMACBase,omitempty"`
	ESLAGESIPrefix        string                        `json:"eslagESIPrefix,omitempty"`
	DefaultMaxPathsEBGP   uint32                        `json:"defaultMaxPathsEBGP,omitempty"`
	MCLAGSessionSubnet    string                        `json:"mclagSessionSubnet,omitempty"`
	GatewayASN            uint32                        `json:"gatewayASN,omitempty"`
	LoopbackWorkaround    bool                          `json:"loopbackWorkaround,omitempty"`
	ProtocolSubnet        string                        `json:"protocolSubnet,omitempty"`
	VTEPSubnet            string                        `json:"vtepSubnet,omitempty"`
	FabricSubnet          string                        `json:"fabricSubnet,omitempty"`
	DisableBFD            bool                          `json:"disableBFD,omitempty"`
}

type AgentSpecConfigCollapsedCore struct{}

type AgentSpecConfigSpineLeaf struct{}

func (a *Agent) IsCollapsedCore() bool {
	return a != nil && a.Spec.Config.CollapsedCore != nil
}

func (a *Agent) IsSpineLeaf() bool {
	return a != nil && a.Spec.Config.SpineLeaf != nil
}

// TODO rename?
type AgentVersion struct {
	// Registry information
	CA       string `json:"ca,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Agent information
	Repo     string `json:"repo,omitempty"`
	Default  string `json:"default,omitempty"`
	Override string `json:"override,omitempty"`

	// Alloy information
	AlloyRepo    string `json:"alloyRepo,omitempty"`
	AlloyVersion string `json:"alloyVersion,omitempty"`
}

type UserCreds struct {
	Name     string   `json:"name,omitempty"`
	Password string   `json:"password,omitempty"`
	Role     string   `json:"role,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
}

type ApplyStatusUpdate struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Generation int64  `json:"generation,omitempty"`
}

// TODO replace flat attempl/apply in agent status with ApplyStatus

// AgentStatus defines the observed state of the agent running on a specific switch and includes information about the
// switch itself as well as the state of the agent and applied configuration.
type AgentStatus struct {
	// Current running agent version
	Version string `json:"version,omitempty"`
	// ID of the agent installation, used to track NOS re-installs
	InstallID string `json:"installID,omitempty"`
	// ID of the agent run, used to track agent restarts
	RunID string `json:"runID,omitempty"`
	// ID for the NOS boot, used to track NOS reboots
	BootID string `json:"bootID,omitempty"`
	// Time of the last heartbeat from the agent
	LastHeartbeat kmetav1.Time `json:"lastHeartbeat,omitempty"`
	// Time of the last attempt to apply configuration
	LastAttemptTime kmetav1.Time `json:"lastAttemptTime,omitempty"`
	// Generation of the last attempt to apply configuration
	LastAttemptGen int64 `json:"lastAttemptGen,omitempty"`
	// Time of the last successful configuration application
	LastAppliedTime kmetav1.Time `json:"lastAppliedTime,omitempty"`
	// Generation of the last successful configuration application
	LastAppliedGen int64 `json:"lastAppliedGen,omitempty"`
	// Detailed switch state updated with each heartbeat
	State SwitchState `json:"state,omitempty"`
	// Status updates from the agent
	StatusUpdates []ApplyStatusUpdate `json:"statusUpdates,omitempty"`
	// Conditions of the agent, includes readiness marker for use with kubectl wait
	Conditions []kmetav1.Condition `json:"conditions"`
}

type SwitchState struct {
	// Information about the switch and NOS
	NOS SwitchStateNOS `json:"nos,omitempty"`
	// Switch interfaces state (incl. physical, management and port channels)
	Interfaces map[string]SwitchStateInterface `json:"interfaces,omitempty"`
	// Breakout ports state (port -> breakout state)
	Breakouts map[string]SwitchStateBreakout `json:"breakouts,omitempty"`
	// Transceivers state (port -> transceiver state)
	Transceivers map[string]SwitchStateTransceiver `json:"transceivers,omitempty"`
	// State of all BGP neighbors (VRF -> neighbor address -> state)
	BGPNeighbors map[string]map[string]SwitchStateBGPNeighbor `json:"bgpNeighbors,omitempty"`
	// State of the switch platform (fans, PSUs, sensors)
	Platform SwitchStatePlatform `json:"platform,omitempty"`
	// State of the critical resources (ACLs, routes, etc.)
	CriticalResources SwitchStateCRM `json:"criticalResources,omitempty"`
	// State of the roce configuration
	RoCE bool `json:"roce,omitempty"`
	// Firmware versions of the switch
	Firmware map[string]string `json:"firmware,omitempty"`
}

type SwitchStateInterface struct {
	Enabled       bool                          `json:"enabled,omitempty"`
	AdminStatus   AdminStatus                   `json:"admin,omitempty"`
	OperStatus    OperStatus                    `json:"oper,omitempty"`
	MAC           string                        `json:"mac,omitempty"`
	LastChange    kmetav1.Time                  `json:"change,omitempty"`
	Speed         string                        `json:"speed,omitempty"`
	AutoNegotiate bool                          `json:"auto,omitempty"`
	FEC           string                        `json:"fec,omitempty"`
	Counters      *SwitchStateInterfaceCounters `json:"counters,omitempty"`
	LLDPNeighbors []SwitchStateLLDPNeighbor     `json:"lldpNeighbors,omitempty"`
}

type SwitchStateInterfaceCounters struct {
	InBits           uint64                                       `json:"inb,omitempty"`
	InBitsPerSecond  float64                                      `json:"inbps,omitempty"`
	InDiscards       uint64                                       `json:"ind,omitempty"`
	InErrors         uint64                                       `json:"ine,omitempty"`
	InPktsPerSecond  float64                                      `json:"inpbs,omitempty"`
	InUtilization    uint8                                        `json:"inu,omitempty"`
	LastClear        kmetav1.Time                                 `json:"clear,omitempty"`
	OutBits          uint64                                       `json:"outb,omitempty"`
	OutBitsPerSecond float64                                      `json:"outbps,omitempty"`
	OutDiscards      uint64                                       `json:"outd,omitempty"`
	OutErrors        uint64                                       `json:"oute,omitempty"`
	OutPktsPerSecond float64                                      `json:"outpps,omitempty"`
	OutUtilization   uint8                                        `json:"outu,omitempty"`
	Queues           map[string]SwitchStateInterfaceCountersQueue `json:"queues,omitempty"`
}

type SwitchStateInterfaceCountersQueue struct {
	DroppedBits           uint64 `json:"db,omitempty"`
	DroppedPkts           uint64 `json:"dp,omitempty"`
	ECNMarkedBits         uint64 `json:"ecnb,omitempty"`
	ECNMarkedPkts         uint64 `json:"ecnp,omitempty"`
	TransmitBits          uint64 `json:"b,omitempty"`
	TransmitBitsPerSecond uint64 `json:"bps,omitempty"`
	TransmitPkts          uint64 `json:"p,omitempty"`
	TransmitPktsPerSecond uint64 `json:"pps,omitempty"`
	WREDDroppedPkts       uint64 `json:"wreddp,omitempty"`
}

type AdminStatus string

const (
	AdminStatusUnset   AdminStatus = ""
	AdminStatusUp      AdminStatus = "up"
	AdminStatusDown    AdminStatus = "down"
	AdminStatusTesting AdminStatus = "testing"
)

func (a AdminStatus) ID() (uint8, error) {
	switch a {
	case AdminStatusUnset:
		return 0, nil
	case AdminStatusUp:
		return 1, nil
	case AdminStatusDown:
		return 2, nil
	case AdminStatusTesting:
		return 3, nil
	default:
		return 0, errors.Errorf("unknown AdminStatus %s", a)
	}
}

type OperStatus string

const (
	OperStatusUnset          OperStatus = ""
	OperStatusUp             OperStatus = "up"
	OperStatusDown           OperStatus = "down"
	OperStatusTesting        OperStatus = "testing"
	OperStatusUnknown        OperStatus = "unknown"
	OperStatusDormant        OperStatus = "dormant"
	OperStatusNotPresent     OperStatus = "notPresent"
	OperStatusLowerLayerDown OperStatus = "lowerLayerDown"
)

func (o OperStatus) ID() (uint8, error) {
	switch o {
	case OperStatusUnset:
		return 0, nil
	case OperStatusUp:
		return 1, nil
	case OperStatusDown:
		return 2, nil
	case OperStatusTesting:
		return 3, nil
	case OperStatusUnknown:
		return 4, nil
	case OperStatusDormant:
		return 5, nil
	case OperStatusNotPresent:
		return 6, nil
	case OperStatusLowerLayerDown:
		return 7, nil
	default:
		return 0, errors.Errorf("unknown OperStatus %s", o)
	}
}

type SwitchStateTransceiver struct {
	Description   string                                   `json:"descr,omitempty"`
	CableClass    string                                   `json:"cable,omitempty"`
	FormFactor    string                                   `json:"formFactor,omitempty"`
	ConnectorType string                                   `json:"connType,omitempty"`
	Present       string                                   `json:"present,omitempty"`
	CableLength   float64                                  `json:"length,omitempty"`
	OperStatus    string                                   `json:"oper,omitempty"`
	Temperature   float64                                  `json:"temp,omitempty"`
	Voltage       float64                                  `json:"voltage,omitempty"`
	SerialNumber  string                                   `json:"serial,omitempty"`
	Vendor        string                                   `json:"vendor,omitempty"`
	VendorPart    string                                   `json:"vendorPart,omitempty"`
	VendorOUI     string                                   `json:"vendorOUI,omitempty"`
	VendorRev     string                                   `json:"vendorRev,omitempty"`
	Firmware      string                                   `json:"firmware,omitempty"`
	CMISStatus    string                                   `json:"cmis,omitempty"`
	CMISRev       string                                   `json:"cmisRev,omitempty"`
	CMISApp       uint8                                    `json:"cmisApp,omitempty"`
	Channels      map[string]SwitchStateTransceiverChannel `json:"channels,omitempty"`
}

type SwitchStateTransceiverChannel struct {
	In   *float64 `json:"in,omitempty"`
	Out  *float64 `json:"out,omitempty"`
	Bias float64  `json:"bias,omitempty"`
}

type SwitchStateBreakout struct {
	Mode   string `json:"mode,omitempty"`
	Status string `json:"status,omitempty"`
}

type SwitchStateLLDPNeighbor struct {
	Name              string `json:"name,omitempty"`
	ChassisID         string `json:"chassis,omitempty"`
	SystemName        string `json:"sysName,omitempty"`
	SystemDescription string `json:"sysDescr,omitempty"`
	PortID            string `json:"portID,omitempty"`
	PortDescription   string `json:"portDescr,omitempty"`

	// LLDP-MED inventory

	Manufacturer string `json:"manuf,omitempty"`
	Model        string `json:"model,omitempty"`
	SerialNumber string `json:"serial,omitempty"`
}

type SwitchStateBGPNeighbor struct {
	ConnectionsDropped     uint64                                    `json:"connDropped,omitempty"`
	Enabled                bool                                      `json:"enabled,omitempty"`
	EstablishedTransitions uint64                                    `json:"estabTrans,omitempty"`
	LastEstablished        kmetav1.Time                              `json:"lastEstab,omitempty"`
	LastRead               kmetav1.Time                              `json:"lastRead,omitempty"`
	LastResetReason        string                                    `json:"lastResetReason,omitempty"`
	LastResetTime          kmetav1.Time                              `json:"lastReset,omitempty"`
	LastWrite              kmetav1.Time                              `json:"lastWrite,omitempty"`
	LocalAS                uint32                                    `json:"localAS,omitempty"`
	Messages               BGPMessages                               `json:"msgs,omitempty"`
	PeerAS                 uint32                                    `json:"peerAS,omitempty"`
	PeerGroup              string                                    `json:"peerGroup,omitempty"`
	PeerPort               uint16                                    `json:"peerPort,omitempty"`
	PeerType               BGPPeerType                               `json:"peerType,omitempty"`
	RemoteRouterID         string                                    `json:"remoteRouterID,omitempty"`
	SessionState           BGPNeighborSessionState                   `json:"state,omitempty"`
	ShutdownMessage        string                                    `json:"shutdownMsg,omitempty"`
	Prefixes               map[string]SwitchStateBGPNeighborPrefixes `json:"prefixes,omitempty"`
}

type SwitchStateBGPNeighborPrefixes struct {
	Received          uint32 `json:"rec,omitempty"`
	ReceivedPrePolicy uint32 `json:"recPre,omitempty"`
	Sent              uint32 `json:"sent,omitempty"`
}

type BGPNeighborSessionState string

const (
	BGPNeighborSessionStateUnset       BGPNeighborSessionState = ""
	BGPNeighborSessionStateIdle        BGPNeighborSessionState = "idle"
	BGPNeighborSessionStateConnect     BGPNeighborSessionState = "connect"
	BGPNeighborSessionStateActive      BGPNeighborSessionState = "active"
	BGPNeighborSessionStateOpenSent    BGPNeighborSessionState = "openSent"
	BGPNeighborSessionStateOpenConfirm BGPNeighborSessionState = "openConfirm"
	BGPNeighborSessionStateEstablished BGPNeighborSessionState = "established"
)

func (b BGPNeighborSessionState) ID() (uint8, error) {
	switch b {
	case BGPNeighborSessionStateUnset:
		return 0, nil
	case BGPNeighborSessionStateIdle:
		return 1, nil
	case BGPNeighborSessionStateConnect:
		return 2, nil
	case BGPNeighborSessionStateActive:
		return 3, nil
	case BGPNeighborSessionStateOpenSent:
		return 4, nil
	case BGPNeighborSessionStateOpenConfirm:
		return 5, nil
	case BGPNeighborSessionStateEstablished:
		return 6, nil
	default:
		return 0, errors.Errorf("unknown BGPNeighborSessionState %s", b)
	}
}

type BGPPeerType string

const (
	BGPPeerTypeUnset    BGPPeerType = ""
	BGPPeerTypeInternal BGPPeerType = "internal"
	BGPPeerTypeExternal BGPPeerType = "external"
)

func (b BGPPeerType) ID() (uint8, error) {
	switch b {
	case BGPPeerTypeUnset:
		return 0, nil
	case BGPPeerTypeInternal:
		return 1, nil
	case BGPPeerTypeExternal:
		return 2, nil
	default:
		return 0, errors.Errorf("unknown BGPPeerType %s", b)
	}
}

type BGPMessages struct {
	Received BGPMessagesCounters `json:"rec,omitempty"`
	Sent     BGPMessagesCounters `json:"sent,omitempty"`
}

type BGPMessagesCounters struct {
	Capability   uint64 `json:"cap,omitempty"`
	Keepalive    uint64 `json:"keepal,omitempty"`
	Notification uint64 `json:"notif,omitempty"`
	Open         uint64 `json:"open,omitempty"`
	RouteRefresh uint64 `json:"rrefresh,omitempty"`
	Update       uint64 `json:"update,omitempty"`
}

type SwitchStatePlatform struct {
	Fans         map[string]SwitchStatePlatformFan         `json:"fans,omitempty"`
	PSUs         map[string]SwitchStatePlatformPSU         `json:"psus,omitempty"`
	Temperatures map[string]SwitchStatePlatformTemperature `json:"temps,omitempty"`
}

type SwitchStatePlatformFan struct {
	Direction string  `json:"dir,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
	Presence  bool    `json:"presence,omitempty"`
	Status    bool    `json:"status,omitempty"`
}

type SwitchStatePlatformPSU struct {
	InputCurrent  float64 `json:"inCurrent,omitempty"`
	InputPower    float64 `json:"inPower,omitempty"`
	InputVoltage  float64 `json:"inVoltage,omitempty"`
	OutputCurrent float64 `json:"outCurrent,omitempty"`
	OutputPower   float64 `json:"outPower,omitempty"`
	OutputVoltage float64 `json:"outVoltage,omitempty"`
	Presence      bool    `json:"presence,omitempty"`
	Status        bool    `json:"status,omitempty"`
}

type SwitchStatePlatformTemperature struct {
	Temperature           float64 `json:"temp,omitempty"`
	Alarms                string  `json:"alarms,omitempty"`
	HighThreshold         float64 `json:"highThreshold,omitempty"`
	CriticalHighThreshold float64 `json:"critHighThreshold,omitempty"`
	LowThreshold          float64 `json:"lowThreshold,omitempty"`
	CriticalLowThreshold  float64 `json:"critLowThreshold,omitempty"`
}

// SwitchStateNOS contains information about the switch and NOS received from the switch itself by the agent
type SwitchStateNOS struct {
	// ASIC name, such as "broadcom" or "vs"
	AsicVersion string `json:"asicVersion,omitempty"`
	// NOS build commit
	BuildCommit string `json:"buildCommit,omitempty"`
	// NOS build date
	BuildDate string `json:"buildDate,omitempty"`
	// NOS build user
	BuiltBy string `json:"builtBy,omitempty"`
	// NOS config DB version, such as "version_4_2_1"
	ConfigDBVersion string `json:"configDBVersion,omitempty"`
	// Distribution version, such as "Debian 10.13"
	DistributionVersion string `json:"distributionVersion,omitempty"`
	// Hardware version, such as "X01"
	HardwareVersion string `json:"hardwareVersion,omitempty"`
	// Hwsku version, such as "DellEMC-S5248f-P-25G-DPB"
	HwskuVersion string `json:"hwskuVersion,omitempty"`
	// Kernel version, such as "5.10.0-21-amd64"
	KernelVersion string `json:"kernelVersion,omitempty"`
	// Manufacturer name, such as "Dell EMC"
	MfgName string `json:"mfgName,omitempty"`
	// Platform name, such as "x86_64-dellemc_s5248f_c3538-r0"
	PlatformName string `json:"platformName,omitempty"`
	// NOS product description, such as "Enterprise SONiC Distribution by Broadcom - Enterprise Base package"
	ProductDescription string `json:"productDescription,omitempty"`
	// NOS product version, empty for Broadcom SONiC
	ProductVersion string `json:"productVersion,omitempty"`
	// Switch serial number
	SerialNumber string `json:"serialNumber,omitempty"`
	// NOS software version, such as "4.2.0-Enterprise_Base"
	SoftwareVersion string `json:"softwareVersion,omitempty"`
	// Switch uptime, such as "21:21:27 up 1 day, 23:26, 0 users, load average: 1.92, 1.99, 2.00 "
	Uptime string `json:"uptime,omitempty"`
}

type SwitchStateCRM struct {
	ACLStats SwitchStateCRMACLStats `json:"aclStats,omitempty"`
	Stats    SwitchStateCRMStats    `json:"stats,omitempty"`
}

type SwitchStateCRMACLStats struct {
	Egress  SwitchStateCRMACLInfo `json:"egress,omitempty"`
	Ingress SwitchStateCRMACLInfo `json:"ingress,omitempty"`
}

type SwitchStateCRMACLInfo struct {
	Lag    SwitchStateCRMACLDetails `json:"lag,omitempty"`
	Port   SwitchStateCRMACLDetails `json:"port,omitempty"`
	RIF    SwitchStateCRMACLDetails `json:"rif,omitempty"`
	Switch SwitchStateCRMACLDetails `json:"switch,omitempty"`
	VLAN   SwitchStateCRMACLDetails `json:"vlan,omitempty"`
}

type SwitchStateCRMACLDetails struct {
	GroupsAvailable uint32 `json:"groupsAvailable,omitempty"`
	GroupsUsed      uint32 `json:"groupsUsed,omitempty"`
	TablesAvailable uint32 `json:"tablesAvailable,omitempty"`
	TablesUsed      uint32 `json:"tablesUsed,omitempty"`
}

type SwitchStateCRMStats struct {
	DnatEntriesAvailable         uint32 `json:"dnatEntriesAvailable,omitempty"`
	DnatEntriesUsed              uint32 `json:"dnatEntriesUsed,omitempty"`
	FdbEntriesAvailable          uint32 `json:"fdbEntriesAvailable,omitempty"`
	FdbEntriesUsed               uint32 `json:"fdbEntriesUsed,omitempty"`
	IpmcEntriesAvailable         uint32 `json:"ipmcEntriesAvailable,omitempty"`
	IpmcEntriesUsed              uint32 `json:"ipmcEntriesUsed,omitempty"`
	Ipv4NeighborsAvailable       uint32 `json:"ipv4NeighborsAvailable,omitempty"`
	Ipv4NeighborsUsed            uint32 `json:"ipv4NeighborsUsed,omitempty"`
	Ipv4NexthopsAvailable        uint32 `json:"ipv4NexthopsAvailable,omitempty"`
	Ipv4NexthopsUsed             uint32 `json:"ipv4NexthopsUsed,omitempty"`
	Ipv4RoutesAvailable          uint32 `json:"ipv4RoutesAvailable,omitempty"`
	Ipv4RoutesUsed               uint32 `json:"ipv4RoutesUsed,omitempty"`
	Ipv6NeighborsAvailable       uint32 `json:"ipv6NeighborsAvailable,omitempty"`
	Ipv6NeighborsUsed            uint32 `json:"ipv6NeighborsUsed,omitempty"`
	Ipv6NexthopsAvailable        uint32 `json:"ipv6NexthopsAvailable,omitempty"`
	Ipv6NexthopsUsed             uint32 `json:"ipv6NexthopsUsed,omitempty"`
	Ipv6RoutesAvailable          uint32 `json:"ipv6RoutesAvailable,omitempty"`
	Ipv6RoutesUsed               uint32 `json:"ipv6RoutesUsed,omitempty"`
	NexthopGroupMembersAvailable uint32 `json:"nexthopGroupMembersAvailable,omitempty"`
	NexthopGroupMembersUsed      uint32 `json:"nexthopGroupMembersUsed,omitempty"`
	NexthopGroupsAvailable       uint32 `json:"nexthopGroupsAvailable,omitempty"`
	NexthopGroupsUsed            uint32 `json:"nexthopGroupsUsed,omitempty"`
	SnatEntriesAvailable         uint32 `json:"snatEntriesAvailable,omitempty"`
	SnatEntriesUsed              uint32 `json:"snatEntriesUsed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=hedgehog,shortName=ag
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`,priority=0
// +kubebuilder:printcolumn:name="Descr",type=string,JSONPath=`.spec.description`,priority=0
// +kubebuilder:printcolumn:name="HWSKU",type=string,JSONPath=`.status.state.nos.hwskuVersion`,priority=1
// +kubebuilder:printcolumn:name="ASIC",type=string,JSONPath=`.status.state.nos.asicVersion`,priority=1
// +kubebuilder:printcolumn:name="Heartbeat",type=date,JSONPath=`.status.lastHeartbeat`,priority=1
// +kubebuilder:printcolumn:name="Applied",type=date,JSONPath=`.status.lastAppliedTime`,priority=0
// +kubebuilder:printcolumn:name="AppliedG",type=integer,JSONPath=`.status.lastAppliedGen`,priority=0
// +kubebuilder:printcolumn:name="CurrentG",type=integer,JSONPath=`.metadata.generation`,priority=0
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,priority=0
// +kubebuilder:printcolumn:name="Software",type=string,JSONPath=`.status.state.nos.softwareVersion`,priority=1
// +kubebuilder:printcolumn:name="Attempt",type=date,JSONPath=`.status.lastAttemptTime`,priority=2
// +kubebuilder:printcolumn:name="AttemptG",type=integer,JSONPath=`.status.lastAttemptGen`,priority=2
// +kubebuilder:printcolumn:name="RoCE",type=boolean,JSONPath=`.spec.switch.roce`,priority=3
// +kubebuilder:printcolumn:name="CurrRoCE",type=boolean,JSONPath=`.status.state.roce`,priority=3
// +kubebuilder:printcolumn:name="ECMPQPN",type=boolean,JSONPath=`.spec.switch.ecmp.roceQPN`,priority=3
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,priority=10
// Agent is an internal API object used by the controller to pass all relevant information to the agent running on a
// specific switch in order to fully configure it and manage its lifecycle. It is not intended to be used directly by
// users. Spec of the object isn't user-editable, it is managed by the controller. Status of the object is updated by
// the agent and is used by the controller to track the state of the agent and the switch it is running on. Name of the
// Agent object is the same as the name of the switch it is running on and it's created in the same namespace as the
// Switch object.
type Agent struct {
	kmetav1.TypeMeta   `json:",inline"`
	kmetav1.ObjectMeta `json:"metadata,omitempty"`

	// +structType=atomic
	// Spec is the desired state of the Agent
	Spec AgentSpec `json:"spec,omitempty"`

	// +structType=atomic
	// Status is the observed state of the Agent
	Status AgentStatus `json:"status,omitempty"`
}

const KindAgent = "Agent"

//+kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	kmetav1.TypeMeta `json:",inline"`
	kmetav1.ListMeta `json:"metadata,omitempty"`
	Items            []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}

func (s *AgentSpec) IsVS() bool {
	return s.Switch.Profile == meta.SwitchProfileVS
}

func (a *Agent) IsFirstInRedundancyGroup() bool {
	red := a.Spec.Switch.Redundancy
	if red.Type == meta.RedundancyTypeNone || len(a.Spec.RedundancyGroupPeers) == 0 {
		return true
	}

	rg := append(slices.Clone(a.Spec.RedundancyGroupPeers), a.Name)
	sort.Strings(rg)

	return rg[0] == a.Name
}
