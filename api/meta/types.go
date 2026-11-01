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

package meta

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/libmeta/pkg/alloy"
	corev1 "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	kyaml "sigs.k8s.io/yaml"
)

//nolint:iface // used outside of the package
type Defaultable interface {
	Default()
}

//nolint:iface // used outside of the package
type Validatable interface {
	Validate(ctx context.Context, kube kclient.Reader, fabricCfg *FabricConfig) (admission.Warnings, error)
}

type Object interface {
	kclient.Object

	Defaultable
	Validatable
}

//nolint:iface // used outside of the package
type ObjectList interface {
	kclient.ObjectList

	GetItems() []Object
}

const (
	SwitchProfileVS     = "vs"
	SwitchProfileVSCLSP = "vs-clsp"
	SwitchProfileCmlsVX = "cumulus-vx"
)

var VirtualSwitchProfiles = []string{
	SwitchProfileVS,
	SwitchProfileVSCLSP,
	SwitchProfileCmlsVX,
}

type UserCreds struct {
	Name     string   `json:"name,omitempty"`
	Password string   `json:"password,omitempty"`
	Role     string   `json:"role,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
}

type FabricConfig struct {
	DeploymentID             string        `json:"deploymentID,omitempty"`
	ControlVIP               string        `json:"controlVIP,omitempty"`
	APIServer                string        `json:"apiServer,omitempty"`
	AgentRepo                string        `json:"agentRepo,omitempty"`
	VPCIRBVLANRanges         []VLANRange   `json:"vpcIRBVLANRange,omitempty"`
	VPCPeeringVLANRanges     []VLANRange   `json:"vpcPeeringVLANRange,omitempty"` // Deprecated: ignored, still set by fabricator
	TH5WorkaroundVLANRange   []VLANRange   `json:"th5WorkaroundVLANRange"`
	VPCPeeringDisabled       bool          `json:"vpcPeeringDisabled,omitempty"`
	ReservedSubnets          []string      `json:"reservedSubnets,omitempty"`
	Users                    []UserCreds   `json:"users,omitempty"`
	FabricMode               FabricMode    `json:"fabricMode,omitempty"`
	BaseVPCCommunity         string        `json:"baseVPCCommunity,omitempty"`  // TODO: rename? now only used to mark external prefixes
	VPCLoopbackSubnet        string        `json:"vpcLoopbackSubnet,omitempty"` // Deprecated: ignored, still set by fabricator
	FabricMTU                uint16        `json:"fabricMTU,omitempty"`
	ServerFacingMTUOffset    uint16        `json:"serverFacingMTUOffset,omitempty"`
	ESLAGMACBase             string        `json:"eslagMACBase,omitempty"`
	ESLAGESIPrefix           string        `json:"eslagESIPrefix,omitempty"`
	AlloyRepo                string        `json:"alloyRepo,omitempty"`
	AlloyVersion             string        `json:"alloyVersion,omitempty"`
	Alloy                    AlloyConfig   `json:"alloy,omitempty"` // TODO: not used anymore, remove in future releases
	AlloyTargets             alloy.Targets `json:"alloyTargets,omitempty"`
	Observability            Observability `json:"observability,omitempty"`
	AgentHeartbeatInterval   uint32        `json:"agentHeartbeatInterval,omitempty"` // seconds, default and minimum 15
	AgentEnforceInterval     uint32        `json:"agentEnforceInterval,omitempty"`   // seconds, default and minimum 120
	ControlProxyURL          string        `json:"controlProxyURL,omitempty"`
	DefaultMaxPathsEBGP      uint32        `json:"defaultMaxPathsEBGP,omitempty"`
	AllowExtraSwitchProfiles bool          `json:"allowExtraSwitchProfiles,omitempty"`
	MCLAGSessionSubnet       string        `json:"mclagSessionSubnet,omitempty"` // TODO: deprecated, remove in future releases
	GatewayASN               uint32        `json:"gatewayASN,omitempty"`         // Temporarily assuming that all GWs are in the same AS
	GatewayAPISync           bool          `json:"gatewayAPISync,omitempty"`
	IncludeSONiCCLSPlus      bool          `json:"includeSONiCCLSPlus,omitempty"` // Include Celestica SONiC+
	IncludeCumulus           bool          `json:"includeCumulus,omitempty"`      // Include Cumulus
	ProtocolSubnet           string        `json:"protocolSubnet,omitempty"`
	VTEPSubnet               string        `json:"vtepSubnet,omitempty"`
	FabricSubnet             string        `json:"fabricSubnet,omitempty"`
	DisableBFD               bool          `json:"disableBFD,omitempty"`
	// Deprecated: not read, the Fabric's DisableBFD covers the gateway sessions too
	GatewayBFD bool `json:"gatewayBFD,omitempty"`
	// These, and GatewayASN and DisableBFD, only seed Fabric/default at first startup (or stand in
	// for it while it does not exist). Editing them afterwards has no effect: edit the Fabric instead
	SpineASN     uint32 `json:"spineASN,omitempty"`
	LeafASNStart uint32 `json:"leafASNStart,omitempty"`
	LeafASNEnd   uint32 `json:"leafASNEnd,omitempty"`

	ManagementSubnet      string            `json:"managementSubnet,omitempty"`
	ManagementDHCPStart   string            `json:"managementDHCPStart,omitempty"`
	ManagementDHCPEnd     string            `json:"managementDHCPEnd,omitempty"`
	GatewayCommunities    map[uint32]string `json:"gatewayCommunities,omitempty"`
	L2ProxyExternalSubnet string            `json:"l2ProxyExternalSubnet,omitempty"`

	// Gateway-specific configuration
	EnableGateway         bool                `json:"enableGateway,omitempty"`
	GatewayNamespace      string              `json:"gatewayNamespace,omitempty"` // Namespace where pods for gateways are deployed
	GatewayTolerations    []corev1.Toleration `json:"gatewayTolerations,omitempty"`
	DataplaneRef          string              `json:"dataplaneRef,omitempty"`
	FRRRef                string              `json:"frrRef,omitempty"`     // TODO: remove after 26.05 release
	ToolboxRef            string              `json:"toolboxRef,omitempty"` // TODO: remove after 26.05 release
	DataplaneMetricsPort  uint16              `json:"dataplaneMetricsPort,omitempty"`
	FRRMetricsPort        uint16              `json:"frrMetricsPort,omitempty"`
	DataplaneValidatorRef string              `json:"dataplaneValidatorRef,omitempty"`
	// Hugepages requested by the gateway dataplane, the gateway nodes have to reserve them before k3s starts. The dataplane
	// currently only uses 1G pages, 2M pages are just ignored by it. None are requested if neither is set.
	GatewayHugePages1G int64 `json:"gatewayHugePages1G,omitempty"`
	GatewayHugePages2M int64 `json:"gatewayHugePages2M,omitempty"`

	ExtraValidators ExtraValidators `json:"-"`

	reservedSubnets []netip.Prefix
}

type FabricMode string

const (
	FabricModeSpineLeaf FabricMode = "spine-leaf"
)

var FabricModes = []FabricMode{
	FabricModeSpineLeaf,
}

type NOSType string

const (
	NOSTypeSONiCBCMBase         NOSType = "sonic-bcm-base"
	NOSTypeSONiCBCMCampus       NOSType = "sonic-bcm-campus"
	NOSTypeSONiCBCMVS           NOSType = "sonic-bcm-vs"
	NOSTypeSONiCCLSPlusBroadcom NOSType = "sonic-cls-plus-broadcom"
	NOSTypeSONiCCLSPlusMarvell  NOSType = "sonic-cls-plus-marvell"
	NOSTypeSONiCCLSPlusVS       NOSType = "sonic-cls-plus-vs"
	NOSTypeCumulusVX            NOSType = "cumulus-vx"
	NOSTypeCumulusMlx           NOSType = "cumulus-mlx"
)

var NOSTypes = []NOSType{
	NOSTypeSONiCBCMBase,
	NOSTypeSONiCBCMCampus,
	NOSTypeSONiCBCMVS,
	NOSTypeSONiCCLSPlusBroadcom,
	NOSTypeSONiCCLSPlusMarvell,
	NOSTypeSONiCCLSPlusVS,
	NOSTypeCumulusVX,
	NOSTypeCumulusMlx,
}

var NOSTypesSONiCBCM = []NOSType{
	NOSTypeSONiCBCMBase,
	NOSTypeSONiCBCMCampus,
	NOSTypeSONiCBCMVS,
}

var NOSTypesSONiCCLSPlus = []NOSType{
	NOSTypeSONiCCLSPlusBroadcom,
	NOSTypeSONiCCLSPlusMarvell,
	NOSTypeSONiCCLSPlusVS,
}

var NOSTypesCumulus = []NOSType{
	NOSTypeCumulusVX,
	NOSTypeCumulusMlx,
}

// +kubebuilder:object:generate=true
type Observability struct {
	Agent ObservabilityAgent `json:"agent,omitempty"`
	Unix  ObservabilityUnix  `json:"unix,omitempty"`
}

// +kubebuilder:object:generate=true
type ObservabilityAgent struct {
	Metrics         bool                      `json:"metrics,omitempty"`
	MetricsInterval uint                      `json:"metricsInterval,omitempty"`
	MetricsRelabel  []alloy.ScrapeRelabelRule `json:"metricsRelabel,omitempty"`
	Logs            bool                      `json:"logs,omitempty"`
}

// +kubebuilder:object:generate=true
type ObservabilityUnix struct {
	Metrics           bool                      `json:"metrics,omitempty"`
	MetricsInterval   uint                      `json:"metricsInterval,omitempty"`
	MetricsRelabel    []alloy.ScrapeRelabelRule `json:"metricsRelabel,omitempty"`
	MetricsCollectors []string                  `json:"metricsCollectors,omitempty"`
	Syslog            bool                      `json:"syslog,omitempty"`
}

type ExtraValidators struct {
	Peering    PeeringValidator
	Connection ConnectionValidator
}

type PeeringValidator func(ctx context.Context, kube kclient.Reader, peering kclient.Object) error

type ConnectionValidator func(ctx context.Context, kube kclient.Reader, conn kclient.Object) error

func (cfg *FabricConfig) ParsedReservedSubnets() []netip.Prefix {
	return cfg.reservedSubnets
}

var idChecker = regexp.MustCompile(`^[a-zA-Z0-9][-a-zA-Z0-9]*[a-zA-Z0-9]?$`)

func LoadFabricConfig(basedir string, extraValidators ExtraValidators) (*FabricConfig, error) {
	path := filepath.Join(basedir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading config %s", path)
	}

	cfg := &FabricConfig{}
	err = kyaml.UnmarshalStrict(data, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling config %s", path)
	}

	return cfg.Init(extraValidators)
}

func (cfg *FabricConfig) Init(extraValidators ExtraValidators) (*FabricConfig, error) {
	if cfg.DeploymentID != "" {
		if len(cfg.DeploymentID) > 16 {
			return nil, errors.Errorf("config: deploymentID must be <= 16 characters")
		}
		if len(cfg.DeploymentID) < 3 {
			return nil, errors.Errorf("config: deploymentID must be >= 3 characters")
		}
		if !idChecker.MatchString(cfg.DeploymentID) {
			return nil, errors.Errorf("config: deploymentID must match %s", idChecker.String())
		}
	}

	if cfg.ControlVIP == "" {
		return nil, errors.Errorf("config: controlVIP is required")
	}
	if cfg.APIServer == "" {
		return nil, errors.Errorf("config: apiServer is required")
	}

	if cfg.AgentRepo == "" {
		return nil, errors.Errorf("config: agentRepo is required")
	}

	if r, err := NormalizedVLANRanges(cfg.VPCIRBVLANRanges); err != nil {
		return nil, errors.Wrapf(err, "config: vpcIRBVLANRange is invalid")
	} else { //nolint:revive
		if len(r) == 0 {
			return nil, errors.Errorf("config: vpcIRBVLANRange is required")
		}
		cfg.VPCIRBVLANRanges = r
		// TODO check total ranges size and expose as limit for API validation
	}

	if r, err := NormalizedVLANRanges(cfg.TH5WorkaroundVLANRange); err != nil {
		return nil, errors.Wrapf(err, "config: th5WorkaroundVLANRange is invalid")
	} else { //nolint:revive
		if len(r) == 0 {
			return nil, errors.Errorf("config: th5WorkaroundVLANRange is required")
		}
		cfg.TH5WorkaroundVLANRange = r
	}

	for _, user := range cfg.Users {
		if user.Name == "" {
			return nil, errors.Errorf("config: users: name is required")
		}
		if user.Password == "" {
			return nil, errors.Errorf("config: users: password is required")
		}
		if user.Role == "" {
			return nil, errors.Errorf("config: users: role is required")
		}
		if user.Role != "admin" && user.Role != "operator" { // TODO config?
			return nil, errors.Errorf("config: users: role must be admin or operator")
		}
	}

	if cfg.FabricMode == "" {
		return nil, errors.Errorf("config: fabricMode is required")
	}
	if !slices.Contains(FabricModes, cfg.FabricMode) {
		return nil, errors.Errorf("config: fabricMode must be one of %v", FabricModes)
	}

	if err := cfg.WithReservedSubnets(); err != nil {
		return nil, err
	}

	if cfg.BaseVPCCommunity == "" {
		return nil, errors.Errorf("config: baseVPCCommunity is required")
	}
	// set as-is on switches, which would refuse an invalid one
	baseComm, err := CanonicalCommunity(cfg.BaseVPCCommunity)
	if err != nil {
		return nil, fmt.Errorf("config: baseVPCCommunity: %w", err)
	}
	cfg.BaseVPCCommunity = baseComm

	if cfg.FabricMTU == 0 {
		return nil, errors.Errorf("config: fabricMTU is required")
	}
	if cfg.FabricMTU > 9216 {
		return nil, errors.Errorf("config: fabricMTU must be <= 9216")
	}
	if cfg.ServerFacingMTUOffset == 0 {
		return nil, errors.Errorf("config: serverFacingMTUOffset is required")
	}

	if cfg.FabricMode == FabricModeSpineLeaf {
		if cfg.ESLAGMACBase == "" {
			return nil, errors.Errorf("config: eslagMACBase is required")
		}
		if mac, err := net.ParseMAC(cfg.ESLAGMACBase); err != nil {
			return nil, errors.Errorf("config: eslagMACBase should be a valid MAC address")
		} else if len(mac) != 6 {
			return nil, errors.Errorf("config: eslagMACBase should be a valid 48 bit MAC address")
		}

		if cfg.ESLAGESIPrefix == "" {
			return nil, errors.Errorf("config: eslagESIPrefix is required")
		}
		if len(cfg.ESLAGESIPrefix) != 12 {
			return nil, errors.Errorf("config: eslagESIPrefix should be a valid 12 hex long prefix, e.g. '00:f2:00:00:'")
		}
	}

	if err := cfg.AlloyTargets.Validate(); err != nil {
		return nil, errors.Wrapf(err, "error validating alloy targets")
	}

	if cfg.DefaultMaxPathsEBGP == 0 {
		return nil, errors.Errorf("config: defaultMaxPathsEBGP is required")
	}

	if cfg.GatewayASN == 0 {
		return nil, errors.Errorf("config: gatewayASN is required")
	}

	if cfg.SpineASN == 0 {
		return nil, errors.Errorf("config: spineASN is required")
	}
	if cfg.LeafASNStart == 0 {
		return nil, errors.Errorf("config: leafASNStart is required")
	}
	if cfg.LeafASNEnd == 0 {
		return nil, errors.Errorf("config: leafASNEnd is required")
	}
	if cfg.LeafASNEnd < cfg.LeafASNStart {
		return nil, errors.Errorf("config: leafASNEnd must be greater than or equal to leafASNStart")
	}
	if cfg.SpineASN >= cfg.LeafASNStart && cfg.SpineASN <= cfg.LeafASNEnd {
		return nil, errors.Errorf("config: spineASN must not be in the leaf ASN range")
	}
	if cfg.ManagementSubnet == "" {
		return nil, errors.Errorf("config: managementSubnet is required")
	}
	if cfg.ManagementDHCPStart == "" {
		return nil, errors.Errorf("config: managementDHCPStart is required")
	}
	if cfg.ManagementDHCPEnd == "" {
		return nil, errors.Errorf("config: managementDHCPEnd is required")
	}
	_, mgmtSubnet, err := net.ParseCIDR(cfg.ManagementSubnet)
	if err != nil {
		return nil, errors.Errorf("config: managementSubnet is invalid: %v", err)
	}
	if ipStart := net.ParseIP(cfg.ManagementDHCPStart); ipStart == nil || !mgmtSubnet.Contains(ipStart) {
		return nil, errors.Errorf("config: managementDHCPStart is not a valid IP in managementSubnet")
	}
	if ipEnd := net.ParseIP(cfg.ManagementDHCPEnd); ipEnd == nil || !mgmtSubnet.Contains(ipEnd) {
		return nil, errors.Errorf("config: managementDHCPEnd is not a valid IP in managementSubnet")
	}

	// up to 1 TiB each, anything above is likely a typo or a size in bytes
	if cfg.GatewayHugePages1G < 0 || cfg.GatewayHugePages1G > 1<<10 {
		return nil, fmt.Errorf("config: gatewayHugePages1G must be between 0 and %d", 1<<10) //nolint:err113
	}
	if cfg.GatewayHugePages2M < 0 || cfg.GatewayHugePages2M > 1<<19 {
		return nil, fmt.Errorf("config: gatewayHugePages2M must be between 0 and %d", 1<<19) //nolint:err113
	}

	for idx, commStr := range cfg.GatewayCommunities {
		commStr, err := CanonicalCommunity(commStr)
		if err != nil {
			return nil, fmt.Errorf("config: gatewayCommunity: %w", err)
		}
		cfg.GatewayCommunities[idx] = commStr
		// routes from externals carry baseVPCCommunity, and would pass as a gateway's
		if commStr == cfg.BaseVPCCommunity {
			return nil, fmt.Errorf("config: gatewayCommunity community %s is the same as baseVPCCommunity", commStr) //nolint:err113
		}
	}

	cfg.ExtraValidators = extraValidators
	if cfg.ExtraValidators.Peering == nil {
		return nil, errors.Errorf("config: ExtraValidators.Peering is required")
	}
	if cfg.ExtraValidators.Connection == nil {
		return nil, fmt.Errorf("config: ExtraValidators.Connection is required") //nolint:err113
	}

	// TODO enable in future releases
	// if cfg.ControlProxyURL == "" {
	// 	return nil, errors.Errorf("config: controlProxyURL is required")
	// }

	// TODO validate format of all fields

	// slog.Debug("Loaded Fabric config", "data", spew.Sdump(cfg))

	return cfg, nil
}

// CanonicalCommunity drops any leading zeros, as communities are compared as strings
func CanonicalCommunity(comm string) (string, error) {
	asn, id, ok := strings.Cut(comm, ":")
	if !ok {
		return "", fmt.Errorf("community %s format is invalid", comm) //nolint:err113
	}
	asnVal, err := strconv.ParseUint(asn, 10, 16)
	if err != nil {
		return "", fmt.Errorf("community %s is invalid: %w", comm, err)
	}
	idVal, err := strconv.ParseUint(id, 10, 16)
	if err != nil {
		return "", fmt.Errorf("community %s is invalid: %w", comm, err)
	}

	return fmt.Sprintf("%d:%d", asnVal, idVal), nil
}

func (cfg *FabricConfig) WithReservedSubnets() error {
	if len(cfg.ReservedSubnets) == 0 {
		return errors.Errorf("config: reservedSubnets is required (it should include at least Fabric subnets)")
	}

	for _, subnet := range cfg.ReservedSubnets {
		ipnet, err := netip.ParsePrefix(subnet)
		if err != nil {
			return errors.Wrapf(err, "config: reservedSubnets: invalid subnet %s", subnet)
		}
		cfg.reservedSubnets = append(cfg.reservedSubnets, ipnet.Masked())
	}

	return nil
}
