package gnmi

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/gnmi/bcom/oc"
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
}

type VPC struct {
	Name       string
	Subnet     string
	VLAN       uint16
	DHCP       bool
	DHCPRelay  string
	DHCPSource string
}

func (plan *Plan) Entries() ([]*Entry, error) {
	res := []*Entry{}

	res = append(res, EntDisableZtp())
	res = append(res, EntHostname(plan.Hostname))

	ip, ipNet, err := net.ParseCIDR(plan.ManagementIP)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse management ip %s", plan.ManagementIP)
	}
	prefixLen, _ := ipNet.Mask.Size()

	res = append(res, EntInterfaceIP(plan.ManagementIface, ip.String(), uint8(prefixLen)))

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

		res = append(res, EntPortGroupSpeed(group, speedStr, speed))
	}

	for _, user := range plan.Users {
		res = append(res, EntUser(user.Name, user.Password, user.Role))
	}

	for _, pChan := range plan.PortChannels {
		if pChan.TrunkVLANRange != nil {
			res = append(res, EntPortChannel(pChan.Name(), pChan.Description, *pChan.TrunkVLANRange))
		} else {
			res = append(res, EntL3PortChannel(pChan.Name(), pChan.Description))
		}

		for _, member := range pChan.Members {
			res = append(res, EntPortChannelMember(pChan.Name(), member))
			res = append(res, EntInterfaceUP(member))
		}
	}

	for _, ifIP := range plan.InterfaceIPs {
		ip, ipNet, err := net.ParseCIDR(ifIP.IP)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse CIDR %s for %s", ifIP.IP, ifIP.Name)
		}
		prefixLen, _ := ipNet.Mask.Size()

		res = append(res, EntInterfaceIP(ifIP.Name, ip.String(), uint8(prefixLen)))
	}

	res = append(res, EntMCLAGDomain(plan.MCLAGDomain.ID, plan.MCLAGDomain.SourceIP, plan.MCLAGDomain.PeerIP, plan.MCLAGDomain.PeerLink))

	for _, member := range plan.MCLAGDomain.Members {
		res = append(res, EntMCLAGMember(plan.MCLAGDomain.ID, member))
	}

	for _, vpc := range plan.VPCs {
		res = append(res, EntVrf(vpc.Name))

		ip, ipNet, err := net.ParseCIDR(vpc.Subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse subnet %s for vpc %s", vpc.Subnet, vpc.Name)
		}
		prefixLen, _ := ipNet.Mask.Size()

		res = append(res, EntVLANInterface(vpc.VLAN, vpc.Name))
		res = append(res, EntVrfMember(vpc.Name, vpc.VLAN))
		res = append(res, EntVLANInterfaceConf(vpc.VLAN, ip.String(), uint8(prefixLen)))

		if vpc.DHCP {
			ip, _, err := net.ParseCIDR(vpc.DHCPRelay)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse DHCP relay %s for vpc %s", vpc.DHCPRelay, vpc.Name)
			}
			res = append(res, EntDHCPRelay(vpc.VLAN, ip.String(), vpc.DHCPSource))
		}
	}

	return res, nil
}

func (plan *Plan) ApplyWith(ctx context.Context, client *Client) error {
	entries, err := plan.Entries()
	if err != nil {
		return errors.Wrap(err, "failed to generate plan entries")
	}

	err = client.Set(context.Background(), entries...)
	if err != nil {
		return errors.Wrap(err, "failed to apply config")
	}

	return nil
}
