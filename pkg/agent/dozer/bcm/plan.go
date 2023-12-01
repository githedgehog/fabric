package bcm

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/iputil"
)

const (
	MCLAG_DOMAIN_ID                           = 100
	MCLAG_PEER_LINK_PORT_CHANNEL_ID           = 250
	MCLAG_SESSION_LINK_PORT_CHANNEL_ID        = 251
	MCLAG_PEER_LINK_TRUNK_VLAN_RANGE          = "2..4094"    // TODO do we need to configure it?
	MCLAG_SESSION_IP_1                        = "172.30.5.0" // TODO move to config
	MCLAG_SESSION_IP_2                        = "172.30.5.1" // TODO move to config
	MCLAG_SESSION_IP_PREFIX_LEN               = 31           // TODO move to config
	AGENT_USER                                = "hhagent"
	NAT_INSTANCE_ID                           = 0
	NAT_ZONE_EXTERNAL                         = 1
	NAT_ANCHOR_VLAN                    uint16 = 500
	VPC_ACL_ENTRY_SEQ_DHCP             uint32 = 10
	VPC_ACL_ENTRY_SEQ_SUBNET           uint32 = 20
	VPC_ACL_ENTRY_VLAN_SHIFT           uint32 = 10000
	VPC_ACL_ENTRY_DENY_ALL_VPC         uint32 = 30000
	VPC_ACL_ENTRY_PERMIT_ANY           uint32 = 40000
	LO_SWITCH                                 = "Loopback0"
	LO_PROTO                                  = "Loopback1"
	LO_VTEP                                   = "Loopback2"
	VRF_DEFAULT                               = "default"
	VTEP_FABRIC                               = "vtepfabric"
	EVPN_NVO                                  = "nvo1"
	ANYCAST_MAC                               = "00:00:00:11:11:11"
	VPC_VLAN_RANGE                            = "1000..1999" // TODO remove
	VPC_LO_PORT_CHANNEL_1                     = 252
	VPC_LO_PORT_CHANNEL_2                     = 253
	ROUTE_MAP_DISALLOW_DIRECT                 = "disallow-direct"
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

	// TODO only for spine-leaf
	err = planFabricConnections(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan fabric connections")
	}

	err = planVPCLoopbacks(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan VPC loopbacks")
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
		return nil, errors.Wrap(err, "failed to plan Spine Leaf VPCs")
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

	if agent.Spec.Switch.Role.IsLeaf() {
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
					Enabled:     boolPtr(true),
					Description: stringPtr(fmt.Sprintf("Fabric %s %s", remote, connName)),
					RemoteAS:    uint32Ptr(peerSw.ASN),
					IPv4Unicast: boolPtr(true),
					L2VPNEVPN:   boolPtr(true),
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
					Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
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
			err := setupPhysicalInterfaceWithPortChannel(spec, portName, descr, connPortChannelName, nil)
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

	maxPaths := uint32(64)
	if agent.Spec.IsVS() {
		maxPaths = 16
	}

	spec.VRFs[VRF_DEFAULT].AnycastMAC = stringPtr(ANYCAST_MAC)
	spec.VRFs[VRF_DEFAULT].BGP = &dozer.SpecVRFBGP{
		AS:                 uint32Ptr(agent.Spec.Switch.ASN),
		RouterID:           stringPtr(ip.String()),
		NetworkImportCheck: boolPtr(true), // default
		Neighbors:          map[string]*dozer.SpecVRFBGPNeighbor{},
		IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
			Enabled:  true,
			MaxPaths: uint32Ptr(maxPaths),
		},
		L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
			Enabled:         true,
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
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagPeerPortChannelName, nil)
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
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagSessionPortChannelName, nil)
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

// func planCollapsedCoreVPCs(agent *agentapi.Agent, spec *dozer.Spec, controlIface string, firstSwitch bool) error {
// 	if !isACLBackend(agent) && !isVRFBackend(agent) {
// 		return errors.Errorf("unknown VPC backend %s", agent.Spec.Config.CollapsedCore.VPCBackend)
// 	}

// 	if isVRFBackend(agent) {
// 		// TODO switch to policy per VPC
// 		spec.RouteMaps[ROUTE_MAP_VPC_NO_ADVERTISE] = &dozer.SpecRouteMap{
// 			NoAdvertise: boolPtr(true),
// 		}
// 	}

// 	for _, vpc := range agent.Spec.VPCs {
// 		cidr, err := iputil.ParseCIDR(vpc.VPC.Subnet)
// 		if err != nil {
// 			return errors.Wrapf(err, "failed to parse subnet %s for vpc %s", vpc.VPC.Subnet, vpc.Name)
// 		}
// 		ip := cidr.Gateway.String()
// 		prefixLen, _ := cidr.Subnet.Mask.Size()

// 		descr := fmt.Sprintf("VPC %s", vpc.Name)
// 		vlanIfaceName, _, err := setupVLANInterfaceWithIP(spec, vpc.VLAN, ip, uint8(prefixLen), descr)
// 		if err != nil {
// 			return errors.Wrapf(err, "failed to setup VLAN interface for vpc %s", vpc.Name)
// 		}

// 		if isACLBackend(agent) {
// 			acl := &dozer.SpecACL{
// 				Description: stringPtr(fmt.Sprintf("VPC %s ACL IN (VLAN %d)", vpc.Name, vpc.VLAN)),
// 				Entries: map[uint32]*dozer.SpecACLEntry{
// 					VPC_ACL_ENTRY_SEQ_SUBNET: {
// 						Description:        stringPtr("Allow own subnet"),
// 						Action:             dozer.SpecACLEntryActionAccept,
// 						DestinationAddress: stringPtr(vpc.VPC.Subnet),
// 					},
// 					VPC_ACL_ENTRY_DENY_ALL_VPC: {
// 						Description:        stringPtr("Deny all other VPCs"),
// 						Action:             dozer.SpecACLEntryActionDrop,
// 						DestinationAddress: stringPtr(VPC_DENY_ALL_SUBNET),
// 					},
// 				},
// 			}

// 			if vpc.VPC.DHCP.Enable {
// 				acl.Entries[VPC_ACL_ENTRY_SEQ_DHCP] = &dozer.SpecACLEntry{
// 					Description:     stringPtr("Allow DHCP"),
// 					Action:          dozer.SpecACLEntryActionAccept,
// 					Protocol:        dozer.SpecACLEntryProtocolUDP,
// 					SourcePort:      uint16Ptr(68),
// 					DestinationPort: uint16Ptr(67),
// 				}
// 			}

// 			if agent.Spec.Config.CollapsedCore.SNATAllowed && vpc.VPC.SNAT || len(filteredDNAT(vpc.DNAT)) > 0 {
// 				acl.Entries[VPC_ACL_ENTRY_PERMIT_ANY] = &dozer.SpecACLEntry{
// 					Description:   stringPtr("Allow any traffic (NAT)"),
// 					Action:        dozer.SpecACLEntryActionAccept,
// 					SourceAddress: stringPtr(vpc.VPC.Subnet),
// 				}
// 			}

// 			aclName := aclName(vpc.VLAN)
// 			spec.ACLs[aclName] = acl
// 			spec.ACLInterfaces[vlanIfaceName] = &dozer.SpecACLInterface{
// 				Ingress: stringPtr(aclName),
// 			}
// 		} else if isVRFBackend(agent) {
// 			vrfName := vpcVrfName(vpc.Name)

// 			spec.VRFs[vrfName] = &dozer.SpecVRF{
// 				Enabled: boolPtr(true),
// 				// Description: stringPtr(fmt.Sprintf("VPC %s", vpc.Name)),
// 				Interfaces: map[string]*dozer.SpecVRFInterface{
// 					vlanIfaceName: {},
// 				},
// 				BGP: &dozer.SpecVRFBGP{
// 					AS:                 uint32Ptr(agent.Spec.Switch.ASN),
// 					NetworkImportCheck: boolPtr(true),
// 					IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
// 						Enabled:    true,
// 						ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
// 						Networks:   map[string]*dozer.SpecVRFBGPNetwork{},
// 					},
// 				},
// 				TableConnections: map[string]*dozer.SpecVRFTableConnection{
// 					dozer.SpecVRFBGPTableConnectionConnected: {
// 						ImportPolicies: []string{ROUTE_MAP_VPC_NO_ADVERTISE},
// 					},
// 					dozer.SpecVRFBGPTableConnectionStatic: {
// 						ImportPolicies: []string{ROUTE_MAP_VPC_NO_ADVERTISE},
// 					},
// 				},
// 			}
// 		}

// 		if vpc.VPC.DHCP.Enable {
// 			dhcpRelayIP, _, err := net.ParseCIDR(agent.Spec.Config.ControlVIP)
// 			if err != nil {
// 				return errors.Wrapf(err, "failed to parse DHCP relay %s (control vip) for vpc %s", agent.Spec.Config.ControlVIP, vpc.Name)
// 			}

// 			spec.DHCPRelays[vlanIfaceName] = &dozer.SpecDHCPRelay{
// 				SourceInterface: stringPtr(controlIface),
// 				RelayAddress:    []string{dhcpRelayIP.String()},
// 				LinkSelect:      true,
// 				VRFSelect:       isVRFBackend(agent),
// 			}
// 		}
// 	}

// 	for _, vpc := range agent.Spec.VPCs {
// 		for _, peerVPCName := range vpc.Peers {
// 			for _, peer := range agent.Spec.VPCs {
// 				if peer.Name != peerVPCName {
// 					continue
// 				}

// 				if isACLBackend(agent) {
// 					spec.ACLs[aclName(peer.VLAN)].Entries[VPC_ACL_ENTRY_VLAN_SHIFT+uint32(vpc.VLAN)] = &dozer.SpecACLEntry{
// 						Description:        stringPtr(fmt.Sprintf("Allow VPC %s (VLAN %d)", vpc.Name, vpc.VLAN)),
// 						Action:             dozer.SpecACLEntryActionAccept,
// 						DestinationAddress: stringPtr(vpc.VPC.Subnet),
// 					}

// 					spec.ACLs[aclName(vpc.VLAN)].Entries[VPC_ACL_ENTRY_VLAN_SHIFT+uint32(peer.VLAN)] = &dozer.SpecACLEntry{
// 						Description:        stringPtr(fmt.Sprintf("Allow VPC %s (VLAN %d)", peer.Name, peer.VLAN)),
// 						Action:             dozer.SpecACLEntryActionAccept,
// 						DestinationAddress: stringPtr(peer.VPC.Subnet),
// 					}
// 				} else if isVRFBackend(agent) {
// 					spec.VRFs[vpcVrfName(vpc.Name)].BGP.IPv4Unicast.ImportVRFs[vpcVrfName(peer.Name)] = &dozer.SpecVRFBGPImportVRF{}
// 					spec.VRFs[vpcVrfName(peer.Name)].BGP.IPv4Unicast.ImportVRFs[vpcVrfName(vpc.Name)] = &dozer.SpecVRFBGPImportVRF{}
// 				}
// 			}
// 		}
// 	}

// 	return nil
// }

func planVPCs(agent *agentapi.Agent, spec *dozer.Spec) error {
	// spec.RouteMaps[ROUTE_MAP_DISALLOW_DIRECT] = &dozer.SpecRouteMap{
	// 	Statements: map[string]*dozer.SpecRouteMapStatement{
	// 		"10": {
	// 			Conditions: dozer.SpecRouteMapConditions{
	// 				DirectlyConnected: boolPtr(true),
	// 			},
	// 			Result: dozer.SpecRouteMapResultReject,
	// 		},
	// 		"20": {
	// 			Result: dozer.SpecRouteMapResultAccept,
	// 		},
	// 	},
	// }

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

		spec.SuppressVLANNeighs[irbIface] = &dozer.SpecSuppressVLANNeigh{}

		if spec.VRFs[vrfName] == nil {
			spec.VRFs[vrfName] = &dozer.SpecVRF{}
		}
		if spec.VRFs[vrfName].Interfaces == nil {
			spec.VRFs[vrfName].Interfaces = map[string]*dozer.SpecVRFInterface{}
		}

		protocolIP, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
		}

		maxPaths := uint32(64)
		if agent.Spec.IsVS() {
			maxPaths = 16
		}

		spec.VRFs[vrfName].Enabled = boolPtr(true)
		spec.VRFs[vrfName].AnycastMAC = stringPtr(ANYCAST_MAC)
		spec.VRFs[vrfName].BGP = &dozer.SpecVRFBGP{
			AS:                 uint32Ptr(agent.Spec.Switch.ASN),
			RouterID:           stringPtr(protocolIP.String()),
			NetworkImportCheck: boolPtr(true),
			IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
				Enabled:    true,
				MaxPaths:   uint32Ptr(maxPaths),
				ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
				// ImportPolicy: stringPtr(ROUTE_MAP_DISALLOW_DIRECT), // TODO
			},
			L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
				Enabled:              true,
				AdvertiseIPv4Unicast: boolPtr(true),
			},
		}
		spec.VRFs[vrfName].TableConnections = map[string]*dozer.SpecVRFTableConnection{
			dozer.SpecVRFBGPTableConnectionConnected: {},
			dozer.SpecVRFBGPTableConnectionStatic:    {},
		}
		spec.VRFs[vrfName].Interfaces[irbIface] = &dozer.SpecVRFInterface{}

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

	attachedVPC := map[string]bool{}
	for _, attach := range agent.Spec.VPCAttachments {
		vpcName := attach.VPCName()
		vpc, exists := agent.Spec.VPCs[vpcName]
		if !exists {
			return errors.Errorf("VPC %s not found", vpcName)
		}

		attachedVPC[vpcName] = true

		vrfName := vpcVrfName(vpcName)

		subnetName := attach.SubnetName()
		subnet := vpc.Subnets[subnetName]
		if subnet == nil {
			return errors.Errorf("VPC %s subnet %s not found", vpcName, subnetName)
		}

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

		subnetVNI := agent.Spec.VNIs[attach.Subnet]
		if subnetVNI == 0 {
			return errors.Errorf("VNI for VPC %s subnet %s not found", vpcName, subnetName)
		}
		spec.VXLANTunnelMap[fmt.Sprintf("map_%d_%s", subnetVNI, subnetIface)] = &dozer.SpecVXLANTunnelMap{
			VTEP: stringPtr(VTEP_FABRIC),
			VNI:  uint32Ptr(subnetVNI),
			VLAN: uint16Ptr(subnetVLAN),
		}

		spec.SuppressVLANNeighs[subnetIface] = &dozer.SpecSuppressVLANNeigh{}

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
	}

	for peeringName, peering := range agent.Spec.VPCPeers {
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

		vrf1Name := vpcVrfName(vpc1Name)
		vrf2Name := vpcVrfName(vpc2Name)

		if !attachedVPC[vpc1Name] || !attachedVPC[vpc2Name] {
			spec.VRFs[vrf1Name].BGP.IPv4Unicast.ImportVRFs[vrf2Name] = &dozer.SpecVRFBGPImportVRF{}
			spec.VRFs[vrf2Name].BGP.IPv4Unicast.ImportVRFs[vrf1Name] = &dozer.SpecVRFBGPImportVRF{}
		} else {
			// TODO apply VPC loopback workaround if both VPCs are local (if any subnets used in VPC peering are local)
		}
	}

	return nil
}

func portChannelName(id uint16) string {
	return fmt.Sprintf("PortChannel%d", id)
}

func vlanName(vlan uint16) string {
	return fmt.Sprintf("Vlan%d", vlan)
}

func setupPhysicalInterfaceWithPortChannel(spec *dozer.Spec, name, description, portChannel string, mtu *uint16) error { // TODO replace with generic function or drop
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
		PortChannel: &portChannel,
		MTU:         mtu,
	}
	spec.Interfaces[name] = physicalIface

	return nil
}

func stringPtr(s string) *string { return &s }

func uint8Ptr(u uint8) *uint8 { return &u }

func uint16Ptr(u uint16) *uint16 { return &u }

func uint32Ptr(u uint32) *uint32 { return &u }

func uint64Ptr(u uint64) *uint64 { return &u }

func boolPtr(b bool) *bool { return &b }
