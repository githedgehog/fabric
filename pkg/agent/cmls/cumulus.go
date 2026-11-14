// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	_ "embed"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/util/logutil"
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
	Ports        []*PortConfig
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
	Name      string
	VNI       uint32
	Subnet    string
	RouteSumm []string
}

type PortConfig struct {
	Name            string
	Breakout        string
	Speed           string
	AutoNeg         string
	Used            bool
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

	if agent.Spec.SwitchProfile == nil {
		return nil, fmt.Errorf("switch profile is missing") //nolint:err113
	}

	api2nos, err := agent.Spec.SwitchProfile.GetAPI2NOSPortsFor(&agent.Spec.Switch)
	if err != nil {
		return nil, fmt.Errorf("getting NOS port names: %w", err)
	}

	apiPorts, err := agent.Spec.SwitchProfile.GetAvailableAPIPorts(&agent.Spec.Switch)
	if err != nil {
		return nil, fmt.Errorf("getting available ports: %w", err)
	}
	apiPortNames := slices.Collect(maps.Keys(apiPorts))

	disabled, err := agent.Spec.SwitchProfile.GetDisabledAPIPorts(&agent.Spec.Switch)
	if err != nil {
		return nil, fmt.Errorf("getting disabled ports: %w", err)
	}

	// all ports get explicit speeds, the configured or default one for the speed ports and the breakout mode one for the
	// breakout ports, as NVUE breakout only sets the number of ports, e.g. 4x on a SN5610 800G port allows up to 200G
	// per port (2 lanes each by default)
	speeds := maps.Clone(agent.Spec.Switch.PortSpeeds)
	if speeds == nil {
		speeds = map[string]string{}
	}

	// broken out ports are configured with the breakout mode, e.g. 4x, and their sub-ports with the rest of the config,
	// ports disabled by the breakout of another port have to be explicitly disabled
	breakouts := map[string]string{}
	for portName, port := range agent.Spec.SwitchProfile.Ports {
		if _, ok := disabled[portName]; ok {
			breakouts[portName] = "disabled"
			apiPortNames = append(apiPortNames, portName)

			continue
		}
		if port.Profile == "" {
			continue
		}

		profile := agent.Spec.SwitchProfile.PortProfiles[port.Profile]
		if profile.Speed != nil {
			if _, ok := speeds[portName]; !ok {
				speeds[portName] = profile.Speed.Default
			}

			continue
		}

		breakout := profile.Breakout
		if breakout == nil {
			continue
		}

		mode, ok := agent.Spec.Switch.PortBreakouts[portName]
		if !ok {
			mode = breakout.Default
		}
		offsets := len(breakout.Supported[mode].Offsets)
		if offsets > 1 {
			breakouts[portName] = fmt.Sprintf("%dx", offsets)
			apiPortNames = append(apiPortNames, portName)
		}

		_, speed, _ := strings.Cut(mode, "x")
		for idx := range offsets {
			speeds[fmt.Sprintf("%s/%d", portName, idx+1)] = speed
		}
	}
	wiringapi.SortPortNames(apiPortNames)

	autoNegAllowed, autoNegDefault, err := agent.Spec.SwitchProfile.GetAutoNegsDefaultsFor(&agent.Spec.Switch)
	if err != nil {
		return nil, fmt.Errorf("getting auto-negotiation defaults: %w", err)
	}
	autoNegs := maps.Clone(agent.Spec.Switch.PortAutoNegs)

	ports := make([]*PortConfig, 0, len(apiPortNames))
	portsByName := map[string]*PortConfig{}
	for _, apiPortName := range apiPortNames {
		if breakout, ok := breakouts[apiPortName]; ok {
			ports = append(ports, &PortConfig{
				Name:     agent.Spec.SwitchProfile.Ports[apiPortName].NOSName,
				Breakout: breakout,
			})

			continue
		}

		name, err := nosPortName(api2nos, apiPortName)
		if err != nil {
			return nil, err
		}

		port := &PortConfig{Name: name, Speed: speeds[apiPortName]}
		delete(speeds, apiPortName)

		autoNeg, ok := autoNegs[apiPortName]
		delete(autoNegs, apiPortName)
		if autoNegAllowed[apiPortName] {
			if !ok {
				autoNeg = autoNegDefault[apiPortName]
			}
			port.AutoNeg = "disabled"
			if autoNeg {
				port.AutoNeg = "enabled"
			}
		} else if ok {
			return nil, fmt.Errorf("port %s auto-negotiation can't be configured", apiPortName) //nolint:err113
		}

		ports = append(ports, port)
		portsByName[name] = port
	}
	for portName := range speeds {
		return nil, fmt.Errorf("port %s speed can't be configured, port isn't available", portName) //nolint:err113
	}
	for portName := range autoNegs {
		return nil, fmt.Errorf("port %s auto-negotiation can't be configured, port isn't available", portName) //nolint:err113
	}

	usePort := func(apiPortName string) (*PortConfig, error) {
		name, err := nosPortName(api2nos, apiPortName)
		if err != nil {
			return nil, err
		}

		port, ok := portsByName[name]
		if !ok {
			return nil, fmt.Errorf("port %s isn't available with the switch profile and port breakouts", apiPortName) //nolint:err113
		}
		if port.Used {
			return nil, fmt.Errorf("port %s is used more than once", apiPortName) //nolint:err113
		}
		port.Used = true

		return port, nil
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
				if link.Leaf.IP == "" || link.Spine.IP == "" {
					return nil, fmt.Errorf("conn %s: unnumbered fabric links are not supported on Cumulus yet", connName) //nolint:err113
				}
				leafIP, err := netip.ParsePrefix(link.Leaf.IP)
				if err != nil {
					return nil, fmt.Errorf("parsing conn %s leaf IP %s: %w", connName, link.Leaf.IP, err)
				}
				port, err := usePort(link.Spine.LocalPortName())
				if err != nil {
					return nil, fmt.Errorf("conn %s: %w", connName, err)
				}
				neighs = append(neighs, BGPNeighbor{
					IP:          leafIP.Addr().String(),
					PeerGroup:   "underlay_leaf",
					Description: "fabric underlay to leaf " + link.Leaf.Port,
				})
				port.IP = link.Spine.IP
				port.AdaptiveRouting = true
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
				if link.Spine.IP == "" || link.Leaf.IP == "" {
					return nil, fmt.Errorf("conn %s: unnumbered fabric links are not supported on Cumulus yet", connName) //nolint:err113
				}
				spineIP, err := netip.ParsePrefix(link.Spine.IP)
				if err != nil {
					return nil, fmt.Errorf("parsing conn %s spine IP %s: %w", connName, link.Spine.IP, err)
				}
				port, err := usePort(link.Leaf.LocalPortName())
				if err != nil {
					return nil, fmt.Errorf("conn %s: %w", connName, err)
				}
				neighs = append(neighs, BGPNeighbor{
					IP:          spineIP.Addr().String(),
					PeerGroup:   "underlay_spine",
					Description: "fabric underlay to spine " + link.Spine.Port,
				})
				port.IP = link.Leaf.IP
				port.AdaptiveRouting = true
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

	routeSumm := map[string][]string{}
	if agent.Annotations != nil {
		for hook := range strings.SplitSeq(agent.Annotations[wiringapi.AnnotationSwitchRouteSumm], ",") {
			parts := strings.SplitN(hook, "=", 2)
			if len(parts) != 2 {
				continue
			}
			routeSumm[parts[0]] = append(routeSumm[parts[0]], parts[1])
		}
		for k, v := range routeSumm {
			slices.Sort(v)
			routeSumm[k] = v
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
				Name:      vpcName,
				VNI:       vni,
				RouteSumm: routeSumm[vpcName],
				// TODO make sure to only readvertise those subnets?
				Subnet: subnet.Subnet,
			})
		}
	}

	for attachName, attach := range agent.Spec.VPCAttachments {
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

		ip := ""
		if p2pSubnet, ok := attach.Annotations[vpcapi.AnnotationVPCAttachmentP2PLink]; ok && p2pSubnet != "" {
			p2p, err := netip.ParsePrefix(p2pSubnet)
			if err != nil {
				return nil, fmt.Errorf("parsing p2p subnet %q for vpc attachment %q: %w", p2pSubnet, attachName, err)
			}
			if !p2p.IsValid() || !p2p.Addr().Is4() || p2p.Bits() != 31 {
				return nil, fmt.Errorf("p2p subnet %q for vpc attachment %q is not /31", p2pSubnet, attachName) //nolint:err113
			}
			ip = netip.PrefixFrom(p2p.Masked().Addr().Next(), p2p.Bits()).String() // even goes to the host, odd to the switch
		}

		port, err := usePort(conn.Unbundled.Link.Switch.LocalPortName())
		if err != nil {
			return nil, fmt.Errorf("vpc attachment %q: %w", attachName, err)
		}
		port.VRF = vpcName
		port.IP = ip
	}

	slices.SortFunc(neighs, func(a, b BGPNeighbor) int {
		// not ideal, but gives stable ordering
		return strings.Compare(a.IP, b.IP)
	})
	slices.SortFunc(vpcs, func(a, b VPC) int {
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
		Ports:        ports,
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

func nosPortName(api2nos map[string]string, apiPortName string) (string, error) {
	name, ok := api2nos[apiPortName]
	if !ok {
		return "", fmt.Errorf("port %s isn't available with the switch profile and port breakouts", apiPortName) //nolint:err113
	}

	return name, nil
}

//go:embed ztp_script.tmpl.sh
var ztpScriptTmpl string

type ZTPIn struct {
	ControlVIP    string
	InitialConfig string
	KubeConfig    string
	AgentConfig   string
	AgentVersion  string
}

func BuildZTPFor(agent *agentapi.Agent, kubeConfig []byte, agentVersion string) (*bytes.Buffer, error) {
	if agentVersion == "" {
		return nil, fmt.Errorf("agent version is empty") //nolint:err113
	}

	controlVIP, err := netip.ParsePrefix(agent.Spec.Config.ControlVIP)
	if err != nil {
		return nil, fmt.Errorf("parsing control VIP: %w", err)
	}

	cfgBuf, err := buildConfigFor(ztpCfgTmpl, agent)
	if err != nil {
		return nil, fmt.Errorf("building config: %w", err)
	}

	agentConfig, err := agent.MarshalInstallConfig()
	if err != nil {
		return nil, fmt.Errorf("marshaling agent config: %w", err)
	}

	ztpIn := ZTPIn{
		ControlVIP:    controlVIP.Addr().String(),
		InitialConfig: cfgBuf.String(),
		KubeConfig:    string(kubeConfig),
		AgentConfig:   string(agentConfig),
		AgentVersion:  agentVersion,
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
