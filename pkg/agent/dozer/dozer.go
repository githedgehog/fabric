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

package dozer

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/pmezard/go-difflib/difflib"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	kyaml "sigs.k8s.io/yaml"
)

type Processor interface {
	EnsureControlLink(ctx context.Context, agent *agentapi.Agent) error
	WaitReady(ctx context.Context) error
	LoadActualState(ctx context.Context, agent *agentapi.Agent) (*Spec, error)
	PlanDesiredState(ctx context.Context, agent *agentapi.Agent) (*Spec, error)
	CalculateActions(ctx context.Context, actual, desired *Spec) ([]Action, error)
	ApplyActions(ctx context.Context, actions []Action) ([]string, error) // warnings
	UpdateSwitchState(ctx context.Context, agent *agentapi.Agent, reg *switchstate.Registry) error
	Reboot(ctx context.Context, force bool) error
	Reinstall(ctx context.Context) error
	FactoryReset(ctx context.Context) error
	GetRoCE(ctx context.Context) (bool, error)
	SetRoCE(ctx context.Context, enable bool) error
}

type Action interface {
	Summary() string
}

type Spec struct {
	ZTP                *bool                             `json:"ztp,omitempty"`
	Hostname           *string                           `json:"hostname,omitempty"`
	LLDP               *SpecLLDP                         `json:"lldp,omitempty"`
	LLDPInterfaces     map[string]*SpecLLDPInterface     `json:"lldpInterfaces,omitempty"`
	NTP                *SpecNTP                          `json:"ntp,omitempty"`
	NTPServers         map[string]*SpecNTPServer         `json:"ntpServers,omitempty"`
	Users              map[string]*SpecUser              `json:"users,omitempty"`
	PortGroups         map[string]*SpecPortGroup         `json:"portGroupSpeeds,omitempty"`
	PortBreakouts      map[string]*SpecPortBreakout      `json:"portBreakouts,omitempty"`
	Interfaces         map[string]*SpecInterface         `json:"interfaces,omitempty"`
	MCLAGs             map[uint32]*SpecMCLAGDomain       `json:"mclags,omitempty"`
	MCLAGInterfaces    map[string]*SpecMCLAGInterface    `json:"mclagInterfaces,omitempty"`
	VRFs               map[string]*SpecVRF               `json:"vrfs,omitempty"`
	RouteMaps          map[string]*SpecRouteMap          `json:"routeMaps,omitempty"`
	PrefixLists        map[string]*SpecPrefixList        `json:"prefixLists,omitempty"`
	CommunityLists     map[string]*SpecCommunityList     `json:"communityLists,omitempty"`
	DHCPRelays         map[string]*SpecDHCPRelay         `json:"dhcpRelays,omitempty"`
	ACLs               map[string]*SpecACL               `json:"acls,omitempty"`
	ACLInterfaces      map[string]*SpecACLInterface      `json:"aclInterfaces,omitempty"`
	VXLANTunnels       map[string]*SpecVXLANTunnel       `json:"vxlanTunnels,omitempty"`
	VXLANEVPNNVOs      map[string]*SpecVXLANEVPNNVO      `json:"vxlanEVPNNVOs,omitempty"`
	VXLANTunnelMap     map[string]*SpecVXLANTunnelMap    `json:"vxlanTunnelMap,omitempty"` // e.g. map_5011_Vlan1000 -> 5011 + Vlan1000
	VRFVNIMap          map[string]*SpecVRFVNIEntry       `json:"vrfVNIMap,omitempty"`
	SuppressVLANNeighs map[string]*SpecSuppressVLANNeigh `json:"suppressVLANNeighs,omitempty"`
	PortChannelConfigs map[string]*SpecPortChannelConfig `json:"portChannelConfigs,omitempty"`
	LSTGroups          map[string]*SpecLSTGroup          `json:"lstGroups,omitempty"`
	LSTInterfaces      map[string]*SpecLSTInterface      `json:"lstInterfaces,omitempty"`
	ECMPRoCEQPN        *bool                             `json:"ecmpRoCEQPN,omitempty"`
	BFDProfiles        map[string]*SpecBFDProfile        `json:"bfdProfiles,omitempty"`
}

