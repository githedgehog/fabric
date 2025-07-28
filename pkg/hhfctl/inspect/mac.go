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

package inspect

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type MACIn struct {
	Value string
}

type MACOut struct {
	Ports      []string           `json:"ports,omitempty"`
	DHCPLeases []*MACOutDHCPLease `json:"dhcpLeases,omitempty"`
}

type MACOutDHCPLease struct {
	Subnet   string       `json:"subnet,omitempty"`
	Expiry   kmetav1.Time `json:"expiry,omitempty"`
	Hostname string       `json:"hostname,omitempty"`
}

func (out *MACOut) MarshalText(_ MACIn, now time.Time) (string, error) {
	str := strings.Builder{}

	if len(out.Ports) > 0 {
		str.WriteString("Ports:\n")

		for _, port := range out.Ports {
			str.WriteString("  " + port + "\n")
		}
	}

	if len(out.DHCPLeases) > 0 {
		for _, lease := range out.DHCPLeases {
			str.WriteString(fmt.Sprintf("DHCP Lease for VPC Subnet %s:\n  Hostname: %s\n  Expiry: %s (%s)\n", lease.Subnet, lease.Hostname, lease.Expiry, HumanizeTime(now, lease.Expiry.Time)))
		}
	}

	return str.String(), nil
}

var _ Func[MACIn, *MACOut] = MAC

func MAC(ctx context.Context, kube kclient.Reader, in MACIn) (*MACOut, error) {
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
