package bcm

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/iputil"
)

const (
	MCLAG_DOMAIN_ID                            = 100
	MCLAG_PEER_LINK_PORT_CHANNEL_ID            = 250
	MCLAG_SESSION_LINK_PORT_CHANNEL_ID         = 251
	MCLAG_PEER_LINK_TRUNK_VLAN_RANGE           = "2..4094"    // TODO do we need to configure it?
	MCLAG_SESSION_IP_1                         = "172.30.5.0" // TODO move to config
	MCLAG_SESSION_IP_2                         = "172.30.5.1" // TODO move to config
	MCLAG_SESSION_IP_PREFIX_LEN                = 31           // TODO move to config
	AGENT_USER                                 = "hhagent"
	NAT_INSTANCE_ID                            = 0
	NAT_ZONE_EXTERNAL                          = 1
	NAT_ANCHOR_VLAN                     uint16 = 500
	VPC_ACL_ENTRY_SEQ_DHCP              uint32 = 10
	VPC_ACL_ENTRY_SEQ_SUBNET            uint32 = 20
	VPC_ACL_ENTRY_VLAN_SHIFT            uint32 = 10000
	VPC_ACL_ENTRY_DENY_ALL_VPC          uint32 = 30000
	VPC_ACL_ENTRY_PERMIT_ANY            uint32 = 40000
	LO_SWITCH                                  = "Loopback0"
	LO_PROTO                                   = "Loopback1"
	LO_VTEP                                    = "Loopback2"
	VRF_DEFAULT                                = "default"
	VTEP_FABRIC                                = "vtepfabric"
	EVPN_NVO                                   = "nvo1"
	ANYCAST_MAC                                = "00:00:00:11:11:11"
	VPC_VLAN_RANGE                             = "1000..1999" // TODO remove
	VPC_LO_PORT_CHANNEL_1                      = 252
	VPC_LO_PORT_CHANNEL_2                      = 253
	ROUTE_MAP_BLOCK_EVPN_DEFAULT_REMOTE        = "evpn-default-remote-block"
	ROUTE_MAP_MAX_STATEMENT                    = 65535
	PREFIX_LIST_ANY                            = "any-prefix"
)