type SpecLLDP struct {
	Enabled           *bool   `json:"enabled,omitempty"`
	HelloTimer        *uint64 `json:"helloTimer,omitempty"`
	SystemName        *string `json:"systemName,omitempty"`
	SystemDescription *string `json:"systemDescription,omitempty"`
}

type SpecLLDPInterface struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	ManagementIPv4 *string `json:"managementIPv4,omitempty"`
}

type SpecNTP struct {
	SourceInterface []string `json:"sourceInterface,omitempty"`
}

type SpecNTPServer struct {
	Prefer *bool `json:"prefer,omitempty"`
}

type SpecUser struct {
	Password       string   `json:"password,omitempty"`
	Role           string   `json:"role,omitempty"`
	AuthorizedKeys []string `json:"authorizedKeys,omitempty"`
}

type SpecPortGroup struct {
	Speed *string
}

type SpecPortBreakout struct {
	Mode string `json:"mode,omitempty"`
}

type SpecInterface struct {
	Description        *string                      `json:"description,omitempty"`
	Enabled            *bool                        `json:"enabled,omitempty"`
	PortChannel        *string                      `json:"portChannel,omitempty"`
	AccessVLAN         *uint16                      `json:"accessVLAN,omitempty"`
	TrunkVLANs         []string                     `json:"trunkVLANs,omitempty"`
	MTU                *uint16                      `json:"mtu,omitempty"`
	Speed              *string                      `json:"speed,omitempty"`
	AutoNegotiate      *bool                        `json:"autoNegotiate,omitempty"`
	VLANIPs            map[string]*SpecInterfaceIP  `json:"vlanIPs,omitempty"`
	VLANAnycastGateway []string                     `json:"vlanAnycastGateway,omitempty"`
	Subinterfaces      map[uint32]*SpecSubinterface `json:"subinterfaces,omitempty"`
}

type SpecInterfaceIP struct {
	PrefixLen *uint8 `json:"prefixLen,omitempty"`
	Secondary *bool  `json:"secondary,omitempty"`
}

type SpecSubinterface struct {
	VLAN            *uint16                     `json:"vlan,omitempty"`
	IPs             map[string]*SpecInterfaceIP `json:"ips,omitempty"`
	AnycastGateways []string                    `json:"anycastGateways,omitempty"`
}

type SpecMCLAGDomain struct {
	SourceIP string `json:"sourceIP,omitempty"`
	PeerIP   string `json:"peerIP,omitempty"`
	PeerLink string `json:"peerLink,omitempty"`
}

type SpecMCLAGInterface struct {
	DomainID uint32 `json:"domainID,omitempty"`
}

type SpecVRF struct {
	Enabled          *bool                              `json:"enabled,omitempty"`
	Description      *string                            `json:"description,omitempty"`
	AnycastMAC       *string                            `json:"anycastMAC,omitempty"`
	Interfaces       map[string]*SpecVRFInterface       `json:"interfaces,omitempty"`
	BGP              *SpecVRFBGP                        `json:"bgp,omitempty"`
	TableConnections map[string]*SpecVRFTableConnection `json:"tableConnections,omitempty"`
	StaticRoutes     map[string]*SpecVRFStaticRoute     `json:"staticRoutes,omitempty"`
	EthernetSegments map[string]*SpecVRFEthernetSegment `json:"ethernetSegments,omitempty"`
	EVPNMH           SpecVRFEVPNMH                      `json:"evpnMH,omitempty"`
	AttachedHosts    map[string]*SpecVRFAttachedHost    `json:"attachedHosts,omitempty"`
}

type SpecVRFInterface struct{}

type SpecVRFBGP struct {
	AS                 *uint32                        `json:"as,omitempty"`
	RouterID           *string                        `json:"routerID,omitempty"`
	NetworkImportCheck *bool                          `json:"networkImportCheck,omitempty"`
	IPv4Unicast        SpecVRFBGPIPv4Unicast          `json:"ipv4Unicast,omitempty"`
	L2VPNEVPN          SpecVRFBGPL2VPNEVPN            `json:"l2vpnEvpn,omitempty"`
	Neighbors          map[string]*SpecVRFBGPNeighbor `json:"neighbors,omitempty"`
}

