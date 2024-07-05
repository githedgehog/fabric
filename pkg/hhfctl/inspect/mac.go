package inspect

import (
	"context"
	"net"
	"slices"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MACIn struct {
	Value string
}

type MACOut struct {
	Ports      []string           `json:"ports,omitempty"`
	DHCPLeases []*MACOutDHCPLease `json:"dhcpLeases,omitempty"`
}

type MACOutDHCPLease struct {
	Subnet   string      `json:"subnet,omitempty"`
	Expiry   metav1.Time `json:"expiry,omitempty"`
	Hostname string      `json:"hostname,omitempty"`
}

func (out *MACOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[MACIn, *MACOut] = MAC

func MAC(ctx context.Context, kube client.Reader, in MACIn) (*MACOut, error) {
	out := &MACOut{}

	mac := strings.ToLower(in.Value)
	if _, err := net.ParseMAC(mac); err != nil {
		return nil, errors.Wrapf(err, "invalid MAC address %s", mac)
	}

	agents := &agentapi.AgentList{}
	if err := kube.List(ctx, agents); err != nil {
		return nil, errors.Wrap(err, "cannot list agents")
	}

	for _, agent := range agents.Items {
		for ifaceName, iface := range agent.Status.State.Interfaces {
			if iface.MAC != "" && strings.ToLower(iface.MAC) == mac {
				out.Ports = append(out.Ports, agent.Name+"/"+ifaceName)
			}
		}
	}

	slices.Sort(out.Ports)

	dhcpSubnets := &dhcpapi.DHCPSubnetList{}
	if err := kube.List(ctx, dhcpSubnets); err != nil {
		return nil, errors.Wrap(err, "cannot list DHCP subnets")
	}

	for _, subnet := range dhcpSubnets.Items {
		if lease, exists := subnet.Status.Allocated[mac]; exists {
			out.DHCPLeases = append(out.DHCPLeases, &MACOutDHCPLease{
				Subnet:   subnet.Spec.Subnet,
				Expiry:   lease.Expiry,
				Hostname: lease.Hostname,
			})
		}
	}

	slices.SortFunc(out.DHCPLeases, func(a, b *MACOutDHCPLease) int {
		if a.Subnet == b.Subnet {
			return strings.Compare(a.Hostname, b.Hostname)
		}

		return strings.Compare(a.Subnet, b.Subnet)
	})

	return out, nil
}