func (p *broadcomProcessor) PlanDesiredState(ctx context.Context, agent *agentapi.Agent) (*dozer.Spec, error) {
	spec := &dozer.Spec{
		ZTP:             boolPtr(false),
		Hostname:        stringPtr(agent.Name),
		LLDP:            &dozer.SpecLLDP{},
		LLDPInterfaces:  map[string]*dozer.SpecLLDPInterface{},
		PortGroups:      map[string]*dozer.SpecPortGroup{},
		PortBreakouts:   map[string]*dozer.SpecPortBreakout{},
		Interfaces:      map[string]*dozer.SpecInterface{},
		MCLAGs:          map[uint32]*dozer.SpecMCLAGDomain{},
		MCLAGInterfaces: map[string]*dozer.SpecMCLAGInterface{},
		Users:           map[string]*dozer.SpecUser{},
		VRFs: map[string]*dozer.SpecVRF{
			VRF_DEFAULT: { // default VRF is always present
				Enabled:          boolPtr(true),
				Interfaces:       map[string]*dozer.SpecVRFInterface{},
				TableConnections: map[string]*dozer.SpecVRFTableConnection{},
				StaticRoutes:     map[string]*dozer.SpecVRFStaticRoute{},
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
	}

	for name, speed := range agent.Spec.Switch.PortGroupSpeeds {
		spec.PortGroups[name] = &dozer.SpecPortGroup{
			Speed: stringPtr(speed),
		}
	}

	for name, mode := range agent.Spec.Switch.PortBreakouts {
		spec.PortBreakouts[name] = &dozer.SpecPortBreakout{
			Mode: mode,
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

	err = planDefaultVRFWithBGP(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan basic BGP")
	}

	err = planFabricConnections(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan fabric connections")
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

	_ /* first */, err = planMCLAGDomain(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan mclag domain")
	}

	err = planVPCs(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan VPCs")
	}

	err = planExternalPeerings(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan external peerings")
	}

	spec.Normalize()

	return spec, nil
}

func planControlLink(agent *agentapi.Agent, spec *dozer.Spec) error {
	direct := false
	controlIface := ""
	controlIP := ""
	otherIP := ""
	for _, conn := range agent.Spec.Connections {
		if conn.Management != nil {
			direct = true
			controlIface = conn.Management.Link.Switch.LocalPortName()
			controlIP = conn.Management.Link.Switch.IP
			otherIP = conn.Management.Link.Server.IP
			break
		}
	}

	if !direct {
		return nil
	}

	if controlIface == "" {
		return errors.Errorf("no control interface found")
	}
	if controlIP == "" {
		return errors.Errorf("no control IP found")
	}
	if otherIP == "" {
		return errors.Errorf("no other IP found")
	}

	ip, ipNet, err := net.ParseCIDR(controlIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse control IP %s", controlIP)
	}
	prefixLen, _ := ipNet.Mask.Size()

	spec.Interfaces[controlIface] = &dozer.SpecInterface{
		Description: stringPtr("Control interface direct"),
		Enabled:     boolPtr(true),
		Speed:       getPortSpeed(agent, controlIface),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip.String(): {
						PrefixLen: uint8Ptr(uint8(prefixLen)),
					},
				},
			},
		},
	}

	if !strings.HasPrefix(controlIface, "Management") {
		ip, _, err = net.ParseCIDR(otherIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse other IP %s", otherIP)
		}

		controlVIP := agent.Spec.Config.ControlVIP
		spec.VRFs[VRF_DEFAULT].StaticRoutes[controlVIP] = &dozer.SpecVRFStaticRoute{
			NextHops: []dozer.SpecVRFStaticRouteNextHop{
				{
					IP:        ip.String(),
					Interface: stringPtr(controlIface),
				},
			},
		}
	}

	return nil
}

func planLLDP(agent *agentapi.Agent, spec *dozer.Spec) error {
	spec.LLDP = &dozer.SpecLLDP{
		Enabled:           boolPtr(true),
		HelloTimer:        uint64Ptr(5), // TODO make configurable?
		SystemName:        stringPtr(agent.Name),
		SystemDescription: stringPtr(fmt.Sprintf("Hedgehog: [control_vip=%s]", agent.Spec.Config.ControlVIP)),
	}

	if !agent.IsSpineLeaf() {
		return nil
	}

	for _, conn := range agent.Spec.Connections {
		if conn.Fabric != nil {
			for _, link := range conn.Fabric.Links {
				mgmtIP := ""
				iface := ""

				if link.Spine.DeviceName() == agent.Name {
					iface = link.Spine.LocalPortName()
					mgmtIP = link.Spine.IP
				} else if link.Leaf.DeviceName() == agent.Name {
					iface = link.Leaf.LocalPortName()
					mgmtIP = link.Leaf.IP
				}

				if mgmtIP != "" {
					parts := strings.Split(mgmtIP, "/")
					if len(parts) != 2 {
						return errors.Errorf("invalid lldp management ip %s", mgmtIP)
					}
					mgmtIP = parts[0]
				}

				if mgmtIP != "" && iface != "" {
					spec.LLDPInterfaces[iface] = &dozer.SpecLLDPInterface{
						Enabled:        boolPtr(true),
						ManagementIPv4: stringPtr(mgmtIP),
					}
				}
			}
		}
	}

	return nil
}

func planLoopbacks(agent *agentapi.Agent, spec *dozer.Spec) error {
	ip, ipNet, err := net.ParseCIDR(agent.Spec.Switch.IP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse switch ip %s", agent.Spec.Switch.IP)
	}
	ipPrefixLen, _ := ipNet.Mask.Size()

	spec.Interfaces[LO_SWITCH] = &dozer.SpecInterface{
		Enabled:     boolPtr(true),
		Description: stringPtr("Switch loopback"),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip.String(): {
						PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
					},
				},
			},
		},
	}

	ip, ipNet, err = net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
	}
	ipPrefixLen, _ = ipNet.Mask.Size()

	spec.Interfaces[LO_PROTO] = &dozer.SpecInterface{
		Enabled:     boolPtr(true),
		Description: stringPtr("Protocol loopback"),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip.String(): {
						PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
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

		spec.Interfaces[LO_VTEP] = &dozer.SpecInterface{
			Enabled:     boolPtr(true),
			Description: stringPtr("VTEP loopback"),
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{
				0: {
					IPs: map[string]*dozer.SpecInterfaceIP{
						ip.String(): {
							PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
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

	spec.RouteMaps[ROUTE_MAP_BLOCK_EVPN_DEFAULT_REMOTE] = &dozer.SpecRouteMap{
		Statements: map[string]*dozer.SpecRouteMapStatement{
			fmt.Sprintf("%d", ROUTE_MAP_MAX_STATEMENT): {
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
			if link.Spine.DeviceName() == agent.Name {
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
				Enabled:     boolPtr(true),
				Description: stringPtr(fmt.Sprintf("Fabric %s %s", remote, connName)),
				Speed:       getPortSpeed(agent, port),
				Subinterfaces: map[uint32]*dozer.SpecSubinterface{
					0: {
						IPs: map[string]*dozer.SpecInterfaceIP{
							ip.String(): {
								PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
							},
						},
					},
				},
			}

			if peerSw, ok := agent.Spec.Switches[peer]; !ok {
				return errors.Errorf("no switch found for peer %s (fabric conn %s)", peer, connName)
			} else {
				ip, _, err := net.ParseCIDR(peerIP)
				if err != nil {
					return errors.Wrapf(err, "failed to parse fabric conn peer ip %s", peerIP)
				}

				spec.VRFs[VRF_DEFAULT].BGP.Neighbors[ip.String()] = &dozer.SpecVRFBGPNeighbor{
					Enabled:                 boolPtr(true),
					Description:             stringPtr(fmt.Sprintf("Fabric %s %s", remote, connName)),
					RemoteAS:                uint32Ptr(peerSw.ASN),
					IPv4Unicast:             boolPtr(true),
					L2VPNEVPN:               boolPtr(true),
					L2VPNEVPNImportPolicies: []string{ROUTE_MAP_BLOCK_EVPN_DEFAULT_REMOTE},
				}
			}
		}
	}

	return nil
}

func planVPCLoopbacks(agent *agentapi.Agent, spec *dozer.Spec) error {
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
					Enabled:       boolPtr(true),
					Description:   stringPtr(fmt.Sprintf("VPC loopback %d.%d %s", linkID, portID, connName)),
					Speed:         getPortSpeed(agent, port),
					Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
				}
			}
		}
	}

	return nil
}

func planExternals(agent *agentapi.Agent, spec *dozer.Spec) error {
	spec.PrefixLists[PREFIX_LIST_ANY] = &dozer.SpecPrefixList{
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
			Enabled:       boolPtr(true),
			Description:   stringPtr(fmt.Sprintf("External %s", connName)),
			Speed:         getPortSpeed(agent, port),
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
		}
	}

	for externalName, external := range agent.Spec.Externals {
		ipnsVrfName := ipnsVrfName(external.IPv4Namespace)

		externalCommsCommList := externalCommsCommListName(external.IPv4Namespace)
		externalCommsRouteMap := externalCommsRouteMapName(external.IPv4Namespace)

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
						MatchCommunityList: stringPtr(externalCommsCommList),
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
				DestinationAddress: stringPtr(subnet),
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
				Enabled: boolPtr(true),
				// Description:      stringPtr(fmt.Sprintf("IPv4NS %s", external.IPv4Namespace)),
				AnycastMAC:       stringPtr(ANYCAST_MAC),
				Interfaces:       map[string]*dozer.SpecVRFInterface{},
				StaticRoutes:     map[string]*dozer.SpecVRFStaticRoute{},
				TableConnections: map[string]*dozer.SpecVRFTableConnection{},
				BGP: &dozer.SpecVRFBGP{
					AS:                 uint32Ptr(agent.Spec.Switch.ASN),
					RouterID:           stringPtr(protocolIP.String()),
					NetworkImportCheck: boolPtr(true),
					IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
						Enabled:    true,
						MaxPaths:   uint32Ptr(getMaxPaths(agent)),
						Networks:   map[string]*dozer.SpecVRFBGPNetwork{},
						ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
					},
					L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
						Enabled:            agent.IsSpineLeaf(),
						AdvertiseDefaultGw: boolPtr(true),
					},
					Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{},
				},
			}
		}

		commList := inboundCommListName(externalName)
		spec.CommunityLists[commList] = &dozer.SpecCommunityList{
			Members: []string{external.InboundCommunity},
		}

		spec.RouteMaps[inboundRouteMapName(externalName)] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchCommunityList: stringPtr(commList),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
				"100": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}

		prefList := outboundPrefixList(externalName)
		spec.PrefixLists[prefList] = &dozer.SpecPrefixList{
			Prefixes: map[uint32]*dozer.SpecPrefixListEntry{},
		}

		spec.RouteMaps[outboundRouteMapName(externalName)] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchPrefixList: stringPtr(prefList),
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
			vlan = uint16Ptr(uint16(attach.Switch.VLAN))
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
					PrefixLen: uint8Ptr(uint8(prefixLength)),
				},
			},
		}

		subIfaceName := fmt.Sprintf("%s.%d", port, attach.Switch.VLAN)

		ipns := external.IPv4Namespace
		ipnsVrfName := ipnsVrfName(ipns)
		spec.VRFs[ipnsVrfName].Interfaces[subIfaceName] = &dozer.SpecVRFInterface{}

		spec.VRFs[ipnsVrfName].BGP.Neighbors[attach.Neighbor.IP] = &dozer.SpecVRFBGPNeighbor{
			Enabled:                   boolPtr(true),
			Description:               stringPtr(fmt.Sprintf("External attach %s", name)),
			RemoteAS:                  uint32Ptr(attach.Neighbor.ASN),
			IPv4Unicast:               boolPtr(true),
			IPv4UnicastImportPolicies: []string{inboundRouteMapName(attach.External)},
			IPv4UnicastExportPolicies: []string{outboundRouteMapName(attach.External)},
		}

		spec.ACLInterfaces[subIfaceName] = &dozer.SpecACLInterface{
			Egress: stringPtr(ipnsEgressAccessList(ipns)),
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

		if conn.MCLAG != nil {
			connType = "MCLAG"
			if conn.MCLAG.MTU != 0 {
				mtu = uint16Ptr(conn.MCLAG.MTU)
			}
			links = conn.MCLAG.Links
		} else if conn.Bundled != nil {
			connType = "Bundled"
			// TODO MTU
			links = conn.Bundled.Links
		} else {
			continue
		}

		for _, link := range links {
			if link.Switch.DeviceName() != agent.Name {
				continue
			}

			portName := link.Switch.LocalPortName()
			portChan := agent.Spec.PortChannels[connName]

			if portChan == 0 {
				return errors.Errorf("no port channel found for conn %s", connName)
			}

			connPortChannelName := portChannelName(portChan)
			connPortChannel := &dozer.SpecInterface{
				Enabled:     boolPtr(true),
				Description: stringPtr(fmt.Sprintf("%s %s %s", connType, link.Server.DeviceName(), connName)),
				TrunkVLANs:  []string{VPC_VLAN_RANGE}, // TODO change
				MTU:         mtu,
			}
			spec.Interfaces[connPortChannelName] = connPortChannel

			if connType == "MCLAG" {
				spec.MCLAGInterfaces[connPortChannelName] = &dozer.SpecMCLAGInterface{
					DomainID: MCLAG_DOMAIN_ID,
				}
			}

			descr := fmt.Sprintf("PC%d %s %s %s", portChan, connType, link.Server.DeviceName(), connName)
			err := setupPhysicalInterfaceWithPortChannel(spec, portName, descr, connPortChannelName, nil, agent)
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

		if conn.Unbundled.Link.Switch.DeviceName() != agent.Name {
			continue
		}

		swPort := conn.Unbundled.Link.Switch

		spec.Interfaces[swPort.LocalPortName()] = &dozer.SpecInterface{
			Enabled:     boolPtr(true),
			Description: stringPtr(fmt.Sprintf("Unbundled %s %s", conn.Unbundled.Link.Server.DeviceName(), connName)),
			Speed:       getPortSpeed(agent, swPort.LocalPortName()),
			TrunkVLANs:  []string{VPC_VLAN_RANGE},
			// MTU:         mtu,
		}
	}

	return nil
}

