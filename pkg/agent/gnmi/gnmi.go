package gnmi

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/gnmi/bcom/oc"
	"go.githedgehog.com/fabric/pkg/util/iputil"
)

type Plan struct {
	Hostname        string
	ManagementIface string
	ManagementIP    string
	MCLAGDomain     MCLAGDomain
	PortChannels    []PortChannel
	InterfaceIPs    []InterfaceIP
	Users           []User
	VPCs            []VPC
	PortGroupSpeeds map[string]string
}

type PortChannel struct {
	ID             uint16 // 1..256
	Description    string
	TrunkVLANRange *string
	Members        []string // Interfaces
}

func PortChannelName(id uint16) string {
	return fmt.Sprintf("PortChannel%d", id)
}

func (pChan *PortChannel) Name() string {
	return PortChannelName(pChan.ID)
}

type InterfaceIP struct {
	Name string
	IP   string
}

type MCLAGDomain struct {
	ID       uint32 // 1..4095
	SourceIP string
	PeerIP   string
	PeerLink string
	Members  []string // PortChannels
	// MCLAGSystemMac string // TODO evaluate if we need it
}

type User struct {
	Name     string
	Password string
	Role     string
	SSHKey   string
}

type VPC struct {
	Name       string
	Subnet     string
	VLAN       uint16
	DHCP       bool
	DHCPRelay  string
	DHCPSource string
	Peers      []string
}

const (
	VPC_VRF_PREFIX        = "V"
	ASN            uint32 = 65101
)

func (plan *Plan) Entries() ([]*Entry, []*Entry, error) {
	earlyApply := []*Entry{}

	earlyApply = append(earlyApply, EntDisableZtp())
	earlyApply = append(earlyApply, EntHostname(plan.Hostname))

	for _, user := range plan.Users {
		earlyApply = append(earlyApply, EntUser(user.Name, user.Password, user.Role, user.SSHKey))
	}

	readyApply := []*Entry{}
	{
		ip, ipNet, err := net.ParseCIDR(plan.ManagementIP)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse management ip %s", plan.ManagementIP)
		}
		prefixLen, _ := ipNet.Mask.Size()

		readyApply = append(readyApply, EntInterfaceIP(plan.ManagementIface, ip.String(), uint8(prefixLen)))
	}

	readyApply = append(readyApply, EntVrfBGP("default", ASN))
	readyApply = append(readyApply, EntBGPRouteDistribution("default", ""))

	for group, speedStr := range plan.PortGroupSpeeds {
		speed := oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET

		for id, enum := range oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"] {
			if enum.Name == speedStr {
				speed = oc.E_OpenconfigIfEthernet_ETHERNET_SPEED(id)
				break
			}
		}

		if speed == oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET || speed == oc.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_UNKNOWN {
			slog.Warn("Skipping unset or unknown speed", "portgroup", group, "speed", speedStr, "speedID", speed)
			continue
		}

		// TODO add some good validation and probably different formats like w/o SPEED_ prefix and show options in error

		readyApply = append(readyApply, EntPortGroupSpeed(group, speedStr, speed))
	}

	for _, pChan := range plan.PortChannels {
		if pChan.TrunkVLANRange != nil {
			readyApply = append(readyApply, EntPortChannel(pChan.Name(), pChan.Description, *pChan.TrunkVLANRange))
		} else {
			readyApply = append(readyApply, EntL3PortChannel(pChan.Name(), pChan.Description))
		}

		for _, member := range pChan.Members {
			readyApply = append(readyApply, EntPortChannelMember(pChan.Name(), member))
			readyApply = append(readyApply, EntInterfaceUP(member))
		}
	}

	for _, ifIP := range plan.InterfaceIPs {
		ip, ipNet, err := net.ParseCIDR(ifIP.IP)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse CIDR %s for %s", ifIP.IP, ifIP.Name)
		}
		prefixLen, _ := ipNet.Mask.Size()

		readyApply = append(readyApply, EntInterfaceIP(ifIP.Name, ip.String(), uint8(prefixLen)))
	}

	readyApply = append(readyApply, EntMCLAGDomain(plan.MCLAGDomain.ID, plan.MCLAGDomain.SourceIP, plan.MCLAGDomain.PeerIP, plan.MCLAGDomain.PeerLink))

	for _, member := range plan.MCLAGDomain.Members {
		readyApply = append(readyApply, EntMCLAGMember(plan.MCLAGDomain.ID, member))
	}

	// TOD per Vrf policy
	policyName := "vpc-no-advertise"
	readyApply = append(readyApply, EntBGPRoutingPolicy(policyName,
		[]oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union{
			oc.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_ADVERTISE,
		},
	))
	for _, vpc := range plan.VPCs {
		vrfName := "Vrf" + VPC_VRF_PREFIX + vpc.Name
		// policyName := vrfName + "_route_map"

		readyApply = append(readyApply, EntVrf(vrfName))
		readyApply = append(readyApply, EntVrfBGP(vrfName, ASN))

		readyApply = append(readyApply, EntBGPRouteDistribution(vrfName, policyName))

		cidr, err := iputil.ParseCIDR(vpc.Subnet)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to parse subnet %s for vpc %s", vpc.Subnet, vpc.Name)
		}
		prefixLen, _ := cidr.Subnet.Mask.Size()

		readyApply = append(readyApply, EntVLANInterface(vpc.VLAN, vpc.Name))
		readyApply = append(readyApply, EntVrfMember(vrfName, vpc.VLAN))
		readyApply = append(readyApply, EntVLANInterfaceConf(vpc.VLAN, cidr.Gateway.String(), uint8(prefixLen)))

		if vpc.DHCP {
			ip, _, err := net.ParseCIDR(vpc.DHCPRelay)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to parse DHCP relay %s for vpc %s", vpc.DHCPRelay, vpc.Name)
			}
			readyApply = append(readyApply, EntDHCPRelay(vpc.VLAN, ip.String(), vpc.DHCPSource))
		}
	}

	for _, vpc := range plan.VPCs {
		vrfName := "Vrf" + VPC_VRF_PREFIX + vpc.Name
		peers := []string{}
		for _, peer := range vpc.Peers {
			peers = append(peers, "Vrf"+VPC_VRF_PREFIX+peer)
		}
		if len(peers) > 0 { // TODO what about case when we removing all peers?
			readyApply = append(readyApply, EntVrfImportRoutes(vrfName, peers))
		}
	}

	return earlyApply, readyApply, nil
}
