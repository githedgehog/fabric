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
	"net"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IPIn struct {
	IP string
}

type IPOut struct {
	found         bool
	IPv4Namespace *string           `json:"ipv4Namespace,omitempty"`
	VPCSubnet     *IPOutVPCSubnet   `json:"vpcSubnet,omitempty"`
	Switch        *IPOutSwitch      `json:"switch,omitempty"`
	Connections   []IPOutConnection `json:"connections,omitempty"`

	// TODO do we need to try searching for IPs available through externals?
	// ExternalPeerings []IPOutExternalPeering `json:"externalPeerings,omitempty"`
}

type IPOutVPCSubnet struct {
	Name             string `json:"name,omitempty"`
	vpcapi.VPCSubnet `json:",inline"`
	DHCPLease        *IPOutDHCPLease `json:"dhcpLease,omitempty"`
}

type IPOutSwitch struct {
	Name       string  `json:"name,omitempty"`
	IP         *string `json:"ip,omitempty"`
	VTEPIP     *string `json:"vtepIP,omitempty"`
	ProtocolIP *string `json:"protocolIP,omitempty"`
}

type IPOutDHCPLease struct {
	MAC      string      `json:"mac,omitempty"`
	Expiry   metav1.Time `json:"expiry,omitempty"`
	Hostname string      `json:"hostname,omitempty"`
}

type IPOutConnection struct {
	Name                     string `json:"name,omitempty"`
	wiringapi.ConnectionSpec `json:",inline"`
}

// type IPOutExternalPeering struct {
// 	Name                       string `json:"name,omitempty"`
// 	vpcapi.ExternalPeeringSpec `json:",inline"`
// }

