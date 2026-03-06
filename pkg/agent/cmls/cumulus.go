// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	_ "embed"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/util/logutil"
	kyaml "sigs.k8s.io/yaml"
)

const (
	cfgFile = "last-config.yaml"
)

//go:embed ztp_config.tmpl.yaml
var ztpCfgTmpl string

//go:embed config.tmpl.yaml
var fullCfgTmpl string

type ConfigIn struct {
	Hostname     string
	Users        []User
	NTPServer    string
	ManagementIP string
	ProtocolIP   string
	VTEPIP       string
	ASN          uint32
	HostSubnet   string
	RouterID     string
	VXLANSource  string
	VPCs         []VPC
	BGPNeighbors []BGPNeighbor
	PortConfigs  []PortConfig
	IsSpine      bool
	IsLeaf       bool
}

type User struct {
	Name           string
	HashedPassword string
	Role           string
	SSHKeys        []SSHKey
}

type SSHKey struct {
	Key  string
	Type string
}

type BGPNeighbor struct {
	IP          string
	PeerGroup   string
	Description string
}

type VPC struct {
	Name   string
	VNI    uint32
	Subnet string
}

type PortConfig struct {
	Name            string
	VRF             string
	IP              string
	AdaptiveRouting bool
}

func buildConfigFor(tmpl string, agent *agentapi.Agent) (*bytes.Buffer, error) {
	isSpine := agent.IsSpineLeaf() && agent.Spec.Switch.Role.IsSpine()
	isLeaf := agent.IsSpineLeaf() && agent.Spec.Switch.Role.IsLeaf()

	controlVIP, err := netip.ParsePrefix(agent.Spec.Config.ControlVIP)
	if err != nil {
		return nil, fmt.Errorf("parsing control VIP: %w", err)
	}

	protocolIP, err := netip.ParsePrefix(agent.Spec.Switch.ProtocolIP)
	if err != nil {
		return nil, fmt.Errorf("parsing protocol IP: %w", err)
	}

	vtepIP, err := netip.ParsePrefix(agent.Spec.Switch.VTEPIP)
	if agent.IsSpineLeaf() && agent.Spec.Switch.Role.IsLeaf() && err != nil {
		return nil, fmt.Errorf("parsing VTEP IP: %w", err)
	}

	users := []User{}
	for _, user := range agent.Spec.Users {
		role := ""
		switch user.Role {
		case "admin":
			role = "system-admin"
		case "operator":
			role = "nvue-monitor"
		}

		if role == "" {
			return nil, fmt.Errorf("invalid role: %s", user.Role) //nolint:err113
		}

		keys := []SSHKey{}
		for _, key := range user.SSHKeys {
			parts := strings.Split(key, " ")
			if len(parts) < 2 {
				return nil, fmt.Errorf("invalid SSH key: %s", key) //nolint:err113
			}

			keys = append(keys, SSHKey{
				Key:  parts[1],
				Type: parts[0],
			})
		}

		users = append(users, User{
			Name:           user.Name,
			HashedPassword: user.Password,
			Role:           role,
			SSHKeys:        keys,
		})
	}

	portConfigs := []PortConfig{}

	neighs := []BGPNeighbor{}
	for connName, conn := range agent.Spec.Connections {
		if conn.Fabric == nil {
			continue
		}

		switch {
		case isSpine:
			leafName := ""
			for _, link := range conn.Fabric.Links {
				if link.Spine.DeviceName() != agent.Name {
					continue
				}

				leafName = link.Leaf.DeviceName()
				leafIP, err := netip.ParsePrefix(link.Leaf.IP)
				if err != nil {
					return nil, fmt.Errorf("parsing conn %s leaf IP %s: %w", connName, link.Leaf.IP, err)
				}
				neighs = append(neighs, BGPNeighbor{
					IP:          leafIP.Addr().String(),
					PeerGroup:   "underlay_leaf",
					Description: "fabric underlay to leaf " + link.Leaf.Port,
				})
				portConfigs = append(portConfigs, PortConfig{
					Name:            swp(link.Spine.LocalPortName()),
					IP:              link.Spine.IP,
					AdaptiveRouting: true,
				})
			}

			leafIP, err := netip.ParsePrefix(agent.Spec.Switches[leafName].ProtocolIP)
			if err != nil {
				return nil, fmt.Errorf("parsing sw %s proto IP %s: %w", leafName, agent.Spec.Switches[leafName].ProtocolIP, err)
			}
			neighs = append(neighs, BGPNeighbor{
				IP:          leafIP.Addr().String(),
				PeerGroup:   "overlay",
				Description: "fabric overlay to leaf " + leafName,
			})
		case isLeaf:
			spineName := ""
			for _, link := range conn.Fabric.Links {
				if link.Leaf.DeviceName() != agent.Name {
					continue
				}

				spineName = link.Spine.DeviceName()
				spineIP, err := netip.ParsePrefix(link.Spine.IP)
				if err != nil {
					return nil, fmt.Errorf("parsing conn %s spine IP %s: %w", connName, link.Spine.IP, err)
				}
				neighs = append(neighs, BGPNeighbor{
					IP:          spineIP.Addr().String(),
					PeerGroup:   "underlay_spine",
					Description: "fabric underlay to spine " + link.Spine.Port,
				})
				portConfigs = append(portConfigs, PortConfig{
					Name:            swp(link.Leaf.LocalPortName()),
					IP:              link.Leaf.IP,
					AdaptiveRouting: true,
				})
			}

			spineIP, err := netip.ParsePrefix(agent.Spec.Switches[spineName].ProtocolIP)
			if err != nil {
				return nil, fmt.Errorf("parsing sw %s proto IP %s: %w", spineName, agent.Spec.Switches[spineName].ProtocolIP, err)
			}
			neighs = append(neighs, BGPNeighbor{
				IP:          spineIP.Addr().String(),
				PeerGroup:   "overlay",
				Description: "fabric overlay to spine " + spineName,
			})
		}
	}

	vpcs := []VPC{}
	for vpcName, vpc := range agent.Spec.VPCs {
		if len(vpc.Subnets) != 1 {
			continue
		}

		for subnetName, subnet := range vpc.Subnets {
			vni, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpcName, subnetName)
			if !ok {
				continue
			}

			vpcs = append(vpcs, VPC{
				Name: vpcName,
				VNI:  vni,
				// TODO make sure to only readvertise those subnets?
				Subnet: subnet.Subnet,
			})
		}
	}

	for _, attach := range agent.Spec.VPCAttachments {
		conn, ok := agent.Spec.Connections[attach.Connection]
		if !ok {
			continue
		}

		if conn.Unbundled == nil {
			continue
		}

		if conn.Unbundled.Link.Switch.DeviceName() != agent.Name {
			continue
		}

		vpcName := attach.VPCName()

		portConfigs = append(portConfigs, PortConfig{
			Name: swp(conn.Unbundled.Link.Switch.LocalPortName()),
			VRF:  vpcName,
		})
	}

	slices.SortFunc(neighs, func(a, b BGPNeighbor) int {
		// not ideal, but gives stable ordering
		return strings.Compare(a.IP, b.IP)
	})
	slices.SortFunc(vpcs, func(a, b VPC) int {
		return strings.Compare(a.Name, b.Name)
	})
	slices.SortFunc(portConfigs, func(a, b PortConfig) int {
		// not ideal, but gives stable ordering
		return strings.Compare(a.Name, b.Name)
	})

	cfgIn := ConfigIn{
		Hostname:     agent.Name,
		Users:        users,
		NTPServer:    controlVIP.Addr().String(),
		ManagementIP: agent.Spec.Switch.IP,
		ProtocolIP:   agent.Spec.Switch.ProtocolIP,
		VTEPIP:       agent.Spec.Switch.VTEPIP,
		ASN:          agent.Spec.Switch.ASN,
		RouterID:     protocolIP.Addr().String(),
		VXLANSource:  vtepIP.Addr().String(),
		VPCs:         vpcs,
		BGPNeighbors: neighs,
		PortConfigs:  portConfigs,
		IsSpine:      isSpine,
		IsLeaf:       isLeaf,

		// TODO remove hard-coded value and properly handle
		HostSubnet: "10.0.0.0/8",

		// TODO: keep ports down by default
	}

	// TODO cache template
	cfgTmpl, err := template.New("cumulus_config").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing config template: %w", err)
	}

	cfgBuf := &bytes.Buffer{}
	if err := cfgTmpl.Execute(cfgBuf, cfgIn); err != nil {
		return nil, fmt.Errorf("executing config template: %w", err)
	}

	return cfgBuf, nil
}

