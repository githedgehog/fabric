package bcm

import (
	"context"
	"fmt"
	"log/slog"
	"net"
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
	VPC_DENY_ALL_SUBNET                       = "10.0.0.0/8" // TODO move to config
	ROUTE_MAP_VPC_NO_ADVERTISE                = "vpc-no-advertise"
	LO_SWITCH                                 = "Loopback0"
	LO_PROTO                                  = "Loopback1"
	LO_VTEP                                   = "Loopback2"
	VRF_DEFAULT                               = "default"
	VTEP_FABRIC                               = "vtepfabric"
	EVPN_NVO                                  = "nvo1"
	ANYCAST_MAC                               = "00:00:00:11:11:11"
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
				Enabled: boolPtr(true),
			},
		},
		RouteMaps:     map[string]*dozer.SpecRouteMap{},
		DHCPRelays:    map[string]*dozer.SpecDHCPRelay{},
		NATs:          map[uint32]*dozer.SpecNAT{},
		ACLs:          map[string]*dozer.SpecACL{},
		ACLInterfaces: map[string]*dozer.SpecACLInterface{},
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

	controlIface, err := planManagementInterface(agent, spec)
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

	first, err := planMCLAGDomain(agent, spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to plan mclag domain")
	}

	if agent.IsCollapsedCore() {
		slog.Info("Planning collapsed core",
			"VPC backend", agent.Spec.Config.CollapsedCore.VPCBackend,
			"SNAT allowed", agent.Spec.Config.CollapsedCore.SNATAllowed)

		err = planCollapsedCoreVPCs(agent, spec, controlIface, first)
		if err != nil {
			return nil, errors.Wrap(err, "failed to plan Collapsed Core VPCs")
		}

		err = planCollapsedCoreNAT(agent, spec, first)
		if err != nil {
			return nil, errors.Wrap(err, "failed to plan Collapsed Core NAT")
		}
	} else if agent.IsSpineLeaf() {
		slog.Info("Planning spine leaf")

		err = planSpineLeafVPCs(agent, spec, controlIface)
		if err != nil {
			return nil, errors.Wrap(err, "failed to plan Spine Leaf VPCs")
		}
	}

	spec.Normalize()

	return spec, nil
}

func planManagementInterface(agent *agentapi.Agent, spec *dozer.Spec) (string, error) {
	controlIface := ""
	controlIP := ""
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.Management != nil {
			controlIface = conn.Spec.Management.Link.Switch.LocalPortName()
			controlIP = conn.Spec.Management.Link.Switch.IP
			break
		}
	}
	if controlIface == "" {
		return "", errors.Errorf("no control interface found")
	}
	if controlIP == "" {
		return "", errors.Errorf("no control IP found")
	}

	ip, ipNet, err := net.ParseCIDR(controlIP)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse control IP %s", controlIP)
	}
	prefixLen, _ := ipNet.Mask.Size()

	spec.Interfaces[controlIface] = &dozer.SpecInterface{
		Description: stringPtr(controlIface),
		Enabled:     boolPtr(true),
		IPs: map[string]*dozer.SpecInterfaceIP{
			ip.String(): {
				PrefixLen: uint8Ptr(uint8(prefixLen)),
			},
		},
	}

	return controlIface, nil
}

