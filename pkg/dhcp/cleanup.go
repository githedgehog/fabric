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
				s.cleanupSubnet(ctx, sn)
			}
		}
	}
}

func (s *Server) cleanupSubnet(ctx context.Context, sn *subnet) {
	// same as a request, so cleanup never holds the subnet's updates for longer
	ctx, cancel := context.WithTimeout(ctx, s.Config.RequestTimeout.Duration)
	defer cancel()

	name := sn.obj.Load().Name
	expired, unstatic, err := sn.cleanup(ctx, s.kube, &s.Config)
	switch {
	case errors.Is(err, errSubnetDeleted):
	case errors.Is(err, errUpdateBudget):
		slog.Debug("Over the update budget, cleanup postponed", "subnet", name, "err", err)
	case errors.Is(err, context.DeadlineExceeded):
		s.budget.expired.Add(1)
		slog.Debug("Cleanup timed out, postponed", "subnet", name, "err", err)
	case err != nil:
		slog.Warn("Failed to clean up subnet", "subnet", name, "err", err)
	case len(expired) > 0 || len(unstatic) > 0:
		slog.Info("Cleaned up subnet", "subnet", name, "expired", expired, "noLongerStatic", unstatic)
	}
}
