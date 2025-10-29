// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"log/slog"
	"time"
)

func (s *Server) startPeriodicCleanup(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(60) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.m.RLock()
			for _, subnet := range s.subnets {
				subnet = subnet.DeepCopy()
				if err := s.updateSubnet(ctx, subnet, cleanup); err != nil {
					slog.Warn("Failed to update cleaned up subnet", "subnet", subnet.Name, "err", err)
				}
			}
			s.m.RUnlock()
		}
	}
}
