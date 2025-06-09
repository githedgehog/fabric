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

package bcm

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

const (
	MCLAGDomainID                 = 100
	MCLAGPeerLinkPortChannelID    = 250
	MCLAGSessionLinkPortChannelID = 251
	MCLAGPeerLinkTrunkVLANRange   = "2..4094" // TODO do we need to configure it?
	AgentUser                     = "hhagent"
	// LoopbackSwitch                 = "Loopback0"
	LoopbackProto                  = "Loopback1"
	LoopbackVTEP                   = "Loopback2"
	VRFDefault                     = "default"
	VTEPFabric                     = "vtepfabric"
	EVPNNVO                        = "nvo1"
	AnycastMAC                     = "00:00:00:11:11:11"
	RouteMapMaxStatement           = 65535
	RouteMapBlockEVPNDefaultRemote = "evpn-default-remote-block"
	RouteMapFilterAttachedHost     = "filter-attached-hosts"
	PrefixListAny                  = "any-prefix"
	PrefixListVPCLoopback          = "vpc-loopback-prefix"
	NoCommunity                    = "no-community"
	LSTGroupSpineLink              = "spinelink"
	BGPCommListAllExternals        = "all-externals"
	MgmtIface                      = "Management0"
)

func (p *BroadcomProcessor) PlanDesiredState(_ context.Context, agent *agentapi.Agent) (*dozer.Spec, error) {
	spec := &dozer.Spec{
		ZTP:             pointer.To(false),
		Hostname:        pointer.To(agent.Name),
		LLDP:            &dozer.SpecLLDP{},
		LLDPInterfaces:  map[string]*dozer.SpecLLDPInterface{},
		NTP:             &dozer.SpecNTP{},
		NTPServers:      map[string]*dozer.SpecNTPServer{},
		PortGroups:      map[string]*dozer.SpecPortGroup{},
		PortBreakouts:   map[string]*dozer.SpecPortBreakout{},
		Interfaces:      map[string]*dozer.SpecInterface{},
		MCLAGs:          map[uint32]*dozer.SpecMCLAGDomain{},
		MCLAGInterfaces: map[string]*dozer.SpecMCLAGInterface{},
		Users:           map[string]*dozer.SpecUser{},
		VRFs: map[string]*dozer.SpecVRF{
			VRFDefault: { // default VRF is always present
				Enabled:          pointer.To(true),
				Interfaces:       map[string]*dozer.SpecVRFInterface{},
				TableConnections: map[string]*dozer.SpecVRFTableConnection{},
				StaticRoutes:     map[string]*dozer.SpecVRFStaticRoute{},
				EthernetSegments: map[string]*dozer.SpecVRFEthernetSegment{},
			},
		},
		RouteMaps:          map[string]*dozer.SpecRouteMap{},
		PrefixLists:        map[string]*dozer.SpecPrefixList{},
		CommunityLists:     map[string]*dozer.SpecCommunityList{},
		DHCPRelays:         map[string]*dozer.SpecDHCPRelay{},
		NATs:               map[uint32]*dozer.SpecNAT{},
		ACLs:               map[string]*dozer.SpecACL{},
		ACLInterfaces:      map[string]*dozer.SpecACLInterface{},
		VXLANTunnels:       map[string]*dozer.SpecVXLANTunnel{},
		VXLANEVPNNVOs:      map[string]*dozer.SpecVXLANEVPNNVO{},
		VXLANTunnelMap:     map[string]*dozer.SpecVXLANTunnelMap{},
		VRFVNIMap:          map[string]*dozer.SpecVRFVNIEntry{},
		SuppressVLANNeighs: map[string]*dozer.SpecSuppressVLANNeigh{},
		PortChannelConfigs: map[string]*dozer.SpecPortChannelConfig{},
		LSTGroups:          map[string]*dozer.SpecLSTGroup{},
		LSTInterfaces:      map[string]*dozer.SpecLSTInterface{},
	}

	for name, speed := range agent.Spec.Switch.PortGroupSpeeds {
		spec.PortGroups[name] = &dozer.SpecPortGroup{
			Speed: pointer.To(speed),
		}
	}

	err := planControlLink(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan management interface")
	}

	err = planLLDP(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan LLDP")
	}

	err = planUsers(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan users")
	}

	err = planLoopbacks(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan switch IP loopbacks")
	}

	err = planNTP(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan NTP")
	}

	if err := planBreakouts(agent, spec); err != nil {
		return nil, errors.Wrap(err, "failed to plan breakouts")
	}

	err = planDefaultVRFWithBGP(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan basic BGP")
	}

	err = planFabricConnections(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan fabric connections")
	}

	err = planGatewayConnections(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan gateway connections")
	}

	err = planVPCLoopbacks(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan VPC loopbacks")
	}

	err = planExternals(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan external connections")
	}

	err = planServerConnections(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan server connections")
	}

	if agent.Spec.Role.IsLeaf() {
		err = planVXLAN(agent, spec)
		if err != nil {
			return nil, errors.Wrap(err, "failed to plan VXLAN")
		}
	}

	if agent.Spec.Switch.Redundancy.Type == meta.RedundancyTypeMCLAG {
		_ /* first */, err = planMCLAGDomain(agent, spec)
		if err != nil {
			return nil, errors.Wrap(err, "failed to plan mclag domain")
		}
	} else if agent.Spec.Switch.Redundancy.Type == meta.RedundancyTypeESLAG {
		err = planESLAG(agent, spec)
		if err != nil {
			return nil, errors.Wrap(err, "failed to plan eslag")
		}
	}

	err = planVPCs(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan VPCs")
	}

	err = planExternalPeerings(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan external peerings")
	}

	err = planStaticExternals(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan static external connections")
	}

	err = planAllPortsUp(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan all ports up")
	}

	err = planPortAutoNegs(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan port auto negs")
	}

	err = translatePortNames(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to translate port names")
	}

	spec.Normalize()

	return spec, nil
}

// TODO do we still need it if only management port is used?
func planControlLink(agent *agentapi.Agent, spec *dozer.Spec) error {
	controlIface := "M1"
	controlIP := agent.Spec.Switch.IP

	ip, ipNet, err := net.ParseCIDR(controlIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse control IP %s", controlIP)
	}
	prefixLen, _ := ipNet.Mask.Size()

	spec.Interfaces[controlIface] = &dozer.SpecInterface{
		Description:   pointer.To("Management link"),
		Enabled:       pointer.To(true),
		AutoNegotiate: pointer.To(true),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip.String(): {
						PrefixLen: pointer.To(uint8(prefixLen)), //nolint:gosec
					},
				},
			},
		},
	}

	return nil
}

func planLLDP(agent *agentapi.Agent, spec *dozer.Spec) error { //nolint:unparam
	spec.LLDP = &dozer.SpecLLDP{
		Enabled:           pointer.To(true),
		HelloTimer:        pointer.To(uint64(5)), // TODO make configurable?
		SystemName:        pointer.To(agent.Name),
		SystemDescription: pointer.To(wiringapi.SwitchLLDPDescription(agent.Spec.Config.DeploymentID)),
	}

	return nil
}

func planNTP(agent *agentapi.Agent, spec *dozer.Spec) error {
	spec.NTP.SourceInterface = []string{MgmtIface}

	if !strings.HasSuffix(agent.Spec.Config.ControlVIP, "/32") {
		return errors.Errorf("invalid control VIP %s", agent.Spec.Config.ControlVIP)
	}
	addr, _ := strings.CutSuffix(agent.Spec.Config.ControlVIP, "/32")

	spec.NTPServers[addr] = &dozer.SpecNTPServer{
		Prefer: pointer.To(true),
	}

	return nil
}

func planBreakouts(agent *agentapi.Agent, spec *dozer.Spec) error {
	def, err := agent.Spec.SwitchProfile.GetBreakoutDefaults(&agent.Spec.Switch)
	if err != nil {
		return errors.Wrap(err, "failed to get breakout defaults")
	}

	for name, mode := range def {
		spec.PortBreakouts[name] = &dozer.SpecPortBreakout{
			Mode: mode,
		}
	}

	for name, mode := range agent.Spec.Switch.PortBreakouts {
		spec.PortBreakouts[name] = &dozer.SpecPortBreakout{
			Mode: mode,
		}
	}

	return nil
}

func planLoopbacks(agent *agentapi.Agent, spec *dozer.Spec) error {
	// ip, ipNet, err := net.ParseCIDR(agent.Spec.Switch.IP)
	// if err != nil {
	// 	return errors.Wrapf(err, "failed to parse switch ip %s", agent.Spec.Switch.IP)
	// }
	// ipPrefixLen, _ := ipNet.Mask.Size()

	// spec.Interfaces[LoopbackSwitch] = &dozer.SpecInterface{
	// 	Enabled:     pointer.To(true),
	// 	Description: pointer.To("Switch loopback"),
	// 	Subinterfaces: map[uint32]*dozer.SpecSubinterface{
	// 		0: {
	// 			IPs: map[string]*dozer.SpecInterfaceIP{
	// 				ip.String(): {
	// 					PrefixLen: pointer.To(uint8(ipPrefixLen)), //nolint:gosec
	// 				},
	// 			},
	// 		},
	// 	},
	// }

	ip, ipNet, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
	}
	ipPrefixLen, _ := ipNet.Mask.Size()

	spec.Interfaces[LoopbackProto] = &dozer.SpecInterface{
		Enabled:     pointer.To(true),
		Description: pointer.To("Protocol loopback"),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip.String(): {
						PrefixLen: pointer.To(uint8(ipPrefixLen)), //nolint:gosec
					},
				},
			},
		},
	}

	if agent.IsSpineLeaf() && agent.Spec.Switch.Role.IsLeaf() {
		ip, ipNet, err = net.ParseCIDR(agent.Spec.Switch.VTEPIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse vtep ip %s", agent.Spec.Switch.VTEPIP)
		}
		ipPrefixLen, _ = ipNet.Mask.Size()

		spec.Interfaces[LoopbackVTEP] = &dozer.SpecInterface{
			Enabled:     pointer.To(true),
			Description: pointer.To("VTEP loopback"),
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{
				0: {
					IPs: map[string]*dozer.SpecInterfaceIP{
						ip.String(): {
							PrefixLen: pointer.To(uint8(ipPrefixLen)), //nolint:gosec
						},
					},
				},
			},
		}
	}

	return nil
}

func planFabricConnections(agent *agentapi.Agent, spec *dozer.Spec) error {
	if !agent.IsSpineLeaf() {
		return nil
	}

	spec.RouteMaps[RouteMapBlockEVPNDefaultRemote] = &dozer.SpecRouteMap{
		Statements: map[string]*dozer.SpecRouteMapStatement{
			fmt.Sprintf("%d", RouteMapMaxStatement): {
				Result: dozer.SpecRouteMapResultAccept,
			},
		},
	}

	for connName, conn := range agent.Spec.Connections {
		if conn.Fabric == nil {
			continue
		}

		for _, link := range conn.Fabric.Links {
			port := ""
			ipStr := ""
			remote := ""
			peer := ""
			peerIP := ""
			if link.Spine.DeviceName() == agent.Name { //nolint:gocritic
				port = link.Spine.LocalPortName()
				ipStr = link.Spine.IP
				remote = link.Leaf.Port
				peer = link.Leaf.DeviceName()
				peerIP = link.Leaf.IP
			} else if link.Leaf.DeviceName() == agent.Name {
				port = link.Leaf.LocalPortName()
				ipStr = link.Leaf.IP
				remote = link.Spine.Port
				peer = link.Spine.DeviceName()
				peerIP = link.Spine.IP
			} else {
				continue
			}

			if ipStr == "" {
				return errors.Errorf("no IP found for fabric conn %s", connName)
			}

			ip, ipNet, err := net.ParseCIDR(ipStr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse fabric conn ip %s", ipStr)
			}
			ipPrefixLen, _ := ipNet.Mask.Size()

			spec.Interfaces[port] = &dozer.SpecInterface{
				Enabled:     pointer.To(true),
				Description: pointer.To(fmt.Sprintf("Fabric %s %s", remote, connName)),
				Speed:       getPortSpeed(agent, port),
				Subinterfaces: map[uint32]*dozer.SpecSubinterface{
					0: {
						IPs: map[string]*dozer.SpecInterfaceIP{
							ip.String(): {
								PrefixLen: pointer.To(uint8(ipPrefixLen)), //nolint:gosec
							},
						},
					},
				},
			}

			peerSw, ok := agent.Spec.Switches[peer]
			if !ok {
				return errors.Errorf("no switch found for peer %s (fabric conn %s)", peer, connName)
			}

			ip, _, err = net.ParseCIDR(peerIP)
			if err != nil {
				return errors.Wrapf(err, "failed to parse fabric conn peer ip %s", peerIP)
			}

			spec.VRFs[VRFDefault].BGP.Neighbors[ip.String()] = &dozer.SpecVRFBGPNeighbor{
				Enabled:                 pointer.To(true),
				Description:             pointer.To(fmt.Sprintf("Fabric %s %s", remote, connName)),
				RemoteAS:                pointer.To(peerSw.ASN),
				IPv4Unicast:             pointer.To(true),
				L2VPNEVPN:               pointer.To(true),
				L2VPNEVPNImportPolicies: []string{RouteMapBlockEVPNDefaultRemote},
				// TODO: We might later specify dedicated neighbors for this.
				L2VPNEVPNAllowOwnAS: pointer.To(true),
			}
		}
	}

	return nil
}

