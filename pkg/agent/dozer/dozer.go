package dozer

import (
	"context"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/pmezard/go-difflib/difflib"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"sigs.k8s.io/yaml"
)

type Processor interface {
	EnsureControlLink(ctx context.Context, agent *agentapi.Agent) error
	WaitReady(ctx context.Context) error
	LoadActualState(ctx context.Context) (*Spec, error)
	PlanDesiredState(ctx context.Context, agent *agentapi.Agent) (*Spec, error)
	CalculateActions(ctx context.Context, actual, desired *Spec) ([]Action, error)
	ApplyActions(ctx context.Context, actions []Action) ([]string, error) // warnings
	Info(ctx context.Context) (*agentapi.NOSInfo, error)
	Reboot(ctx context.Context, force bool) error
	Reinstall(ctx context.Context) error
	FactoryReset(ctx context.Context) error
}

type Action interface {
	Summary() string
}

type Spec struct {
	ZTP                *bool                             `json:"ztp,omitempty"`
	Hostname           *string                           `json:"hostname,omitempty"`
	LLDP               *SpecLLDP                         `json:"lldp,omitempty"`
	LLDPInterfaces     map[string]*SpecLLDPInterface     `json:"lldpInterfaces,omitempty"`
	Users              map[string]*SpecUser              `json:"users,omitempty"`
	PortGroups         map[string]*SpecPortGroup         `json:"portGroupSpeeds,omitempty"`
	PortBreakouts      map[string]*SpecPortBreakout      `json:"portBreakouts,omitempty"`
	Interfaces         map[string]*SpecInterface         `json:"interfaces,omitempty"`
	MCLAGs             map[uint32]*SpecMCLAGDomain       `json:"mclags,omitempty"`
	MCLAGInterfaces    map[string]*SpecMCLAGInterface    `json:"mclagInterfaces,omitempty"`
	VRFs               map[string]*SpecVRF               `json:"vrfs,omitempty"`
	RouteMaps          map[string]*SpecRouteMap          `json:"routingMaps,omitempty"`
	DHCPRelays         map[string]*SpecDHCPRelay         `json:"dhcpRelays,omitempty"`
	NATs               map[uint32]*SpecNAT               `json:"nats,omitempty"`
	ACLs               map[string]*SpecACL               `json:"acls,omitempty"`
	ACLInterfaces      map[string]*SpecACLInterface      `json:"aclInterfaces,omitempty"`
	VXLANTunnels       map[string]*SpecVXLANTunnel       `json:"vxlanTunnels,omitempty"`
	VXLANEVPNNVOs      map[string]*SpecVXLANEVPNNVO      `json:"vxlanEVPNNVOs,omitempty"`
	VXLANTunnelMap     map[string]*SpecVXLANTunnelMap    `json:"vxlanTunnelMap,omitempty"` // e.g. map_5011_Vlan1000 -> 5011 + Vlan1000
	VRFVNIMap          map[string]*SpecVRFVNIEntry       `json:"vrfVNIMap,omitempty"`
	SuppressVLANNeighs map[string]*SpecSuppressVLANNeigh `json:"suppressVLANNeighs,omitempty"`
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
	NATZone            *uint8                       `json:"natZone,omitempty"`
	AccessVLAN         *uint16                      `json:"accessVLAN,omitempty"`
	TrunkVLANs         []string                     `json:"trunkVLANs,omitempty"`
	MTU                *uint16                      `json:"mtu,omitempty"`
	Speed              *string                      `json:"speed,omitempty"`
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
	Networks     map[string]*SpecVRFBGPNetwork   `json:"networks,omitempty"`
	ImportVRFs   map[string]*SpecVRFBGPImportVRF `json:"importVRFs,omitempty"`
	ImportPolicy *string                         `json:"importPolicy,omitempty"`
}

type SpecVRFBGPL2VPNEVPN struct {
	Enabled              bool  `json:"enable,omitempty"`
	DefaultOriginateIPv4 *bool `json:"defaultOriginateIPv4,omitempty"`
	AdvertiseAllVNI      *bool `json:"advertiseAllVnis,omitempty"`
	AdvertiseIPv4Unicast *bool `json:"advertiseIPv4Unicast,omitempty"`
}

type SpecVRFBGPNetwork struct{}

type SpecVRFBGPNeighbor struct {
	Enabled     *bool   `json:"enabled,omitempty"`
	Description *string `json:"description,omitempty"`
	RemoteAS    *uint32 `json:"remoteAS,omitempty"`
	PeerType    *string `json:"peerType,omitempty"`
	IPv4Unicast *bool   `json:"ipv4Unicast,omitempty"`
	L2VPNEVPN   *bool   `json:"l2vpnEvpn,omitempty"`
}

const (
	SpecVRFBGPNeighborPeerTypeInternal = "internal"
	SpecVRFBGPNeighborPeerTypeExternal = "external"
)

