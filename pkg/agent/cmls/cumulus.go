// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
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
	fmeta "go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"go.githedgehog.com/fabric/pkg/util/logutil"
	"go.githedgehog.com/fabric/pkg/version"
)

const (
	cfgFile         = "last-config.yaml"
	diffFile        = "last-diff.yaml"
	appliedDiffFile = "last-diff-applied.yaml"
)

//go:embed ztp_config.tmpl.yaml
var ztpCfgTmpl string

//go:embed config.tmpl.yaml
var fullCfgTmpl string

type ConfigIn struct {
	Hostname      string
	Users         []User
	NTPServer     string
	ManagementIP  string
	ProtocolIP    string
	VTEPIP        string
	ASN           uint32
	RouterID      string
	VXLANSource   string
	VPCs          []VPC
	Externals     []External
	BGPNeighbors  []BGPNeighbor
	Gateway       *GatewayConfig
	GatewayPrios  []GatewayPrio
	Ports         []*PortConfig
	Bonds         []*BondConfig
	Subinterfaces []*SubinterfaceConfig
	Subnets       []*SubnetConfig
	SubnetACLs    []*SubnetConfig
	Restricted    []*SubnetConfig
	DHCPServer    string
	IsSpine       bool
	IsLeaf        bool
	IsSpectrum4   bool
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

type GatewayConfig struct {
	ASN        uint32
	VTEPSubnet string
}

// GatewayPrio is a gateway priority community, the routes with a higher priority (a lower number) get a higher local
// preference on the leaves
type GatewayPrio struct {
	Name      string
	Community string
	RuleID    int
	LocalPref int
}

type VPC struct {
	Name        string
	VRF         string
	VNI         uint32
	Subnets     map[uint32]string // subnet VNI % 100 -> subnet, as NVUE only allows a single prefix per rule
	RouteSumm   []string
	Peers       []VPCPeer
	PeerSubnets map[uint32]string // subnet VNI -> subnet of all peered VPCs
	Routes      map[string][]string
}

// VPCPeer is a peered VPC, only its own subnets are imported from its VRF
type VPCPeer struct {
	Name   string
	VRF    string
	RuleID uint32
}

func vpcVRF(vpcName string) string {
	return "vpc-" + vpcName
}

// External is an external attached to the switch, with the external VNI as L3VNI of its VRF
type External struct {
	Name string
	VRF  string
	VNI  uint32
}

func extVRF(extName string) string {
	return "ext-" + extName
}

type PortConfig struct {
	Name            string
	Breakout        string
	Speed           string
	AutoNeg         string
	FEC             string
	Used            bool
	Up              bool
	Description     string
	IP              string
	VRF             string
	AdaptiveRouting bool
	BridgeMember
}

// SubinterfaceConfig is a VLAN subinterface of a port with an external attachment
type SubinterfaceConfig struct {
	Name        string
	Port        string
	VLAN        uint16
	IP          string
	VRF         string
	Description string
}

type BondConfig struct {
	Name        string
	Members     []*PortConfig
	LACPBypass  bool
	Attached    bool
	Up          bool
	Description string
	BridgeMember

	id uint16
}

// BridgeMember is a port or bond carrying the VLANs of the VPC subnets attached to it
type BridgeMember struct {
	VLANs    []uint16
	Untagged uint16
	ACLs     []string
}

func (m *BridgeMember) attach(vlan uint16, native bool) error {
	if slices.Contains(m.VLANs, vlan) {
		return fmt.Errorf("vlan %d is attached more than once", vlan) //nolint:err113
	}
	m.VLANs = append(m.VLANs, vlan)
	slices.Sort(m.VLANs)

	if native {
		if m.Untagged != 0 {
			return fmt.Errorf("native vlan is already %d", m.Untagged) //nolint:err113
		}
		m.Untagged = vlan
	}

	return nil
}

// SubnetConfig is an attached VPC subnet: a VLAN mapped to the subnet VNI and an SVI with the anycast gateway
type SubnetConfig struct {
	Name          string
	VLAN          uint16
	VNI           uint32
	VRF           string
	Subnet        string
	Gateway       string
	IP            string
	Description   string
	DHCP          bool
	ACL           string
	Deny          map[uint32]string // subnet ID -> isolated or peered VPC subnet this one isn't permitted to reach
	RestrictedACL string
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

	// PFC buffers from the NVIDIA Spectrum-X deployment guide for EVPN and adaptive routing are only set on Spectrum-4,
	// other silicons use the PFC buffers calculated by Cumulus as the fixed ones don't fit them, e.g. exceed the SN2201
	// port headroom max, and adaptive routing is only supported on Spectrum-4 ports at 200G and 400G
	isSpectrum4 := agent.Spec.SwitchProfile.SwitchSilicon == switchprofile.SiliconNVIDIASpectrum4
	adaptiveRouting := func(port *PortConfig) bool {
		return isSpectrum4 && (port.Speed == "200G" || port.Speed == "400G")
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

	// FEC is only set where configured, other ports keep the NVUE default (auto), the base name of a non broken out
	// breakout port is an alias of its only sub-port
	fecConfigurable, err := agent.Spec.SwitchProfile.GetFECConfigurablePorts(&agent.Spec.Switch)
	if err != nil {
		return nil, fmt.Errorf("getting FEC configurable ports: %w", err)
	}
	for apiPortName, fec := range agent.Spec.Switch.PortFECs {
		if !fecConfigurable[apiPortName] {
			return nil, fmt.Errorf("port %s FEC can't be configured", apiPortName) //nolint:err113
		}

		name, err := nosPortName(api2nos, apiPortName)
		if err != nil {
			return nil, err
		}

		port, ok := portsByName[name]
		if !ok {
			return nil, fmt.Errorf("port %s FEC can't be configured, port isn't available", apiPortName) //nolint:err113
		}
		if port.FEC != "" {
			return nil, fmt.Errorf("port %s FEC is configured more than once", apiPortName) //nolint:err113
		}

		switch fec {
		case wiringapi.PortFECModeRS:
			port.FEC = "rs"
		case wiringapi.PortFECModeFC:
			port.FEC = "baser"
		case wiringapi.PortFECModeAuto:
			port.FEC = "auto"
		case wiringapi.PortFECModeDisabled:
			port.FEC = "off"
		default:
			return nil, fmt.Errorf("port %s FEC mode %q is invalid", apiPortName, fec) //nolint:err113
		}
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
	var gateway *GatewayConfig
	for connName, conn := range agent.Spec.Connections {
		switch {
		case conn.Gateway != nil:
			for _, link := range conn.Gateway.Links {
				if link.Switch.DeviceName() != agent.Name {
					continue
				}

				// the gateway only gets the VTEPs and only what it originates is taken from it, so it's never a transit
				if gateway == nil {
					domains := agent.Spec.Switch.Topology.Domains
					if len(domains) != 1 {
						return nil, fmt.Errorf("conn %s: switch with a gateway connection has to be in a single domain", connName) //nolint:err113
					}
					asn := agent.Spec.Config.Domains[domains[0]].GatewayASN
					if asn == 0 {
						return nil, fmt.Errorf("conn %s: no gateway ASN for domain %s", connName, domains[0]) //nolint:err113
					}
					vtepSubnet, err := netip.ParsePrefix(agent.Spec.Config.VTEPSubnet)
					if err != nil {
						return nil, fmt.Errorf("conn %s: parsing VTEP subnet: %w", connName, err)
					}
					gateway = &GatewayConfig{ASN: asn, VTEPSubnet: vtepSubnet.Masked().String()}
				}

				ip, err := netip.ParsePrefix(link.Switch.IP)
				if err != nil {
					return nil, fmt.Errorf("conn %s: parsing switch IP %q: %w", connName, link.Switch.IP, err)
				}
				gatewayIP, err := netip.ParsePrefix(link.Gateway.IP)
				if err != nil {
					return nil, fmt.Errorf("conn %s: parsing gateway IP %q: %w", connName, link.Gateway.IP, err)
				}

				port, err := usePort(link.Switch.LocalPortName())
				if err != nil {
					return nil, fmt.Errorf("conn %s: %w", connName, err)
				}
				port.Description = fmt.Sprintf("Gateway %s %s", link.Gateway.Port, connName)
				port.IP = ip.String()
				neighs = append(neighs, BGPNeighbor{
					IP:          gatewayIP.Addr().String(),
					PeerGroup:   "gateway",
					Description: fmt.Sprintf("Gateway %s %s", link.Gateway.Port, connName),
				})
			}
		case conn.External != nil:
			if conn.External.Link.Switch.DeviceName() != agent.Name {
				continue
			}

			port, err := usePort(conn.External.Link.Switch.LocalPortName())
			if err != nil {
				return nil, fmt.Errorf("conn %s: %w", connName, err)
			}
			port.Description = "External " + connName
		}
	}

	// bundled connections are LACP bonds named after their port channel IDs
	bonds := []*BondConfig{}
	bondsByConn := map[string]*BondConfig{}
	bondsByID := map[uint16]string{}
	for connName, conn := range agent.Spec.Connections {
		if conn.Bundled == nil {
			continue
		}

		bond := &BondConfig{LACPBypass: conn.Bundled.Fallback}
		memberPortNames := []string{}
		for _, link := range conn.Bundled.Links {
			if link.Switch.DeviceName() != agent.Name {
				continue
			}

			memberPortNames = append(memberPortNames, link.Switch.LocalPortName())
			bond.Description = fmt.Sprintf("Bundled %s %s", link.Server.DeviceName(), connName)
		}
		if len(memberPortNames) == 0 {
			continue
		}

		bond.id = agent.Spec.Catalog.PortChannelIDs[connName]
		if bond.id == 0 {
			return nil, fmt.Errorf("conn %s: no port channel ID", connName) //nolint:err113
		}
		if other, ok := bondsByID[bond.id]; ok {
			return nil, fmt.Errorf("conn %s: port channel ID %d is already used by conn %s", connName, bond.id, other) //nolint:err113
		}
		bondsByID[bond.id] = connName
		bond.Name = fmt.Sprintf("bond%d", bond.id)

		wiringapi.SortPortNames(memberPortNames)
		for _, memberPortName := range memberPortNames {
			port, err := usePort(memberPortName)
			if err != nil {
				return nil, fmt.Errorf("conn %s: %w", connName, err)
			}
			port.Description = bond.Description
			bond.Members = append(bond.Members, port)
		}

		bonds = append(bonds, bond)
		bondsByConn[connName] = bond
	}
	slices.SortFunc(bonds, func(a, b *BondConfig) int {
		return cmp.Compare(a.id, b.id)
	})

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
				port.Description = fmt.Sprintf("Fabric %s %s", link.Leaf.Port, connName)
				port.IP = link.Spine.IP
				port.AdaptiveRouting = adaptiveRouting(port)
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
				port.Description = fmt.Sprintf("Fabric %s %s", link.Spine.Port, connName)
				port.IP = link.Leaf.IP
				port.AdaptiveRouting = adaptiveRouting(port)
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

	// the leaves prefer the gateway routes by their priority communities, the local preference isn't passed on by the
	// spines, so it's set on the routes from them
	gwPrios := []GatewayPrio{}
	if isLeaf {
		prios := map[uint64]string{}
		for prioStr, comm := range agent.Spec.Config.GatewayCommunities {
			prio, err := strconv.ParseUint(prioStr, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("parsing gateway community priority %q: %w", prioStr, err)
			}
			prios[prio] = comm
		}
		if len(prios) > 100 {
			return nil, fmt.Errorf("too many gateway communities: %d, max is 100", len(prios)) //nolint:err113
		}
		for idx, prio := range slices.Sorted(maps.Keys(prios)) {
			gwPrios = append(gwPrios, GatewayPrio{
				Name:      fmt.Sprintf("gw-prio-%d", prio),
				Community: prios[prio],
				RuleID:    idx + 1,
				LocalPref: 200 + len(prios) - idx,
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

	vpcVNIs := map[string]uint32{}
	vpcSubnets := map[string]map[uint32]string{} // vpc -> subnet VNI -> subnet
	for vpcName, vpc := range agent.Spec.VPCs {
		vni := agent.Spec.Catalog.VPCVNIs[vpcName]
		if vni == 0 {
			return nil, fmt.Errorf("vpc %s: no VNI", vpcName) //nolint:err113
		}
		// VNI / 100 is the import route-map rule ID of the VPC, 50000 and up are reserved
		if vni%100 != 0 || vni/100 >= 40000 {
			return nil, fmt.Errorf("vpc %s: VNI %d isn't a multiple of 100 below 4000000", vpcName, vni) //nolint:err113
		}
		vpcVNIs[vpcName] = vni

		vpcSubnets[vpcName] = map[uint32]string{}
		for subnetName, subnet := range vpc.Subnets {
			subnetVNI, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpcName, subnetName)
			if !ok || subnetVNI == 0 {
				return nil, fmt.Errorf("subnet %s/%s: no VNI", vpcName, subnetName) //nolint:err113
			}
			vpcSubnets[vpcName][subnetVNI] = subnet.Subnet
		}
	}

	// peered VPCs import only each other's own subnets from each other's VRFs, so peering isn't transitive
	peers := map[string][]VPCPeer{}
	peerSubnets := map[string]map[uint32]string{}
	addPeer := func(vpcName, peerName string) {
		peers[vpcName] = append(peers[vpcName], VPCPeer{Name: peerName, VRF: vpcVRF(peerName), RuleID: 10000 + vpcVNIs[peerName]/100})
		if peerSubnets[vpcName] == nil {
			peerSubnets[vpcName] = map[uint32]string{}
		}
		maps.Copy(peerSubnets[vpcName], vpcSubnets[peerName])
	}
	for _, peeringName := range slices.Sorted(maps.Keys(agent.Spec.VPCPeerings)) {
		peering := agent.Spec.VPCPeerings[peeringName]
		vpc1, vpc2, err := peering.VPCs()
		if err != nil {
			return nil, fmt.Errorf("vpc peering %s: %w", peeringName, err)
		}
		for _, vpcName := range []string{vpc1, vpc2} {
			if _, ok := agent.Spec.VPCs[vpcName]; !ok {
				return nil, fmt.Errorf("vpc peering %s: vpc %s not found", peeringName, vpcName) //nolint:err113
			}
		}
		addPeer(vpc1, vpc2)
		addPeer(vpc2, vpc1)
	}

	// a VRF per VPC attached to the switch or peered with one, with the VPC VNI as L3VNI
	vpcs := []VPC{}
	for vpcName, vpc := range agent.Spec.VPCs {
		subnets := map[uint32]string{}
		for subnetVNI, subnet := range vpcSubnets[vpcName] {
			subnets[subnetVNI%100] = subnet
		}

		// static routes are only local to the switches the VPC is attached to, they aren't redistributed
		routes := map[string][]string{}
		for idx, route := range vpc.StaticRoutes {
			if !agent.Spec.AttachedVPCs[vpcName] {
				break
			}
			prefix, err := netip.ParsePrefix(route.Prefix)
			if err != nil || !prefix.Addr().Is4() {
				return nil, fmt.Errorf("vpc %s: static route #%d: invalid prefix %q", vpcName, idx, route.Prefix) //nolint:err113
			}
			if _, ok := routes[prefix.String()]; ok {
				return nil, fmt.Errorf("vpc %s: static route #%d: prefix %s is used more than once", vpcName, idx, prefix) //nolint:err113
			}
			if len(route.NextHops) == 0 {
				return nil, fmt.Errorf("vpc %s: static route #%d: no next hops", vpcName, idx) //nolint:err113
			}
			for _, nextHop := range route.NextHops {
				addr, err := netip.ParseAddr(nextHop)
				if err != nil || !addr.Is4() {
					return nil, fmt.Errorf("vpc %s: static route #%d: invalid next hop %q", vpcName, idx, nextHop) //nolint:err113
				}
				routes[prefix.String()] = append(routes[prefix.String()], addr.String())
			}
			slices.Sort(routes[prefix.String()])
		}

		vpcs = append(vpcs, VPC{
			Name:        vpcName,
			VRF:         vpcVRF(vpcName),
			VNI:         vpcVNIs[vpcName],
			Subnets:     subnets,
			RouteSumm:   routeSumm[vpcName],
			Peers:       slices.SortedFunc(slices.Values(peers[vpcName]), func(a, b VPCPeer) int { return cmp.Compare(a.Name, b.Name) }),
			PeerSubnets: peerSubnets[vpcName],
			Routes:      routes,
		})
	}

	// the externals attached to the switch get a VRF with the external VNI as L3VNI, the attachments get the IP in it
	// on a VLAN subinterface or on the port itself without a VLAN
	// TODO BGP sessions to the externals, static routes to the static ones and the hardened inbound ACL
	externals := map[string]External{}
	subifs := []*SubinterfaceConfig{}
	extVLANs := map[string]map[uint16]string{} // port -> VLAN -> attachment
	for _, attachName := range slices.Sorted(maps.Keys(agent.Spec.ExternalAttachments)) {
		attach := agent.Spec.ExternalAttachments[attachName]
		conn, ok := agent.Spec.Connections[attach.Connection]
		if !ok {
			return nil, fmt.Errorf("external attachment %s: conn %s not found", attachName, attach.Connection) //nolint:err113
		}
		if conn.External == nil || conn.External.Link.Switch.DeviceName() != agent.Name {
			return nil, fmt.Errorf("external attachment %s: conn %s isn't an external conn of the switch", attachName, attach.Connection) //nolint:err113
		}
		if _, ok := agent.Spec.Externals[attach.External]; !ok {
			return nil, fmt.Errorf("external attachment %s: external %s not found", attachName, attach.External) //nolint:err113
		}
		if _, ok := externals[attach.External]; !ok {
			vni := agent.Spec.Catalog.VPCVNIs[librarian.ReqForExt(attach.External)]
			if vni == 0 {
				return nil, fmt.Errorf("external %s: no VNI", attach.External) //nolint:err113
			}
			externals[attach.External] = External{Name: attach.External, VRF: extVRF(attach.External), VNI: vni}
		}

		vlan, ip := attach.Switch.VLAN, attach.Switch.IP
		if attach.Static != nil {
			vlan, ip = attach.Static.VLAN, attach.Static.IP
		}
		// no IP is BGP unnumbered
		if ip != "" {
			prefix, err := netip.ParsePrefix(ip)
			if err != nil || !prefix.Addr().Is4() {
				return nil, fmt.Errorf("external attachment %s: invalid IP %q", attachName, ip) //nolint:err113
			}
			ip = prefix.String()
		}

		portName, err := nosPortName(api2nos, conn.External.Link.Switch.LocalPortName())
		if err != nil {
			return nil, fmt.Errorf("external attachment %s: %w", attachName, err)
		}
		if other, ok := extVLANs[portName][vlan]; ok {
			return nil, fmt.Errorf("external attachment %s: vlan %d on port %s is already used by %s", attachName, vlan, portName, other) //nolint:err113
		}
		if extVLANs[portName] == nil {
			extVLANs[portName] = map[uint16]string{}
		}
		extVLANs[portName][vlan] = attachName

		if vlan == 0 {
			port := portsByName[portName]
			port.IP = ip
			port.VRF = extVRF(attach.External)

			continue
		}
		subifs = append(subifs, &SubinterfaceConfig{
			Name:        fmt.Sprintf("%s.%d", portName, vlan),
			Port:        portName,
			VLAN:        vlan,
			IP:          ip,
			VRF:         extVRF(attach.External),
			Description: "External " + attachName,
		})
	}
	slices.SortFunc(subifs, func(a, b *SubinterfaceConfig) int {
		return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.VLAN, b.VLAN))
	})

	// the DHCP relay sends a reply out of the first interface with the IP from the link selection it got, so the SVIs
	// need unique IPs of their own, taken as base + VLAN
	sviSubnet, err := netip.ParsePrefix(cmp.Or(agent.Spec.Config.CumulusSVIWorkaroundSubnet, fmeta.DefaultCumulusSVIWorkaroundSubnet))
	if err != nil {
		return nil, fmt.Errorf("parsing Cumulus SVI workaround subnet: %w", err)
	}
	if !sviSubnet.Addr().Is4() {
		return nil, fmt.Errorf("parsing Cumulus SVI workaround subnet: %s isn't IPv4", sviSubnet) //nolint:err113
	}

	// attached VPC subnets are VLANs mapped to the subnet VNIs with an SVI in the VPC VRF, carried by the ports and
	// bonds of the attached connections
	subnetsByName := map[string]*SubnetConfig{}
	subnetsByVLAN := map[uint16]string{}
	unbundledPorts := map[string]*PortConfig{}
	for _, attachName := range slices.Sorted(maps.Keys(agent.Spec.VPCAttachments)) {
		attach := agent.Spec.VPCAttachments[attachName]
		conn, ok := agent.Spec.Connections[attach.Connection]
		if !ok {
			continue
		}

		var member *BridgeMember
		if bond := bondsByConn[attach.Connection]; bond != nil {
			bond.Attached = true
			member = &bond.BridgeMember
		} else if conn.Unbundled != nil && conn.Unbundled.Link.Switch.DeviceName() == agent.Name {
			port, ok := unbundledPorts[attach.Connection]
			if !ok {
				var err error
				port, err = usePort(conn.Unbundled.Link.Switch.LocalPortName())
				if err != nil {
					return nil, fmt.Errorf("vpc attachment %q: %w", attachName, err)
				}
				port.Description = fmt.Sprintf("Unbundled %s %s", conn.Unbundled.Link.Server.DeviceName(), attach.Connection)
				unbundledPorts[attach.Connection] = port
			}
			member = &port.BridgeMember
		} else {
			_, _, connPorts, _, err := conn.Endpoints()
			if err != nil {
				return nil, fmt.Errorf("vpc attachment %q: getting conn %s endpoints: %w", attachName, attach.Connection, err)
			}
			if slices.ContainsFunc(connPorts, func(port string) bool { return strings.HasPrefix(port, agent.Name+"/") }) {
				return nil, fmt.Errorf("vpc attachment %q: conn %s type isn't supported", attachName, attach.Connection) //nolint:err113
			}

			continue
		}

		subnetCfg, ok := subnetsByName[attach.Subnet]
		if !ok {
			vpcName, subnetName := attach.VPCName(), attach.SubnetName()
			vpc, ok := agent.Spec.VPCs[vpcName]
			if !ok {
				return nil, fmt.Errorf("vpc attachment %q: vpc %s not found", attachName, vpcName) //nolint:err113
			}
			subnet, ok := vpc.Subnets[subnetName]
			if !ok || subnet == nil {
				return nil, fmt.Errorf("vpc attachment %q: subnet %s not found", attachName, attach.Subnet) //nolint:err113
			}
			vni, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpcName, subnetName)
			if !ok || vni == 0 {
				return nil, fmt.Errorf("vpc attachment %q: subnet %s: no VNI", attachName, attach.Subnet) //nolint:err113
			}
			if subnet.VLAN == 0 {
				return nil, fmt.Errorf("vpc attachment %q: subnet %s: no VLAN", attachName, attach.Subnet) //nolint:err113
			}
			if other, ok := subnetsByVLAN[subnet.VLAN]; ok {
				return nil, fmt.Errorf("vpc attachment %q: vlan %d is used by both subnet %s and %s", attachName, subnet.VLAN, attach.Subnet, other) //nolint:err113
			}
			prefix, err := netip.ParsePrefix(subnet.Subnet)
			if err != nil {
				return nil, fmt.Errorf("vpc attachment %q: parsing subnet %s: %w", attachName, subnet.Subnet, err)
			}
			gateway, err := netip.ParseAddr(subnet.Gateway)
			if err != nil {
				return nil, fmt.Errorf("vpc attachment %q: parsing gateway %s: %w", attachName, subnet.Gateway, err)
			}
			sviBase := sviSubnet.Masked().Addr().As4()
			sviIP := netip.AddrFrom4([4]byte(binary.BigEndian.AppendUint32(nil, binary.BigEndian.Uint32(sviBase[:])+uint32(subnet.VLAN))))
			if !sviSubnet.Contains(sviIP) {
				return nil, fmt.Errorf("vpc attachment %q: vlan %d doesn't fit Cumulus SVI workaround subnet %s", attachName, subnet.VLAN, sviSubnet) //nolint:err113
			}

			subnetCfg = &SubnetConfig{
				Name:        fmt.Sprintf("vlan%d", subnet.VLAN),
				VLAN:        subnet.VLAN,
				VNI:         vni,
				VRF:         vpcVRF(vpcName),
				Subnet:      subnet.Subnet,
				Gateway:     netip.PrefixFrom(gateway, prefix.Bits()).String(),
				IP:          netip.PrefixFrom(sviIP, 32).String(),
				Description: "VPC " + attach.Subnet,
				DHCP:        subnet.DHCP.Enable,
				ACL:         fmt.Sprintf("vpc-filtering--%s--%s", vpcName, subnetName),
				Deny:        map[uint32]string{},
			}
			if err := denyIsolatedSubnets(agent, subnetCfg, vpcName, subnetName); err != nil {
				return nil, fmt.Errorf("vpc attachment %q: %w", attachName, err)
			}
			// an ACL on the SVI only gets the routed traffic, so the traffic within a restricted subnet is dropped on the
			// ingress of the ports and bonds it's attached to
			if vpc.IsSubnetRestricted(subnetName) {
				subnetCfg.RestrictedACL = fmt.Sprintf("vpc-restricted--%s--%s", vpcName, subnetName)
			}
			subnetsByName[attach.Subnet] = subnetCfg
			subnetsByVLAN[subnet.VLAN] = attach.Subnet
		}

		if err := member.attach(subnetCfg.VLAN, attach.NativeVLAN); err != nil {
			return nil, fmt.Errorf("vpc attachment %q: %w", attachName, err)
		}
		if subnetCfg.RestrictedACL != "" {
			member.ACLs = append(member.ACLs, subnetCfg.RestrictedACL)
		}
	}

	// the routes are imported for the whole peered VPCs, so the traffic to the peered subnets that aren't permitted is
	// dropped on the ingress of the attached subnets
	for _, peeringName := range slices.Sorted(maps.Keys(agent.Spec.VPCPeerings)) {
		peering := agent.Spec.VPCPeerings[peeringName]
		vpc1, vpc2, err := peering.VPCs()
		if err != nil {
			return nil, fmt.Errorf("vpc peering %s: %w", peeringName, err)
		}
		if err := denyPeerSubnets(agent, subnetsByName, peering, vpc1, vpc2); err != nil {
			return nil, fmt.Errorf("vpc peering %s: %w", peeringName, err)
		}
		if err := denyPeerSubnets(agent, subnetsByName, peering, vpc2, vpc1); err != nil {
			return nil, fmt.Errorf("vpc peering %s: %w", peeringName, err)
		}
	}

	subnets := slices.SortedFunc(maps.Values(subnetsByName), func(a, b *SubnetConfig) int {
		return cmp.Compare(a.VLAN, b.VLAN)
	})

	for _, port := range ports {
		if port.Breakout != "" {
			continue
		}

		port.Up = port.Used || agent.Spec.Switch.EnableAllPorts
		if !port.Used {
			port.Description = "Unused"
		}
	}

	// bonds and their members are only up when attached, the same as the unbundled ports
	for _, bond := range bonds {
		bond.Up = bond.Attached || agent.Spec.Switch.EnableAllPorts
		for _, port := range bond.Members {
			port.Up = bond.Up
		}
	}

	slices.SortFunc(neighs, func(a, b BGPNeighbor) int {
		// not ideal, but gives stable ordering
		return strings.Compare(a.IP, b.IP)
	})
	slices.SortFunc(vpcs, func(a, b VPC) int {
		return strings.Compare(a.Name, b.Name)
	})

	cfgIn := ConfigIn{
		Hostname:      agent.Name,
		Users:         users,
		NTPServer:     controlVIP.Addr().String(),
		ManagementIP:  agent.Spec.Switch.IP,
		ProtocolIP:    agent.Spec.Switch.ProtocolIP,
		VTEPIP:        agent.Spec.Switch.VTEPIP,
		ASN:           agent.Spec.Switch.ASN,
		RouterID:      protocolIP.Addr().String(),
		VXLANSource:   vtepIP.Addr().String(),
		VPCs:          vpcs,
		Externals:     slices.SortedFunc(maps.Values(externals), func(a, b External) int { return cmp.Compare(a.Name, b.Name) }),
		BGPNeighbors:  neighs,
		Gateway:       gateway,
		GatewayPrios:  gwPrios,
		Ports:         ports,
		Bonds:         bonds,
		Subinterfaces: subifs,
		Subnets:       subnets,
		SubnetACLs:    slices.DeleteFunc(slices.Clone(subnets), func(subnet *SubnetConfig) bool { return len(subnet.Deny) == 0 }),
		Restricted:    slices.DeleteFunc(slices.Clone(subnets), func(subnet *SubnetConfig) bool { return subnet.RestrictedACL == "" }),
		IsSpine:       isSpine,
		IsLeaf:        isLeaf,
		IsSpectrum4:   isSpectrum4,
	}

	// the DHCP requests from the subnets with DHCP enabled are relayed to the control node over the management network
	if slices.ContainsFunc(subnets, func(subnet *SubnetConfig) bool { return subnet.DHCP }) {
		cfgIn.DHCPServer = controlVIP.Addr().String()
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

// denyIsolatedSubnets adds the other isolated subnets of the VPC to the attached subnet, unless a VPC permit list
// includes both of them
func denyIsolatedSubnets(agent *agentapi.Agent, subnetCfg *SubnetConfig, vpcName, subnetName string) error {
	vpc := agent.Spec.VPCs[vpcName]
	permitted := map[string]bool{}
	for idx, permit := range vpc.Permit {
		if !slices.Contains(permit, subnetName) {
			continue
		}
		for _, otherName := range permit {
			if _, ok := vpc.Subnets[otherName]; !ok {
				return fmt.Errorf("vpc %s: permit #%d: subnet %s not found", vpcName, idx, otherName) //nolint:err113
			}
			permitted[otherName] = true
		}
	}

	for otherName, other := range vpc.Subnets {
		if otherName == subnetName || other == nil || permitted[otherName] || !vpc.IsSubnetIsolated(otherName) {
			continue
		}
		subnetID := agent.Spec.Catalog.SubnetIDs[other.Subnet]
		if subnetID < 100 || subnetID >= 65000 {
			return fmt.Errorf("subnet %s/%s: subnet ID %d isn't between 100 and 65000", vpcName, otherName, subnetID) //nolint:err113
		}
		subnetCfg.Deny[subnetID] = other.Subnet
	}

	return nil
}

// denyPeerSubnets adds the subnets of the peer VPC that aren't permitted to the attached subnets of the VPC, the subnet
// lists of a permit entry default to all subnets
func denyPeerSubnets(agent *agentapi.Agent, subnetsByName map[string]*SubnetConfig, peering vpcapi.VPCPeeringSpec, vpcName, peerName string) error {
	peer := agent.Spec.VPCs[peerName]
	for subnetName := range agent.Spec.VPCs[vpcName].Subnets {
		subnetCfg, ok := subnetsByName[vpcName+"/"+subnetName]
		if !ok {
			continue
		}

		permitted := map[string]bool{}
		for _, permit := range peering.Permit {
			if own := permit[vpcName].Subnets; len(own) > 0 && !slices.Contains(own, subnetName) {
				continue
			}
			peerSubnets := permit[peerName].Subnets
			if len(peerSubnets) == 0 {
				peerSubnets = slices.Collect(maps.Keys(peer.Subnets))
			}
			for _, peerSubnetName := range peerSubnets {
				permitted[peerSubnetName] = true
			}
		}

		for peerSubnetName, peerSubnet := range peer.Subnets {
			if permitted[peerSubnetName] {
				continue
			}
			subnetID := agent.Spec.Catalog.SubnetIDs[peerSubnet.Subnet]
			if subnetID < 100 || subnetID >= 65000 {
				return fmt.Errorf("subnet %s/%s: subnet ID %d isn't between 100 and 65000", peerName, peerSubnetName, subnetID) //nolint:err113
			}
			subnetCfg.Deny[subnetID] = peerSubnet.Subnet
		}
	}

	return nil
}

// checkSupported rejects the features the full config can't express yet, the ZTP config doesn't include them, so it's
// only checked when enforcing
func checkSupported(agent *agentapi.Agent) error {
	for _, vpcName := range slices.Sorted(maps.Keys(agent.Spec.VPCs)) {
		vpc := agent.Spec.VPCs[vpcName]
		if vpc.Mode != vpcapi.VPCModeL2VNI {
			return fmt.Errorf("vpc %s: mode %s isn't supported", vpcName, vpc.Mode) //nolint:err113
		}

		for _, subnetName := range slices.Sorted(maps.Keys(vpc.Subnets)) {
			subnet := vpc.Subnets[subnetName]
			if subnet == nil {
				continue
			}
			if subnet.HostBGP {
				return fmt.Errorf("vpc %s: subnet %s: host BGP isn't supported", vpcName, subnetName) //nolint:err113
			}
			if subnet.DHCP.Relay != "" || subnet.DHCP.RelayVPC != "" {
				return fmt.Errorf("vpc %s: subnet %s: DHCP relay to other servers isn't supported", vpcName, subnetName) //nolint:err113
			}
		}
	}

	for _, peeringName := range slices.Sorted(maps.Keys(agent.Spec.VPCPeerings)) {
		peering := agent.Spec.VPCPeerings[peeringName]
		if peering.Remote != "" {
			return fmt.Errorf("vpc peering %s: remote peerings aren't supported", peeringName) //nolint:err113
		}
	}
	for _, connName := range slices.Sorted(maps.Keys(agent.Spec.Connections)) {
		if agent.Spec.Connections[connName].StaticExternal != nil {
			return fmt.Errorf("conn %s: static externals aren't supported", connName) //nolint:err113
		}
	}
	for _, attachName := range slices.Sorted(maps.Keys(agent.Spec.ExternalAttachments)) {
		if static := agent.Spec.ExternalAttachments[attachName].Static; static != nil && static.Proxy {
			return fmt.Errorf("external attachment %s: proxy-ARP isn't supported", attachName) //nolint:err113
		}
	}

	return nil
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
	if err := checkSupported(agent); err != nil {
		return err
	}

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

	// TODO use API instead

	if err := run(ctx, "nv-replace", "nv", "config", "replace", cfgPath); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}

	// diff exits with 0 if there is nothing to apply and with 1 if there are changes, the apply is only skipped if the
	// output is empty too to not rely on the exit code alone
	diff, diffErr := runOutput(ctx, "nv-diff", "nv", "config", "diff", "applied")
	if err := os.WriteFile(filepath.Join(basedir, diffFile), diff, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("writing diff file: %w", err)
	}
	if diffErr != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](diffErr); !ok || exitErr.ExitCode() != 1 {
			return fmt.Errorf("diffing config: %w", diffErr)
		}
	} else if len(bytes.TrimSpace(diff)) == 0 {
		slog.Debug("Config is already applied")

		return nil
	}

	msg := fmt.Sprintf("hh gen %d %s", agent.Generation, version.Version)
	if err := run(ctx, "nv-apply", "nv", "config", "apply", "-y", "-m", msg); err != nil {
		return fmt.Errorf("applying config: %w", err)
	}

	if err := os.WriteFile(filepath.Join(basedir, appliedDiffFile), diff, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("writing applied diff file: %w", err)
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

func runOutput(ctx context.Context, name, command string, arg ...string) ([]byte, error) {
	out := &bytes.Buffer{}
	cmd := exec.CommandContext(ctx, command, arg...)
	cmd.Stdout = io.MultiWriter(out, logutil.NewSink(ctx, slog.Debug, name+": "))
	cmd.Stderr = logutil.NewSink(ctx, slog.Debug, name+": ")

	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("running %s: %w", name, err)
	}

	return out.Bytes(), nil
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
	nosVersion := ""
	for line := range strings.Lines(string(osReleaseData)) {
		if after, ok := strings.CutPrefix(line, "VERSION="); ok {
			after = strings.TrimSpace(after)
			if len(after) > 2 {
				nosVersion = after[1 : len(after)-1]
			}

			break
		}
	}

	swState.NOS = agentapi.SwitchStateNOS{
		SoftwareVersion: nosVersion,
	}

	reg.SaveSwitchState(swState)

	return nil
}