func (out *IPOut) MarshalText() (string, error) {
	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[IPIn, *IPOut] = IP

func IP(ctx context.Context, kube client.Reader, in IPIn) (*IPOut, error) {
	ip := net.ParseIP(in.IP)
	if ip == nil {
		return nil, errors.Errorf("invalid IP address: %s", in.IP)
	}

	if ip.To4() == nil {
		return nil, errors.Errorf("only valid IPv4 address is supported: %s", in.IP)
	}

	out := &IPOut{}

	if err := ipInIPNS(ctx, out, kube, ip); err != nil {
		return nil, errors.Wrap(err, "failed to inspect IP in IPv4Namespaces and VPCs")
	}

	if err := ipInSwitches(ctx, out, kube, ip); err != nil {
		return nil, errors.Wrap(err, "failed to inspect IP in Switches")
	}

	if err := ipInConnections(ctx, out, kube, ip); err != nil {
		return nil, errors.Wrap(err, "failed to inspect IP in Connections")
	}

	// if err := ipInExternal(ctx, out, kube, ip); err != nil {
	// 	return nil, errors.Wrap(err, "failed to inspect IP in ExternalPeerings")
	// }

	return out, nil
}

func ipInIPNS(ctx context.Context, res *IPOut, kube client.Reader, ip net.IP) error {
	if res.found {
		return nil
	}

	ipnsList := &vpcapi.IPv4NamespaceList{}
	err := kube.List(ctx, ipnsList)
	if err != nil {
		return errors.Wrap(err, "cannot list IPv4Namespace")
	}

	for _, ipns := range ipnsList.Items {
		for _, subnetStr := range ipns.Spec.Subnets {
			_, subnetNet, err := net.ParseCIDR(subnetStr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse ipns %s subnet %q", ipns.Name, subnetStr)
			}

			if subnetNet.Contains(ip) {
				res.IPv4Namespace = pointer.To(ipns.Name)

				vpcs := &vpcapi.VPCList{}
				err = kube.List(ctx, vpcs, client.MatchingLabels{
					vpcapi.LabelIPv4NS: ipns.Name,
				})
				if err != nil {
					return errors.Wrap(err, "cannot list VPC")
				}

				for _, vpc := range vpcs.Items {
					for subnetName, subnet := range vpc.Spec.Subnets {
						_, subnetNet, err := net.ParseCIDR(subnet.Subnet)
						if err != nil {
							return errors.Wrapf(err, "failed to parse vpc %s subnet %q", vpc.Name, subnet.Subnet)
						}

						if subnetNet.Contains(ip) {
							res.found = true
							res.VPCSubnet = &IPOutVPCSubnet{
								Name:      vpc.Name + "/" + subnetName,
								VPCSubnet: *subnet,
							}

							if subnet.DHCP.Enable {
								dhcpSubnet := &dhcpapi.DHCPSubnet{}
								err = kube.Get(ctx, client.ObjectKey{Name: vpc.Name + "--" + subnetName, Namespace: metav1.NamespaceDefault}, dhcpSubnet)
								if err != nil {
									return errors.Wrapf(err, "failed to get DHCPSubnet %s", vpc.Name+"-"+subnetName)
								}

								ipStr := ip.String()
								for mac, lease := range dhcpSubnet.Status.Allocated {
									if lease.IP == ipStr {
										res.VPCSubnet.DHCPLease = &IPOutDHCPLease{
											MAC:      mac,
											Expiry:   lease.Expiry,
											Hostname: lease.Hostname,
										}

										break
									}
								}
							}

							return nil
						}
					}
				}

				return nil
			}
		}
	}

	return nil
}

func ipInSwitches(ctx context.Context, res *IPOut, kube client.Reader, ip net.IP) error {
	if res.found {
		return nil
	}

	sws := &wiringapi.SwitchList{}
	err := kube.List(ctx, sws)
	if err != nil {
		return errors.Wrap(err, "cannot list Switch")
	}

	for _, sw := range sws.Items {
		if strings.SplitN(sw.Spec.IP, "/", 2)[0] == ip.String() {
			res.found = true
			res.Switch = &IPOutSwitch{
				Name: sw.Name,
				IP:   pointer.To(sw.Spec.IP),
			}

			break
		} else if strings.SplitN(sw.Spec.VTEPIP, "/", 2)[0] == ip.String() {
			res.found = true
			res.Switch = &IPOutSwitch{
				Name:   sw.Name,
				VTEPIP: pointer.To(sw.Spec.VTEPIP),
			}

			break
		} else if strings.SplitN(sw.Spec.ProtocolIP, "/", 2)[0] == ip.String() {
			res.found = true
			res.Switch = &IPOutSwitch{
				Name:       sw.Name,
				ProtocolIP: pointer.To(sw.Spec.ProtocolIP),
			}

			break
		}
	}

	return nil
}

func ipInConnections(ctx context.Context, res *IPOut, kube client.Reader, ip net.IP) error {
	if res.found {
		return nil
	}

	conns := &wiringapi.ConnectionList{}
	err := kube.List(ctx, conns)
	if err != nil {
		return errors.Wrap(err, "cannot list Connection")
	}

	for _, conn := range conns.Items {
		ips := []string{}
		subnets := []string{}

		if conn.Spec.Management != nil {
			ips = append(ips, conn.Spec.Management.Link.Server.IP, conn.Spec.Management.Link.Switch.IP)
		} else if conn.Spec.Fabric != nil {
			for _, link := range conn.Spec.Fabric.Links {
				ips = append(ips, link.Spine.IP, link.Leaf.IP)
			}
		} else if conn.Spec.StaticExternal != nil {
			ips = append(ips, conn.Spec.StaticExternal.Link.Switch.IP, conn.Spec.StaticExternal.Link.Switch.NextHop)
			subnets = append(subnets, conn.Spec.StaticExternal.Link.Switch.Subnets...)
		}

		for _, ipStr := range ips {
			if strings.SplitN(ipStr, "/", 2)[0] == ip.String() {
				res.found = true
				res.Connections = append(res.Connections, IPOutConnection{
					Name:           conn.Name,
					ConnectionSpec: conn.Spec,
				})
			}
		}

		for _, subnetStr := range subnets {
			_, subnetNet, err := net.ParseCIDR(subnetStr)
			if err != nil {
				return errors.Wrapf(err, "failed to parse connection %s subnet %q", conn.Name, subnetStr)
			}

			if subnetNet.Contains(ip) {
				res.found = true
				res.Connections = append(res.Connections, IPOutConnection{
					Name:           conn.Name,
					ConnectionSpec: conn.Spec,
				})
			}
		}
	}

	return nil
}

// func ipInExternal(ctx context.Context, res *IPOut, kube client.Reader, ip net.IP) error {
// 	if res.found {
// 		return nil
// 	}

// 	extPeerings := &vpcapi.ExternalPeeringList{}
// 	err := kube.List(ctx, extPeerings)
// 	if err != nil {
// 		return errors.Wrap(err, "cannot list ExternalPeering")
// 	}

// 	for _, extPeering := range extPeerings.Items {
// 		for _, prefix := range extPeering.Spec.Permit.External.Prefixes {
// 			_, prefixNet, err := net.ParseCIDR(prefix.Prefix)
// 			if err != nil {
// 				return errors.Wrapf(err, "failed to parse external peering %s prefix %q", extPeering.Name, prefix)
// 			}

// 			if prefixNet.Contains(ip) {
// 				res.found = true
// 				res.ExternalPeerings = append(res.ExternalPeerings, IPOutExternalPeering{
// 					Name:                extPeering.Name,
// 					ExternalPeeringSpec: extPeering.Spec,
// 				})
// 			}
// 		}
// 	}

// 	return nil
// }