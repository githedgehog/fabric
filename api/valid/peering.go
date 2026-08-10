// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package valid

import (
	"context"
	"fmt"
	"slices"
	"strings"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ meta.PeeringValidator = Peering

func Peering(ctx context.Context, kube kclient.Reader, checkPeering kclient.Object) error {
	checkName := checkPeering.GetName()
	checkNs := checkPeering.GetNamespace()

	var vpcs, exts []string
	switch peering := any(checkPeering).(type) {
	case *vpcapi.VPCPeering:
		vpc1, vpc2, err := peering.Spec.VPCs()
		if err != nil {
			return fmt.Errorf("invalid VPCPeering %q: %w", checkName, err)
		}
		vpcs = []string{vpc1, vpc2}
	case *vpcapi.ExternalPeering:
		vpcs = []string{peering.Spec.Permit.VPC.Name}
		exts = []string{peering.Spec.Permit.External.Name}
	case *gwapi.GatewayPeering:
		for vpc := range peering.Spec.Peering {
			if ext, ok := strings.CutPrefix(vpc, gwapi.VPCInfoExtPrefix); ok {
				exts = append(exts, ext)
			} else {
				vpcs = append(vpcs, vpc)
			}
		}
	default:
		return fmt.Errorf("unexpected peering type %T for %q", checkPeering, checkName) //nolint:err113
	}

	if len(vpcs)+len(exts) != 2 {
		return fmt.Errorf("invalid peering %q: expected exactly 2 unique vpcs/externals, got %d + %d", checkName, len(vpcs), len(exts)) //nolint:err113
	}

	slices.Sort(vpcs)
	slices.Sort(exts)

	// VPCPeerings are always between 2 VPCs
	if len(vpcs) == 2 {
		vpcPeerings := &vpcapi.VPCPeeringList{}
		if err := kube.List(ctx, vpcPeerings, kclient.MatchingLabels{
			vpcapi.ListLabelVPC(vpcs[0]): vpcapi.ListLabelValue,
			vpcapi.ListLabelVPC(vpcs[1]): vpcapi.ListLabelValue,
		}); err != nil {
			return fmt.Errorf("failed to list VPCPeerings: %w", err)
		}
		_, checkSelf := any(checkPeering).(*vpcapi.VPCPeering)
		for _, peer := range vpcPeerings.Items {
			if checkSelf && peer.Name == checkName && peer.Namespace == checkNs {
				continue
			}

			return fmt.Errorf("VPCPeering %q already exists for VPCs %s and %s", peer.Name, vpcs[0], vpcs[1]) //nolint:err113
		}
	}

	// ExternalPeerings are always between 1 VPC and 1 External
	if len(vpcs) == 1 && len(exts) == 1 {
		extPeerings := &vpcapi.ExternalPeeringList{}
		if err := kube.List(ctx, extPeerings, kclient.MatchingLabels{
			vpcapi.LabelVPC:      vpcs[0],
			vpcapi.LabelExternal: exts[0],
		}); err != nil {
			return fmt.Errorf("failed to list ExternalPeerings: %w", err)
		}
		_, checkSelf := any(checkPeering).(*vpcapi.ExternalPeering)
		for _, peer := range extPeerings.Items {
			if checkSelf && peer.Name == checkName && peer.Namespace == checkNs {
				continue
			}

			return fmt.Errorf("ExternalPeering %q already exists for VPC %s and External %s", peer.Name, vpcs[0], exts[0]) //nolint:err113
		}
	}

	// GatewayPeerings could be for any combination of VPCs and Externals
	{
		gwPeers := &gwapi.GatewayPeeringList{}
		labels := kclient.MatchingLabels{}
		for _, vpc := range vpcs {
			labels[gwapi.ListLabelVPC(vpc)] = gwapi.ListLabelValue
		}
		for _, ext := range exts {
			labels[gwapi.ListLabelVPC(gwapi.VPCInfoExtPrefix+ext)] = gwapi.ListLabelValue
		}
		if err := kube.List(ctx, gwPeers, labels); err != nil {
			return fmt.Errorf("failed to list GatewayPeerings: %w", err)
		}
		_, checkSelf := any(checkPeering).(*gwapi.GatewayPeering)
		for _, peer := range gwPeers.Items {
			if checkSelf && peer.Name == checkName && peer.Namespace == checkNs {
				continue
			}

			return fmt.Errorf("GatewayPeering %q already exists for VPCs %s", peer.Name, strings.Join(append(vpcs, exts...), ", ")) //nolint:err113
		}
	}

	return nil
}
