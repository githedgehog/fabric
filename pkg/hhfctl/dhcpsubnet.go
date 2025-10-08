// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package hhfctl

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type DHCPSubnetCleanupOptions struct {
	VPC       string
	Subnet    string
	OlderThan string
	DryRun    bool
}

func DHCPSubnetCleanup(ctx context.Context, options DHCPSubnetCleanupOptions) error {
	olderThan, err := time.ParseDuration(options.OlderThan)
	if err != nil {
		return errors.Wrap(err, "cannot parse older than duration")
	}

	slog.Info("Cleaning up DHCP leases", "vpc", options.VPC, "subnet", options.Subnet, "older", olderThan)

	if options.DryRun {
		slog.Info("Dry-run so no leases will be removed")
	}

	kube, err := kubeutil.NewClient(ctx, "",
		vpcapi.SchemeBuilder, dhcpapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	dhcpSubnet := &dhcpapi.DHCPSubnet{}
	name := fmt.Sprintf("%s--%s", options.VPC, options.Subnet)
	if err := kube.Get(ctx, kclient.ObjectKey{Namespace: "default", Name: name}, dhcpSubnet); err != nil {
		return errors.Wrap(err, "cannot get DHCP subnet")
	}

	changed := false
	for mac, lease := range dhcpSubnet.Status.Allocated {
		if time.Since(lease.Expiry.Time) > olderThan {
			slog.Debug("Lease to be removed", "mac", mac, "expiry", lease.Expiry.Time, "hostname", lease.Hostname)
			if !options.DryRun {
				delete(dhcpSubnet.Status.Allocated, mac)
				changed = true
			}
		}
	}

	if !options.DryRun {
		if changed {
			if err := kube.Status().Update(ctx, dhcpSubnet); err != nil {
				return errors.Wrap(err, "cannot update DHCP subnet")
			}
			slog.Info("Leases removed")
		} else {
			slog.Info("No leases to remove")
		}
	}

	return nil
}