func planGatewayConnections(agent *agentapi.Agent, spec *dozer.Spec) error {
	if !agent.IsSpineLeaf() {
		return nil
	}

	for connName, conn := range agent.Spec.Connections {
		if conn.Gateway == nil {
			continue
		}

		if agent.Spec.Config.GatewayASN == 0 {
			return errors.Errorf("gateway ASN not set")
		}

		for _, link := range conn.Gateway.Links {
			port := ""
			ipStr := ""
			remote := ""
			// peer := ""
			peerIP := ""
			if link.Spine.DeviceName() == agent.Name {
				port = link.Spine.LocalPortName()
				ipStr = link.Spine.IP
				remote = link.Gateway.Port
				// peer = link.Gateway.DeviceName()
				peerIP = link.Gateway.IP
			} else {
				continue
			}

			if ipStr == "" {
				return errors.Errorf("no IP found for gateway conn %s", connName)
			}

			ip, ipNet, err := net.ParseCIDR(ipStr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse gateway conn ip %s", ipStr)
			}
			ipPrefixLen, _ := ipNet.Mask.Size()

			spec.Interfaces[port] = &dozer.SpecInterface{
				Enabled:     pointer.To(true),
				Description: pointer.To(fmt.Sprintf("Gateway %s %s", remote, connName)),
				Speed:       getPortSpeed(agent, port),
				Subinterfaces: map[uint32]*dozer.SpecSubinterface{
					0: {
						IPs: map[string]*dozer.SpecInterfaceIP{
							ip.String(): {
								PrefixLen: pointer.To(uint8(ipPrefixLen)), //nolint:gosec
							},
						},
					},
				},
			}

			ip, _, err = net.ParseCIDR(peerIP)
			if err != nil {
				return errors.Wrapf(err, "failed to parse gateway conn peer ip %s", peerIP)
			}

			spec.VRFs[VRFDefault].BGP.Neighbors[ip.String()] = &dozer.SpecVRFBGPNeighbor{
				Enabled:             pointer.To(true),
				Description:         pointer.To(fmt.Sprintf("Gateway %s %s", remote, connName)),
				RemoteAS:            pointer.To(agent.Spec.Config.GatewayASN), // TODO load peer GW and get ASN from it
				IPv4Unicast:         pointer.To(true),
				L2VPNEVPN:           pointer.To(true),
				L2VPNEVPNAllowOwnAS: pointer.To(true),
			}
		}
	}

	return nil
}

func planVPCLoopbacks(agent *agentapi.Agent, spec *dozer.Spec) error { //nolint:unparam
	for connName, conn := range agent.Spec.Connections {
		if conn.VPCLoopback == nil {
			continue
		}

		for linkID, link := range conn.VPCLoopback.Links {
			if link.Switch1.DeviceName() != agent.Name || link.Switch2.DeviceName() != agent.Name {
				continue
			}

			for portID, port := range []string{link.Switch1.LocalPortName(), link.Switch2.LocalPortName()} {
				spec.Interfaces[port] = &dozer.SpecInterface{
					Enabled:       pointer.To(true),
					Description:   pointer.To(fmt.Sprintf("VPC loopback %d.%d %s", linkID, portID, connName)),
					Speed:         getPortSpeed(agent, port),
					Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
				}
			}
		}
	}

	return nil
}

func planExternals(agent *agentapi.Agent, spec *dozer.Spec) error {
	spec.PrefixLists[PrefixListAny] = &dozer.SpecPrefixList{
		Prefixes: map[uint32]*dozer.SpecPrefixListEntry{
			10: {
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: "0.0.0.0/0",
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			},
		},
	}

	for connName, conn := range agent.Spec.Connections {
		if conn.External == nil {
			continue
		}

		port := conn.External.Link.Switch.LocalPortName()

		spec.Interfaces[port] = &dozer.SpecInterface{
			Enabled:       pointer.To(true),
			Description:   pointer.To(fmt.Sprintf("External %s", connName)),
			Speed:         getPortSpeed(agent, port),
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
		}
	}

	for ipnsName, ipns := range agent.Spec.IPv4Namespaces {
		spec.PrefixLists[ipnsSubnetsPrefixListName(ipnsName)] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		for idx, subnet := range ipns.Subnets {
			spec.PrefixLists[ipnsSubnetsPrefixListName(ipnsName)].Prefixes[uint32(idx+1)] = &dozer.SpecPrefixListEntry{ //nolint:gosec
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}
		}
	}

	attachedExternals := map[string]bool{}
	for _, attach := range agent.Spec.ExternalAttachments {
		attachedExternals[attach.External] = true
	}

	if agent.IsSpineLeaf() {
		spec.CommunityLists[BGPCommListAllExternals] = &dozer.SpecCommunityList{
			Members: []string{},
		}

		spec.RouteMaps[RouteMapBlockEVPNDefaultRemote].Statements[fmt.Sprintf("%d", RouteMapMaxStatement-10)] = &dozer.SpecRouteMapStatement{
			Conditions: dozer.SpecRouteMapConditions{
				MatchCommunityList: pointer.To(BGPCommListAllExternals),
			},
			SetLocalPreference: pointer.To(uint32(500)),
			Result:             dozer.SpecRouteMapResultAccept,
		}
	}

	for externalName, external := range agent.Spec.Externals {
		if agent.IsSpineLeaf() && !slices.Contains(spec.CommunityLists[BGPCommListAllExternals].Members, external.InboundCommunity) {
			spec.CommunityLists[BGPCommListAllExternals].Members = append(spec.CommunityLists[BGPCommListAllExternals].Members, external.InboundCommunity)
		}

		if !attachedExternals[externalName] {
			continue
		}

		ipnsVrfName := ipnsVrfName(external.IPv4Namespace)

		externalCommsCommList := ipNsExtCommsCommListName(external.IPv4Namespace)
		externalCommsRouteMap := ipNsExternalCommsRouteMapName(external.IPv4Namespace)

		if _, exists := spec.CommunityLists[externalCommsCommList]; !exists {
			spec.CommunityLists[externalCommsCommList] = &dozer.SpecCommunityList{
				Members: []string{},
			}
		}
		spec.CommunityLists[externalCommsCommList].Members = append(spec.CommunityLists[externalCommsCommList].Members, external.InboundCommunity)

		spec.RouteMaps[externalCommsRouteMap] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchCommunityList: pointer.To(externalCommsCommList),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
				"100": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}

		spec.ACLs[ipnsEgressAccessList(external.IPv4Namespace)] = &dozer.SpecACL{
			Entries: map[uint32]*dozer.SpecACLEntry{
				65535: {
					Action: dozer.SpecACLEntryActionAccept,
				},
			},
		}

		ipns, exists := agent.Spec.IPv4Namespaces[external.IPv4Namespace]
		if !exists {
			return errors.Errorf("ipv4 namespace %s not found for external %s", external.IPv4Namespace, externalName)
		}
		seq := uint32(10)
		for _, subnet := range ipns.Subnets {
			spec.ACLs[ipnsEgressAccessList(external.IPv4Namespace)].Entries[seq] = &dozer.SpecACLEntry{
				DestinationAddress: pointer.To(subnet),
				Action:             dozer.SpecACLEntryActionDrop,
			}
			seq += 10
		}

		if spec.VRFs[ipnsVrfName] == nil {
			protocolIP, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
			if err != nil {
				return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
			}

			spec.VRFs[ipnsVrfName] = &dozer.SpecVRF{
				Enabled: pointer.To(true),
				// Description:      pointer.To(fmt.Sprintf("IPv4NS %s", external.IPv4Namespace)),
				AnycastMAC:       pointer.To(AnycastMAC),
				Interfaces:       map[string]*dozer.SpecVRFInterface{},
				StaticRoutes:     map[string]*dozer.SpecVRFStaticRoute{},
				TableConnections: map[string]*dozer.SpecVRFTableConnection{},
				BGP: &dozer.SpecVRFBGP{
					AS:                 pointer.To(agent.Spec.Switch.ASN),
					RouterID:           pointer.To(protocolIP.String()),
					NetworkImportCheck: pointer.To(true),
					IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
						Enabled:    true,
						MaxPaths:   pointer.To(getMaxPaths(agent)),
						Networks:   map[string]*dozer.SpecVRFBGPNetwork{},
						ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
					},
					L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
						Enabled:            agent.IsSpineLeaf(),
						AdvertiseDefaultGw: pointer.To(true),
					},
					Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{},
				},
			}
		}

		commList := extInboundCommListName(externalName)
		spec.CommunityLists[commList] = &dozer.SpecCommunityList{
			Members: []string{external.InboundCommunity},
		}

		spec.RouteMaps[extInboundRouteMapName(externalName)] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"5": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(ipnsSubnetsPrefixListName(external.IPv4Namespace)),
					},
					Result: dozer.SpecRouteMapResultReject,
				},
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchCommunityList: pointer.To(commList),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
				"100": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}

		prefList := extOutboundPrefixList(externalName)
		spec.PrefixLists[prefList] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		spec.RouteMaps[extOutboundRouteMapName(externalName)] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(prefList),
					},
					SetCommunities: []string{external.OutboundCommunity},
					Result:         dozer.SpecRouteMapResultAccept,
				},
				"100": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}
	}

	for name, attach := range agent.Spec.ExternalAttachments {
		conn, exists := agent.Spec.Connections[attach.Connection]
		if !exists {
			return errors.Errorf("connection %s not found for external attach %s", attach.Connection, name)
		}
		if conn.External == nil {
			return errors.Errorf("connection %s is not external for external attach %s", attach.Connection, name)
		}

		external, exists := agent.Spec.Externals[attach.External]
		if !exists {
			return errors.Errorf("external %s not found for external attach %s", attach.External, name)
		}

		port := conn.External.Link.Switch.LocalPortName()
		var vlan *uint16
		if attach.Switch.VLAN != 0 {
			vlan = pointer.To(attach.Switch.VLAN)
		}

		ip, ipNet, err := net.ParseCIDR(attach.Switch.IP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse external attach switch ip %s", attach.Switch.IP)
		}
		prefixLength, _ := ipNet.Mask.Size()

		spec.Interfaces[port].Subinterfaces[uint32(attach.Switch.VLAN)] = &dozer.SpecSubinterface{
			VLAN: vlan,
			IPs: map[string]*dozer.SpecInterfaceIP{
				ip.String(): {
					PrefixLen: pointer.To(uint8(prefixLength)), //nolint:gosec
				},
			},
		}

		ifaceName := port
		if attach.Switch.VLAN != 0 {
			ifaceName = fmt.Sprintf("%s.%d", port, attach.Switch.VLAN)
		}

		ipns := external.IPv4Namespace
		ipnsVrfName := ipnsVrfName(ipns)
		spec.VRFs[ipnsVrfName].Interfaces[ifaceName] = &dozer.SpecVRFInterface{}

		spec.VRFs[ipnsVrfName].BGP.Neighbors[attach.Neighbor.IP] = &dozer.SpecVRFBGPNeighbor{
			Enabled:                   pointer.To(true),
			Description:               pointer.To(fmt.Sprintf("External attach %s", name)),
			RemoteAS:                  pointer.To(attach.Neighbor.ASN),
			IPv4Unicast:               pointer.To(true),
			IPv4UnicastImportPolicies: []string{extInboundRouteMapName(attach.External)},
			IPv4UnicastExportPolicies: []string{extOutboundRouteMapName(attach.External)},
		}

		spec.ACLInterfaces[ifaceName] = &dozer.SpecACLInterface{
			Egress: pointer.To(ipnsEgressAccessList(ipns)),
		}
	}

	return nil
}