func planDefaultVRFWithBGP(agent *agentapi.Agent, spec *dozer.Spec) error {
	ip, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
	}

	spec.VRFs[VRF_DEFAULT].AnycastMAC = stringPtr(ANYCAST_MAC)
	spec.VRFs[VRF_DEFAULT].BGP = &dozer.SpecVRFBGP{
		AS:                 uint32Ptr(agent.Spec.Switch.ASN),
		RouterID:           stringPtr(ip.String()),
		NetworkImportCheck: boolPtr(true), // default
		Neighbors:          map[string]*dozer.SpecVRFBGPNeighbor{},
		IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
			Enabled:  true,
			MaxPaths: uint32Ptr(getMaxPaths(agent)),
		},
		L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
			Enabled:         agent.IsSpineLeaf(),
			AdvertiseAllVNI: boolPtr(true),
		},
	}
	spec.VRFs[VRF_DEFAULT].TableConnections = map[string]*dozer.SpecVRFTableConnection{
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
		VTEP_FABRIC: {
			SourceIP:        stringPtr(ip.String()),
			SourceInterface: stringPtr(LO_VTEP),
		},
	}

	spec.VXLANEVPNNVOs = map[string]*dozer.SpecVXLANEVPNNVO{
		EVPN_NVO: {
			SourceVTEP: stringPtr(VTEP_FABRIC),
		},
	}

	return nil
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

	// using the same IP pair with switch with name < peer switch name getting first IP
	sourceIP := MCLAG_SESSION_IP_1
	peerIP := MCLAG_SESSION_IP_2
	if agent.Name > mclagPeerSwitch {
		sourceIP, peerIP = peerIP, sourceIP
	}

	mclagPeerPortChannelName := portChannelName(MCLAG_PEER_LINK_PORT_CHANNEL_ID)
	mclagPeerPortChannel := &dozer.SpecInterface{
		Description: stringPtr(fmt.Sprintf("MCLAG peer %s", mclagPeerSwitch)),
		Enabled:     boolPtr(true),
		TrunkVLANs:  []string{MCLAG_PEER_LINK_TRUNK_VLAN_RANGE},
	}
	spec.Interfaces[mclagPeerPortChannelName] = mclagPeerPortChannel
	for iface, peerPort := range mclagPeerLinks {
		descr := fmt.Sprintf("PC%d MCLAG peer %s", MCLAG_PEER_LINK_PORT_CHANNEL_ID, peerPort)
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagPeerPortChannelName, nil, agent)
		if err != nil {
			return false, errors.Wrapf(err, "failed to setup physical interface %s for MCLAG peer link", iface)
		}
	}

	mclagSessionPortChannelName := portChannelName(MCLAG_SESSION_LINK_PORT_CHANNEL_ID)
	mclagSessionPortChannel := &dozer.SpecInterface{
		Description: stringPtr(fmt.Sprintf("MCLAG session %s", mclagPeerSwitch)),
		Enabled:     boolPtr(true),
		Subinterfaces: map[uint32]*dozer.SpecSubinterface{
			0: {
				IPs: map[string]*dozer.SpecInterfaceIP{
					sourceIP: {
						PrefixLen: uint8Ptr(MCLAG_SESSION_IP_PREFIX_LEN),
					},
				},
			},
		},
	}
	spec.Interfaces[mclagSessionPortChannelName] = mclagSessionPortChannel
	for iface, peerPort := range mclagSessionLinks {
		descr := fmt.Sprintf("PC%d MCLAG session %s", MCLAG_SESSION_LINK_PORT_CHANNEL_ID, peerPort)
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagSessionPortChannelName, nil, agent)
		if err != nil {
			return false, errors.Wrapf(err, "failed to setup physical interface %s for MCLAG session link", iface)
		}
	}

	spec.MCLAGs[MCLAG_DOMAIN_ID] = &dozer.SpecMCLAGDomain{
		SourceIP: sourceIP,
		PeerIP:   peerIP,
		PeerLink: mclagPeerPortChannelName,
	}

	spec.VRFs[VRF_DEFAULT].BGP.Neighbors[peerIP] = &dozer.SpecVRFBGPNeighbor{
		Enabled:     boolPtr(true),
		Description: stringPtr(fmt.Sprintf("MCLAG session %s", mclagPeerSwitch)),
		PeerType:    stringPtr(dozer.SpecVRFBGPNeighborPeerTypeInternal),
		IPv4Unicast: boolPtr(true),
	}

	return sourceIP == MCLAG_SESSION_IP_1, nil
}

