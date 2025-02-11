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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"
)

type Defaultable interface {
	Default()
}

type Validatable interface {
	Validate(ctx context.Context, kube client.Reader, fabricCfg *FabricConfig) (admission.Warnings, error)
}

type Object interface {
	client.Object

	Defaultable
	Validatable
}

type ObjectList interface {
	client.ObjectList

	GetItems() []Object
}

const (
	SwitchProfileVS = "vs"
)

type UserCreds struct {
	Name     string   `json:"name,omitempty"`
	Password string   `json:"password,omitempty"`
	Role     string   `json:"role,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
}

type FabricConfig struct {
	DeploymentID             string      `json:"deploymentID,omitempty"`
	ControlVIP               string      `json:"controlVIP,omitempty"`
	APIServer                string      `json:"apiServer,omitempty"`
	AgentRepo                string      `json:"agentRepo,omitempty"`
	VPCIRBVLANRanges         []VLANRange `json:"vpcIRBVLANRange,omitempty"`
	VPCPeeringVLANRanges     []VLANRange `json:"vpcPeeringVLANRange,omitempty"` // TODO rename (loopback workaround)
	VPCPeeringDisabled       bool        `json:"vpcPeeringDisabled,omitempty"`
	ReservedSubnets          []string    `json:"reservedSubnets,omitempty"`
	Users                    []UserCreds `json:"users,omitempty"`
	FabricMode               FabricMode  `json:"fabricMode,omitempty"`
	BaseVPCCommunity         string      `json:"baseVPCCommunity,omitempty"`
	VPCLoopbackSubnet        string      `json:"vpcLoopbackSubnet,omitempty"`
	FabricMTU                uint16      `json:"fabricMTU,omitempty"`
	ServerFacingMTUOffset    uint16      `json:"serverFacingMTUOffset,omitempty"`
	ESLAGMACBase             string      `json:"eslagMACBase,omitempty"`
	ESLAGESIPrefix           string      `json:"eslagESIPrefix,omitempty"`
	AlloyRepo                string      `json:"alloyRepo,omitempty"`
	AlloyVersion             string      `json:"alloyVersion,omitempty"`
	Alloy                    AlloyConfig `json:"alloy,omitempty"`
	DefaultMaxPathsEBGP      uint32      `json:"defaultMaxPathsEBGP,omitempty"`
	AllowExtraSwitchProfiles bool        `json:"allowExtraSwitchProfiles,omitempty"`
	MCLAGSessionSubnet       string      `json:"mclagSessionSubnet,omitempty"`
	GatewayASN               uint32      `json:"gatewayASN,omitempty"` // Temporarily assuming that all GWs are in the same AS

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
	NOSTypeSONiCBCMBase   NOSType = "sonic-bcm-base"
	NOSTypeSONiCBCMCampus NOSType = "sonic-bcm-campus"
	NOSTypeSONiCBCMVS     NOSType = "sonic-bcm-vs"
)

var NOSTypes = []NOSType{
	NOSTypeSONiCBCMBase,
	NOSTypeSONiCBCMCampus,
	NOSTypeSONiCBCMVS,
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
	err = yaml.UnmarshalStrict(data, cfg)
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

	cfg.Alloy.Default()
	if err := cfg.Alloy.Validate(); err != nil {
		return nil, errors.Wrapf(err, "error validating alloy config")
	}

	if cfg.DefaultMaxPathsEBGP == 0 {
		return nil, errors.Errorf("config: defaultMaxPathsEBGP is required")
	}

	if cfg.GatewayASN == 0 {
		return nil, errors.Errorf("config: gatewayASN is required")
	}

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
