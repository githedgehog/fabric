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

package vpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) updateDHCPSubnets(ctx context.Context, vpc *vpcapi.VPC) error {
	err := r.deleteDHCPSubnets(ctx, client.ObjectKey{Name: vpc.Name, Namespace: vpc.Namespace}, vpc.Spec.Subnets)
	if err != nil {
		return errors.Wrapf(err, "error deleting obsolete dhcp subnets")
	}

	for subnetName, subnet := range vpc.Spec.Subnets {
		if !subnet.DHCP.Enable || subnet.VLAN == 0 || subnet.DHCP.Range == nil {
			continue
		}

		dhcp := &dhcpapi.DHCPSubnet{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s--%s", vpc.Name, subnetName), Namespace: vpc.Namespace}}
		_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, dhcp, func() error {
			pxeURL := ""
			dnsServers := []string{}
			timeServers := []string{}
			mtu := uint16(9036) // TODO constant

			if subnet.DHCP.Options != nil {
				pxeURL = subnet.DHCP.Options.PXEURL
				dnsServers = subnet.DHCP.Options.DNSServers
				timeServers = subnet.DHCP.Options.TimeServers

				if subnet.DHCP.Options.InterfaceMTU > 0 {
					mtu = subnet.DHCP.Options.InterfaceMTU
				}
			}

			dhcp.Labels = map[string]string{
				vpcapi.LabelVPC:    vpc.Name,
				vpcapi.LabelSubnet: subnetName,
			}
			dhcp.Spec = dhcpapi.DHCPSubnetSpec{
				Subnet:       fmt.Sprintf("%s/%s", vpc.Name, subnetName),
				CIDRBlock:    subnet.Subnet,
				Gateway:      subnet.Gateway,
				StartIP:      subnet.DHCP.Range.Start,
				EndIP:        subnet.DHCP.Range.End,
				VRF:          fmt.Sprintf("VrfV%s", vpc.Name),    // TODO move to utils
				CircuitID:    fmt.Sprintf("Vlan%d", subnet.VLAN), // TODO move to utils
				PXEURL:       pxeURL,
				DNSServers:   dnsServers,
				TimeServers:  timeServers,
				InterfaceMTU: mtu,
			}

			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "error creating dhcp subnet for %s/%s", vpc.Name, subnetName)
		}
	}

	return nil
}

func (r *Reconciler) deleteDHCPSubnets(ctx context.Context, vpcKey client.ObjectKey, subnets map[string]*vpcapi.VPCSubnet) error {
	dhcpSubnets := &dhcpapi.DHCPSubnetList{}
	err := r.List(ctx, dhcpSubnets, client.MatchingLabels{vpcapi.LabelVPC: vpcKey.Name})
	if err != nil {
		return errors.Wrapf(err, "error listing dhcp subnets")
	}

	for _, subnet := range dhcpSubnets.Items {
		subnetName := "default"
		parts := strings.Split(subnet.Spec.Subnet, "/")
		if len(parts) == 2 {
			subnetName = parts[1]
		}

		if _, exists := subnets[subnetName]; exists {
			continue
		}

		err = r.Delete(ctx, &subnet)
		if client.IgnoreNotFound(err) != nil {
			return errors.Wrapf(err, "error deleting dhcp subnet %s", subnet.Name)
		}
	}

	return nil
}