func planStaticExternals(agent *agentapi.Agent, spec *dozer.Spec) error {
	for connName, conn := range agent.Spec.Connections {
		if conn.StaticExternal == nil {
			continue
		}
		if conn.StaticExternal.Link.Switch.DeviceName() != agent.Name {
			continue
		}

		cfg := conn.StaticExternal.Link.Switch
		ip, ipNet, err := net.ParseCIDR(cfg.IP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse static external %s ip %s", connName, cfg.IP)
		}
		ipPrefixLen, _ := ipNet.Mask.Size()

		var vlan *uint16
		if cfg.VLAN != 0 {
			vlan = pointer.To(cfg.VLAN)
		}

		spec.Interfaces[cfg.LocalPortName()] = &dozer.SpecInterface{
			Enabled:     pointer.To(true),
			Description: pointer.To(fmt.Sprintf("StaticExt %s", connName)),
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{
				uint32(cfg.VLAN): {
					VLAN: vlan,
					IPs: map[string]*dozer.SpecInterfaceIP{
						ip.String(): {
							PrefixLen: pointer.To(uint8(ipPrefixLen)), //nolint:gosec
						},
					},
				},
			},
		}

		ifName := cfg.LocalPortName()
		if cfg.VLAN != 0 {
			ifName = fmt.Sprintf("%s.%d", cfg.LocalPortName(), cfg.VLAN)
		}

		vrfName := VRFDefault
		if conn.StaticExternal.WithinVPC != "" {
			vrfName = vpcVrfName(conn.StaticExternal.WithinVPC)

			if spec.VRFs[vrfName] == nil {
				return errors.Errorf("vpc %s vrf %s not found for static external %s", conn.StaticExternal.WithinVPC, vrfName, connName)
			}
			if spec.VRFs[vrfName].Interfaces == nil {
				spec.VRFs[vrfName].Interfaces = map[string]*dozer.SpecVRFInterface{}
			}

			spec.VRFs[vrfName].Interfaces[ifName] = &dozer.SpecVRFInterface{}
		}

		subnets := []string{ipNet.String()}

		for _, subnet := range cfg.Subnets {
			spec.VRFs[vrfName].StaticRoutes[subnet] = &dozer.SpecVRFStaticRoute{
				NextHops: []dozer.SpecVRFStaticRouteNextHop{
					{
						IP:        cfg.NextHop,
						Interface: pointer.To(ifName),
					},
				},
			}

			subnets = append(subnets, subnet)
		}

		if conn.StaticExternal.WithinVPC != "" {
			vpcName := conn.StaticExternal.WithinVPC
			prefixList := spec.PrefixLists[vpcStaticExtSubnetsPrefixListName(vpcName)]
			if prefixList == nil {
				return errors.Errorf("prefix list %s not found for static external %s", vpcStaticExtSubnetsPrefixListName(vpcName), connName)
			}

			for _, subnet := range subnets {
				subnetID := agent.Spec.Catalog.SubnetIDs[subnet]
				// TODO dedup
				if subnetID == 0 {
					return errors.Errorf("no subnet id found for static ext subnet %s", subnet)
				}
				if subnetID < 100 {
					return errors.Errorf("subnet id for static ext subnet %s is too small", subnet)
				}
				if subnetID >= 65000 {
					return errors.Errorf("subnet id for static ext subnet %s is too large", subnet)
				}

				_, ipNet, err := net.ParseCIDR(subnet)
				if err != nil {
					return errors.Wrapf(err, "failed to parse static external subnet %s", subnet)
				}
				prefixLen, _ := ipNet.Mask.Size()

				prefixList.Prefixes[subnetID] = &dozer.SpecPrefixListEntry{
					Prefix: dozer.SpecPrefixListPrefix{
						Prefix: subnet,
						Le:     uint8(prefixLen), //nolint:gosec
					},
					Action: dozer.SpecPrefixListActionPermit,
				}
			}
		}
	}

	return nil
}

func planServerConnections(agent *agentapi.Agent, spec *dozer.Spec) error {
	// handle connections which should be configured as port channels
	for connName, conn := range agent.Spec.Connections {
		connType := ""
		var mtu *uint16
		var links []wiringapi.ServerToSwitchLink
		fallback := agent.IsFirstInRedundancyGroup()

		if conn.MCLAG != nil { //nolint:gocritic
			connType = "MCLAG"
			if conn.MCLAG.MTU != 0 {
				mtu = pointer.To(conn.MCLAG.MTU) //nolint:ineffassign,staticcheck
			}
			fallback = fallback && conn.MCLAG.Fallback
			links = conn.MCLAG.Links
		} else if conn.Bundled != nil {
			connType = "Bundled"
			if conn.Bundled.MTU != 0 {
				mtu = pointer.To(conn.Bundled.MTU) //nolint:ineffassign,staticcheck
			}
			links = conn.Bundled.Links
		} else if conn.ESLAG != nil {
			connType = "ESLAG"
			if conn.ESLAG.MTU != 0 {
				mtu = pointer.To(conn.ESLAG.MTU) //nolint:ineffassign,staticcheck
			}
			fallback = fallback && conn.ESLAG.Fallback
			links = conn.ESLAG.Links
		} else {
			continue
		}

		// TODO remove when we have a way to configure MTU for port channels reliably
		// if mtu == nil {
		mtu = pointer.To(agent.Spec.Config.FabricMTU - agent.Spec.Config.ServerFacingMTUOffset)
		//}

		if err := conn.ValidateServerFacingMTU(agent.Spec.Config.FabricMTU, agent.Spec.Config.ServerFacingMTUOffset); err != nil {
			return errors.Wrapf(err, "failed to validate server facing MTU for conn %s", connName)
		}

		for _, link := range links {
			if link.Switch.DeviceName() != agent.Name {
				continue
			}

			portName := link.Switch.LocalPortName()
			portChan := agent.Spec.Catalog.PortChannelIDs[connName]
			if portChan == 0 {
				return errors.Errorf("no port channel found for conn %s", connName)
			}

			connPortChannelName := portChannelName(portChan)
			connPortChannel := &dozer.SpecInterface{
				Enabled:     pointer.To(true),
				Description: pointer.To(fmt.Sprintf("%s %s %s", connType, link.Server.DeviceName(), connName)),
				TrunkVLANs:  []string{},
				MTU:         mtu,
			}
			spec.Interfaces[connPortChannelName] = connPortChannel

			if connType == "MCLAG" {
				spec.MCLAGInterfaces[connPortChannelName] = &dozer.SpecMCLAGInterface{
					DomainID: MCLAGDomainID,
				}
				spec.PortChannelConfigs[connPortChannelName] = &dozer.SpecPortChannelConfig{
					Fallback: pointer.To(fallback),
				}
			} else if connType == "ESLAG" {
				mac, err := net.ParseMAC(agent.Spec.Config.ESLAGMACBase)
				if err != nil {
					return errors.Wrapf(err, "failed to parse ESLAG MAC base %s", agent.Spec.Config.ESLAGMACBase)
				}

				macVal := binary.BigEndian.Uint64(append([]byte{0, 0}, mac...))
				id := agent.Spec.Catalog.ConnectionIDs[connName]
				if id == 0 {
					return errors.Errorf("no connection id found for conn %s", connName)
				}
				macVal += uint64(id)

				newMACVal := make([]byte, 8)
				binary.BigEndian.PutUint64(newMACVal, macVal)

				mac = newMACVal[2:]
				spec.PortChannelConfigs[connPortChannelName] = &dozer.SpecPortChannelConfig{
					SystemMAC: pointer.To(mac.String()),
					Fallback:  pointer.To(fallback),
				}

				esi := strings.ReplaceAll(agent.Spec.Config.ESLAGESIPrefix+mac.String(), ":", "")
				spec.VRFs[VRFDefault].EthernetSegments[connPortChannelName] = &dozer.SpecVRFEthernetSegment{
					ESI: esi,
				}
			}

			descr := fmt.Sprintf("PC%d %s %s %s", portChan, connType, link.Server.DeviceName(), connName)
			err := setupPhysicalInterfaceWithPortChannel(spec, portName, descr, connPortChannelName, mtu, agent)
			if err != nil {
				return errors.Wrapf(err, "failed to setup physical interface %s", portName)
			}
		}
	}

	// handle non-portchannel connections
	for connName, conn := range agent.Spec.Connections {
		if conn.Unbundled == nil {
			continue
		}

		var mtu *uint16
		if conn.Unbundled.MTU != 0 {
			mtu = pointer.To(conn.Unbundled.MTU)
		}

		if mtu == nil {
			mtu = pointer.To(agent.Spec.Config.FabricMTU - agent.Spec.Config.ServerFacingMTUOffset)
		}

		if err := conn.ValidateServerFacingMTU(agent.Spec.Config.FabricMTU, agent.Spec.Config.ServerFacingMTUOffset); err != nil {
			return errors.Wrapf(err, "failed to validate server facing MTU for conn %s", connName)
		}

		if conn.Unbundled.Link.Switch.DeviceName() != agent.Name {
			continue
		}

		swPort := conn.Unbundled.Link.Switch

		spec.Interfaces[swPort.LocalPortName()] = &dozer.SpecInterface{
			Enabled:     pointer.To(true),
			Description: pointer.To(fmt.Sprintf("Unbundled %s %s", conn.Unbundled.Link.Server.DeviceName(), connName)),
			Speed:       getPortSpeed(agent, swPort.LocalPortName()),
			TrunkVLANs:  []string{},
			MTU:         mtu,
		}
	}

	return nil
}

func planDefaultVRFWithBGP(agent *agentapi.Agent, spec *dozer.Spec) error {
	ip, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
	}

	spec.VRFs[VRFDefault].AnycastMAC = pointer.To(AnycastMAC)
	spec.VRFs[VRFDefault].BGP = &dozer.SpecVRFBGP{
		AS:                 pointer.To(agent.Spec.Switch.ASN),
		RouterID:           pointer.To(ip.String()),
		NetworkImportCheck: pointer.To(true), // default
		Neighbors:          map[string]*dozer.SpecVRFBGPNeighbor{},
		IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
			Enabled:  true,
			MaxPaths: pointer.To(getMaxPaths(agent)),
		},
		L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
			Enabled:         agent.IsSpineLeaf(),
			AdvertiseAllVNI: pointer.To(true),
		},
	}
	spec.VRFs[VRFDefault].TableConnections = map[string]*dozer.SpecVRFTableConnection{
		dozer.SpecVRFBGPTableConnectionConnected: {},
		dozer.SpecVRFBGPTableConnectionStatic:    {},
	}

	return nil
}

func planVXLAN(agent *agentapi.Agent, spec *dozer.Spec) error {
	if !agent.IsSpineLeaf() {
		return nil
	}

	ip, _, err := net.ParseCIDR(agent.Spec.Switch.VTEPIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse vtep ip %s", agent.Spec.Switch.VTEPIP)
	}

	spec.VXLANTunnels = map[string]*dozer.SpecVXLANTunnel{
		VTEPFabric: {
			SourceIP:        pointer.To(ip.String()),
			SourceInterface: pointer.To(LoopbackVTEP),
		},
	}

	spec.VXLANEVPNNVOs = map[string]*dozer.SpecVXLANEVPNNVO{
		EVPNNVO: {
			SourceVTEP: pointer.To(VTEPFabric),
		},
	}

	return nil
}

func spineLinkTracking(agent *agentapi.Agent, spec *dozer.Spec) {
	for _, conn := range agent.Spec.Connections {
		if conn.Fabric == nil {
			continue
		}

		for _, link := range conn.Fabric.Links {
			if link.Leaf.DeviceName() != agent.Name {
				continue
			}

			port := link.Leaf.LocalPortName()

			spec.LSTInterfaces[port] = &dozer.SpecLSTInterface{
				Groups: []string{LSTGroupSpineLink},
			}
		}
	}
}

