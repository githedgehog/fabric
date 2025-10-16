// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	ListenInterface string

	kube    kclient.WithWatch
	subnets map[string]*dhcpapi.DHCPSubnet
	m       sync.RWMutex
}

func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if s.subnets == nil {
		s.subnets = map[string]*dhcpapi.DHCPSubnet{}
	}

	if err := s.setupKube(ctx); err != nil {
		return err
	}

	wg := sync.WaitGroup{}

	wg.Go(func() {
		retry := false
		for {
			if retry {
				select {
				case <-ctx.Done():
					os.Exit(1) // TODO graceful handling
				case <-time.After(1 * time.Second):
					// Retry watching after a delay
				}
			}
			retry = true

			if err := s.watchKube(ctx); err != nil {
				slog.Debug("Watch K8s failed, will retry", "err", err)

				continue
			}
		}
	})

	wg.Go(func() {
		if err := s.startCoreDHCP(ctx); err != nil {
			slog.Error("Start CoreDHCP failed", "err", err)
			os.Exit(2) // TODO graceful handling
		}
	})

	wg.Go(func() {
		if err := s.startPeriodicCleanup(ctx); err != nil {
			slog.Error("Start periodic cleanup failed", "err", err)
			os.Exit(3) // TODO graceful handling
		}
	})

	wg.Wait()

	return nil
}