func swp(in string) string {
	if portStr, ok := strings.CutPrefix(in, "E1/"); ok {
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return "port-invalid-uint"
		}

		return fmt.Sprintf("swp%d", port)
	}

	return "port-invalid-prefix"
}

//go:embed ztp_script.tmpl.sh
var ztpScriptTmpl string

type ZTPIn struct {
	ControlVIP    string
	InitialConfig string
	KubeConfig    string
	AgentConfig   string
}

func BuildZTPFor(agent *agentapi.Agent, kubeConfig []byte) (*bytes.Buffer, error) {
	controlVIP, err := netip.ParsePrefix(agent.Spec.Config.ControlVIP)
	if err != nil {
		return nil, fmt.Errorf("parsing control VIP: %w", err)
	}

	cfgBuf, err := buildConfigFor(ztpCfgTmpl, agent)
	if err != nil {
		return nil, fmt.Errorf("building config: %w", err)
	}

	agent.Status = agentapi.AgentStatus{}
	agentConfig, err := kyaml.Marshal(agent)
	if err != nil {
		return nil, fmt.Errorf("marshaling agent config: %w", err)
	}

	ztpIn := ZTPIn{
		ControlVIP:    controlVIP.Addr().String(),
		InitialConfig: cfgBuf.String(),
		KubeConfig:    string(kubeConfig),
		AgentConfig:   string(agentConfig),
	}

	// TODO cache template
	ztpTmpl, err := template.New("cumulus_ztp").Parse(ztpScriptTmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing ztp template: %w", err)
	}

	ztpBuf := &bytes.Buffer{}
	if err := ztpTmpl.Execute(ztpBuf, ztpIn); err != nil {
		return nil, fmt.Errorf("executing ztp template: %w", err)
	}

	return ztpBuf, nil
}