type SpecVRFBGPIPv4Unicast struct {
	Enabled      bool                            `json:"enable,omitempty"`
	MaxPaths     *uint32                         `json:"maxPaths,omitempty"`
	MaxPathsIBGP *uint32                         `json:"maxPathsIBGP,omitempty"`
	Networks     map[string]*SpecVRFBGPNetwork   `json:"networks,omitempty"`
	ImportVRFs   map[string]*SpecVRFBGPImportVRF `json:"importVRFs,omitempty"`
	ImportPolicy *string                         `json:"importPolicy,omitempty"`
	TableMap     *string                         `json:"tableMap,omitempty"`
}

type SpecVRFBGPL2VPNEVPN struct {
	Enabled                       bool     `json:"enable,omitempty"`
	AdvertiseAllVNI               *bool    `json:"advertiseAllVnis,omitempty"`
	AdvertiseIPv4Unicast          *bool    `json:"advertiseIPv4Unicast,omitempty"`
	AdvertiseIPv4UnicastRouteMaps []string `json:"advertiseIPv4UnicastRouteMaps,omitempty"`
	AdvertiseDefaultGw            *bool    `json:"advertiseDefaultGw,omitempty"`
}

type SpecVRFBGPNetwork struct{}

type SpecVRFBGPNeighbor struct {
	Enabled                   *bool    `json:"enabled,omitempty"`
	Description               *string  `json:"description,omitempty"`
	RemoteAS                  *uint32  `json:"remoteAS,omitempty"`
	PeerType                  *string  `json:"peerType,omitempty"`
	IPv4Unicast               *bool    `json:"ipv4Unicast,omitempty"`
	IPv4UnicastImportPolicies []string `json:"ipv4UnicastImportPolicies,omitempty"`
	IPv4UnicastExportPolicies []string `json:"ipv4UnicastExportPolicies,omitempty"`
	L2VPNEVPN                 *bool    `json:"l2vpnEvpn,omitempty"`
	L2VPNEVPNImportPolicies   []string `json:"l2vpnEvpnImportPolicies,omitempty"`
	L2VPNEVPNAllowOwnAS       *bool    `json:"l2vpnEvpnAllowOwnAS,omitempty"`
	BFDProfile                *string  `json:"bfdProfile,omitempty"`
	DisableConnectedCheck     *bool    `json:"disableConnectedCheck,omitempty"`
	UpdateSource              *string  `json:"updateSource,omitempty"`
}

const (
	SpecVRFBGPNeighborPeerTypeInternal = "internal"
	SpecVRFBGPNeighborPeerTypeExternal = "external"
)

type SpecVRFTableConnection struct {
	ImportPolicies []string `json:"importPolicies,omitempty"`
}

type SpecVRFStaticRoute struct {
	NextHops []SpecVRFStaticRouteNextHop `json:"nextHops,omitempty"`
}

type SpecVRFStaticRouteNextHop struct {
	IP        string  `json:"ip,omitempty"`
	Interface *string `json:"interface,omitempty"`
}

type SpecVRFEthernetSegment struct {
	ESI string `json:"esi,omitempty"`
}

type SpecVRFEVPNMH struct {
	MACHoldtime  *uint32 `json:"macHoldtime,omitempty"`
	StartupDelay *uint32 `json:"startupDelay,omitempty"`
}

type SpecVRFAttachedHost struct{}

type SpecRouteMap struct {
	Statements map[string]*SpecRouteMapStatement `json:"statements,omitempty"`
}

type SpecRouteMapStatement struct {
	Conditions         SpecRouteMapConditions `json:"conditions,omitempty"`
	SetCommunities     []string               `json:"setCommunities,omitempty"`
	SetLocalPreference *uint32                `json:"setLocalPreference,omitempty"`
	Result             SpecRouteMapResult     `json:"result,omitempty"`
}

type SpecRouteMapConditions struct {
	AttachedHost           *bool   `json:"attachedHost,omitempty"`
	DirectlyConnected      *bool   `json:"directlyConnected,omitempty"`
	MatchEVPNDefaultRoute  *bool   `json:"matchEvpnDefaultRoute,omitempty"`
	MatchEVPNVNI           *uint32 `json:"matchEvpnVni,omitempty"`
	MatchPrefixList        *string `json:"matchPrefixLists,omitempty"`
	MatchNextHopPrefixList *string `json:"matchNextHopPrefixLists,omitempty"`
	MatchCommunityList     *string `json:"matchCommunityLists,omitempty"`
	MatchSourceVRF         *string `json:"matchSourceVrf,omitempty"`
	Call                   *string `json:"call,omitempty"`
}