func planLLDP(agent *agentapi.Agent, spec *dozer.Spec) error {
	spec.LLDP = &dozer.SpecLLDP{
		Enabled:           boolPtr(true),
		HelloTimer:        uint64Ptr(5), // TODO make configurable?
		SystemName:        stringPtr(agent.Name),
		SystemDescription: stringPtr(fmt.Sprintf("Hedgehog: [control_vip=%s]", agent.Spec.Config.ControlVIP)),
	}

	for _, conn := range agent.Spec.Connections {
		if conn.Spec.Fabric != nil {
			for _, link := range conn.Spec.Fabric.Links {
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
		IPs: map[string]*dozer.SpecInterfaceIP{
			ip.String(): {
				PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
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
		IPs: map[string]*dozer.SpecInterfaceIP{
			ip.String(): {
				PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
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
			IPs: map[string]*dozer.SpecInterfaceIP{
				ip.String(): {
					PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
				},
			},
		}
	}

	return nil
}

func planFabricConnections(agent *agentapi.Agent, spec *dozer.Spec) error {
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.Fabric == nil {
			continue
		}

		for _, link := range conn.Spec.Fabric.Links {
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
				return errors.Errorf("no IP found for fabric conn %s", conn.Name)
			}

			ip, ipNet, err := net.ParseCIDR(ipStr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse fabric conn ip %s", ipStr)
			}
			ipPrefixLen, _ := ipNet.Mask.Size()

			spec.Interfaces[port] = &dozer.SpecInterface{
				Enabled:     boolPtr(true),
				Description: stringPtr(fmt.Sprintf("Fabric %s // %s", remote, conn.Name)),
				IPs: map[string]*dozer.SpecInterfaceIP{
					ip.String(): {
						PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
					},
				},
			}

			if peerSw, ok := agent.Spec.Switches[peer]; !ok {
				return errors.Errorf("no switch found for peer %s (fabric conn %s)", peer, conn.Name)
			} else {
				ip, _, err := net.ParseCIDR(peerIP)
				if err != nil {
					return errors.Wrapf(err, "failed to parse fabric conn peer ip %s", peerIP)
				}

				spec.VRFs[VRF_DEFAULT].BGP.Neighbors[ip.String()] = &dozer.SpecVRFBGPNeighbor{
					Enabled:     boolPtr(true),
					Description: stringPtr(fmt.Sprintf("Fabric %s // %s", remote, conn.Name)),
					RemoteAS:    uint32Ptr(peerSw.ASN),
					IPv4Unicast: boolPtr(true),
					L2VPNEVPN:   boolPtr(true),
				}
			}
		}
	}

	return nil
}

func planServerConnections(agent *agentapi.Agent, spec *dozer.Spec) error {
	// handle connections which should be configured as port channels
	for _, conn := range agent.Spec.Connections {
		connType := ""
		var mtu *uint16
		var links []wiringapi.ServerToSwitchLink

		if conn.Spec.MCLAG != nil {
			connType = "MCLAG"
			if conn.Spec.MCLAG.MTU != 0 {
				mtu = uint16Ptr(conn.Spec.MCLAG.MTU)
			}
			links = conn.Spec.MCLAG.Links
		} else if conn.Spec.Bundled != nil {
			connType = "Bundled"
			// TODO MTU
			links = conn.Spec.Bundled.Links
		} else {
			continue
		}

		for _, link := range links {
			if link.Switch.DeviceName() != agent.Name {
				continue
			}

			portName := link.Switch.LocalPortName()
			portChan := agent.Spec.PortChannels[conn.Name]

			if portChan == 0 {
				return errors.Errorf("no port channel found for conn %s", conn.Name)
			}

			connPortChannelName := portChannelName(portChan)
			connPortChannel := &dozer.SpecInterface{
				Enabled:     boolPtr(true),
				Description: stringPtr(fmt.Sprintf("%s conn %s", connType, conn.Name)),
				TrunkVLANs:  []string{agent.Spec.VPCVLANRange},
				MTU:         mtu,
			}
			spec.Interfaces[connPortChannelName] = connPortChannel

			if connType == "MCLAG" {
				spec.MCLAGInterfaces[connPortChannelName] = &dozer.SpecMCLAGInterface{
					DomainID: MCLAG_DOMAIN_ID,
				}
			}

			descr := fmt.Sprintf("%s %s conn %s", connPortChannelName, connType, conn.Name)
			err := setupPhysicalInterfaceWithPortChannel(spec, portName, descr, connPortChannelName, nil)
			if err != nil {
				return errors.Wrapf(err, "failed to setup physical interface %s", portName)
			}
		}
	}

	// handle non-portchannel connections
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.Unbundled == nil {
			continue
		}

		if conn.Spec.Unbundled.Link.Switch.DeviceName() != agent.Name {
			continue
		}

		swPort := conn.Spec.Unbundled.Link.Switch

		spec.Interfaces[swPort.LocalPortName()] = &dozer.SpecInterface{
			Enabled:     boolPtr(true),
			Description: stringPtr(fmt.Sprintf("Unbundled conn %s", conn.Name)),
			TrunkVLANs:  []string{agent.Spec.VPCVLANRange},
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

	spec.VRFs[VRF_DEFAULT] = &dozer.SpecVRF{
		Enabled:    boolPtr(true),
		Interfaces: map[string]*dozer.SpecVRFInterface{},
		AnycastMAC: stringPtr(ANYCAST_MAC),
		BGP: &dozer.SpecVRFBGP{
			AS:                 uint32Ptr(agent.Spec.Switch.ASN),
			RouterID:           stringPtr(ip.String()),
			NetworkImportCheck: boolPtr(true), // default
			Neighbors:          map[string]*dozer.SpecVRFBGPNeighbor{},
			IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
				Enabled:  true,
				MaxPaths: uint32Ptr(64),
			},
			L2VPNEVPN: dozer.SpecVRFBGPL2VPNEVPN{
				Enabled:         true,
				AdvertiseAllVNI: boolPtr(true),
			},
		},
		TableConnections: map[string]*dozer.SpecVRFTableConnection{
			dozer.SpecVRFBGPTableConnectionConnected: {},
			dozer.SpecVRFBGPTableConnectionStatic:    {},
		},
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
	mclagPeerLinks := []string{}
	mclagSessionLinks := []string{}
	mclagPeerSwitch := ""
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.MCLAGDomain != nil {
			ok = true
			for _, link := range conn.Spec.MCLAGDomain.PeerLinks {
				if link.Switch1.DeviceName() == agent.Name {
					mclagPeerLinks = append(mclagPeerLinks, link.Switch1.LocalPortName())
					mclagPeerSwitch = link.Switch2.DeviceName()
				} else if link.Switch2.DeviceName() == agent.Name {
					mclagPeerLinks = append(mclagPeerLinks, link.Switch2.LocalPortName())
					mclagPeerSwitch = link.Switch1.DeviceName()
				}
			}
			for _, link := range conn.Spec.MCLAGDomain.SessionLinks {
				if link.Switch1.DeviceName() == agent.Name {
					mclagSessionLinks = append(mclagSessionLinks, link.Switch1.LocalPortName())
				} else if link.Switch2.DeviceName() == agent.Name {
					mclagSessionLinks = append(mclagSessionLinks, link.Switch2.LocalPortName())
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
		Description: stringPtr("MCLAG peer link"),
		Enabled:     boolPtr(true),
		TrunkVLANs:  []string{MCLAG_PEER_LINK_TRUNK_VLAN_RANGE},
	}
	spec.Interfaces[mclagPeerPortChannelName] = mclagPeerPortChannel
	for _, iface := range mclagPeerLinks {
		descr := fmt.Sprintf("MCLAG peer link %s", mclagPeerPortChannelName)
		err := setupPhysicalInterfaceWithPortChannel(spec, iface, descr, mclagPeerPortChannelName, nil)
		if err != nil {
			return false, errors.Wrapf(err, "failed to setup physical interface %s for MCLAG peer link", iface)
		}
	}

	mclagSessionPortChannelName := portChannelName(MCLAG_SESSION_LINK_PORT_CHANNEL_ID)
	mclagSessionPortChannel := &dozer.SpecInterface{
		Description: stringPtr("MCLAG session link"),
		Enabled:     boolPtr(true),
		IPs: map[string]*dozer.SpecInterfaceIP{
			sourceIP: {
				PrefixLen: uint8Ptr(MCLAG_SESSION_IP_PREFIX_LEN),
			},
		},
	}
	spec.Interfaces[mclagSessionPortChannelName] = mclagSessionPortChannel
	for _, iface := range mclagSessionLinks {
		descr := fmt.Sprintf("MCLAG session link %s", mclagSessionPortChannelName)
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
		Description: stringPtr(fmt.Sprintf("MCLAG peer %s", mclagPeerSwitch)),
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

func isACLBackend(agent *agentapi.Agent) bool {
	return agent.IsCollapsedCore() && agent.Spec.Config.CollapsedCore.VPCBackend == string(agentapi.VPCBackendACL)
}

func isVRFBackend(agent *agentapi.Agent) bool {
	return agent.IsCollapsedCore() && agent.Spec.Config.CollapsedCore.VPCBackend == string(agentapi.VPCBackendVRF)
}

func filteredDNAT(dnatInfo map[string]string) map[string]string {
	filtered := map[string]string{}
	for key, value := range dnatInfo {
		if strings.HasPrefix(value, "@") {
			continue
		}

		filtered[key] = value
	}

	return filtered
}

func planCollapsedCoreVPCs(agent *agentapi.Agent, spec *dozer.Spec, controlIface string, firstSwitch bool) error {
	if !isACLBackend(agent) && !isVRFBackend(agent) {
		return errors.Errorf("unknown VPC backend %s", agent.Spec.Config.CollapsedCore.VPCBackend)
	}

	if isVRFBackend(agent) {
		// TODO switch to policy per VPC
		spec.RouteMaps[ROUTE_MAP_VPC_NO_ADVERTISE] = &dozer.SpecRouteMap{
			NoAdvertise: boolPtr(true),
		}
	}

	for _, vpc := range agent.Spec.VPCs {
		cidr, err := iputil.ParseCIDR(vpc.VPC.Subnet)
		if err != nil {
			return errors.Wrapf(err, "failed to parse subnet %s for vpc %s", vpc.VPC.Subnet, vpc.Name)
		}
		ip := cidr.Gateway.String()
		prefixLen, _ := cidr.Subnet.Mask.Size()

		descr := fmt.Sprintf("VPC %s", vpc.Name)
		vlanIfaceName, _, err := setupVLANInterfaceWithIP(spec, vpc.VLAN, ip, uint8(prefixLen), descr)
		if err != nil {
			return errors.Wrapf(err, "failed to setup VLAN interface for vpc %s", vpc.Name)
		}

		if isACLBackend(agent) {
			acl := &dozer.SpecACL{
				Description: stringPtr(fmt.Sprintf("VPC %s ACL IN (VLAN %d)", vpc.Name, vpc.VLAN)),
				Entries: map[uint32]*dozer.SpecACLEntry{
					VPC_ACL_ENTRY_SEQ_SUBNET: {
						Description:        stringPtr("Allow own subnet"),
						Action:             dozer.SpecACLEntryActionAccept,
						DestinationAddress: stringPtr(vpc.VPC.Subnet),
					},
					VPC_ACL_ENTRY_DENY_ALL_VPC: {
						Description:        stringPtr("Deny all other VPCs"),
						Action:             dozer.SpecACLEntryActionDrop,
						DestinationAddress: stringPtr(VPC_DENY_ALL_SUBNET),
					},
				},
			}

			if vpc.VPC.DHCP.Enable {
				acl.Entries[VPC_ACL_ENTRY_SEQ_DHCP] = &dozer.SpecACLEntry{
					Description:     stringPtr("Allow DHCP"),
					Action:          dozer.SpecACLEntryActionAccept,
					Protocol:        dozer.SpecACLEntryProtocolUDP,
					SourcePort:      uint16Ptr(68),
					DestinationPort: uint16Ptr(67),
				}
			}

			if agent.Spec.Config.CollapsedCore.SNATAllowed && vpc.VPC.SNAT || len(filteredDNAT(vpc.DNAT)) > 0 {
				acl.Entries[VPC_ACL_ENTRY_PERMIT_ANY] = &dozer.SpecACLEntry{
					Description:   stringPtr("Allow any traffic (NAT)"),
					Action:        dozer.SpecACLEntryActionAccept,
					SourceAddress: stringPtr(vpc.VPC.Subnet),
				}
			}

			aclName := aclName(vpc.VLAN)
			spec.ACLs[aclName] = acl
			spec.ACLInterfaces[vlanIfaceName] = &dozer.SpecACLInterface{
				Ingress: stringPtr(aclName),
			}
		} else if isVRFBackend(agent) {
			vrfName := vpcVrfName(vpc.Name)

			spec.VRFs[vrfName] = &dozer.SpecVRF{
				Enabled: boolPtr(true),
				// Description: stringPtr(fmt.Sprintf("VPC %s", vpc.Name)),
				Interfaces: map[string]*dozer.SpecVRFInterface{
					vlanIfaceName: {},
				},
				BGP: &dozer.SpecVRFBGP{
					AS:                 uint32Ptr(agent.Spec.Switch.ASN),
					NetworkImportCheck: boolPtr(true),
					IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
						Enabled:    true,
						ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
						Networks:   map[string]*dozer.SpecVRFBGPNetwork{},
					},
				},
				TableConnections: map[string]*dozer.SpecVRFTableConnection{
					dozer.SpecVRFBGPTableConnectionConnected: {
						ImportPolicies: []string{ROUTE_MAP_VPC_NO_ADVERTISE},
					},
					dozer.SpecVRFBGPTableConnectionStatic: {
						ImportPolicies: []string{ROUTE_MAP_VPC_NO_ADVERTISE},
					},
				},
			}
		}

		if vpc.VPC.DHCP.Enable {
			dhcpRelayIP, _, err := net.ParseCIDR(agent.Spec.Config.ControlVIP)
			if err != nil {
				return errors.Wrapf(err, "failed to parse DHCP relay %s (control vip) for vpc %s", agent.Spec.Config.ControlVIP, vpc.Name)
			}

			spec.DHCPRelays[vlanIfaceName] = &dozer.SpecDHCPRelay{
				SourceInterface: stringPtr(controlIface),
				RelayAddress:    []string{dhcpRelayIP.String()},
				LinkSelect:      true,
				VRFSelect:       isVRFBackend(agent),
			}
		}
	}

	for _, vpc := range agent.Spec.VPCs {
		for _, peerVPCName := range vpc.Peers {
			for _, peer := range agent.Spec.VPCs {
				if peer.Name != peerVPCName {
					continue
				}

				if isACLBackend(agent) {
					spec.ACLs[aclName(peer.VLAN)].Entries[VPC_ACL_ENTRY_VLAN_SHIFT+uint32(vpc.VLAN)] = &dozer.SpecACLEntry{
						Description:        stringPtr(fmt.Sprintf("Allow VPC %s (VLAN %d)", vpc.Name, vpc.VLAN)),
						Action:             dozer.SpecACLEntryActionAccept,
						DestinationAddress: stringPtr(vpc.VPC.Subnet),
					}

					spec.ACLs[aclName(vpc.VLAN)].Entries[VPC_ACL_ENTRY_VLAN_SHIFT+uint32(peer.VLAN)] = &dozer.SpecACLEntry{
						Description:        stringPtr(fmt.Sprintf("Allow VPC %s (VLAN %d)", peer.Name, peer.VLAN)),
						Action:             dozer.SpecACLEntryActionAccept,
						DestinationAddress: stringPtr(peer.VPC.Subnet),
					}
				} else if isVRFBackend(agent) {
					spec.VRFs[vpcVrfName(vpc.Name)].BGP.IPv4Unicast.ImportVRFs[vpcVrfName(peer.Name)] = &dozer.SpecVRFBGPImportVRF{}
					spec.VRFs[vpcVrfName(peer.Name)].BGP.IPv4Unicast.ImportVRFs[vpcVrfName(vpc.Name)] = &dozer.SpecVRFBGPImportVRF{}
				}
			}
		}
	}

	return nil
}

func planCollapsedCoreNAT(agent *agentapi.Agent, spec *dozer.Spec, firstSwitch bool) error {
	var natConn *wiringapi.ConnNAT
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.NAT != nil && conn.Spec.NAT.Link.Switch.DeviceName() == agent.Name {
			if conn.Spec.NAT.Link.NAT.Port != "default" {
				return errors.Errorf("only default NAT is supported")
			}
			natConn = conn.Spec.NAT
			break
		}
	}

	if natConn == nil || agent.Spec.NAT.Subnet == "" {
		return nil
	}

	sw := natConn.Link.Switch
	ip, ipNet, err := net.ParseCIDR(sw.IP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse external interface ip %s", sw.IP)
	}
	ipPrefixLen, _ := ipNet.Mask.Size()

	cidr, err := iputil.ParseCIDR(agent.Spec.NAT.Subnet)
	if err != nil {
		return errors.Wrapf(err, "cannot parse NAT subnet %s", agent.Spec.NAT.Subnet)
	}
	subnetPrefixLen, _ := cidr.Subnet.Mask.Size()

	publicIface := sw.LocalPortName()
	natName := natConn.Link.NAT.Port
	natVRF := VRF_DEFAULT // NAT is only supported in the default VRF

	spec.Interfaces[publicIface] = &dozer.SpecInterface{
		Description: stringPtr(fmt.Sprintf("NAT external %s", natName)),
		Enabled:     boolPtr(true),
		IPs: map[string]*dozer.SpecInterfaceIP{
			ip.String(): {
				PrefixLen: uint8Ptr(uint8(ipPrefixLen)),
			},
		},
		NATZone: uint8Ptr(NAT_ZONE_EXTERNAL),
	}

	anchorVLANIface := vlanName(NAT_ANCHOR_VLAN)
	spec.Interfaces[anchorVLANIface] = &dozer.SpecInterface{
		Description: stringPtr(fmt.Sprintf("NAT anchor %s", natName)),
		Enabled:     boolPtr(false),
		IPs: map[string]*dozer.SpecInterfaceIP{
			cidr.Gateway.String(): {
				VLAN:      true,
				PrefixLen: uint8Ptr(uint8(subnetPrefixLen)),
			},
		},
		NATZone: uint8Ptr(NAT_ZONE_EXTERNAL),
	}

	networks := map[string]*dozer.SpecVRFBGPNetwork{}
	if agent.Spec.Config.CollapsedCore.SNATAllowed {
		for _, network := range natConn.Link.Switch.SNAT.Pool {
			networks[network] = &dozer.SpecVRFBGPNetwork{}
		}
	}

	static := map[string]*dozer.SpecNATEntry{}

	if isACLBackend(agent) || isVRFBackend(agent) && firstSwitch {
		for _, vpcInfo := range agent.Spec.VPCs {
			for internalIP, externalIP := range filteredDNAT(vpcInfo.DNAT) {
				static[externalIP] = &dozer.SpecNATEntry{
					InternalAddress: stringPtr(internalIP),
					Type:            dozer.SpecNATTypeDNAT,
				}
				networks[externalIP+"/32"] = &dozer.SpecVRFBGPNetwork{}
			}
		}
	}

	vrf := &dozer.SpecVRF{
		Enabled:    boolPtr(true),
		Interfaces: map[string]*dozer.SpecVRFInterface{},
		BGP: &dozer.SpecVRFBGP{
			AS:                 uint32Ptr(agent.Spec.Switch.ASN),
			NetworkImportCheck: boolPtr(false),
			Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{
				sw.NeighborIP: {
					Enabled:     boolPtr(true),
					Description: stringPtr(fmt.Sprintf("NAT %s", natName)),
					RemoteAS:    uint32Ptr(sw.RemoteAS),
					IPv4Unicast: boolPtr(true),
				},
			},
			IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
				Enabled:    true,
				Networks:   networks,
				ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
			},
		},
	}

	if isVRFBackend(agent) && firstSwitch {
		for _, vpc := range agent.Spec.VPCs {
			if agent.Spec.Config.CollapsedCore.SNATAllowed && vpc.VPC.SNAT || len(filteredDNAT(vpc.DNAT)) > 0 {
				spec.VRFs[vpcVrfName(vpc.Name)].BGP.IPv4Unicast.ImportVRFs[natVRF] = &dozer.SpecVRFBGPImportVRF{}
				vrf.BGP.IPv4Unicast.ImportVRFs[vpcVrfName(vpc.Name)] = &dozer.SpecVRFBGPImportVRF{}
			}
		}
	}

	spec.VRFs[natVRF] = vrf

	pools := map[string]*dozer.SpecNATPool{}
	bindings := map[string]*dozer.SpecNATBinding{}

	if agent.Spec.Config.CollapsedCore.SNATAllowed {
		for idx, cidr := range natConn.Link.Switch.SNAT.Pool {
			first, last, err := iputil.Range(cidr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse nat pool cidr #%d %s", idx, cidr)
			}

			name := fmt.Sprintf("%s-%d", natName, idx)
			pools[name] = &dozer.SpecNATPool{
				Range: stringPtr(fmt.Sprintf("%s-%s", first, last)),
			}
			bindings[name] = &dozer.SpecNATBinding{
				Pool: stringPtr(name),
			}
		}
	}

	spec.NATs[NAT_INSTANCE_ID] = &dozer.SpecNAT{
		Enable:   boolPtr(true),
		Pools:    pools,
		Bindings: bindings,
		Static:   static,
	}

	return nil
}

func planSpineLeafVPCs(agent *agentapi.Agent, spec *dozer.Spec, controlIface string) error {
	// TODO
	return nil
}

func portChannelName(id uint16) string {
	return fmt.Sprintf("PortChannel%d", id)
}

func vlanName(vlan uint16) string {
	return fmt.Sprintf("Vlan%d", vlan)
}

func aclName(vlan uint16) string {
	return fmt.Sprintf("vpc-vlan%d-in", vlan)
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

func setupVLANInterfaceWithIP(spec *dozer.Spec, vlan uint16, ip string, prefixLen uint8, description string) (string, *dozer.SpecInterface, error) { // TODO replace with generic function or drop
	name := vlanName(vlan)
	if iface, exist := spec.Interfaces[name]; exist {
		descr := ""
		if iface.Description != nil {
			descr = ", description: " + *iface.Description
		}
		return "", nil, errors.Errorf("vlan interface %s already used for something%s", name, descr)
	}

	vlanIface := &dozer.SpecInterface{
		Description: stringPtr(description),
		Enabled:     boolPtr(true),
		IPs: map[string]*dozer.SpecInterfaceIP{
			ip: {
				VLAN:      true,
				PrefixLen: uint8Ptr(prefixLen),
			},
		},
	}
	spec.Interfaces[name] = vlanIface

	return name, vlanIface, nil
}

func stringPtr(s string) *string { return &s }

func uint8Ptr(u uint8) *uint8 { return &u }

func uint16Ptr(u uint16) *uint16 { return &u }

func uint32Ptr(u uint32) *uint32 { return &u }

func uint64Ptr(u uint64) *uint64 { return &u }

func boolPtr(b bool) *bool { return &b }
