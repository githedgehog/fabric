// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

func (s *Server) startPeriodicCleanup(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(60) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.mu.RLock()
			subnets := slices.Collect(maps.Values(s.subnets))
			s.mu.RUnlock()

			for _, sn := range subnets {
				if err := sn.persist(ctx, s.kube, func(*dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
					return cleanup, nil
				}); err != nil && !errors.Is(err, errSubnetDeleted) {
					slog.Warn("Failed to update cleaned up subnet", "subnet", sn.obj.Load().Name, "err", err)
				}
			}
		}
	}
}
