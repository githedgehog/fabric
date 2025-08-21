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
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/pkg/errors"
	"go.githedgehog.com/libmeta/pkg/alloy"
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
)

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
	VPCPeeringVLANRanges     []VLANRange   `json:"vpcPeeringVLANRange,omitempty"` // TODO rename (loopback workaround)
	VPCPeeringDisabled       bool          `json:"vpcPeeringDisabled,omitempty"`
	ReservedSubnets          []string      `json:"reservedSubnets,omitempty"`
	Users                    []UserCreds   `json:"users,omitempty"`
	FabricMode               FabricMode    `json:"fabricMode,omitempty"`
	BaseVPCCommunity         string        `json:"baseVPCCommunity,omitempty"`
	VPCLoopbackSubnet        string        `json:"vpcLoopbackSubnet,omitempty"`
	FabricMTU                uint16        `json:"fabricMTU,omitempty"`
	ServerFacingMTUOffset    uint16        `json:"serverFacingMTUOffset,omitempty"`
	ESLAGMACBase             string        `json:"eslagMACBase,omitempty"`
	ESLAGESIPrefix           string        `json:"eslagESIPrefix,omitempty"`
	AlloyRepo                string        `json:"alloyRepo,omitempty"`
	AlloyVersion             string        `json:"alloyVersion,omitempty"`
	Alloy                    AlloyConfig   `json:"alloy,omitempty"` // TODO: not used anymore, remove in future releases
	AlloyTargets             alloy.Targets `json:"alloyTargets,omitempty"`
	Observability            Observability `json:"observability,omitempty"`
	ControlProxyURL          string        `json:"controlProxyURL,omitempty"`
	DefaultMaxPathsEBGP      uint32        `json:"defaultMaxPathsEBGP,omitempty"`
	AllowExtraSwitchProfiles bool          `json:"allowExtraSwitchProfiles,omitempty"`
	MCLAGSessionSubnet       string        `json:"mclagSessionSubnet,omitempty"`
	GatewayASN               uint32        `json:"gatewayASN,omitempty"` // Temporarily assuming that all GWs are in the same AS
	GatewayAPISync           bool          `json:"gatewayAPISync,omitempty"`
	LoopbackWorkaround       bool          `json:"loopbackWorkaround,omitempty"`
	IncludeSONiCCLSPlus      bool          `json:"includeSONiCCLSPlus,omitempty"` // Include Celestica SONiC+
	ProtocolSubnet           string        `json:"protocolSubnet,omitempty"`
	VTEPSubnet               string        `json:"vtepSubnet,omitempty"`
	FabricSubnet             string        `json:"fabricSubnet,omitempty"`
	DisableBFD               bool          `json:"disableBFD,omitempty"`

	reservedSubnets []*net.IPNet
}

type FabricMode string

const (
	FabricModeCollapsedCore FabricMode = "collapsed-core"
	FabricModeSpineLeaf     FabricMode = "spine-leaf"
)

var FabricModes = []FabricMode{
	FabricModeCollapsedCore,
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
)

var NOSTypes = []NOSType{
	NOSTypeSONiCBCMBase,
	NOSTypeSONiCBCMCampus,
	NOSTypeSONiCBCMVS,
	NOSTypeSONiCCLSPlusBroadcom,
	NOSTypeSONiCCLSPlusMarvell,
	NOSTypeSONiCCLSPlusVS,
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

func (cfg *FabricConfig) ParsedReservedSubnets() []*net.IPNet {
	return cfg.reservedSubnets
}

var idChecker = regexp.MustCompile(`^[a-zA-Z0-9][-a-zA-Z0-9]*[a-zA-Z0-9]?$`)

func LoadFabricConfig(basedir string) (*FabricConfig, error) {
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

	return cfg.Init()
}

func (cfg *FabricConfig) Init() (*FabricConfig, error) {
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

	if cfg.MCLAGSessionSubnet == "" {
		return nil, errors.Errorf("config: mclagSessionSubnet is required")
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
		_, ipnet, err := net.ParseCIDR(subnet)
		if err != nil {
			return errors.Wrapf(err, "config: reservedSubnets: invalid subnet %s", subnet)
		}
		cfg.reservedSubnets = append(cfg.reservedSubnets, ipnet)
	}

	return nil
}
