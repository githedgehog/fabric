package dozer

import (
	"context"
	"sort"

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
	ZTP             *bool                          `json:"ztp,omitempty"`
	Hostname        *string                        `json:"hostname,omitempty"`
	Users           map[string]*SpecUser           `json:"users,omitempty"`
	PortGroups      map[string]*SpecPortGroup      `json:"portGroupSpeeds,omitempty"`
	Interfaces      map[string]*SpecInterface      `json:"interfaces,omitempty"`
	MCLAGs          map[uint32]*SpecMCLAGDomain    `json:"mclags,omitempty"`
	MCLAGInterfaces map[string]*SpecMCLAGInterface `json:"mclagInterfaces,omitempty"`
	VRFs            map[string]*SpecVRF            `json:"vrfs,omitempty"`
	RouteMaps       map[string]*SpecRouteMap       `json:"routingMaps,omitempty"`
	DHCPRelays      map[string]*SpecDHCPRelay      `json:"dhcpRelays,omitempty"`
	NATs            map[uint32]*SpecNAT            `json:"nats,omitempty"`
	ACLs            map[string]*SpecACL            `json:"acls,omitempty"`
	ACLInterfaces   map[string]*SpecACLInterface   `json:"aclInterfaces,omitempty"`
}

type SpecUser struct {
	Password       string   `json:"password,omitempty"`
	Role           string   `json:"role,omitempty"`
	AuthorizedKeys []string `json:"authorizedKeys,omitempty"`
}

type SpecPortGroup struct {
	Speed *string
}

type SpecInterface struct {
	Description    *string                     `json:"description,omitempty"`
	Enabled        *bool                       `json:"enabled,omitempty"`
	IPs            map[string]*SpecInterfaceIP `json:"ips,omitempty"`
	PortChannel    *string                     `json:"portChannel,omitempty"`
	NATZone        *uint8                      `json:"natZone,omitempty"`
	TrunkVLANRange *string                     `json:"trunkVLANRange,omitempty"`
}

type SpecInterfaceIP struct {
	VLAN      *bool  `json:"vlan,omitempty"`
	PrefixLen *uint8 `json:"prefixLen,omitempty"`
	Secondary *bool  `json:"secondary,omitempty"`
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
	Interfaces       map[string]*SpecVRFInterface       `json:"interfaces,omitempty"`
	BGP              *SpecVRFBGP                        `json:"bgp,omitempty"`
	TableConnections map[string]*SpecVRFTableConnection `json:"tableConnections,omitempty"` // TODO enum for key: "connected" or "static"?
}

type SpecVRFInterface struct{}

type SpecVRFBGP struct {
	AS                 *uint32                         `json:"as,omitempty"`
	NetworkImportCheck *bool                           `json:"networkImportCheck,omitempty"`
	Networks           map[string]*SpecVRFBGPNetwork   `json:"networks,omitempty"`
	Neighbors          map[string]*SpecVRFBGPNeighbor  `json:"neighbors,omitempty"`
	ImportVRFs         map[string]*SpecVRFBGPImportVRF `json:"importVRFs,omitempty"`
}

type SpecVRFBGPNetwork struct{}

type SpecVRFBGPNeighbor struct {
	Enabled     *bool   `json:"enabled,omitempty"`
	IPv4Unicast *bool   `json:"ipv4Unicast,omitempty"`
	RemoteAS    *uint32 `json:"remoteAS,omitempty"`
}

type SpecVRFTableConnection struct {
	ImportPolicies []string `json:"importPolicies,omitempty"`
}

type SpecRouteMap struct {
	NoAdvertise *bool `json:"noAdvertise,omitempty"`
}

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
}

func (s *Spec) CleanupSensetive() {
	for _, user := range s.Users {
		user.Password = "<hidden>"
		user.AuthorizedKeys = []string{}
	}
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
	_ SpecPart = (*SpecUser)(nil)
	_ SpecPart = (*SpecPortGroup)(nil)
	_ SpecPart = (*SpecInterface)(nil)
	_ SpecPart = (*SpecInterfaceIP)(nil)
	_ SpecPart = (*SpecMCLAGDomain)(nil)
	_ SpecPart = (*SpecMCLAGInterface)(nil)
	_ SpecPart = (*SpecVRF)(nil)
	_ SpecPart = (*SpecVRFInterface)(nil)
	_ SpecPart = (*SpecVRFBGP)(nil)
	_ SpecPart = (*SpecVRFBGPNetwork)(nil)
	_ SpecPart = (*SpecVRFBGPNeighbor)(nil)
	_ SpecPart = (*SpecVRFTableConnection)(nil)
	_ SpecPart = (*SpecRouteMap)(nil)
	_ SpecPart = (*SpecDHCPRelay)(nil)
	_ SpecPart = (*SpecNAT)(nil)
	_ SpecPart = (*SpecNATPool)(nil)
	_ SpecPart = (*SpecNATBinding)(nil)
	_ SpecPart = (*SpecNATEntry)(nil)
	_ SpecPart = (*SpecACL)(nil)
	_ SpecPart = (*SpecACLEntry)(nil)
	_ SpecPart = (*SpecACLInterface)(nil)
)

func (s *Spec) IsNil() bool {
	return s == nil
}

func (s *SpecUser) IsNil() bool {
	return s == nil
}

func (s *SpecPortGroup) IsNil() bool {
	return s == nil
}

func (s *SpecInterface) IsNil() bool {
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

func (s *SpecVRFTableConnection) IsNil() bool {
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