func planUsers(agent *agentapi.Agent, spec *dozer.Spec) error {
	for _, user := range agent.Spec.Users {
		if user.Name == AGENT_USER {
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

func planVPCs(agent *agentapi.Agent, spec *dozer.Spec) error {
	for vpcName := range agent.Spec.VPCs {
		vrfName := vpcVrfName(vpcName)

		irbVLAN := agent.Spec.IRBVLANs[vpcName]
		if irbVLAN == 0 {
			return errors.Errorf("IRB VLAN for VPC %s not found", vpcName)
		}

		irbIface := vlanName(irbVLAN)
		spec.Interfaces[irbIface] = &dozer.SpecInterface{
			Enabled:     boolPtr(true),
			Description: stringPtr(fmt.Sprintf("VPC %s IRB", vpcName)),
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

		vpcPeersCommList := vpcPeersCommListName(vpcName)
		spec.CommunityLists[vpcPeersCommList] = &dozer.SpecCommunityList{
			Members: []string{},
		}

		importVrfRouteMap := importVrfRouteMapName(vpcName)
		if _, exists := spec.RouteMaps[importVrfRouteMap]; !exists {
			spec.RouteMaps[importVrfRouteMap] = &dozer.SpecRouteMap{
				Statements: map[string]*dozer.SpecRouteMapStatement{
					"1": {
						Conditions: dozer.SpecRouteMapConditions{
							MatchCommunityList: stringPtr(vpcPeersCommList),
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

		stampVPCRouteMap := stampVPCRouteMapName(vpcName)
		spec.RouteMaps[stampVPCRouteMap] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					SetCommunities: []string{vpcComm},
					Result:         dozer.SpecRouteMapResultAccept,
				},
			},
		}

		protocolIP, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
		}

		spec.VRFs[vrfName].Enabled = boolPtr(true)
		spec.VRFs[vrfName].AnycastMAC = stringPtr(ANYCAST_MAC)
		spec.VRFs[vrfName].BGP = &dozer.SpecVRFBGP{
			AS:                 uint32Ptr(agent.Spec.Switch.ASN),
			RouterID:           stringPtr(protocolIP.String()),
			NetworkImportCheck: boolPtr(true),
			IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
				Enabled:      true,
				MaxPaths:     uint32Ptr(getMaxPaths(agent)),
				ImportPolicy: stringPtr(importVrfRouteMap),
				ImportVRFs:   map[string]*dozer.SpecVRFBGPImportVRF{},
			},
			L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
				Enabled:              agent.IsSpineLeaf(),
				AdvertiseIPv4Unicast: boolPtr(true),
			},
		}
		spec.VRFs[vrfName].TableConnections = map[string]*dozer.SpecVRFTableConnection{
			dozer.SpecVRFBGPTableConnectionConnected: {
				ImportPolicies: []string{stampVPCRouteMap},
			},
			dozer.SpecVRFBGPTableConnectionStatic: {},
		}
		spec.VRFs[vrfName].Interfaces[irbIface] = &dozer.SpecVRFInterface{}

		if agent.IsSpineLeaf() {
			spec.SuppressVLANNeighs[irbIface] = &dozer.SpecSuppressVLANNeigh{}

			vpcVNI := agent.Spec.VNIs[vpcName]
			if vpcVNI == 0 {
				return errors.Errorf("VNI for VPC %s not found", vpcName)
			}
			spec.VRFVNIMap[vrfName] = &dozer.SpecVRFVNIEntry{
				VNI: uint32Ptr(vpcVNI),
			}
			spec.VXLANTunnelMap[fmt.Sprintf("map_%d_%s", vpcVNI, irbIface)] = &dozer.SpecVXLANTunnelMap{
				VTEP: stringPtr(VTEP_FABRIC),
				VNI:  uint32Ptr(vpcVNI),
				VLAN: uint16Ptr(irbVLAN),
			}
		}
	}

	attachedVPCs := map[string]bool{}
	for _, attach := range agent.Spec.VPCAttachments {
		vpcName := attach.VPCName()
		vpc, exists := agent.Spec.VPCs[vpcName]
		if !exists {
			return errors.Errorf("VPC %s not found", vpcName)
		}

		attachedVPCs[vpcName] = true

		subnetName := attach.SubnetName()
		subnet := vpc.Subnets[subnetName]
		if subnet == nil {
			return errors.Errorf("VPC %s subnet %s not found", vpcName, subnetName)
		}

		err := planVPCSubnet(agent, spec, vpcName, subnetName, subnet)
		if err != nil {
			return errors.Wrapf(err, "failed to plan VPC %s subnet %s", vpcName, subnetName)
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

		err := planVPCSubnet(agent, spec, vpcName, subnetName, subnet)
		if err != nil {
			return errors.Wrapf(err, "failed to plan VPC %s subnet %s for configuredSubnets", vpcName, subnetName)
		}
	}

	for peeringName, peering := range agent.Spec.VPCPeerings {
		vpc1Name, vpc2Name, err := peering.VPCs()
		if err != nil {
			return errors.Wrapf(err, "failed to parse VPCs for VPC peering %s", peeringName)
		}

		_, exists := agent.Spec.VPCs[vpc1Name]
		if !exists {
			return errors.Errorf("VPC %s not found for VPC peering %s", vpc1Name, peeringName)
		}
		_, exists = agent.Spec.VPCs[vpc2Name]
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

		vrf1Name := vpcVrfName(vpc1Name)
		vrf2Name := vpcVrfName(vpc2Name)

		vpc1Attached := attachedVPCs[vpc1Name] || agent.Spec.MCLAGAttachedVPCs[vpc1Name]
		vpc2Attached := attachedVPCs[vpc2Name] || agent.Spec.MCLAGAttachedVPCs[vpc2Name]

		if !vpc1Attached || !vpc2Attached {
			spec.VRFs[vrf1Name].BGP.IPv4Unicast.ImportVRFs[vrf2Name] = &dozer.SpecVRFBGPImportVRF{}
			spec.VRFs[vrf2Name].BGP.IPv4Unicast.ImportVRFs[vrf1Name] = &dozer.SpecVRFBGPImportVRF{}

			// remote
			if !vpc1Attached && !vpc2Attached {
				spec.VRFs[vrf1Name].BGP.L2VPNEVPN.DefaultOriginateIPv4 = boolPtr(true)
				spec.VRFs[vrf2Name].BGP.L2VPNEVPN.DefaultOriginateIPv4 = boolPtr(true)

				// TODO dedup

				vni1 := agent.Spec.VNIs[vpc1Name]
				if vni1 == 0 {
					return errors.Errorf("VNI for VPC %s not found", vpc1Name)
				}
				if vni1%100 != 0 {
					return errors.Errorf("VNI for VPC %s is not a multiple of 100", vpc1Name)
				}
				if vni1/100 >= ROUTE_MAP_MAX_STATEMENT {
					return errors.Errorf("VNI for VPC %s is too large", vpc1Name)
				}
				vni2 := agent.Spec.VNIs[vpc2Name]
				if vni2 == 0 {
					return errors.Errorf("VNI for VPC %s not found", vpc2Name)
				}
				if vni2%100 != 0 {
					return errors.Errorf("VNI for VPC %s is not a multiple of 100", vpc2Name)
				}
				if vni2/100 >= ROUTE_MAP_MAX_STATEMENT {
					return errors.Errorf("VNI for VPC %s is too large", vpc2Name)
				}

				spec.RouteMaps[ROUTE_MAP_BLOCK_EVPN_DEFAULT_REMOTE].Statements[fmt.Sprintf("%d", uint(vni1/100))] = &dozer.SpecRouteMapStatement{
					Conditions: dozer.SpecRouteMapConditions{
						MatchEVPNVNI:          uint32Ptr(vni1),
						MatchEVPNDefaultRoute: boolPtr(true),
					},
					Result: dozer.SpecRouteMapResultReject,
				}
				spec.RouteMaps[ROUTE_MAP_BLOCK_EVPN_DEFAULT_REMOTE].Statements[fmt.Sprintf("%d", uint(vni2/100))] = &dozer.SpecRouteMapStatement{
					Conditions: dozer.SpecRouteMapConditions{
						MatchEVPNVNI:          uint32Ptr(vni2),
						MatchEVPNDefaultRoute: boolPtr(true),
					},
					Result: dozer.SpecRouteMapResultReject,
				}
			}
		} else if peering.Remote == "" {
			vlan := agent.Spec.VPCLoopbackVLANs[peeringName]
			if vlan == 0 {
				return errors.Errorf("workaround VLAN for VPC peering %s not found", peeringName)
			}

			link := agent.Spec.VPCLoopbackLinks[peeringName]
			if link == "" {
				return errors.Errorf("workaround link for VPC peering %s not found", peeringName)
			}

			ports := strings.Split(link, "--")
			if len(ports) != 2 {
				return errors.Errorf("workaround link for VPC peering %s is invalid", peeringName)
			}
			if spec.Interfaces[ports[0]] == nil {
				return errors.Errorf("workaround link port %s for VPC peering %s not found", ports[0], peeringName)
			}
			if spec.Interfaces[ports[1]] == nil {
				return errors.Errorf("workaround link port %s for VPC peering %s not found", ports[1], peeringName)
			}

			ip1, ip2, err := vpcWorkaroundIPs("172.30.224.0/19", vlan) // TODO move to config
			if err != nil {
				return errors.Wrapf(err, "failed to get workaround IPs for VPC peering")
			}

			spec.Interfaces[ports[0]].Subinterfaces[uint32(vlan)] = &dozer.SpecSubinterface{
				VLAN: &vlan,
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip1: {
						PrefixLen: uint8Ptr(31),
					},
				},
			}

			spec.Interfaces[ports[1]].Subinterfaces[uint32(vlan)] = &dozer.SpecSubinterface{
				VLAN:            &vlan,
				AnycastGateways: []string{ip2 + "/31"},
			}

			sub1 := fmt.Sprintf("%s.%d", ports[0], vlan)
			sub2 := fmt.Sprintf("%s.%d", ports[1], vlan)

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
							Interface: stringPtr(strings.ReplaceAll(sub2, "Ethernet", "Eth")),
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
							Interface: stringPtr(strings.ReplaceAll(sub1, "Ethernet", "Eth")),
						},
					},
				}
			}
		}
	}

	return nil
}

