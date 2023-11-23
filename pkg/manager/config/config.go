package config

import (
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/api/meta"
	"sigs.k8s.io/yaml"
)

type Fabric struct {
	ControlVIP           string               `json:"controlVIP,omitempty"`
	APIServer            string               `json:"apiServer,omitempty"`
	AgentRepo            string               `json:"agentRepo,omitempty"`
	AgentRepoCA          string               `json:"agentRepoCA,omitempty"`
	VPCIRBVLANRangse     []meta.VLANRange     `json:"vpcIRBVLANRange,omitempty"`
	VPCPeeringVLANRanges []meta.VLANRange     `json:"vpcPeeringVLANRange,omitempty"`
	VPCPeeringDisabled   bool                 `json:"vpcPeeringDisabled,omitempty"`
	ReservedSubnets      []string             `json:"reservedSubnets,omitempty"`
	Users                []agentapi.UserCreds `json:"users,omitempty"`
	DHCPDConfigMap       string               `json:"dhcpdConfigMap,omitempty"`
	DHCPDConfigKey       string               `json:"dhcpdConfigKey,omitempty"`
	FabricMode           FabricMode           `json:"fabricMode,omitempty"`

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

func Load(basedir string) (*Fabric, error) {
	path := filepath.Join(basedir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading config %s", path)
	}

	cfg := &Fabric{}
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling config %s", path)
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
	if cfg.AgentRepoCA == "" {
		return nil, errors.Errorf("config: agentRepoCA is required")
	}

	if r, err := meta.NormalizedVLANRanges(cfg.VPCIRBVLANRangse); err != nil {
		return nil, errors.Wrapf(err, "config: vpcIRBVLANRange is invalid")
	} else {
		if len(r) == 0 {
			return nil, errors.Errorf("config: vpcIRBVLANRange is required")
		}
		cfg.VPCIRBVLANRangse = r
		// TODO check total ranges size and expose as limit for API validation
	}

	if r, err := meta.NormalizedVLANRanges(cfg.VPCPeeringVLANRanges); err != nil {
		return nil, errors.Wrapf(err, "config: vpcPeeringVLANRange is invalid")
	} else {
		if len(r) == 0 {
			return nil, errors.Errorf("config: vpcPeeringVLANRange is required")
		}
		cfg.VPCPeeringVLANRanges = r
		// TODO check total ranges size and expose as limit for API validation
	}

	if cfg.DHCPDConfigMap == "" {
		return nil, errors.Errorf("config: dhcpdConfigMap is required")
	}
	if cfg.DHCPDConfigKey == "" {
		return nil, errors.Errorf("config: dhcpdConfigKey is required")
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
	if !slices.Contains(FabricModes, FabricMode(cfg.FabricMode)) {
		return nil, errors.Errorf("config: fabricMode must be one of %v", FabricModes)
	}

	if len(cfg.ReservedSubnets) == 0 {
		return nil, errors.Errorf("config: reservedSubnets is required (it should include at least Fabric subnets)")
	}
	for _, subnet := range cfg.ReservedSubnets {
		_, ipnet, err := net.ParseCIDR(subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "config: reservedSubnets: invalid subnet %s", subnet)
		}
		cfg.reservedSubnets = append(cfg.reservedSubnets, ipnet)
	}

	slog.Debug("Loaded config", "data", spew.Sdump(cfg))

	return cfg, nil
}

func (cfg *Fabric) ParsedReservedSubnets() []*net.IPNet {
	return cfg.reservedSubnets
}
