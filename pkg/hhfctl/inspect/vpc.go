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
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type VPCIn struct {
	Name   string
	Subnet string
}

type VPCOut struct {
	Name             string                                  `json:"name,omitempty"`
	Subnet           string                                  `json:"subnet,omitempty"`
	Spec             vpcapi.VPCSpec                          `json:"spec,omitempty"`
	VPCAttachments   map[string]*vpcapi.VPCAttachmentSpec    `json:"vpcAttachments,omitempty"`
	VPCPeerings      map[string]*vpcapi.VPCPeeringSpec       `json:"vpcPeerings,omitempty"`
	ExternalPeerings map[string]*vpcapi.ExternalPeeringSpec  `json:"externalPeerings,omitempty"`
	Access           map[string]*apiutil.ReachableFromSubnet `json:"access,omitempty"`
}

func (out *VPCOut) MarshalText(_ VPCIn, now time.Time) (string, error) {
	str := strings.Builder{}

	// TODO helper func
	str.WriteString(fmt.Sprintf("VRF Name (on all switches): VrfV%s\n", out.Name))

	str.WriteString(fmt.Sprintf("VLAN Namespace: %s\n", out.Spec.VLANNamespace))
	str.WriteString(fmt.Sprintf("IPv4 Namespace: %s\n", out.Spec.IPv4Namespace))

	str.WriteString("Subnets:\n")
	for subnetName, subnetSpec := range out.Spec.Subnets {
		if out.Subnet != "" && subnetName != out.Subnet {
			continue
		}

		str.WriteString(fmt.Sprintf("  %s:\n", subnetName))
		str.WriteString(fmt.Sprintf("    Subnet: %s\n", subnetSpec.Subnet))
		str.WriteString(fmt.Sprintf("    Gateway: %s\n", subnetSpec.Gateway))
		str.WriteString(fmt.Sprintf("    VLAN: %d\n", subnetSpec.VLAN))

		access, ok := out.Access[subnetName]
		if !ok {
			continue
		}

		if access.WithinSameSubnet == nil {
			str.WriteString("    Restricted (hosts can't reach each other within the subnet)\n")
		} else {
			str.WriteString("    Not restricted (hosts can reach each other within the subnet)\n")
		}

		if len(access.SameVPCSubnets) == 0 {
			if len(out.Spec.Subnets) > 1 {
				str.WriteString("    Isolated (no access to other subnets in the same VPC)\n")
			}
		} else {
			str.WriteString("    Reachable subnets in the same VPC:\n")
			for _, peerSubnet := range access.SameVPCSubnets {
				str.WriteString(fmt.Sprintf("      %s (%s)\n", peerSubnet.Name, peerSubnet.Subnet))
			}
		}

		if len(access.OtherVPCSubnets) == 0 {
			str.WriteString("    No access to other VPCs\n")
		} else {
			str.WriteString("    Reachable subnets in other VPCs:\n")
			for vpcName, peerSubnets := range access.OtherVPCSubnets {
				str.WriteString(fmt.Sprintf("      vpc %s:\n", vpcName))
				for _, peerSubnet := range peerSubnets {
					str.WriteString(fmt.Sprintf("        %s (%s)\n", peerSubnet.Name, peerSubnet.Subnet))
				}
			}
		}

		attaches := []string{}
		for attachName, attachSpec := range out.VPCAttachments {
			subnet := strings.SplitN(attachSpec.Subnet, "/", 2)[1]
			if subnet != subnetName {
				continue
			}

			vlan := ""
			if attachSpec.NativeVLAN {
				vlan = fmt.Sprintf(" (native VLAN %d)", subnetSpec.VLAN)
			}

			attaches = append(attaches, fmt.Sprintf("%s: %s%s", attachName, attachSpec.Connection, vlan))
		}

		if len(attaches) > 0 {
			str.WriteString("    Attachements:\n")
			for _, attach := range attaches {
				str.WriteString(fmt.Sprintf("      %s\n", attach))
			}
		} else {
			str.WriteString("    Not attached to any connection\n")
		}

		str.WriteString("\n")
	}

	return str.String(), nil
}

var _ Func[VPCIn, *VPCOut] = VPC

func VPC(ctx context.Context, kube kclient.Reader, in VPCIn) (*VPCOut, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}

	name := in.Name
	subnet := in.Subnet

	out := &VPCOut{
		Name:             name,
		Subnet:           subnet,
		VPCAttachments:   map[string]*vpcapi.VPCAttachmentSpec{},
		VPCPeerings:      map[string]*vpcapi.VPCPeeringSpec{},
		ExternalPeerings: map[string]*vpcapi.ExternalPeeringSpec{},
		Access:           map[string]*apiutil.ReachableFromSubnet{},
	}

	vpc := &vpcapi.VPC{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, vpc); err != nil {
		return nil, errors.Wrap(err, "failed to get VPC")
	}

	if subnet != "" {
		if _, exist := vpc.Spec.Subnets[subnet]; !exist {
			return nil, errors.Errorf("subnet %q not found in VPC %q", subnet, name)
		}
	}

	out.Spec = vpc.Spec

	vpcAttaches := &vpcapi.VPCAttachmentList{}
	if err := kube.List(ctx, vpcAttaches, kclient.MatchingLabels{
		vpcapi.LabelVPC: name,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list VPC attachments")
	}

	for _, vpcAttach := range vpcAttaches.Items {
		attachSubnet := strings.SplitN(vpcAttach.Spec.Subnet, "/", 2)[1]

		if subnet != "" && attachSubnet != subnet {
			continue
		}

		out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)
	}

	vpcPeerings := &vpcapi.VPCPeeringList{}
	if err := kube.List(ctx, vpcPeerings, kclient.MatchingLabels{
		vpcapi.ListLabelVPC(name): vpcapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list VPC peerings")
	}

	for _, vpcPeering := range vpcPeerings.Items {
		if subnet != "" {
			found := false
			for _, permit := range vpcPeering.Spec.Permit {
				if peer, exist := permit[name]; exist && slices.Contains(peer.Subnets, subnet) {
					found = true

					break
				}
			}

			if !found {
				continue
			}
		}

		out.VPCPeerings[vpcPeering.Name] = pointer.To(vpcPeering.Spec)
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := kube.List(ctx, extPeerings, kclient.MatchingLabels{
		vpcapi.LabelVPC: name,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list external peerings")
	}

	for _, extPeering := range extPeerings.Items {
		if subnet != "" && !slices.Contains(extPeering.Spec.Permit.VPC.Subnets, subnet) {
			continue
		}

		out.ExternalPeerings[extPeering.Name] = pointer.To(extPeering.Spec)
	}

	access, err := apiutil.GetReachableFrom(ctx, kube, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reachable from vpc")
	}

	for subnetName, subnetAccess := range access {
		if subnet != "" && subnetName != subnet {
			continue
		}

		out.Access[subnetName] = subnetAccess
	}

	return out, nil
}