func planVPCSubnet(agent *agentapi.Agent, spec *dozer.Spec, vpcName, subnetName string, subnet *vpcapi.VPCSubnet) error {
	vrfName := vpcVrfName(vpcName)

	vlanRaw, err := strconv.ParseUint(subnet.VLAN, 10, 16)
	if err != nil {
		return errors.Wrapf(err, "failed to parse subnet VLAN %s for VPC %s", subnet.VLAN, vpcName)
	}
	subnetVLAN := uint16(vlanRaw)

	subnetCIDR, err := iputil.ParseCIDR(subnet.Subnet)
	if err != nil {
		return errors.Wrapf(err, "failed to parse subnet %s for VPC %s", subnet.Subnet, vpcName)
	}
	prefixLen, _ := subnetCIDR.Subnet.Mask.Size()

	subnetIface := vlanName(subnetVLAN)
	spec.Interfaces[subnetIface] = &dozer.SpecInterface{
		Enabled:     boolPtr(true),
		Description: stringPtr(fmt.Sprintf("VPC %s/%s", vpcName, subnetName)),
		VLANAnycastGateway: []string{
			fmt.Sprintf("%s/%d", subnetCIDR.Gateway.String(), prefixLen),
		},
	}

	spec.VRFs[vrfName].Interfaces[subnetIface] = &dozer.SpecVRFInterface{}

	if agent.IsSpineLeaf() {
		spec.SuppressVLANNeighs[subnetIface] = &dozer.SpecSuppressVLANNeigh{}

		subnetVNI := agent.Spec.VNIs[fmt.Sprintf("%s/%s", vpcName, subnetName)]
		if subnetVNI == 0 {
			return errors.Errorf("VNI for VPC %s subnet %s not found", vpcName, subnetName)
		}
		spec.VXLANTunnelMap[fmt.Sprintf("map_%d_%s", subnetVNI, subnetIface)] = &dozer.SpecVXLANTunnelMap{
			VTEP: stringPtr(VTEP_FABRIC),
			VNI:  uint32Ptr(subnetVNI),
			VLAN: uint16Ptr(subnetVLAN),
		}
	}

	if subnet.DHCP.Enable {
		dhcpRelayIP, _, err := net.ParseCIDR(agent.Spec.Config.ControlVIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse DHCP relay %s (control vip) for vpc %s", agent.Spec.Config.ControlVIP, vpcName)
		}

		spec.DHCPRelays[subnetIface] = &dozer.SpecDHCPRelay{
			SourceInterface: stringPtr(LO_SWITCH),
			RelayAddress:    []string{dhcpRelayIP.String()},
			LinkSelect:      true,
			VRFSelect:       true,
		}
	}

	return nil
}