func Enforce(ctx context.Context, _ /* processor */ dozer.Processor, agent *agentapi.Agent, basedir string, dryRun bool) error {
	cfgBuf, err := buildConfigFor(fullCfgTmpl, agent)
	if err != nil {
		return fmt.Errorf("building full config: %w", err)
	}

	if dryRun {
		slog.Warn("Dry run, exiting")

		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cfgPath := filepath.Join(basedir, cfgFile)
	if err := os.WriteFile(cfgPath, cfgBuf.Bytes(), 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("writing config file: %w", err)
	}

	// TODO use API instead and probably create a revision named after agent generation + agent version?

	if err := run(ctx, "nv-replace", "nv", "config", "replace", cfgPath); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}

	// Diff exit code is 1 if it's non-empty
	if err := run(ctx, "nv-diff", "nv", "config", "diff", "applied"); err != nil {
		// return fmt.Errorf("diffing config: %w", err)
		slog.Debug("Diff", "err", err.Error())
	}

	if err := run(ctx, "nv-apply", "nv", "config", "apply", "-y"); err != nil {
		return fmt.Errorf("applying config: %w", err)
	}

	return nil
}

func run(ctx context.Context, name, command string, arg ...string) error {
	cmd := exec.CommandContext(ctx, command, arg...)
	cmd.Stdout = logutil.NewSink(ctx, slog.Debug, name+": ")
	cmd.Stderr = logutil.NewSink(ctx, slog.Debug, name+": ")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %s: %w", name, err)
	}

	return nil
}

// TODO proper impl by reading from API
func (c *CumulusProcessor) UpdateSwitchState(ctx context.Context, agent *agentapi.Agent, reg *switchstate.Registry) error {
	swState := &agentapi.SwitchState{
		Interfaces:   map[string]agentapi.SwitchStateInterface{},
		Breakouts:    map[string]agentapi.SwitchStateBreakout{},
		Transceivers: map[string]agentapi.SwitchStateTransceiver{},
		BGPNeighbors: map[string]map[string]agentapi.SwitchStateBGPNeighbor{},
		Platform: agentapi.SwitchStatePlatform{
			Fans:         map[string]agentapi.SwitchStatePlatformFan{},
			PSUs:         map[string]agentapi.SwitchStatePlatformPSU{},
			Temperatures: map[string]agentapi.SwitchStatePlatformTemperature{},
		},
		Firmware: map[string]string{},
	}

	osReleaseData, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("reading os-release: %w", err)
	}
	version := ""
	for line := range strings.Lines(string(osReleaseData)) {
		if after, ok := strings.CutPrefix(line, "VERSION="); ok {
			after = strings.TrimSpace(after)
			if len(after) > 2 {
				version = after[1 : len(after)-1]
			}

			break
		}
	}

	swState.NOS = agentapi.SwitchStateNOS{
		SoftwareVersion: version,
	}

	reg.SaveSwitchState(swState)

	return nil
}