type SpecRouteMapResult string

const (
	SpecRouteMapResultAccept SpecRouteMapResult = "accept"
	SpecRouteMapResultReject SpecRouteMapResult = "reject"
)

type SpecPrefixList struct {
	Prefixes map[uint32]*SpecPrefixListEntry `json:"prefixes,omitempty"`
}

type SpecPrefixListEntry struct {
	Prefix SpecPrefixListPrefix `json:"prefix,omitempty"`
	Action SpecPrefixListAction `json:"action,omitempty"`
}

type SpecPrefixListPrefix struct {
	Prefix string `json:"prefix,omitempty"`
	Ge     uint8  `json:"ge,omitempty"`
	Le     uint8  `json:"le,omitempty"`
}

type SpecPrefixListAction string

type SpecCommunityList struct {
	Members []string `json:"members,omitempty"`
}

const (
	SpecPrefixListActionUnset  SpecPrefixListAction = ""
	SpecPrefixListActionPermit SpecPrefixListAction = "permit"
	SpecPrefixListActionDeny   SpecPrefixListAction = "deny"
)

const (
	SpecVRFBGPTableConnectionConnected    = "connected"
	SpecVRFBGPTableConnectionStatic       = "static"
	SpecVRFBGPTableConnectionAttachedHost = "attachedhost"
)

type SpecVRFBGPImportVRF struct{}

type SpecBFDProfile struct {
	PassiveMode              *bool   `json:"passiveMode,omitempty"`
	RequiredMinimumReceive   *uint32 `json:"requiredMinimumReceive,omitempty"`
	DesiredMinimumTxInterval *uint32 `json:"desiredMinimumTxInterval,omitempty"`
	DetectionMultiplier      *uint8  `json:"detectionMultiplier,omitempty"`
}

type SpecDHCPRelay struct {
	SourceInterface *string  `json:"sourceInterface,omitempty"`
	RelayAddress    []string `json:"relayAddress,omitempty"`
	LinkSelect      bool     `json:"linkSelect,omitempty"`
	VRFSelect       bool     `json:"vrfSelect,omitempty"`
}

type SpecACL struct {
	Description *string                  `json:"description,omitempty"`
	Entries     map[uint32]*SpecACLEntry `json:"entries,omitempty"`
}

type SpecACLEntry struct {
	Description        *string              `json:"description,omitempty"`
	Action             SpecACLEntryAction   `json:"action,omitempty"`
	Protocol           SpecACLEntryProtocol `json:"protocol,omitempty"`
	SourceAddress      *string              `json:"sourceAddress,omitempty"`
	SourcePort         *uint16              `json:"sourcePort,omitempty"`
	DestinationAddress *string              `json:"destinationAddress,omitempty"`
	DestinationPort    *uint16              `json:"destinationPort,omitempty"`
}

type SpecACLEntryProtocol string

const (
	SpecACLEntryProtocolUnset SpecACLEntryProtocol = ""
	SpecACLEntryProtocolUDP   SpecACLEntryProtocol = "UDP"
)

type SpecACLEntryAction string

const (
	SpecACLEntryActionAccept  SpecACLEntryAction = "ACCEPT"  // permit
	SpecACLEntryActionDrop    SpecACLEntryAction = "DROP"    // deny
	SpecACLEntryActionDiscard SpecACLEntryAction = "DISCARD" // discard
	SpecACLEntryActionTransit SpecACLEntryAction = "TRANSIT" // transit
)

type SpecACLInterface struct {
	Ingress *string `json:"ingress,omitempty"`
	Egress  *string `json:"egress,omitempty"`
}

type SpecVXLANTunnel struct {
	SourceIP        *string `json:"sourceIP,omitempty"`
	SourceInterface *string `json:"sourceInterface,omitempty"`
	QoSUniform      *bool   `json:"qosUniform,omitempty"`
}

type SpecVXLANEVPNNVO struct {
	SourceVTEP *string `json:"sourceVtep,omitempty"`
}

type SpecVXLANTunnelMap struct {
	VTEP *string `json:"vtep,omitempty"` // name
	VNI  *uint32 `json:"vni,omitempty"`
	VLAN *uint16 `json:"vlan,omitempty"`
}