func planExternalPeerings(agent *agentapi.Agent, spec *dozer.Spec) error {
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

			vni, exists := agent.Spec.VNIs[fmt.Sprintf("%s/%s", vpcName, subnetName)]
			if !exists {
				return errors.Errorf("VNI for VPC %s subnet %s not found for external peering %s", vpcName, subnetName, name)
			}

			spec.PrefixLists[outboundPrefixList(externalName)].Prefixes[vni] = &dozer.SpecPrefixListEntry{
				Prefix: dozer.SpecPrefixListPrefix{
					Prefix: subnet.Subnet,
				},
				Action: dozer.SpecPrefixListActionPermit,
			}
		}

		// Start of Import VRF business
		{
			prefixes := map[uint32]*dozer.SpecPrefixListEntry{}
			for _, prefix := range peering.Permit.External.Prefixes {
				idx := agent.Spec.ExternalPeeringPrefixIDs[prefix.Prefix]
				if idx == 0 {
					return errors.Errorf("no external peering prefix id for prefix %s in peering %s", prefix.Prefix, name)
				}
				if idx >= 65000 {
					return errors.Errorf("external peering prefix id for prefix %s in peering %s is too large", prefix.Prefix, name)
				}

				prefixes[idx] = &dozer.SpecPrefixListEntry{
					Prefix: dozer.SpecPrefixListPrefix{
						Prefix: prefix.Prefix,
						Ge:     prefix.Ge,
						Le:     prefix.Le,
					},
					Action: dozer.SpecPrefixListActionPermit,
				}
			}

			importVrfPrefixList := importVrfPrefixListName(vpcName, externalName)
			spec.PrefixLists[importVrfPrefixList] = &dozer.SpecPrefixList{
				Prefixes: prefixes,
			}

			importVrfRouteMap := importVrfRouteMapName(vpcName)
			idx := agent.Spec.ExternalSeqs[externalName]
			if idx == 0 {
				return errors.Errorf("no external seq for external %s", externalName)
			}
			if idx < 10 {
				return errors.Errorf("external seq for external %s is too small", externalName)
			}
			if idx >= 30000 {
				return errors.Errorf("external seq for external %s is too large", externalName)
			}
			spec.RouteMaps[importVrfRouteMap].Statements[fmt.Sprintf("%d", idx)] = &dozer.SpecRouteMapStatement{
				Conditions: dozer.SpecRouteMapConditions{
					MatchCommunityList: stringPtr(inboundCommListName(externalName)),
					MatchPrefixList:    stringPtr(importVrfPrefixList),
				},
				SetLocalPreference: uint32Ptr(500),
				Result:             dozer.SpecRouteMapResultAccept,
			}
			spec.RouteMaps[importVrfRouteMap].Statements[fmt.Sprintf("%d", idx+30000)] = &dozer.SpecRouteMapStatement{
				Conditions: dozer.SpecRouteMapConditions{
					MatchCommunityList: stringPtr(inboundCommListName(externalName)),
					MatchPrefixList:    stringPtr(PREFIX_LIST_ANY),
				},
				Result: dozer.SpecRouteMapResultReject,
			}

			ipnsVrf := ipnsVrfName(external.IPv4Namespace)
			vpcVrf := vpcVrfName(vpcName)

			spec.VRFs[ipnsVrf].BGP.IPv4Unicast.ImportVRFs[vpcVrf] = &dozer.SpecVRFBGPImportVRF{}
			spec.VRFs[vpcVrf].BGP.IPv4Unicast.ImportVRFs[ipnsVrf] = &dozer.SpecVRFBGPImportVRF{}
		}

		// TODO handle case if vpc is local
	}

	return nil
}