func planMCLAGDomain(agent *agentapi.Agent, spec *dozer.Spec) (bool, error) {
	ok := false
	mclagPeerLinks := map[string]string{}
	mclagSessionLinks := map[string]string{}
	mclagPeerSwitch := ""
	for _, conn := range agent.Spec.Connections {
		if conn.MCLAGDomain != nil {
			ok = true
			for _, link := range conn.MCLAGDomain.PeerLinks {
				if link.Switch1.DeviceName() == agent.Name {
					mclagPeerLinks[link.Switch1.LocalPortName()] = link.Switch2.Port
					mclagPeerSwitch = link.Switch2.DeviceName()
				} else if link.Switch2.DeviceName() == agent.Name {
					mclagPeerLinks[link.Switch2.LocalPortName()] = link.Switch1.Port
					mclagPeerSwitch = link.Switch1.DeviceName()
				}
			}
			for _, link := range conn.MCLAGDomain.SessionLinks {
				if link.Switch1.DeviceName() == agent.Name {
					mclagSessionLinks[link.Switch1.LocalPortName()] = link.Switch2.Port
				} else if link.Switch2.DeviceName() == agent.Name {
					mclagSessionLinks[link.Switch2.LocalPortName()] = link.Switch1.Port
				}
			}

			break
		}
	}

	// if there is no MCLAG domain, we are done
	if !ok {
		return false, nil
	}

	if len(mclagPeerLinks) == 0 {
		return false, errors.Errorf("no mclag peer links found")
	}
	if len(mclagSessionLinks) == 0 {
		return false, errors.Errorf("no mclag session links found")
	}
	if mclagPeerSwitch == "" {
		return false, errors.Errorf("no mclag peer switch found")
	}

	mclagSessionSubnet, err := netip.ParsePrefix(agent.Spec.Config.MCLAGSessionSubnet)
	if err != nil {
		return false, errors.Wrapf(err, "failed to parse MCLAG session subnet %s", agent.Spec.Config.MCLAGSessionSubnet)
	}

	mclagSessionIP1 := mclagSessionSubnet.Addr().String()
	mclagSessionIP2 := mclagSessionSubnet.Addr().Next().String()

	// using the same IP pair with switch with name < peer switch name getting first IP
	sourceIP := mclagSessionIP1
	peerIP := mclagSessionIP2
	if agent.Name > mclagPeerSwitch {
		sourceIP, peerIP = peerIP, sourceIP
	}

	mclagPeerPortChannelName := portChannelName(MCLAGPeerLinkPortChannelID)
	mclagPeerPortChannel := &dozer.SpecInterface{
		Description: pointer.To(fmt.Sprintf("MCLAG peer %s", mclagPeerSwitch)),
		Enabled:     pointer.To(true),
		TrunkVLANs:  []string{MCLAGPeerLinkTrunkVLANRange},
	}
	spec.Interfaces[mclagPeerPortChannelName] = mclagPeerPortChannel
	for iface, peerPort := range mclagPeerLinks {
		descr := fmt.Sprintf("PC%d MCLAG peer %s", MCLAGPeerLinkPortChannelID, peerPort)
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagPeerPortChannelName, nil, agent)
		if err != nil {
			return false, errors.Wrapf(err, "failed to setup physical interface %s for MCLAG peer link", iface)
		}
	}

	mclagSessionPortChannelName := portChannelName(MCLAGSessionLinkPortChannelID)
	mclagSessionPortChannel := &dozer.SpecInterface{
		Description: pointer.To(fmt.Sprintf("MCLAG session %s", mclagPeerSwitch)),
		Enabled:     pointer.To(true),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					sourceIP: {
						PrefixLen: pointer.To(uint8(mclagSessionSubnet.Bits())), //nolint:gosec
					},
				},
			},
		},
	}
	spec.Interfaces[mclagSessionPortChannelName] = mclagSessionPortChannel
	for iface, peerPort := range mclagSessionLinks {
		descr := fmt.Sprintf("PC%d MCLAG session %s", MCLAGSessionLinkPortChannelID, peerPort)
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagSessionPortChannelName, nil, agent)
		if err != nil {
			return false, errors.Wrapf(err, "failed to setup physical interface %s for MCLAG session link", iface)
		}
	}

	spec.MCLAGs[MCLAGDomainID] = &dozer.SpecMCLAGDomain{
		SourceIP: sourceIP,
		PeerIP:   peerIP,
		PeerLink: mclagPeerPortChannelName,
	}

	spec.VRFs[VRFDefault].BGP.Neighbors[peerIP] = &dozer.SpecVRFBGPNeighbor{
		Enabled:     pointer.To(true),
		Description: pointer.To(fmt.Sprintf("MCLAG session %s", mclagPeerSwitch)),
		PeerType:    pointer.To(dozer.SpecVRFBGPNeighborPeerTypeInternal),
		IPv4Unicast: pointer.To(true),
	}

	spec.LSTGroups[LSTGroupSpineLink] = &dozer.SpecLSTGroup{
		AllEVPNESDownstream: nil,
		AllMCLAGDownstream:  pointer.To(true),
		Timeout:             pointer.To(uint16(5)),
	}

	spineLinkTracking(agent, spec)

	return sourceIP == mclagSessionIP1, nil
}

func planESLAG(agent *agentapi.Agent, spec *dozer.Spec) error { //nolint:unparam
	spec.VRFs[VRFDefault].EVPNMH = dozer.SpecVRFEVPNMH{
		MACHoldtime:  pointer.To(uint32(60)),
		StartupDelay: pointer.To(uint32(60)),
	}

	if !agent.Spec.Role.IsLeaf() {
		return nil
	}

	spec.LSTGroups[LSTGroupSpineLink] = &dozer.SpecLSTGroup{
		AllEVPNESDownstream: pointer.To(true),
		AllMCLAGDownstream:  nil,
		Timeout:             pointer.To(uint16(60)),
	}

	spineLinkTracking(agent, spec)

	return nil
}

func planUsers(agent *agentapi.Agent, spec *dozer.Spec) error { //nolint:unparam
	for _, user := range agent.Spec.Users {
		if user.Name == AgentUser {
			// never configure agent user other than through agent setup
			continue
		}

		spec.Users[user.Name] = &dozer.SpecUser{
			Password:       user.Password,
			Role:           user.Role,
			AuthorizedKeys: user.SSHKeys,
		}
	}

	return nil
}

func vrfName(name string) string {
	return "Vrf" + name
}

func vpcVrfName(vpcName string) string {
	return vrfName("V" + vpcName)
}

func ipnsVrfName(ipnsName string) string {
	return vrfName("I" + ipnsName)
}

// normalize nexthops as we get them in an inconsistent order, and otherwise
// this can cause a diff when there is none
func NextHopCompare(a, b dozer.SpecVRFStaticRouteNextHop) int {
	if a.IP < b.IP {
		return -1
	} else if a.IP > b.IP {
		return 1
	}
	if a.Interface == nil && b.Interface != nil {
		return -1
	}
	if a.Interface != nil && b.Interface == nil {
		return 1
	}
	if a.Interface != nil && b.Interface != nil {
		return strings.Compare(*a.Interface, *b.Interface)
	}

	return 0
}