type SpecVRFVNIEntry struct {
	VNI *uint32 `json:"vni,omitempty"`
}

type SpecSuppressVLANNeigh struct{}

type SpecPortChannelConfig struct {
	SystemMAC *string `json:"systemMAC,omitempty"`
	Fallback  *bool   `json:"fallback,omitempty"`
}

type SpecLSTGroup struct {
	AllEVPNESDownstream *bool   `json:"allEvpnEsDownstream,omitempty"`
	AllMCLAGDownstream  *bool   `json:"allMclagDownstream,omitempty"`
	Timeout             *uint16 `json:"timeout,omitempty"`
}

type SpecLSTInterface struct {
	Groups []string `json:"groups,omitempty"`
}

func (s *Spec) Normalize() {
	for _, user := range s.Users {
		if user.AuthorizedKeys == nil {
			continue
		}

		sort.Slice(user.AuthorizedKeys, func(i, j int) bool {
			return user.AuthorizedKeys[i] < user.AuthorizedKeys[j]
		})
	}

	for _, dhcp := range s.DHCPRelays {
		sort.Strings(dhcp.RelayAddress)
	}

	for name, iface := range s.Interfaces {
		if len(iface.TrunkVLANs) > 0 {
			sort.Strings(iface.TrunkVLANs)
		}

		if strings.HasPrefix(name, "PortChannel") || strings.HasPrefix(name, "Ethernet") {
			if iface.Subinterfaces == nil {
				iface.Subinterfaces = map[uint32]*SpecSubinterface{}
			}
			if _, exists := iface.Subinterfaces[0]; !exists {
				iface.Subinterfaces[0] = &SpecSubinterface{}
			}
		}
	}

	for _, comm := range s.CommunityLists {
		slices.Sort(comm.Members)
	}
}

func (s *Spec) CleanupSensetive() {
	users := map[string]*SpecUser{}
	for name, user := range s.Users {
		users[name] = &SpecUser{
			Role: user.Role,
		}
	}
	s.Users = users
}

func (s *Spec) MarshalYAML() ([]byte, error) {
	s.CleanupSensetive()

	data, err := kyaml.Marshal(s)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal spec")
	}

	return data, nil
}

func SpecTextDiff(actual, desired []byte) ([]byte, error) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(actual)),
		B:        difflib.SplitLines(string(desired)),
		FromFile: "Actual State",
		ToFile:   "Desired State",
		Context:  4,
	}

	diffText, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate diff")
	}

	return []byte(diffText), nil
}

type SpecPart interface {
	IsNil() bool
}

var (
	_ SpecPart = (*Spec)(nil)
	_ SpecPart = (*SpecLLDP)(nil)
	_ SpecPart = (*SpecLLDPInterface)(nil)
	_ SpecPart = (*SpecNTP)(nil)
	_ SpecPart = (*SpecNTPServer)(nil)
	_ SpecPart = (*SpecUser)(nil)
	_ SpecPart = (*SpecPortGroup)(nil)
	_ SpecPart = (*SpecPortBreakout)(nil)
	_ SpecPart = (*SpecInterface)(nil)
	_ SpecPart = (*SpecSubinterface)(nil)
	_ SpecPart = (*SpecInterfaceIP)(nil)
	_ SpecPart = (*SpecMCLAGDomain)(nil)
	_ SpecPart = (*SpecMCLAGInterface)(nil)
	_ SpecPart = (*SpecVRF)(nil)
	_ SpecPart = (*SpecVRFInterface)(nil)
	_ SpecPart = (*SpecVRFBGP)(nil)
	_ SpecPart = (*SpecVRFBGPNetwork)(nil)
	_ SpecPart = (*SpecVRFBGPNeighbor)(nil)
	_ SpecPart = (*SpecVRFBGPImportVRF)(nil)
	_ SpecPart = (*SpecVRFTableConnection)(nil)
	_ SpecPart = (*SpecVRFStaticRoute)(nil)
	_ SpecPart = (*SpecVRFEthernetSegment)(nil)
	_ SpecPart = (*SpecVRFAttachedHost)(nil)
	_ SpecPart = (*SpecRouteMap)(nil)
	_ SpecPart = (*SpecRouteMapStatement)(nil)
	_ SpecPart = (*SpecPrefixList)(nil)
	_ SpecPart = (*SpecPrefixListEntry)(nil)
	_ SpecPart = (*SpecCommunityList)(nil)
	_ SpecPart = (*SpecDHCPRelay)(nil)
	_ SpecPart = (*SpecACL)(nil)
	_ SpecPart = (*SpecACLEntry)(nil)
	_ SpecPart = (*SpecACLInterface)(nil)
	_ SpecPart = (*SpecVXLANTunnel)(nil)
	_ SpecPart = (*SpecVXLANEVPNNVO)(nil)
	_ SpecPart = (*SpecVXLANTunnelMap)(nil)
	_ SpecPart = (*SpecVRFVNIEntry)(nil)
	_ SpecPart = (*SpecSuppressVLANNeigh)(nil)
	_ SpecPart = (*SpecPortChannelConfig)(nil)
	_ SpecPart = (*SpecLSTGroup)(nil)
	_ SpecPart = (*SpecLSTInterface)(nil)
	_ SpecPart = (*SpecBFDProfile)(nil)
)

