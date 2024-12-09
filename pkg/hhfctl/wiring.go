// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package hhfctl

import (
	"context"
	"fmt"
	"os"

	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
)

type WiringExportOptions struct {
	VPCs           bool
	Externals      bool
	SwitchProfiles bool
}

func WiringExport(ctx context.Context, opts WiringExportOptions) error {
	kube, err := kubeutil.NewClient(ctx, "", wiringapi.SchemeBuilder, vpcapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	out := os.Stdout
	objs := new(int)

	if opts.SwitchProfiles {
		if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.SwitchProfileList{}, objs); err != nil {
			return fmt.Errorf("printing switch profiles: %w", err)
		}
	}

	if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.VLANNamespaceList{}, objs); err != nil {
		return fmt.Errorf("printing vlan namespaces: %w", err)
	}

	if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.SwitchGroupList{}, objs); err != nil {
		return fmt.Errorf("printing switch groups: %w", err)
	}

	if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.SwitchList{}, objs); err != nil {
		return fmt.Errorf("printing switches: %w", err)
	}

	if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.ServerList{}, objs); err != nil {
		return fmt.Errorf("printing servers: %w", err)
	}

	if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.ConnectionList{}, objs); err != nil {
		return fmt.Errorf("printing connections: %w", err)
	}

	if err := kubeutil.PrintObjectList(ctx, kube, out, &wiringapi.ServerProfileList{}, objs); err != nil {
		return fmt.Errorf("printing server profiles: %w", err)
	}

	if opts.VPCs || opts.Externals {
		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.IPv4NamespaceList{}, objs); err != nil {
			return fmt.Errorf("printing ipv4 namespaces: %w", err)
		}
	}

	if opts.VPCs {
		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.VPCList{}, objs); err != nil {
			return fmt.Errorf("printing vpcs: %w", err)
		}

		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.VPCAttachmentList{}, objs); err != nil {
			return fmt.Errorf("printing vpc attachments: %w", err)
		}

		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.VPCPeeringList{}, objs); err != nil {
			return fmt.Errorf("printing vpc peerings: %w", err)
		}
	}

	if opts.Externals {
		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.ExternalList{}, objs); err != nil {
			return fmt.Errorf("printing externals: %w", err)
		}

		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.ExternalAttachmentList{}, objs); err != nil {
			return fmt.Errorf("printing external attachments: %w", err)
		}

		if err := kubeutil.PrintObjectList(ctx, kube, out, &vpcapi.ExternalPeeringList{}, objs); err != nil {
			return fmt.Errorf("printing external peerings: %w", err)
		}
	}

	return nil
}