func getPortSpeed(agent *agentapi.Agent, port string) *string {
	if agent.Spec.Switch.PortSpeeds == nil {
		return nil
	}

	if speed, exists := agent.Spec.Switch.PortSpeeds[port]; exists {
		return &speed
	}

	return nil
}

func getMaxPaths(agent *agentapi.Agent) uint32 {
	if agent.Spec.IsVS() || agent.Status.NOSInfo.HwskuVersion == "Accton-AS4630-54NPE" { // TODO move to SwitchProfile
		return 16
	}

	return 64
}

// TODO test
func vpcWorkaroundIPs(subnet string, vlan uint16) (string, string, error) {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", "", err
	}
	prefixLen, _ := ipNet.Mask.Size()
	if prefixLen < 19 {
		return "", "", errors.Errorf("subnet should be at least /19")
	}
	ip := ipNet.IP.To4()
	ip[2] += byte(vlan / 128)
	ip[3] += byte(vlan % 128 * 2)

	res1 := ip.String()
	ip[3] += 1
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
		Description: stringPtr(description),
		Enabled:     boolPtr(true),
		Speed:       getPortSpeed(agent, name),
		PortChannel: &portChannel,
		MTU:         mtu,
	}
	spec.Interfaces[name] = physicalIface

	return nil
}