func (s *Spec) IsNil() bool {
	return s == nil
}

func (s *SpecLLDP) IsNil() bool {
	return s == nil
}

func (s *SpecLLDPInterface) IsNil() bool {
	return s == nil
}

func (s *SpecNTP) IsNil() bool {
	return s == nil
}

func (s *SpecNTPServer) IsNil() bool {
	return s == nil
}

func (s *SpecUser) IsNil() bool {
	return s == nil
}

func (s *SpecPortGroup) IsNil() bool {
	return s == nil
}

func (s *SpecPortBreakout) IsNil() bool {
	return s == nil
}

func (s *SpecInterface) IsNil() bool {
	return s == nil
}

func (s *SpecSubinterface) IsNil() bool {
	return s == nil
}

func (s *SpecInterfaceIP) IsNil() bool {
	return s == nil
}

func (s *SpecMCLAGInterface) IsNil() bool {
	return s == nil
}

func (s *SpecMCLAGDomain) IsNil() bool {
	return s == nil
}

func (s *SpecVRF) IsNil() bool {
	return s == nil
}

func (s *SpecVRFInterface) IsNil() bool {
	return s == nil
}

func (s *SpecVRFBGP) IsNil() bool {
	return s == nil
}

func (s *SpecVRFBGPNetwork) IsNil() bool {
	return s == nil
}

func (s *SpecVRFBGPNeighbor) IsNil() bool {
	return s == nil
}

func (s *SpecVRFBGPImportVRF) IsNil() bool {
	return s == nil
}

func (s *SpecVRFTableConnection) IsNil() bool {
	return s == nil
}

func (s *SpecVRFStaticRoute) IsNil() bool {
	return s == nil
}

func (s *SpecVRFEthernetSegment) IsNil() bool {
	return s == nil
}

func (s *SpecVRFAttachedHost) IsNil() bool {
	return s == nil
}

func (s *SpecRouteMap) IsNil() bool {
	return s == nil
}

func (s *SpecRouteMapStatement) IsNil() bool {
	return s == nil
}

func (s *SpecPrefixList) IsNil() bool {
	return s == nil
}

func (s *SpecPrefixListEntry) IsNil() bool {
	return s == nil
}

func (s *SpecCommunityList) IsNil() bool {
	return s == nil
}

func (s *SpecDHCPRelay) IsNil() bool {
	return s == nil
}

func (s *SpecACL) IsNil() bool {
	return s == nil
}

func (s *SpecACLEntry) IsNil() bool {
	return s == nil
}

func (s *SpecACLInterface) IsNil() bool {
	return s == nil
}

func (s *SpecVXLANTunnel) IsNil() bool {
	return s == nil
}

func (s *SpecVXLANEVPNNVO) IsNil() bool {
	return s == nil
}

func (s *SpecVXLANTunnelMap) IsNil() bool {
	return s == nil
}

func (s *SpecVRFVNIEntry) IsNil() bool {
	return s == nil
}

func (s *SpecSuppressVLANNeigh) IsNil() bool {
	return s == nil
}

func (s *SpecPortChannelConfig) IsNil() bool {
	return s == nil
}

func (s *SpecLSTGroup) IsNil() bool {
	return s == nil
}

func (s *SpecLSTInterface) IsNil() bool {
	return s == nil
}

func (s *SpecBFDProfile) IsNil() bool {
	return s == nil
}
