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

type Defaultable interface {
	Default()
}

type Validatable interface {
	Validate(ctx context.Context, kube kclient.Reader, fabricCfg *FabricConfig) (admission.Warnings, error)
}

type Object interface {
	kclient.Object

	Defaultable
	Validatable
}

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
	DeploymentID             string            `json:"deploymentID,omitempty"`
	ControlVIP               string            `json:"controlVIP,omitempty"`
	APIServer                string            `json:"apiServer,omitempty"`
	AgentRepo                string            `json:"agentRepo,omitempty"`
	VPCIRBVLANRanges         []VLANRange       `json:"vpcIRBVLANRange,omitempty"`
	VPCPeeringVLANRanges     []VLANRange       `json:"vpcPeeringVLANRange,omitempty"` // TODO rename (loopback workaround)
	TH5WorkaroundVLANRange   []VLANRange       `json:"th5WorkaroundVLANRange"`
	VPCPeeringDisabled       bool              `json:"vpcPeeringDisabled,omitempty"`
	ReservedSubnets          []string          `json:"reservedSubnets,omitempty"`
	Users                    []UserCreds       `json:"users,omitempty"`
	FabricMode               FabricMode        `json:"fabricMode,omitempty"`
	BaseVPCCommunity         string            `json:"baseVPCCommunity,omitempty"`
	VPCLoopbackSubnet        string            `json:"vpcLoopbackSubnet,omitempty"`
	FabricMTU                uint16            `json:"fabricMTU,omitempty"`
	ServerFacingMTUOffset    uint16            `json:"serverFacingMTUOffset,omitempty"`
	ESLAGMACBase             string            `json:"eslagMACBase,omitempty"`
	ESLAGESIPrefix           string            `json:"eslagESIPrefix,omitempty"`
	AlloyRepo                string            `json:"alloyRepo,omitempty"`
	AlloyVersion             string            `json:"alloyVersion,omitempty"`
	Alloy                    AlloyConfig       `json:"alloy,omitempty"` // TODO: not used anymore, remove in future releases
	AlloyTargets             alloy.Targets     `json:"alloyTargets,omitempty"`
	Observability            Observability     `json:"observability,omitempty"`
	ControlProxyURL          string            `json:"controlProxyURL,omitempty"`
	DefaultMaxPathsEBGP      uint32            `json:"defaultMaxPathsEBGP,omitempty"`
	AllowExtraSwitchProfiles bool              `json:"allowExtraSwitchProfiles,omitempty"`
	MCLAGSessionSubnet       string            `json:"mclagSessionSubnet,omitempty"` // TODO: deprecated, remove in future releases
	GatewayASN               uint32            `json:"gatewayASN,omitempty"`         // Temporarily assuming that all GWs are in the same AS
	GatewayAPISync           bool              `json:"gatewayAPISync,omitempty"`
	LoopbackWorkaround       bool              `json:"loopbackWorkaround,omitempty"`
	IncludeSONiCCLSPlus      bool              `json:"includeSONiCCLSPlus,omitempty"` // Include Celestica SONiC+
	IncludeCumulus           bool              `json:"includeCumulus,omitempty"`      // Include Cumulus
	ProtocolSubnet           string            `json:"protocolSubnet,omitempty"`
	VTEPSubnet               string            `json:"vtepSubnet,omitempty"`
	FabricSubnet             string            `json:"fabricSubnet,omitempty"`
	DisableBFD               bool              `json:"disableBFD,omitempty"`
	GatewayBFD               bool              `json:"gatewayBFD,omitempty"`
	SpineASN                 uint32            `json:"spineASN,omitempty"`
	LeafASNStart             uint32            `json:"leafASNStart,omitempty"`
	LeafASNEnd               uint32            `json:"leafASNEnd,omitempty"`
	ManagementSubnet         string            `json:"managementSubnet,omitempty"`
	ManagementDHCPStart      string            `json:"managementDHCPStart,omitempty"`
	ManagementDHCPEnd        string            `json:"managementDHCPEnd,omitempty"`
	GatewayCommunities       map[uint32]string `json:"gatewayCommunities,omitempty"`
	L2ProxyExternalSubnet    string            `json:"l2ProxyExternalSubnet,omitempty"`

	// Gateway-specific configuration
	EnableGateway         bool                `json:"enableGateway,omitempty"`
	GatewayNamespace      string              `json:"gatewayNamespace,omitempty"` // Namespace where pods for gateways are deployed
	GatewayTolerations    []corev1.Toleration `json:"gatewayTolerations,omitempty"`
	DataplaneRef          string              `json:"dataplaneRef,omitempty"`
	FRRRef                string              `json:"frrRef,omitempty"`
	ToolboxRef            string              `json:"toolboxRef,omitempty"`
	DataplaneMetricsPort  uint16              `json:"dataplaneMetricsPort,omitempty"`
	FRRMetricsPort        uint16              `json:"frrMetricsPort,omitempty"`
	DataplaneValidatorRef string              `json:"dataplaneValidatorRef,omitempty"`

	ExtraValidators ExtraValidators `json:"-"`

	reservedSubnets []netip.Prefix
}

