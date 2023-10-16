package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"sigs.k8s.io/yaml"
)

type Fabric struct {
	ControlVIP     string               `json:"controlVIP,omitempty"`
	APIServer      string               `json:"apiServer,omitempty"`
	AgentRepo      string               `json:"agentRepo,omitempty"`
	AgentRepoCA    string               `json:"agentRepoCA,omitempty"`
	VPCVLANRange   VLANRange            `json:"vpcVLANRange,omitempty"`
	Users          []agentapi.UserCreds `json:"users,omitempty"`
	DHCPDConfigMap string               `json:"dhcpdConfigMap,omitempty"`
	DHCPDConfigKey string               `json:"dhcpdConfigKey,omitempty"`
	VPCBackend     string               `json:"vpcBackend,omitempty"`
	SNATAllowed    bool                 `json:"snatAllowed,omitempty"`
	VPCSubnet      string               `json:"vpcSubnet,omitempty"`
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
	if err := cfg.VPCVLANRange.Validate(); err != nil {
		return nil, errors.Wrapf(err, "config: vpcVLANRange is invalid")
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
	if cfg.VPCBackend == "" {
		return nil, errors.Errorf("config: vpcBackend is required")
	}
	if !slices.Contains(agentapi.VPCBackendValues, agentapi.VPCBackend(cfg.VPCBackend)) {
		return nil, errors.Errorf("config: vpcBackend must be one of %v", agentapi.VPCBackendValues)
	}
	if cfg.VPCSubnet == "" {
		return nil, errors.Errorf("config: vpcSubnet is required")
	}

	slog.Debug("Loaded config", "data", spew.Sdump(cfg))

	return cfg, nil
}

type VLANRange struct {
	Min uint16 `json:"min,omitempty"`
	Max uint16 `json:"max,omitempty"`
}

func (r *VLANRange) Validate() error {
	if r.Min == 0 {
		return errors.Errorf("min is required")
	}
	if r.Max == 0 {
		return errors.Errorf("max is required")
	}
	if r.Min >= r.Max {
		return errors.Errorf("min must be less than max")
	}

	return nil
}