func inboundCommListName(external string) string {
	return fmt.Sprintf("ext-inbound--%s", external)
}

func inboundRouteMapName(external string) string {
	return fmt.Sprintf("ext-inbound--%s", external)
}

func outboundPrefixList(external string) string {
	return fmt.Sprintf("ext-outbound--%s", external)
}

func outboundRouteMapName(external string) string {
	return fmt.Sprintf("ext-outbound--%s", external)
}

func externalCommsCommListName(ipns string) string {
	return fmt.Sprintf("ipns-ext-communities--%s", ipns)
}

func externalCommsRouteMapName(ipns string) string {
	return fmt.Sprintf("ipns-ext-communities--%s", ipns)
}

func importVrfPrefixListName(vpc, ext string) string {
	return fmt.Sprintf("import-vrf--%s--%s", vpc, ext)
}

func importVrfRouteMapName(vpc string) string {
	return fmt.Sprintf("import-vrf--%s", vpc)
}

func vpcPeersCommListName(vpc string) string {
	return fmt.Sprintf("vpc-peers--%s", vpc)
}

func ipnsEgressAccessList(ipns string) string {
	return fmt.Sprintf("ipns-egress--%s", ipns)
}

func stampVPCRouteMapName(vpc string) string {
	return fmt.Sprintf("stamp-vpc--%s", vpc)
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

	vni, exists := agent.Spec.VNIs[vpc]
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

func stringPtr(s string) *string { return &s }

func uint8Ptr(u uint8) *uint8 { return &u }

func uint16Ptr(u uint16) *uint16 { return &u }

func uint32Ptr(u uint32) *uint32 { return &u }

func uint64Ptr(u uint64) *uint64 { return &u }

func boolPtr(b bool) *bool { return &b }