// Fabric-owned community namespaces for external routes. They replace the ISP-controlled
// communities we used to derive internal preference from, so nothing an external system sends us
// can steer our own route selection. Kept out of pkg/agent so the gateway controller can name the
// same values, and the natural home for ExternalCommunitiesConfig should they become configurable.
const (
	// ExtRankCommBase carries both priority axes packed together, stamped when a route is leaked
	// into a VPC VRF
	ExtRankCommBase = 50002
	// ExtIDCommBase carries the identity of an External, stamped at ingress
	ExtIDCommBase = 50003
	// UplinkPrioCommBase carries the uplink axis priority, stamped at ingress
	UplinkPrioCommBase = 50004
	// ExtAttachIDCommBase carries the identity of one attachment, stamped at ingress
	ExtAttachIDCommBase = 50005

	// ExternalPreference is the local preference an unranked external route gets. Ranked ones sit
	// below it, one step per priority level, and the whole band stays above the 100 that plain VPC
	// routes use.
	ExternalPreference = 150

	// MaxExtPrioLevels is the number of preference classes on the External axis: which External a
	// VPC prefers. Bounds External.spec.priority and ExternalPeering...external.priority.
	MaxExtPrioLevels = 4
	// MaxUplinkPrioLevels is the number of preference classes on the uplink axis: which link to
	// one External. Bounds ExternalAttachment.spec.priority.
	MaxUplinkPrioLevels = 4
	// The two are packed into a single rank when a route is leaked into a VPC VRF, so their
	// product is what the preference band has to hold. 4x4 leaves 135..150.
)

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
	Peering PeeringValidator
}

type PeeringValidator func(ctx context.Context, kube kclient.Reader, peering kclient.Object) error

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

	if r, err := NormalizedVLANRanges(cfg.VPCPeeringVLANRanges); err != nil {
		return nil, errors.Wrapf(err, "config: vpcPeeringVLANRange is invalid")
	} else { //nolint:revive
		if len(r) == 0 {
			return nil, errors.Errorf("config: vpcPeeringVLANRange is required")
		}
		cfg.VPCPeeringVLANRanges = r
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
	if cfg.VPCLoopbackSubnet == "" {
		return nil, errors.Errorf("config: vpcLoopbackSubnet is required")
	}

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

	for _, commStr := range cfg.GatewayCommunities {
		parts := strings.Split(commStr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("config: gatewayCommunity community %s format is invalid", commStr) //nolint:err113
		}
		if _, err := strconv.ParseUint(parts[0], 10, 16); err != nil {
			return nil, fmt.Errorf("config: gatewayCommunity community %s is invalid: %w", commStr, err)
		}
		if _, err := strconv.ParseUint(parts[1], 10, 16); err != nil {
			return nil, fmt.Errorf("config: gatewayCommunity community %s is invalid: %w", commStr, err)
		}
	}

	cfg.ExtraValidators = extraValidators
	if cfg.ExtraValidators.Peering == nil {
		return nil, errors.Errorf("config: ExtraValidators.Peering is required")
	}

	// TODO enable in future releases
	// if cfg.ControlProxyURL == "" {
	// 	return nil, errors.Errorf("config: controlProxyURL is required")
	// }

	// TODO validate format of all fields

	// slog.Debug("Loaded Fabric config", "data", spew.Sdump(cfg))

	return cfg, nil
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
