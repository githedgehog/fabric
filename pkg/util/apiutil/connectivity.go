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

package apiutil

import (
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO expose how exactly source can reach dest (which port, bond?, vlan, etc)
// type TargetReachableOn struct {
// 	Connection string
// 	Interfaces []string
// 	VLAN       uint16
// }

func IsServerReachable(ctx context.Context, kube client.Client, sourceServer, destServer string) (bool, error) {
	sourceSubnets, err := GetAttachedSubnets(ctx, kube, sourceServer)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get attached subnets for server %s", sourceServer)
	}

	destSubnets, err := GetAttachedSubnets(ctx, kube, destServer)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get attached subnets for server %s", destServer)
	}

	for sourceSubnetName := range sourceSubnets {
		for destSubnetName := range destSubnets {
			reachable, err := IsSubnetReachable(ctx, kube, sourceSubnetName, destSubnetName)
			if err != nil {
				return false, err
			}

			if reachable { // TODO return list of ways to reach
				return true, nil
			}
		}
	}

	return false, nil
}

func IsSubnetReachable(ctx context.Context, kube client.Client, source, dest string) (bool, error) {
	sourceParts := strings.SplitN(source, "/", 2)
	destParts := strings.SplitN(dest, "/", 2)

	sourceVPC, sourceSubnet := sourceParts[0], sourceParts[1]
	destVPC, destSubnet := destParts[0], destParts[1]

	if sourceVPC == destVPC {
		return IsSubnetReachableWithinVPC(ctx, kube, sourceVPC, sourceSubnet, destSubnet)
	}

	return IsSubnetReachableBetweenVPCs(ctx, kube, sourceVPC, sourceSubnet, destVPC, destSubnet)
}

func IsSubnetReachableWithinVPC(ctx context.Context, kube client.Client, vpcName, source, dest string) (bool, error) {
	vpc := vpcapi.VPC{}
	if err := kube.Get(ctx, client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      vpcName,
	}, &vpc); err != nil {
		return false, errors.Wrapf(err, "failed to get VPC %s", vpcName)
	}

	if vpc.Spec.Subnets[source] == nil {
		return false, errors.Errorf("source subnet %s not found in VPC %s", source, vpcName)
	}
	if vpc.Spec.Subnets[dest] == nil {
		return false, errors.Errorf("destination subnet %s not found in VPC %s", dest, vpcName)
	}

	if source == dest {
		return !vpc.Spec.IsSubnetRestricted(source), nil
	}

	if !vpc.Spec.IsSubnetIsolated(source) && !vpc.Spec.IsSubnetIsolated(dest) {
		return true, nil
	}

	for _, permit := range vpc.Spec.Permit {
		if slices.Contains(permit, source) && slices.Contains(permit, dest) {
			return true, nil
		}
	}

	return false, nil
}

func IsSubnetReachableBetweenVPCs(ctx context.Context, kube client.Client, vpc1Name, vpc1Subnet, vpc2Name, vpc2Subnet string) (bool, error) {
	if vpc1Name == vpc2Name {
		return false, errors.Errorf("VPCs %s and %s are the same", vpc1Name, vpc2Name)
	}

	vpc1 := vpcapi.VPC{}
	if err := kube.Get(ctx, client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      vpc1Name,
	}, &vpc1); err != nil {
		return false, errors.Wrapf(err, "failed to get VPC %s", vpc1Name)
	}

	vpc2 := vpcapi.VPC{}
	if err := kube.Get(ctx, client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      vpc2Name,
	}, &vpc2); err != nil {
		return false, errors.Wrapf(err, "failed to get VPC %s", vpc2Name)
	}

	if vpc1.Spec.Subnets[vpc1Subnet] == nil {
		return false, errors.Errorf("source subnet %s not found in VPC %s", vpc1Subnet, vpc1Name)
	}
	if vpc2.Spec.Subnets[vpc2Subnet] == nil {
		return false, errors.Errorf("destination subnet %s not found in VPC %s", vpc2Subnet, vpc2Name)
	}

	vpcPeerings := vpcapi.VPCPeeringList{}
	if err := kube.List(ctx, &vpcPeerings,
		client.InNamespace(metav1.NamespaceDefault),
		client.MatchingLabels{
			vpcapi.ListLabelVPC(vpc1Name): vpcapi.ListLabelValue,
			vpcapi.ListLabelVPC(vpc2Name): vpcapi.ListLabelValue,
		},
	); err != nil {
		return false, errors.Wrapf(err, "failed to list VPC peerings")
	}

	for _, vpcPeering := range vpcPeerings.Items {
		if vpcPeering.Spec.Remote != "" {
			if err := kube.Get(ctx, client.ObjectKey{
				Namespace: metav1.NamespaceDefault,
				Name:      vpcPeering.Spec.Remote,
			}, &wiringapi.SwitchGroup{}); err != nil {
				return false, errors.Wrapf(err, "failed to get switch group %s", vpcPeering.Spec.Remote)
			}

			switches := wiringapi.SwitchList{}
			if err := kube.List(ctx, &switches,
				client.InNamespace(metav1.NamespaceDefault),
				wiringapi.MatchingLabelsForSwitchGroup(vpcPeering.Spec.Remote),
			); err != nil {
				return false, errors.Wrapf(err, "failed to list switches")
			}

			if len(switches.Items) == 0 {
				return false, nil
			}
		}

		for _, permit := range vpcPeering.Spec.Permit {
			vpc1Permit, exist := permit[vpc1Name]
			if !exist {
				continue
			}

			vpc2Permit, exist := permit[vpc2Name]
			if !exist {
				continue
			}

			vpc1SubnetContains := len(vpc1Permit.Subnets) == 0 || slices.Contains(vpc1Permit.Subnets, vpc1Subnet)
			vpc2SubnetContains := len(vpc2Permit.Subnets) == 0 || slices.Contains(vpc2Permit.Subnets, vpc2Subnet)

			if vpc1SubnetContains && vpc2SubnetContains {
				return true, nil
			}
		}
	}

	return false, nil
}