func planVPCs(agent *agentapi.Agent, spec *dozer.Spec) error {
	spec.PrefixLists[PrefixListVPCLoopback] = &dozer.SpecPrefixList{
		Prefixes: map[uint32]*dozer.SpecPrefixListEntry{
			10: {
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: agent.Spec.Config.VPCLoopbackSubnet,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			},
		},
	}

	spec.CommunityLists[NoCommunity] = &dozer.SpecCommunityList{
		Members: []string{"REGEX:^$"},
	}

	spec.RouteMaps[RouteMapFilterAttachedHost] = &dozer.SpecRouteMap{
		Statements: map[string]*dozer.SpecRouteMapStatement{
			"10": {
				Conditions: dozer.SpecRouteMapConditions{
					AttachedHost: pointer.To(true),
				},
				Result: dozer.SpecRouteMapResultReject,
			},
			"20": {
				Conditions: dozer.SpecRouteMapConditions{},
				Result:     dozer.SpecRouteMapResultAccept,
			},
		},
	}

	for vpcName, vpc := range agent.Spec.VPCs {
		vrfName := vpcVrfName(vpcName)

		irbVLAN := agent.Spec.Catalog.IRBVLANs[vpcName]
		if irbVLAN == 0 {
			return errors.Errorf("IRB VLAN for VPC %s not found", vpcName)
		}

		irbIface := vlanName(irbVLAN)
		spec.Interfaces[irbIface] = &dozer.SpecInterface{
			Enabled:     pointer.To(true),
			Description: pointer.To(fmt.Sprintf("VPC %s IRB", vpcName)),
		}

		if spec.VRFs[vrfName] == nil {
			spec.VRFs[vrfName] = &dozer.SpecVRF{}
		}
		if spec.VRFs[vrfName].Interfaces == nil {
			spec.VRFs[vrfName].Interfaces = map[string]*dozer.SpecVRFInterface{}
		}
		if spec.VRFs[vrfName].StaticRoutes == nil {
			spec.VRFs[vrfName].StaticRoutes = map[string]*dozer.SpecVRFStaticRoute{}
		}
		if spec.VRFs[vrfName].AttachedHost == nil {
			spec.VRFs[vrfName].AttachedHost = &dozer.SpecVRFAttachedHost{}
		}

		peerComm, err := communityForVPC(agent, vpcName)
		if err != nil {
			return errors.Wrapf(err, "failed to get community for VPC %s", vpcName)
		}

		vpcPeersCommList := vpcPeersCommListName(vpcName)
		spec.CommunityLists[vpcPeersCommList] = &dozer.SpecCommunityList{
			Members: []string{peerComm},
		}

		spec.PrefixLists[vpcPeersPrefixListName(vpcName)] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		spec.PrefixLists[vpcSubnetsPrefixListName(vpcName)] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		extPrefixesName := vpcExtPrefixesPrefixListName(vpcName)
		if _, exists := spec.PrefixLists[extPrefixesName]; !exists {
			spec.PrefixLists[extPrefixesName] = &dozer.SpecPrefixList{
				Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
			}
		}

		spec.PrefixLists[vpcNotSubnetsPrefixListName(vpcName)] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{
				65535: {
					Prefix: dozer.SpecPrefixListPrefix{
						Prefix: "0.0.0.0/0",
						Le:     32,
					},
					Action: dozer.SpecPrefixListActionPermit,
				},
			},
		}

		spec.PrefixLists[vpcStaticExtSubnetsPrefixListName(vpcName)] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		for subnetName, subnet := range vpc.Subnets {
			vni, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpcName, subnetName)
			if vni == 0 || !ok {
				return errors.Errorf("VNI for VPC %s subnet %s not found", vpcName, subnetName)
			}
			vni %= 100

			spec.PrefixLists[vpcSubnetsPrefixListName(vpcName)].Prefixes[vni] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet.Subnet,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}

			spec.PrefixLists[vpcNotSubnetsPrefixListName(vpcName)].Prefixes[vni] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet.Subnet,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionDeny,
			}
		}

		importVrfRouteMap := vpcExtImportVrfRouteMapName(vpcName)
		if _, exists := spec.RouteMaps[importVrfRouteMap]; !exists {
			spec.RouteMaps[importVrfRouteMap] = &dozer.SpecRouteMap{
				Statements: map[string]*dozer.SpecRouteMapStatement{
					"1": {
						Conditions: dozer.SpecRouteMapConditions{
							MatchNextHopPrefixList: pointer.To(PrefixListVPCLoopback),
						},
						Result: dozer.SpecRouteMapResultReject,
					},
					"50000": {
						Conditions: dozer.SpecRouteMapConditions{
							MatchCommunityList: pointer.To(vpcPeersCommList),
						},
						Result: dozer.SpecRouteMapResultAccept,
					},
					"50001": {
						Conditions: dozer.SpecRouteMapConditions{
							MatchCommunityList: pointer.To(NoCommunity),
							MatchPrefixList:    pointer.To(vpcPeersPrefixListName(vpcName)),
						},
						Result: dozer.SpecRouteMapResultAccept,
					},
					"65535": {
						Result: dozer.SpecRouteMapResultReject,
					},
				},
			}
		}

		vpcComm, err := communityForVPC(agent, vpcName)
		if err != nil {
			return errors.Wrapf(err, "failed to get community for VPC %s", vpcName)
		}

		vpcRedistributeConnectedRouteMap := vpcRedistributeConnectedRouteMapName(vpcName)
		spec.RouteMaps[vpcRedistributeConnectedRouteMap] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"1": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(PrefixListVPCLoopback),
					},
					Result: dozer.SpecRouteMapResultReject,
				},
				"5": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(vpcSubnetsPrefixListName(vpcName)),
					},
					SetCommunities: []string{vpcComm},
					Result:         dozer.SpecRouteMapResultAccept,
				},
				"6": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(vpcStaticExtSubnetsPrefixListName(vpcName)),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
				"10": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}

		vpcRedistributeStaticRouteMap := vpcRedistributeStaticRouteMapName(vpcName)
		spec.RouteMaps[vpcRedistributeStaticRouteMap] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"1": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(PrefixListVPCLoopback),
					},
					Result: dozer.SpecRouteMapResultReject,
				},
				"5": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(vpcStaticExtSubnetsPrefixListName(vpcName)),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: pointer.To(vpcExtPrefixesPrefixListName(vpcName)),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
			},
		}

		protocolIP, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
		}

		spec.VRFs[vrfName].Enabled = pointer.To(true)
		spec.VRFs[vrfName].AnycastMAC = pointer.To(AnycastMAC)
		spec.VRFs[vrfName].BGP = &dozer.SpecVRFBGP{
			AS:                 pointer.To(agent.Spec.Switch.ASN),
			RouterID:           pointer.To(protocolIP.String()),
			NetworkImportCheck: pointer.To(true),
			IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
				Enabled:      true,
				MaxPaths:     pointer.To(getMaxPaths(agent)),
				ImportPolicy: pointer.To(importVrfRouteMap),
				ImportVRFs:   map[string]*dozer.SpecVRFBGPImportVRF{},
			},
			L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
				Enabled:                       agent.IsSpineLeaf(),
				AdvertiseIPv4Unicast:          pointer.To(true),
				AdvertiseIPv4UnicastRouteMaps: []string{RouteMapFilterAttachedHost},
			},
		}
		spec.VRFs[vrfName].TableConnections = map[string]*dozer.SpecVRFTableConnection{
			dozer.SpecVRFBGPTableConnectionConnected: {
				ImportPolicies: []string{vpcRedistributeConnectedRouteMap},
			},
			dozer.SpecVRFBGPTableConnectionStatic: {
				ImportPolicies: []string{vpcRedistributeStaticRouteMap},
			},
			dozer.SpecVRFBGPTableConnectionAttachedHost: {},
		}
		spec.VRFs[vrfName].Interfaces[irbIface] = &dozer.SpecVRFInterface{}

		if agent.IsSpineLeaf() {
			spec.SuppressVLANNeighs[irbIface] = &dozer.SpecSuppressVLANNeigh{}

			vpcVNI := agent.Spec.Catalog.VPCVNIs[vpcName]
			if vpcVNI == 0 {
				return errors.Errorf("VNI for VPC %s not found", vpcName)
			}
			spec.VRFVNIMap[vrfName] = &dozer.SpecVRFVNIEntry{
				VNI: pointer.To(vpcVNI),
			}
			spec.VXLANTunnelMap[fmt.Sprintf("map_%d_%s", vpcVNI, irbIface)] = &dozer.SpecVXLANTunnelMap{
				VTEP: pointer.To(VTEPFabric),
				VNI:  pointer.To(vpcVNI),
				VLAN: pointer.To(irbVLAN),
			}
		}

		if agent.Spec.AttachedVPCs[vpcName] {
			for _, route := range vpc.StaticRoutes {
				nextHops := []dozer.SpecVRFStaticRouteNextHop{}
				for _, nextHop := range route.NextHops {
					nextHops = append(nextHops, dozer.SpecVRFStaticRouteNextHop{IP: nextHop})
				}
				slices.SortStableFunc(nextHops, NextHopCompare)

				spec.VRFs[vrfName].StaticRoutes[route.Prefix] = &dozer.SpecVRFStaticRoute{
					NextHops: nextHops,
				}
			}
		}
	}

	for attachName, attach := range agent.Spec.VPCAttachments {
		vpcName := attach.VPCName()
		vpc, exists := agent.Spec.VPCs[vpcName]
		if !exists {
			return errors.Errorf("VPC %s not found", vpcName)
		}

		subnetName := attach.SubnetName()
		subnet := vpc.Subnets[subnetName]
		if subnet == nil {
			return errors.Errorf("VPC %s subnet %s not found", vpcName, subnetName)
		}

		err := planVPCSubnet(agent, spec, vpcName, vpc, subnetName, subnet)
		if err != nil {
			return errors.Wrapf(err, "failed to plan VPC %s subnet %s", vpcName, subnetName)
		}

		conn, exists := agent.Spec.Connections[attach.Connection]
		if !exists {
			return errors.Errorf("connection %s not found for VPC attachment %s", attach.Connection, attachName)
		}

		ifaces := []string{}
		if conn.MCLAG != nil { //nolint:gocritic
			for _, link := range conn.MCLAG.Links {
				if link.Switch.DeviceName() != agent.Name {
					continue
				}

				portChan := agent.Spec.Catalog.PortChannelIDs[attach.Connection]
				if portChan == 0 {
					return errors.Errorf("no port channel found for conn %s", attach.Connection)
				}

				ifaces = append(ifaces, portChannelName(portChan))
			}
		} else if conn.ESLAG != nil {
			for _, link := range conn.ESLAG.Links {
				if link.Switch.DeviceName() != agent.Name {
					continue
				}

				portChan := agent.Spec.Catalog.PortChannelIDs[attach.Connection]
				if portChan == 0 {
					return errors.Errorf("no port channel found for conn %s", attach.Connection)
				}

				ifaces = append(ifaces, portChannelName(portChan))
			}
		} else if conn.Bundled != nil {
			for _, link := range conn.Bundled.Links {
				if link.Switch.DeviceName() != agent.Name {
					continue
				}

				portChan := agent.Spec.Catalog.PortChannelIDs[attach.Connection]
				if portChan == 0 {
					return errors.Errorf("no port channel found for conn %s", attach.Connection)
				}

				ifaces = append(ifaces, portChannelName(portChan))
			}
		} else if conn.Unbundled != nil {
			if conn.Unbundled.Link.Switch.DeviceName() != agent.Name {
				continue
			}

			ifaces = append(ifaces, conn.Unbundled.Link.Switch.LocalPortName())
		}

		for _, iface := range ifaces {
			if attach.NativeVLAN {
				spec.Interfaces[iface].AccessVLAN = pointer.To(subnet.VLAN)
			} else {
				vlanStr := fmt.Sprintf("%d", subnet.VLAN)
				if !slices.Contains(spec.Interfaces[iface].TrunkVLANs, vlanStr) {
					spec.Interfaces[iface].TrunkVLANs = append(spec.Interfaces[iface].TrunkVLANs, vlanStr)
				}
			}
		}
	}

	for configuredSubnet, val := range agent.Spec.ConfiguredVPCSubnets {
		if !val {
			continue
		}

		parts := strings.Split(configuredSubnet, "/")
		if len(parts) != 2 {
			return errors.Errorf("invalid configured subnet %s", configuredSubnet)
		}

		vpcName := parts[0]
		subnetName := parts[1]

		vpc, exists := agent.Spec.VPCs[vpcName]
		if !exists {
			return errors.Errorf("VPC %s not found", vpcName)
		}
		subnet, exists := vpc.Subnets[subnetName]
		if !exists {
			return errors.Errorf("VPC %s subnet %s not found", vpcName, subnetName)
		}

		err := planVPCSubnet(agent, spec, vpcName, vpc, subnetName, subnet)
		if err != nil {
			return errors.Wrapf(err, "failed to plan VPC %s subnet %s for configuredSubnets", vpcName, subnetName)
		}
	}

	for peeringName, peering := range agent.Spec.VPCPeerings {
		vpc1Name, vpc2Name, err := peering.VPCs()
		if err != nil {
			return errors.Wrapf(err, "failed to parse VPCs for VPC peering %s", peeringName)
		}

		vpc1, exists := agent.Spec.VPCs[vpc1Name]
		if !exists {
			return errors.Errorf("VPC %s not found for VPC peering %s", vpc1Name, peeringName)
		}
		vpc2, exists := agent.Spec.VPCs[vpc2Name]
		if !exists {
			return errors.Errorf("VPC %s not found for VPC peering %s", vpc2Name, peeringName)
		}

		peerComm, err := communityForVPC(agent, vpc2Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get community for VPC %s", vpc2Name)
		}
		if !slices.Contains(spec.CommunityLists[vpcPeersCommListName(vpc1Name)].Members, peerComm) {
			spec.CommunityLists[vpcPeersCommListName(vpc1Name)].Members = append(spec.CommunityLists[vpcPeersCommListName(vpc1Name)].Members, peerComm)
			sort.Strings(spec.CommunityLists[vpcPeersCommListName(vpc1Name)].Members)
		}

		peerComm, err = communityForVPC(agent, vpc1Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get community for VPC %s", vpc1Name)
		}
		if !slices.Contains(spec.CommunityLists[vpcPeersCommListName(vpc2Name)].Members, peerComm) {
			spec.CommunityLists[vpcPeersCommListName(vpc2Name)].Members = append(spec.CommunityLists[vpcPeersCommListName(vpc2Name)].Members, peerComm)
			sort.Strings(spec.CommunityLists[vpcPeersCommListName(vpc2Name)].Members)
		}

		peersPrefixList := vpcPeersPrefixListName(vpc2Name)
		for subnetName, subnet := range vpc1.Subnets {
			vni, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpc1Name, subnetName)
			if vni == 0 || !ok {
				return errors.Errorf("VNI for VPC %s subnet %s not found", vpc1Name, subnetName)
			}

			spec.PrefixLists[peersPrefixList].Prefixes[vni] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet.Subnet,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}
		}

		peersPrefixList = vpcPeersPrefixListName(vpc1Name)
		for subnetName, subnet := range vpc2.Subnets {
			vni, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpc2Name, subnetName)
			if vni == 0 || !ok {
				return errors.Errorf("VNI for VPC %s subnet %s not found", vpc2Name, subnetName)
			}

			spec.PrefixLists[peersPrefixList].Prefixes[vni] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet.Subnet,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}
		}

		// TODO dedup
		vni1 := agent.Spec.Catalog.VPCVNIs[vpc1Name]
		if vni1 == 0 {
			return errors.Errorf("VNI for VPC %s not found", vpc1Name)
		}
		if vni1%100 != 0 {
			return errors.Errorf("VNI for VPC %s is not a multiple of 100", vpc1Name)
		}
		if vni1/100 >= 40000 { // 50k is reserved for external-related in import vpc route map
			return errors.Errorf("VNI for VPC %s is too large", vpc1Name)
		}
		vni2 := agent.Spec.Catalog.VPCVNIs[vpc2Name]
		if vni2 == 0 {
			return errors.Errorf("VNI for VPC %s not found", vpc2Name)
		}
		if vni2%100 != 0 {
			return errors.Errorf("VNI for VPC %s is not a multiple of 100", vpc2Name)
		}
		if vni2/100 >= 40000 { // 50k is reserved for external-related in import vpc route map
			return errors.Errorf("VNI for VPC %s is too large", vpc2Name)
		}

		spec.RouteMaps[vpcExtImportVrfRouteMapName(vpc1Name)].Statements[fmt.Sprintf("%d", 10000+vni2/100)] = &dozer.SpecRouteMapStatement{
			Conditions: dozer.SpecRouteMapConditions{
				MatchPrefixList: pointer.To(vpcNotSubnetsPrefixListName(vpc2Name)),
				MatchSourceVRF:  pointer.To(vpcVrfName(vpc2Name)),
			},
			Result: dozer.SpecRouteMapResultReject,
		}
		spec.RouteMaps[vpcExtImportVrfRouteMapName(vpc2Name)].Statements[fmt.Sprintf("%d", 10000+vni1/100)] = &dozer.SpecRouteMapStatement{
			Conditions: dozer.SpecRouteMapConditions{
				MatchPrefixList: pointer.To(vpcNotSubnetsPrefixListName(vpc1Name)),
				MatchSourceVRF:  pointer.To(vpcVrfName(vpc1Name)),
			},
			Result: dozer.SpecRouteMapResultReject,
		}

		if err := extendVPCFilteringACL(agent, spec, vpc1Name, vpc2Name, vpc1, vpc2, peering); err != nil {
			return errors.Wrapf(err, "failed to extend VPC filtering ACL for VPC peering %s", peeringName)
		}

		vrf1Name := vpcVrfName(vpc1Name)
		vrf2Name := vpcVrfName(vpc2Name)

		vpc1Attached := agent.Spec.AttachedVPCs[vpc1Name]
		vpc2Attached := agent.Spec.AttachedVPCs[vpc2Name]

		if peering.Remote == "" {
			if vpc1Attached && !vpc2Attached || !agent.Spec.Config.LoopbackWorkaround {
				spec.VRFs[vrf1Name].BGP.IPv4Unicast.ImportVRFs[vrf2Name] = &dozer.SpecVRFBGPImportVRF{}
			}

			if !vpc1Attached && vpc2Attached || !agent.Spec.Config.LoopbackWorkaround {
				spec.VRFs[vrf2Name].BGP.IPv4Unicast.ImportVRFs[vrf1Name] = &dozer.SpecVRFBGPImportVRF{}
			}

			if vpc1Attached && vpc2Attached && agent.Spec.Config.LoopbackWorkaround {
				sub1, sub2, ip1, ip2, err := planLoopbackWorkaround(agent, spec, librarian.LoWReqForVPC(peeringName))
				if err != nil {
					return errors.Wrapf(err, "failed to plan loopback workaround for VPC peering %s", peeringName)
				}

				spec.VRFs[vrf1Name].Interfaces[sub1] = &dozer.SpecVRFInterface{}
				spec.VRFs[vrf2Name].Interfaces[sub2] = &dozer.SpecVRFInterface{}

				// TODO deduplicate
				for subnetName, subnet := range agent.Spec.VPCs[vpc1Name].Subnets {
					_, ipNet, err := net.ParseCIDR(subnet.Subnet)
					if err != nil {
						return errors.Wrapf(err, "failed to parse subnet %s (%s) for VPC %s", subnetName, subnet.Subnet, vpc1Name)
					}
					prefixLen, _ := ipNet.Mask.Size()

					spec.VRFs[vrf2Name].StaticRoutes[fmt.Sprintf("%s/%d", ipNet.IP.String(), prefixLen)] = &dozer.SpecVRFStaticRoute{
						NextHops: []dozer.SpecVRFStaticRouteNextHop{
							{
								IP:        ip1,
								Interface: pointer.To(sub2),
							},
						},
					}
				}

				for subnetName, subnet := range agent.Spec.VPCs[vpc2Name].Subnets {
					_, ipNet, err := net.ParseCIDR(subnet.Subnet)
					if err != nil {
						return errors.Wrapf(err, "failed to parse subnet %s (%s) for VPC %s", subnetName, subnet.Subnet, vpc1Name)
					}
					prefixLen, _ := ipNet.Mask.Size()

					spec.VRFs[vrf1Name].StaticRoutes[fmt.Sprintf("%s/%d", ipNet.IP.String(), prefixLen)] = &dozer.SpecVRFStaticRoute{
						NextHops: []dozer.SpecVRFStaticRouteNextHop{
							{
								IP:        ip2,
								Interface: pointer.To(sub1),
							},
						},
					}
				}
			}
		} else if slices.Contains(agent.Spec.Switch.Groups, peering.Remote) {
			if vpc1Attached || vpc2Attached {
				slog.Warn("Skipping remote VPCPeering because one of the VPCs is locally attached",
					"vpcPeering", peeringName,
					"vpc1", vpc1Name, "vpc1Attached", vpc1Attached,
					"vpc2", vpc2Name, "vpc2Attached", vpc2Attached)

				continue
			}

			spec.VRFs[vrf1Name].BGP.IPv4Unicast.ImportVRFs[vrf2Name] = &dozer.SpecVRFBGPImportVRF{}
			spec.VRFs[vrf2Name].BGP.IPv4Unicast.ImportVRFs[vrf1Name] = &dozer.SpecVRFBGPImportVRF{}

			spec.RouteMaps[RouteMapBlockEVPNDefaultRemote].Statements[fmt.Sprintf("%d", uint(vni1/100))] = &dozer.SpecRouteMapStatement{
				Conditions: dozer.SpecRouteMapConditions{
					MatchEVPNVNI:          pointer.To(vni1),
					MatchEVPNDefaultRoute: pointer.To(true),
				},
				Result: dozer.SpecRouteMapResultReject,
			}
			spec.RouteMaps[RouteMapBlockEVPNDefaultRemote].Statements[fmt.Sprintf("%d", uint(vni2/100))] = &dozer.SpecRouteMapStatement{
				Conditions: dozer.SpecRouteMapConditions{
					MatchEVPNVNI:          pointer.To(vni2),
					MatchEVPNDefaultRoute: pointer.To(true),
				},
				Result: dozer.SpecRouteMapResultReject,
			}
		}
	}

	// cleanup empty (only a single permit) ACLs for all VPC/subnets
	for vpcName, vpc := range agent.Spec.VPCs {
		for subnetName, subnet := range vpc.Subnets {
			aclName := vpcFilteringAccessListName(vpcName, subnetName)
			if acl, ok := spec.ACLs[aclName]; ok {
				if len(acl.Entries) == 1 {
					delete(spec.ACLs, aclName)

					subnetIface := vlanName(subnet.VLAN)
					if aclIface, ok := spec.ACLInterfaces[subnetIface]; ok {
						if aclIface.Ingress != nil && *aclIface.Ingress == aclName {
							aclIface.Ingress = nil

							if aclIface.Egress == nil {
								delete(spec.ACLInterfaces, subnetIface)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

func planVPCSubnet(agent *agentapi.Agent, spec *dozer.Spec, vpcName string, vpc vpcapi.VPCSpec, subnetName string, subnet *vpcapi.VPCSubnet) error {
	vrfName := vpcVrfName(vpcName)

	subnetCIDR, err := iputil.ParseCIDR(subnet.Subnet)
	if err != nil {
		return errors.Wrapf(err, "failed to parse subnet %s for VPC %s", subnet.Subnet, vpcName)
	}
	prefixLen, _ := subnetCIDR.Subnet.Mask.Size()

	subnetIface := vlanName(subnet.VLAN)
	spec.Interfaces[subnetIface] = &dozer.SpecInterface{
		Enabled:     pointer.To(true),
		Description: pointer.To(fmt.Sprintf("VPC %s/%s", vpcName, subnetName)),
		VLANAnycastGateway: []string{
			fmt.Sprintf("%s/%d", subnet.Gateway, prefixLen),
		},
	}

	spec.VRFs[vrfName].Interfaces[subnetIface] = &dozer.SpecVRFInterface{}

	if !slices.Contains(spec.VRFs[vrfName].AttachedHost.Interfaces, subnetIface) {
		spec.VRFs[vrfName].AttachedHost.Interfaces = append(spec.VRFs[vrfName].AttachedHost.Interfaces, subnetIface)
	}

	vpcFilteringACL := vpcFilteringAccessListName(vpcName, subnetName)
	spec.ACLInterfaces[subnetIface] = &dozer.SpecACLInterface{
		Ingress: pointer.To(vpcFilteringACL),
	}

	spec.ACLs[vpcFilteringACL], err = buildVPCFilteringACL(agent, vpcName, vpc, subnetName, subnet)
	if err != nil {
		return errors.Wrapf(err, "failed to plan VPC filtering ACL for VPC %s subnet %s", vpcName, subnetName)
	}

	if agent.IsSpineLeaf() {
		spec.SuppressVLANNeighs[subnetIface] = &dozer.SpecSuppressVLANNeigh{}

		subnetVNI, ok := agent.Spec.Catalog.GetVPCSubnetVNI(vpcName, subnetName)
		if subnetVNI == 0 || !ok {
			return errors.Errorf("VNI for VPC %s subnet %s not found", vpcName, subnetName)
		}
		spec.VXLANTunnelMap[fmt.Sprintf("map_%d_%s", subnetVNI, subnetIface)] = &dozer.SpecVXLANTunnelMap{
			VTEP: pointer.To(VTEPFabric),
			VNI:  pointer.To(subnetVNI),
			VLAN: pointer.To(subnet.VLAN),
		}
	}

	if subnet.DHCP.Enable || subnet.DHCP.Relay != "" {
		var dhcpRelayIP net.IP

		if subnet.DHCP.Enable {
			dhcpRelayIP, _, err = net.ParseCIDR(agent.Spec.Config.ControlVIP)
			if err != nil {
				return errors.Wrapf(err, "failed to parse DHCP relay %s (control vip) for vpc %s", agent.Spec.Config.ControlVIP, vpcName)
			}
		} else {
			dhcpRelayIP, _, err = net.ParseCIDR(subnet.DHCP.Relay)
			if err != nil {
				return errors.Wrapf(err, "failed to parse DHCP relay %s for vpc %s", subnet.DHCP.Relay, vpcName)
			}
		}

		spec.DHCPRelays[subnetIface] = &dozer.SpecDHCPRelay{
			SourceInterface: pointer.To(MgmtIface),
			RelayAddress:    []string{dhcpRelayIP.String()},
			LinkSelect:      true,
			VRFSelect:       true,
		}
	}

	return nil
}

func buildVPCFilteringACL(agent *agentapi.Agent, vpcName string, vpc vpcapi.VPCSpec, subnetName string, subnet *vpcapi.VPCSubnet) (*dozer.SpecACL, error) {
	acl := &dozer.SpecACL{
		Entries: map[uint32]*dozer.SpecACLEntry{
			65535: {
				Action: dozer.SpecACLEntryActionAccept,
			},
		},
	}

	if vpc.IsSubnetRestricted(subnetName) {
		acl.Entries[1] = &dozer.SpecACLEntry{
			SourceAddress:      pointer.To(subnet.Subnet),
			DestinationAddress: pointer.To(subnet.Subnet),
			Action:             dozer.SpecACLEntryActionDrop,
		}
	}

	denySubnets := map[string]bool{}

	for otherSubnetName, otherSubnet := range vpc.Subnets {
		if otherSubnetName == subnetName {
			continue
		}

		if vpc.IsSubnetIsolated(otherSubnetName) {
			denySubnets[otherSubnet.Subnet] = true
		}
	}

	for permitIdx, permitPolicy := range vpc.Permit {
		if !slices.Contains(permitPolicy, subnetName) {
			continue
		}

		for _, otherSubnetName := range permitPolicy {
			if otherSubnetName == subnetName {
				continue
			}

			if otherSubnet, ok := vpc.Subnets[otherSubnetName]; ok {
				delete(denySubnets, otherSubnet.Subnet)
			} else {
				return nil, errors.Errorf("permit policy #%d: subnet %s not found in VPC %s", permitIdx, otherSubnetName, vpcName)
			}
		}
	}

	for subnet := range denySubnets {
		subnetID := agent.Spec.Catalog.SubnetIDs[subnet]
		if subnetID == 0 {
			return nil, errors.Errorf("no subnet id found for vpc %s subnet %s", vpcName, subnet)
		}
		if subnetID < 100 {
			return nil, errors.Errorf("subnet id for vpc %s subnet %s is too small", vpcName, subnet)
		}
		if subnetID >= 65000 {
			return nil, errors.Errorf("subnet id for vpc %s subnet %s is too large", vpcName, subnet)
		}

		acl.Entries[subnetID] = &dozer.SpecACLEntry{
			DestinationAddress: pointer.To(subnet),
			Action:             dozer.SpecACLEntryActionDrop,
		}
	}

	return acl, nil
}

func extendVPCFilteringACL(agent *agentapi.Agent, spec *dozer.Spec, vpc1Name, vpc2Name string, vpc1, vpc2 vpcapi.VPCSpec, vpcPeering vpcapi.VPCPeeringSpec) error {
	vpc1Deny := map[string]map[string]bool{}
	vpc2Deny := map[string]map[string]bool{}

	for vpc1SubnetName := range vpc1.Subnets {
		for vpc2SubnetName := range vpc2.Subnets {
			if vpc1Deny[vpc1SubnetName] == nil {
				vpc1Deny[vpc1SubnetName] = map[string]bool{}
			}
			if vpc2Deny[vpc2SubnetName] == nil {
				vpc2Deny[vpc2SubnetName] = map[string]bool{}
			}

			vpc1Deny[vpc1SubnetName][vpc2SubnetName] = true
			vpc2Deny[vpc2SubnetName][vpc1SubnetName] = true
		}
	}

	for _, permitPolicy := range vpcPeering.Permit {
		vpc1Subnets := permitPolicy[vpc1Name].Subnets
		if len(vpc1Subnets) == 0 {
			for subnetName := range vpc1.Subnets {
				vpc1Subnets = append(vpc1Subnets, subnetName)
			}
		}

		vpc2Subnets := permitPolicy[vpc2Name].Subnets
		if len(vpc2Subnets) == 0 {
			for subnetName := range vpc2.Subnets {
				vpc2Subnets = append(vpc2Subnets, subnetName)
			}
		}

		for _, vpc1SubnetName := range vpc1Subnets {
			for _, vpc2SubnetName := range vpc2Subnets {
				delete(vpc1Deny[vpc1SubnetName], vpc2SubnetName)
				delete(vpc2Deny[vpc2SubnetName], vpc1SubnetName)
			}
		}
	}

	if err := addVPCFilteringACLEntryiesForVPC(agent, spec, vpc1Name, vpc2Name, vpc2, vpc1Deny); err != nil {
		return errors.Wrapf(err, "failed to add VPC filtering ACL entries for VPC %s", vpc1Name)
	}
	if err := addVPCFilteringACLEntryiesForVPC(agent, spec, vpc2Name, vpc1Name, vpc1, vpc2Deny); err != nil {
		return errors.Wrapf(err, "failed to add VPC filtering ACL entries for VPC %s", vpc2Name)
	}

	return nil
}

func addVPCFilteringACLEntryiesForVPC(agent *agentapi.Agent, spec *dozer.Spec, vpc1Name, vpc2Name string, vpc2 vpcapi.VPCSpec, vpc1Deny map[string]map[string]bool) error {
	for vpc1SubnetName, vpc1SubnetDeny := range vpc1Deny {
		for vpc2SubnetName, deny := range vpc1SubnetDeny {
			if !deny {
				continue
			}

			vpc2Subnet, ok := vpc2.Subnets[vpc2SubnetName]
			if !ok {
				return errors.Errorf("VPC %s subnet %s not found", vpc2Name, vpc2SubnetName)
			}

			subnetID := agent.Spec.Catalog.SubnetIDs[vpc2Subnet.Subnet]
			// TODO dedup
			if subnetID == 0 {
				return errors.Errorf("no subnet id found for vpc %s subnet %s", vpc2Name, vpc2SubnetName)
			}
			if subnetID < 100 {
				return errors.Errorf("subnet id for vpc %s subnet %s is too small", vpc2Name, vpc2SubnetName)
			}
			if subnetID >= 65000 {
				return errors.Errorf("subnet id for vpc %s subnet %s is too large", vpc2Name, vpc2SubnetName)
			}

			aclName := vpcFilteringAccessListName(vpc1Name, vpc1SubnetName)
			if spec.ACLs[aclName] != nil {
				spec.ACLs[aclName].Entries[subnetID] = &dozer.SpecACLEntry{
					DestinationAddress: pointer.To(vpc2Subnet.Subnet),
					Action:             dozer.SpecACLEntryActionDrop,
				}
			}
		}
	}

	return nil
}

func planExternalPeerings(agent *agentapi.Agent, spec *dozer.Spec) error {
	attachedVPCs := map[string]bool{}
	for _, attach := range agent.Spec.VPCAttachments {
		vpcName := attach.VPCName()
		_, exists := agent.Spec.VPCs[vpcName]
		if !exists {
			return errors.Errorf("VPC %s not found", vpcName)
		}

		attachedVPCs[vpcName] = true
	}

	for name, peering := range agent.Spec.ExternalPeerings {
		externalName := peering.Permit.External.Name
		external, exists := agent.Spec.Externals[externalName]
		if !exists {
			return errors.Errorf("external %s not found for external peering %s", externalName, name)
		}

		vpcName := peering.Permit.VPC.Name
		vpc, exists := agent.Spec.VPCs[vpcName]
		if !exists {
			return errors.Errorf("VPC %s not found for external peering %s", vpcName, name)
		}

		for _, subnetName := range peering.Permit.VPC.Subnets {
			subnet, exists := vpc.Subnets[subnetName]
			if !exists {
				return errors.Errorf("VPC %s subnet %s not found for external peering %s", vpcName, subnetName, name)
			}

			vni, exists := agent.Spec.Catalog.GetVPCSubnetVNI(vpcName, subnetName)
			if vni == 0 || !exists {
				return errors.Errorf("VNI for VPC %s subnet %s not found for external peering %s", vpcName, subnetName, name)
			}

			spec.PrefixLists[extOutboundPrefixList(externalName)].Prefixes[vni] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet.Subnet,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}
		}

		extPrefixesName := vpcExtPrefixesPrefixListName(vpcName)
		if _, exists := spec.PrefixLists[extPrefixesName]; !exists {
			spec.PrefixLists[extPrefixesName] = &dozer.SpecPrefixList{
				Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
			}
		}

		for _, prefix := range peering.Permit.External.Prefixes {
			idx := agent.Spec.Catalog.SubnetIDs[prefix.Prefix]
			if idx == 0 {
				return errors.Errorf("no external peering prefix id for prefix %s in peering %s", prefix.Prefix, name)
			}
			if idx >= 65000 {
				return errors.Errorf("external peering prefix id for prefix %s in peering %s is too large", prefix.Prefix, name)
			}

			spec.PrefixLists[extPrefixesName].Prefixes[idx] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: prefix.Prefix,
					Le:     32,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}
		}

		ipnsVrf := ipnsVrfName(external.IPv4Namespace)
		vpcVrf := vpcVrfName(vpcName)

		if !attachedVPCs[vpcName] || !agent.Spec.Config.LoopbackWorkaround {
			prefixes := map[uint32]*dozer.SpecPrefixListEntry{}
			for _, prefix := range peering.Permit.External.Prefixes {
				idx := agent.Spec.Catalog.SubnetIDs[prefix.Prefix]
				if idx == 0 {
					return errors.Errorf("no external peering prefix id for prefix %s in peering %s", prefix.Prefix, name)
				}
				if idx >= 65000 {
					return errors.Errorf("external peering prefix id for prefix %s in peering %s is too large", prefix.Prefix, name)
				}

				prefixes[idx] = &dozer.SpecPrefixListEntry{
					Prefix: dozer.SpecPrefixListPrefix{
						Prefix: prefix.Prefix,
						Le:     32,
					},
					Action: dozer.SpecPrefixListActionPermit,
				}
			}

			importVrfPrefixList := vpcExtImportVrfPrefixListName(vpcName, externalName)
			spec.PrefixLists[importVrfPrefixList] = &dozer.SpecPrefixList{
				Prefixes: prefixes,
			}

			importVrfRouteMap := vpcExtImportVrfRouteMapName(vpcName)
			spec.RouteMaps[importVrfRouteMap].Statements["5"] = &dozer.SpecRouteMapStatement{
				Conditions: dozer.SpecRouteMapConditions{
					MatchPrefixList: pointer.To(ipnsSubnetsPrefixListName(vpc.IPv4Namespace)),
					MatchSourceVRF:  pointer.To(ipnsVrfName(vpc.IPv4Namespace)),
				},
				Result: dozer.SpecRouteMapResultReject,
			}

			idx := agent.Spec.Catalog.ExternalIDs[externalName]
			if idx == 0 {
				return errors.Errorf("no external seq for external %s", externalName)
			}
			if idx < 10 { // first 10 reserved for static statements
				return errors.Errorf("external seq for external %s is too small", externalName)
			}
			if idx >= 10000 {
				return errors.Errorf("external seq for external %s is too large", externalName)
			}
			spec.RouteMaps[importVrfRouteMap].Statements[fmt.Sprintf("%d", 50000+idx)] = &dozer.SpecRouteMapStatement{
				Conditions: dozer.SpecRouteMapConditions{
					MatchCommunityList: pointer.To(extInboundCommListName(externalName)),
					MatchPrefixList:    pointer.To(importVrfPrefixList),
				},
				SetLocalPreference: pointer.To(uint32(500)),
				Result:             dozer.SpecRouteMapResultAccept,
			}

			spec.VRFs[ipnsVrf].BGP.IPv4Unicast.ImportVRFs[vpcVrf] = &dozer.SpecVRFBGPImportVRF{}
			spec.VRFs[vpcVrf].BGP.IPv4Unicast.ImportVRFs[ipnsVrf] = &dozer.SpecVRFBGPImportVRF{}
		} else {
			sub1, sub2, ip1, ip2, err := planLoopbackWorkaround(agent, spec, librarian.LoWReqForExt(name))
			if err != nil {
				return errors.Wrapf(err, "failed to plan loopback workaround for external peering %s", name)
			}

			spec.VRFs[vpcVrf].Interfaces[sub1] = &dozer.SpecVRFInterface{}
			spec.VRFs[ipnsVrf].Interfaces[sub2] = &dozer.SpecVRFInterface{}

			spec.ACLInterfaces[sub1] = &dozer.SpecACLInterface{
				Egress: pointer.To(ipnsEgressAccessList(external.IPv4Namespace)),
			}

			for _, subnetName := range peering.Permit.VPC.Subnets {
				subnet, exists := vpc.Subnets[subnetName]
				if !exists {
					return errors.Errorf("VPC %s subnet %s not found for external peering %s", vpcName, subnetName, name)
				}

				_, ipNet, err := net.ParseCIDR(subnet.Subnet)
				if err != nil {
					return errors.Wrapf(err, "failed to parse subnet %s (%s) for VPC %s", subnetName, subnet.Subnet, vpcName)
				}
				prefixLen, _ := ipNet.Mask.Size()

				spec.VRFs[ipnsVrf].StaticRoutes[fmt.Sprintf("%s/%d", ipNet.IP.String(), prefixLen)] = &dozer.SpecVRFStaticRoute{
					NextHops: []dozer.SpecVRFStaticRouteNextHop{
						{
							IP:        ip1,
							Interface: pointer.To(sub2),
						},
					},
				}

				spec.VRFs[ipnsVrf].BGP.IPv4Unicast.Networks[subnet.Subnet] = &dozer.SpecVRFBGPNetwork{}
			}

			for _, prefix := range peering.Permit.External.Prefixes {
				_, ipNet, err := net.ParseCIDR(prefix.Prefix)
				if err != nil {
					return errors.Wrapf(err, "failed to parse prefix %s for external peering %s", prefix.Prefix, name)
				}
				prefixLen, _ := ipNet.Mask.Size()

				spec.VRFs[vpcVrf].StaticRoutes[fmt.Sprintf("%s/%d", ipNet.IP.String(), prefixLen)] = &dozer.SpecVRFStaticRoute{
					NextHops: []dozer.SpecVRFStaticRouteNextHop{
						{
							IP:        ip2,
							Interface: pointer.To(sub1),
						},
					},
				}
			}
		}
	}

	return nil
}

func planLoopbackWorkaround(agent *agentapi.Agent, spec *dozer.Spec, loWReq string) (string, string, string, string, error) {
	vlan := agent.Spec.Catalog.LoopbackWorkaroundVLANs[loWReq]
	if vlan == 0 {
		return "", "", "", "", errors.Errorf("workaround VLAN for peering %s not found", loWReq)
	}

	link := agent.Spec.Catalog.LooopbackWorkaroundLinks[loWReq]
	if link == "" {
		return "", "", "", "", errors.Errorf("workaround link for peering %s not found", loWReq)
	}

	ports := strings.Split(link, "--")
	if len(ports) != 2 {
		return "", "", "", "", errors.Errorf("workaround link for peering %s is invalid", loWReq)
	}
	if spec.Interfaces[ports[0]] == nil {
		return "", "", "", "", errors.Errorf("workaround link port %s for peering %s not found", ports[0], loWReq)
	}
	if spec.Interfaces[ports[1]] == nil {
		return "", "", "", "", errors.Errorf("workaround link port %s for peering %s not found", ports[1], loWReq)
	}

	ip1, ip2, err := vpcWorkaroundIPs(agent, vlan)
	if err != nil {
		return "", "", "", "", errors.Wrapf(err, "failed to get workaround IPs for peering")
	}

	spec.Interfaces[ports[0]].Subinterfaces[uint32(vlan)] = &dozer.SpecSubinterface{
		VLAN: &vlan,
		IPs: map[string]*dozer.SpecInterfaceIP{
			ip1: {
				PrefixLen: pointer.To(uint8(31)),
			},
		},
	}

	spec.Interfaces[ports[1]].Subinterfaces[uint32(vlan)] = &dozer.SpecSubinterface{
		VLAN:            &vlan,
		AnycastGateways: []string{ip2 + "/31"},
	}

	sub1 := fmt.Sprintf("%s.%d", ports[0], vlan)
	sub2 := fmt.Sprintf("%s.%d", ports[1], vlan)

	return sub1, sub2, ip1, ip2, nil
}

func getPortSpeed(agent *agentapi.Agent, port string) *string {
	var speed *string

	sp := agent.Spec.SwitchProfile
	if sp != nil && sp.Ports != nil && sp.PortProfiles != nil {
		if port, exists := sp.Ports[port]; exists && port.Group == "" && port.Profile != "" {
			if profile, exists := sp.PortProfiles[port.Profile]; exists && profile.Speed != nil {
				speed = &profile.Speed.Default
			}
		}
	}

	if agent.Spec.Switch.PortSpeeds != nil {
		if cSpeed, exists := agent.Spec.Switch.PortSpeeds[port]; exists {
			speed = &cSpeed
		}
	}

	return speed
}

func getMaxPaths(agent *agentapi.Agent) uint32 {
	if agent.Spec.SwitchProfile != nil && agent.Spec.SwitchProfile.Config.MaxPathsEBGP > 0 {
		return agent.Spec.SwitchProfile.Config.MaxPathsEBGP
	}

	return agent.Spec.Config.DefaultMaxPathsEBGP
}

// TODO test
func vpcWorkaroundIPs(agent *agentapi.Agent, vlan uint16) (string, string, error) {
	_, ipNet, err := net.ParseCIDR(agent.Spec.Config.VPCLoopbackSubnet)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to parse VPC loopback subnet %s", agent.Spec.Config.VPCLoopbackSubnet)
	}

	prefixLen, _ := ipNet.Mask.Size()
	if prefixLen > 20 {
		return "", "", errors.Errorf("subnet should be at least /20")
	}
	ip := ipNet.IP.To4()
	ip[2] += byte(vlan / 128) // TODO ?? (vlan - starting vlan) / 128
	ip[3] += byte(vlan % 128 * 2)

	res1 := ip.String()

	ip[3]++

	if !ipNet.Contains(ip) {
		return "", "", errors.Errorf("subnet %s is too small for VLAN %d", agent.Spec.Config.VPCLoopbackSubnet, vlan)
	}

	res2 := ip.String()

	return res1, res2, nil
}

func portChannelName(id uint16) string {
	return fmt.Sprintf("PortChannel%d", id)
}

func vlanName(vlan uint16) string {
	return fmt.Sprintf("Vlan%d", vlan)
}

func setupPhysicalInterfaceWithPortChannel(spec *dozer.Spec, name, description, portChannel string, mtu *uint16, agent *agentapi.Agent) error { // TODO replace with generic function or drop
	if iface, exist := spec.Interfaces[name]; exist {
		descr := ""
		if iface.Description != nil {
			descr = ", description: " + *iface.Description
		}

		return errors.Errorf("physical interface %s already used for something%s", name, descr)
	}

	physicalIface := &dozer.SpecInterface{
		Description: pointer.To(description),
		Enabled:     pointer.To(true),
		Speed:       getPortSpeed(agent, name),
		PortChannel: &portChannel,
		MTU:         mtu,
	}
	spec.Interfaces[name] = physicalIface

	return nil
}

func extInboundCommListName(external string) string {
	return fmt.Sprintf("ext-inbound--%s", external)
}

func extInboundRouteMapName(external string) string {
	return fmt.Sprintf("ext-inbound--%s", external)
}

func extOutboundPrefixList(external string) string {
	return fmt.Sprintf("ext-outbound--%s", external)
}

func extOutboundRouteMapName(external string) string {
	return fmt.Sprintf("ext-outbound--%s", external)
}

func ipNsExtCommsCommListName(ipns string) string {
	return fmt.Sprintf("ipns-ext-communities--%s", ipns)
}

func ipNsExternalCommsRouteMapName(ipns string) string {
	return fmt.Sprintf("ipns-ext-communities--%s", ipns)
}

func vpcExtImportVrfPrefixListName(vpc, ext string) string {
	return fmt.Sprintf("import-vrf--%s--%s", vpc, ext)
}

func vpcExtImportVrfRouteMapName(vpc string) string {
	return fmt.Sprintf("import-vrf--%s", vpc)
}

func vpcPeersCommListName(vpc string) string {
	return fmt.Sprintf("vpc-peers--%s", vpc)
}

func vpcPeersPrefixListName(vpc string) string {
	return fmt.Sprintf("vpc-peers--%s", vpc)
}

func vpcSubnetsPrefixListName(vpc string) string {
	return fmt.Sprintf("vpc-subnets--%s", vpc)
}

func vpcNotSubnetsPrefixListName(vpc string) string {
	return fmt.Sprintf("vpc-not-subnets--%s", vpc)
}

func vpcStaticExtSubnetsPrefixListName(vpc string) string {
	return fmt.Sprintf("vpc-static-ext-subnets--%s", vpc)
}

func vpcExtPrefixesPrefixListName(vpc string) string {
	return fmt.Sprintf("vpc-ext-prefixes--%s", vpc)
}

func ipnsEgressAccessList(ipns string) string {
	return fmt.Sprintf("ipns-egress--%s", ipns)
}

func vpcRedistributeConnectedRouteMapName(vpc string) string {
	return fmt.Sprintf("vpc-redistribute-connected--%s", vpc)
}

func vpcRedistributeStaticRouteMapName(vpc string) string {
	return fmt.Sprintf("vpc-redistribute-static--%s", vpc)
}

func ipnsSubnetsPrefixListName(ipns string) string {
	return fmt.Sprintf("ipns-subnets--%s", ipns)
}

func vpcFilteringAccessListName(vpc string, subnet string) string {
	return fmt.Sprintf("vpc-filtering--%s--%s", vpc, subnet)
}

func communityForVPC(agent *agentapi.Agent, vpc string) (string, error) {
	baseParts := strings.Split(agent.Spec.Config.BaseVPCCommunity, ":")
	if len(baseParts) != 2 {
		return "", errors.Errorf("invalid base VPC community %s", agent.Spec.Config.BaseVPCCommunity)
	}
	base, err := strconv.ParseUint(baseParts[1], 10, 16)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse base VPC community %s", agent.Spec.Config.BaseVPCCommunity)
	}

	vni, exists := agent.Spec.Catalog.VPCVNIs[vpc]
	if !exists {
		return "", errors.Errorf("VNI for VPC %s not found", vpc)
	}
	if vni%100 != 0 {
		return "", errors.Errorf("VNI for VPC %s is not a multiple of 100", vpc)
	}

	id := base + uint64(vni)/100
	if id >= 65535 {
		return "", errors.Errorf("VPC %s community id is too large", vpc)
	}

	return fmt.Sprintf("%s:%d", baseParts[0], id), nil
}

func planAllPortsUp(agent *agentapi.Agent, spec *dozer.Spec) error {
	if !agent.Spec.Switch.EnableAllPorts {
		return nil
	}

	ports, err := agent.Spec.SwitchProfile.GetAvailableAPIPorts(&agent.Spec.Switch)
	if err != nil {
		return errors.Wrapf(err, "failed to get available API ports for switch")
	}

	for port := range ports {
		if iface, exists := spec.Interfaces[port]; exists {
			if iface.Enabled != nil && *iface.Enabled {
				continue
			}
		} else {
			spec.Interfaces[port] = &dozer.SpecInterface{}
		}

		spec.Interfaces[port].Enabled = pointer.To(true)
		spec.Interfaces[port].Description = pointer.To("Unused")
		spec.Interfaces[port].Speed = getPortSpeed(agent, port)
	}

	return nil
}

func planPortAutoNegs(agent *agentapi.Agent, spec *dozer.Spec) error {
	autoNegAllowed, autoNegDefault, err := agent.Spec.SwitchProfile.GetAutoNegsDefaultsFor(&agent.Spec.Switch)
	if err != nil {
		return errors.Wrapf(err, "failed to get auto-negotiation settings for switch")
	}

	for name, iface := range spec.Interfaces {
		if !isHedgehogPortName(name) || strings.HasPrefix(name, wiringapi.ManagementPortPrefix) {
			continue
		}

		iface.AutoNegotiate = pointer.To(autoNegDefault[name])
	}

	for name, autoNeg := range agent.Spec.Switch.PortAutoNegs {
		if !isHedgehogPortName(name) || strings.HasPrefix(name, wiringapi.ManagementPortPrefix) || !autoNegAllowed[name] {
			continue
		}

		if iface, exists := spec.Interfaces[name]; exists {
			iface.AutoNegotiate = pointer.To(autoNeg)
		}
	}

	return nil
}

func translatePortNames(agent *agentapi.Agent, spec *dozer.Spec) error {
	sp := agent.Spec.SwitchProfile

	if sp == nil {
		return errors.Errorf("switch profile not found")
	}

	var err error

	ports, err := sp.GetAPI2NOSPortsFor(&agent.Spec.Switch)
	if err != nil {
		return errors.Wrapf(err, "failed to get NOS port mapping for switch")
	}

	newIfaces := map[string]*dozer.SpecInterface{}
	for name, iface := range spec.Interfaces {
		portName := name
		if isHedgehogPortName(name) {
			portName, err = getNOSPortName(ports, name)
			if err != nil {
				return errors.Wrapf(err, "failed to translate port name for spec interfaces %s", name)
			}
		}

		newIfaces[portName] = iface
	}
	spec.Interfaces = newIfaces

	newACLIfaces := map[string]*dozer.SpecACLInterface{}
	for name, iface := range spec.ACLInterfaces {
		portName := name
		if isHedgehogPortName(name) {
			portName, err = getNOSPortName(ports, name)
			if err != nil {
				return errors.Wrapf(err, "failed to translate port name for ACL interfaces %s", name)
			}
		}

		newACLIfaces[portName] = iface
	}
	spec.ACLInterfaces = newACLIfaces

	newLLDPIfaces := map[string]*dozer.SpecLLDPInterface{}
	for name, iface := range spec.LLDPInterfaces {
		portName := name
		if isHedgehogPortName(name) {
			portName, err = getNOSPortName(ports, name)
			if err != nil {
				return errors.Wrapf(err, "failed to translate port name for LLDP interfaces %s", name)
			}
		}

		newLLDPIfaces[portName] = iface
	}
	spec.LLDPInterfaces = newLLDPIfaces

	newPortGroups := map[string]*dozer.SpecPortGroup{}
	for name, group := range spec.PortGroups {
		groupProfile, exists := sp.PortGroups[name]
		if !exists {
			return errors.Errorf("port group %s not found in NOS port mapping", name)
		}

		newPortGroups[groupProfile.NOSName] = group
	}
	spec.PortGroups = newPortGroups

	newPortBreakouts := map[string]*dozer.SpecPortBreakout{}
	for name, breakout := range spec.PortBreakouts {
		port, exists := sp.Ports[name]
		if !exists {
			return errors.Errorf("port %s not found in NOS port mapping", name)
		}

		newPortBreakouts[port.NOSName] = breakout
	}
	spec.PortBreakouts = newPortBreakouts

	newLSTIfaces := map[string]*dozer.SpecLSTInterface{}
	for name, iface := range spec.LSTInterfaces {
		portName := name
		if isHedgehogPortName(name) {
			portName, err = getNOSPortName(ports, name)
			if err != nil {
				return errors.Wrapf(err, "failed to translate port name for LST interfaces %s", name)
			}
		}

		newLSTIfaces[portName] = iface
	}
	spec.LSTInterfaces = newLSTIfaces

	for vrfName, vrf := range spec.VRFs {
		newIfaces := map[string]*dozer.SpecVRFInterface{}
		for name, iface := range vrf.Interfaces {
			portName := name
			if isHedgehogPortName(name) {
				portName, err = getNOSPortName(ports, name)
				if err != nil {
					return errors.Wrapf(err, "failed to translate port name for VRF %s interfaces %s", vrfName, name)
				}
			}

			newIfaces[portName] = iface
		}
		vrf.Interfaces = newIfaces

		for routeName, route := range vrf.StaticRoutes {
			for idx, nextHop := range route.NextHops {
				if nextHop.Interface == nil {
					continue
				}

				iface := *nextHop.Interface
				if isHedgehogPortName(iface) {
					portName, err := getNOSPortName(ports, iface)
					if err != nil {
						return errors.Wrapf(err, "failed to translate port name %s for next hop of %s in vrf %s", iface, routeName, vrfName)
					}

					if strings.Contains(portName, ".") {
						portName = strings.ReplaceAll(portName, "Ethernet", "Eth")
					}

					route.NextHops[idx].Interface = &portName
				}
			}
		}
	}

	return nil
}

func getNOSPortName(ports map[string]string, name string) (string, error) {
	sub := ""
	if strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)
		name = parts[0]
		sub = "." + parts[1]
	}

	portName, exists := ports[name]
	if !exists {
		return "", errors.Errorf("port %s not found in NOS port mapping", name)
	}

	return portName + sub, nil
}

func isHedgehogPortName(name string) bool {
	return strings.HasPrefix(name, "M") || strings.HasPrefix(name, "E")
}
