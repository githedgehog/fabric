package agent

import (
	"fmt"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/gnmi"
	"golang.org/x/exp/maps"
)

const (
	MCLAG_DOMAIN_ID                    = 100
	MCLAG_PEER_LINK_PORT_CHANNEL_ID    = 250
	MCLAG_SESSION_LINK_PORT_CHANNEL_ID = 251
	MCLAG_PEER_LINK_TRUNK_VLAN_RANGE   = "2..4094" // TODO do we need to configure it?
	MCLAG_SESSION_IP_1                 = "172.21.0.0/31"
	MCLAG_SESSION_IP_2                 = "172.21.0.1/31"
)

func PreparePlan(agent *agentapi.Agent) (*gnmi.Plan, error) {
	plan := &gnmi.Plan{}

	plan.Hostname = agent.Name
	plan.PortGroupSpeeds = agent.Spec.Switch.PortGroupSpeeds

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
		return nil, errors.Errorf("no control interface found for %s", agent.Name)
	}

	plan.ManagementIface = controlIface // TODO we only support switches connected to control node using management interface for now
	plan.ManagementIP = controlIP

	// mclag peer link interfaces
	mclagPeerLinks := []string{}
	mclagSessionLinks := []string{}
	mclagPeerSwitch := ""
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.MCLAGDomain != nil {
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
	if len(mclagPeerLinks) == 0 {
		return nil, errors.Errorf("no mclag peer links found for %s", agent.Name)
	}
	if len(mclagSessionLinks) == 0 {
		return nil, errors.Errorf("no mclag session links found for %s", agent.Name)
	}
	if mclagPeerSwitch == "" {
		return nil, errors.Errorf("no mclag peer switch found for %s", agent.Name)
	}

	// mclag peer link port channel
	mclagPeerLink := gnmi.PortChannel{
		ID:             MCLAG_PEER_LINK_PORT_CHANNEL_ID,
		Description:    "MCLAG peer link",
		TrunkVLANRange: ygot.String(MCLAG_PEER_LINK_TRUNK_VLAN_RANGE),
		Members:        mclagPeerLinks,
	}
	plan.PortChannels = append(plan.PortChannels, mclagPeerLink)

	// mclag session link port channel
	mclagSessionLink := gnmi.PortChannel{
		ID:          MCLAG_SESSION_LINK_PORT_CHANNEL_ID,
		Description: "MCLAG session link",
		Members:     mclagSessionLinks,
	}
	plan.PortChannels = append(plan.PortChannels, mclagSessionLink)

	// using the same IP pair with switch with name < peer switch name getting first IP
	sourceIP := MCLAG_SESSION_IP_1
	peerIP := MCLAG_SESSION_IP_2
	if agent.Name > mclagPeerSwitch {
		sourceIP, peerIP = peerIP, sourceIP
	}

	// mclag domain
	plan.MCLAGDomain = gnmi.MCLAGDomain{
		ID:       MCLAG_DOMAIN_ID,
		SourceIP: sourceIP,
		PeerIP:   peerIP,
		PeerLink: mclagPeerLink.Name(),
	}

	// ip for mclag session link port channel
	plan.InterfaceIPs = append(plan.InterfaceIPs, gnmi.InterfaceIP{
		Name: mclagSessionLink.Name(),
		IP:   plan.MCLAGDomain.SourceIP,
	})

	// PortChannel for mclag servers
	// add mclag server interfaces to port channel
	// add port channel to mclag domain
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.MCLAG != nil {
			for _, link := range conn.Spec.MCLAG.Links {
				if link.Switch.DeviceName() == agent.Name {
					portName := link.Switch.LocalPortName()
					portChan := agent.Spec.PortChannels[conn.Name]

					if portChan == 0 {
						return nil, errors.Errorf("no port channel found for conn %s", conn.Name)
					}

					pChan := gnmi.PortChannel{
						ID:             portChan,
						Description:    fmt.Sprintf("MCLAG for %s, conn %s", portName, conn.Name),
						TrunkVLANRange: ygot.String(agent.Spec.VPCVLANRange),
						Members:        []string{portName},
					}
					plan.PortChannels = append(plan.PortChannels, pChan)
					plan.MCLAGDomain.Members = append(plan.MCLAGDomain.Members, pChan.Name())
				}
			}
		}
	}

	for _, user := range agent.Spec.Users {
		if user.Name == gnmi.AGENT_USER {
			// never configure agent user other than through agent setup
			continue
		}
		planUser := gnmi.User{
			Name:     user.Name,
			Password: user.Password,
			Role:     user.Role,
		}
		if len(user.SSHKeys) > 0 {
			planUser.SSHKey = user.SSHKeys[0]
		}
		// We're directly configuring authorized_keys
		// if len(user.SSHKeys) > 1 {
		// 	slog.Info("More than one ssh keys specified for user, only the first one will be used", "user", user.Name)
		// }

		plan.Users = append(plan.Users, planUser)
	}

	for _, vpcInfo := range agent.Spec.VPCs {
		vpc := gnmi.VPC{
			Name:   vpcInfo.Name,
			VLAN:   vpcInfo.VLAN,
			Subnet: vpcInfo.VPC.Subnet,
		}
		if vpcInfo.VPC.DHCP.Enable {
			vpc.DHCP = true
			vpc.DHCPRelay = agent.Spec.ControlVIP
			vpc.DHCPSource = controlIface
		}
		vpc.Peers = vpcInfo.Peers

		plan.VPCs = append(plan.VPCs, vpc)
	}

	var natConn *wiringapi.ConnectionSpec
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.NAT != nil && conn.Spec.NAT.Link.Switch.DeviceName() == agent.Name {
			natConn = &conn.Spec
			break
		}
	}

	if natConn != nil {
		info := natConn.NAT.Link.Switch

		plan.NAT = gnmi.NAT{
			PublicIface: info.LocalPortName(),
			PublicIP:    info.IP,
			AnchorIP:    info.AnchorIP, // TODO just use first address from the pool
			Pool:        info.SNAT.Pool,
			Neighbor:    info.NeighborIP,
			RemoteAS:    info.RemoteAS,
		}
	}

	plan.StaticNAT = map[string]string{}
	for _, vpcInfo := range agent.Spec.VPCs {
		maps.Copy(plan.StaticNAT, vpcInfo.DNAT)
	}

	return plan, nil
}