// TODO check if allowed prefix contains destSubnet
func IsExternalSubnetReachable(ctx context.Context, kube client.Client, sourceServer, destSubnet string) (bool, error) {
	sourceSubnets, err := GetAttachedSubnets(ctx, kube, sourceServer)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get attached subnets for server %s", sourceServer)
	}

	for subnetName := range sourceSubnets {
		sourceParts := strings.SplitN(subnetName, "/", 2)
		sourceVPC, sourceSubnet := sourceParts[0], sourceParts[1]

		extPeerings := vpcapi.ExternalPeeringList{}
		if err := kube.List(ctx, &extPeerings,
			client.InNamespace(metav1.NamespaceDefault),
			client.MatchingLabels{
				vpcapi.LabelVPC: sourceVPC,
			},
		); err != nil {
			return false, errors.Wrapf(err, "failed to list external peerings")
		}

		for _, extPeering := range extPeerings.Items {
			if !slices.Contains(extPeering.Spec.Permit.VPC.Subnets, sourceSubnet) {
				continue
			}

			for _, prefix := range extPeering.Spec.Permit.External.Prefixes {
				if prefix.Prefix != destSubnet {
					continue
				}

				extAttaches := vpcapi.ExternalAttachmentList{}
				if err := kube.List(ctx, &extAttaches,
					client.InNamespace(metav1.NamespaceDefault),
					client.MatchingLabels{
						vpcapi.LabelExternal: extPeering.Spec.Permit.External.Name,
					},
				); err != nil {
					return false, errors.Wrapf(err, "failed to list external attachments")
				}

				if len(extAttaches.Items) == 0 {
					return false, nil
				}

				return true, nil
			}
		}
	}

	return false, nil
}

type ServerAttachment struct {
	Connection string
	Interfaces []string
	NativeVLAN bool
}

func GetAttachedSubnets(ctx context.Context, kube client.Client, server string) (map[string]ServerAttachment, error) {
	ret := map[string]ServerAttachment{}

	srv := wiringapi.Server{}
	if err := kube.Get(ctx, client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      server,
	}, &srv); err != nil {
		return nil, errors.Wrapf(err, "failed to get server %s", server)
	}

	if srv.IsControl() {
		return nil, errors.Errorf("server %s is a control node", server)
	}

	conns := wiringapi.ConnectionList{}
	if err := kube.List(ctx, &conns,
		client.InNamespace(metav1.NamespaceDefault),
		wiringapi.MatchingLabelsForListLabelServer(server),
	); err != nil {
		return nil, errors.Wrapf(err, "failed to list connections for server %s", server)
	}

	for _, conn := range conns.Items {
		_, _, ports, _, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get endpoints for connection %s of server %s", conn.Name, server)
		}
		serverPrefix := server + "/"
		ifaces := []string{}
		for _, port := range ports {
			if !strings.HasPrefix(port, serverPrefix) {
				continue
			}

			ifaces = append(ifaces, port)
		}

		vpcAttaches := vpcapi.VPCAttachmentList{}
		if err := kube.List(ctx, &vpcAttaches,
			client.InNamespace(metav1.NamespaceDefault),
			client.MatchingLabels{
				wiringapi.LabelConnection: conn.Name,
			},
		); err != nil {
			return nil, errors.Wrapf(err, "failed to list VPC attachments for connection %s of server %s", conn.Name, server)
		}

		for _, vpcAttach := range vpcAttaches.Items {
			ret[vpcAttach.Spec.Subnet] = ServerAttachment{
				Connection: conn.Name,
				Interfaces: ifaces,
				NativeVLAN: vpcAttach.Spec.NativeVLAN,
			}
		}
	}

	return ret, nil
}
