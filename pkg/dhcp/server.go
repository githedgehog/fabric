// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	ListenInterface  string
	AnyDeviceOnMgmt  bool
	HealthListenAddr string // address for the liveness/readiness HTTP server; empty disables it

	kube           kclient.WithWatch
	subnets        map[string]*dhcpapi.DHCPSubnet
	switchToIP     map[string]string   // switch name → relay IP (for cleanup)
	relayAllowlist map[string]struct{} // known leaf relay IPs
	m              sync.RWMutex

	boundIfIndex int         // ifindex of ListenInterface at startup (0 when listening on an IP)
	serveStopped atomic.Bool // set when the coredhcp serve loop exits
}

func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if s.subnets == nil {
		s.subnets = map[string]*dhcpapi.DHCPSubnet{}
	}
	if s.switchToIP == nil {
		s.switchToIP = map[string]string{}
	}
	if s.relayAllowlist == nil {
		s.relayAllowlist = map[string]struct{}{}
	}

	if err := s.setupKube(ctx); err != nil {
		return err
	}

	// Capture the ifindex of the listen interface so the health check can detect
	// the socket being left bound to a stale interface after a teardown/recreate.
	// When listening on an IP rather than a named interface there is no
	// SO_BINDTODEVICE binding to go stale, so the check is skipped (boundIfIndex 0).
	listenHost := listenInterfaceName(s.ListenInterface)
	if net.ParseIP(listenHost) == nil {
		iface, err := net.InterfaceByName(listenHost)
		if err != nil {
			return fmt.Errorf("resolving listen interface %s: %w", listenHost, err)
		}
		s.boundIfIndex = iface.Index
	}

	wg := sync.WaitGroup{}

	wg.Go(func() { s.watchWithRetry(ctx, "DHCPSubnet", s.watchDHCPSubnets) })
	wg.Go(func() { s.watchWithRetry(ctx, "Switch", s.watchSwitches) })

	wg.Go(func() {
		if err := s.startCoreDHCP(ctx); err != nil {
			s.serveStopped.Store(true)
			slog.Error("Start CoreDHCP failed", "err", err)
			os.Exit(2) // TODO graceful handling
		}
	})

	if s.HealthListenAddr != "" {
		wg.Go(func() {
			if err := s.startHealthz(ctx, s.HealthListenAddr); err != nil {
				slog.Error("Healthz server failed", "err", err)
				os.Exit(4) // TODO graceful handling
			}
		})
	}

	wg.Go(func() {
		if err := s.startPeriodicCleanup(ctx); err != nil {
			slog.Error("Start periodic cleanup failed", "err", err)
			os.Exit(3) // TODO graceful handling
		}
	})

	wg.Wait()

	return nil
}

func (s *Server) watchWithRetry(ctx context.Context, name string, fn func(context.Context) error) {
	first := true
	for {
		if !first {
			select {
			case <-ctx.Done():
				os.Exit(1) // TODO graceful handling
			case <-time.After(1 * time.Second):
			}
		}
		first = false

		if err := fn(ctx); err != nil {
			slog.Debug("Watch failed, will retry", "name", name, "err", err)
		}
	}
}