type SpecVRFTableConnection struct {
	ImportPolicies []string `json:"importPolicies,omitempty"`
}

type SpecVRFStaticRoute struct {
	Description *string                     `json:"description,omitempty"`
	NextHops    []SpecVRFStaticRouteNextHop `json:"nextHops,omitempty"`
}

type SpecVRFStaticRouteNextHop struct {
	IP        string  `json:"ip,omitempty"`
	Interface *string `json:"interface,omitempty"`
}

type SpecRouteMap struct {
	Statements map[string]*SpecRouteMapStatement `json:"statements,omitempty"`
}

type SpecRouteMapStatement struct {
	Conditions SpecRouteMapConditions `json:"conditions,omitempty"`
	Result     SpecRouteMapResult     `json:"result,omitempty"`
}

type SpecRouteMapConditions struct {
	DirectlyConnected *bool `json:"directlyConnected,omitempty"`
}

type SpecRouteMapResult string

const (
	SpecRouteMapResultAccept SpecRouteMapResult = "accept"
	SpecRouteMapResultReject SpecRouteMapResult = "reject"
)

const (
	SpecVRFBGPTableConnectionConnected = "connected"
	SpecVRFBGPTableConnectionStatic    = "static"
)

type SpecVRFBGPImportVRF struct{}

type SpecDHCPRelay struct {
	SourceInterface *string  `json:"sourceInterface,omitempty"`
	RelayAddress    []string `json:"relayAddress,omitempty"`
	LinkSelect      bool     `json:"linkSelect,omitempty"`
	VRFSelect       bool     `json:"vrfSelect,omitempty"`
}

type SpecNAT struct {
	Enable   *bool                      `json:"enable,omitempty"`
	Pools    map[string]*SpecNATPool    `json:"pools,omitempty"`
	Bindings map[string]*SpecNATBinding `json:"bindings,omitempty"`
	Static   map[string]*SpecNATEntry   `json:"static,omitempty"` // external -> internal
}

type SpecNATPool struct {
	Range *string `json:"range,omitempty"`
}

type SpecNATBinding struct {
	Pool *string     `json:"pool,omitempty"`
	Type SpecNATType `json:"type,omitempty"`
}

type SpecNATEntry struct {
	InternalAddress *string     `json:"internalAddress,omitempty"`
	Type            SpecNATType `json:"type,omitempty"`
}

type SpecNATType string

const (
	SpecNATTypeUnset SpecNATType = ""
	SpecNATTypeDNAT  SpecNATType = "DNAT"
	SpecNATTypeSNAT  SpecNATType = "SNAT"
)

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
	SpecACLEntryActionAccept SpecACLEntryAction = "ACCEPT" // permit
	SpecACLEntryActionDrop   SpecACLEntryAction = "DROP"   // deny
)

type SpecACLInterface struct {
	Ingress *string `json:"ingress,omitempty"`
}

type SpecVXLANTunnel struct {
	SourceIP        *string `json:"sourceIP,omitempty"`
	SourceInterface *string `json:"sourceInterface,omitempty"`
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
		if strings.HasPrefix(name, "PortChannel") || strings.HasPrefix(name, "Ethernet") {
			if iface.Subinterfaces == nil {
				iface.Subinterfaces = map[uint32]*SpecSubinterface{}
			}
			if _, exists := iface.Subinterfaces[0]; !exists {
				iface.Subinterfaces[0] = &SpecSubinterface{}
			}
		}
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

	data, err := yaml.Marshal(s)
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
	_ SpecPart = (*SpecRouteMap)(nil)
	_ SpecPart = (*SpecDHCPRelay)(nil)
	_ SpecPart = (*SpecNAT)(nil)
	_ SpecPart = (*SpecNATPool)(nil)
	_ SpecPart = (*SpecNATBinding)(nil)
	_ SpecPart = (*SpecNATEntry)(nil)
	_ SpecPart = (*SpecACL)(nil)
	_ SpecPart = (*SpecACLEntry)(nil)
	_ SpecPart = (*SpecACLInterface)(nil)
	_ SpecPart = (*SpecVXLANTunnel)(nil)
	_ SpecPart = (*SpecVXLANEVPNNVO)(nil)
	_ SpecPart = (*SpecVXLANTunnelMap)(nil)
	_ SpecPart = (*SpecVRFVNIEntry)(nil)
	_ SpecPart = (*SpecSuppressVLANNeigh)(nil)
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

func (s *SpecRouteMap) IsNil() bool {
	return s == nil
}

func (s *SpecDHCPRelay) IsNil() bool {
	return s == nil
}

func (s *SpecNAT) IsNil() bool {
	return s == nil
}

func (s *SpecNATPool) IsNil() bool {
	return s == nil
}

func (s *SpecNATBinding) IsNil() bool {
	return s == nil
}

func (s *SpecNATEntry) IsNil() bool {
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
