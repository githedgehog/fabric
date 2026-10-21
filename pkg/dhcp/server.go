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

	dhcpcfg "go.githedgehog.com/fabric/pkg/dhcp/config"
	toolscache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
	Config dhcpcfg.ServerConfig

	kube           kclient.Client
	subnets        map[string]*subnet
	switchToIP     map[string]string   // switch name → relay IP (for cleanup)
	relayAllowlist map[string]struct{} // known leaf relay IPs
	mu             sync.RWMutex        // guards the maps only, never held while taking subnet.mu
	controlVIP     net.IP              // DHCP server identifier, looked up per request if nil
	budget         budgetStats

	boundIfIndex int         // ifindex of ListenInterface at startup (0 when listening on an IP)
	serveStopped atomic.Bool // set when the coredhcp serve loop exits
}

func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if s.subnets == nil {
		s.subnets = map[string]*subnet{}
	}
	if s.switchToIP == nil {
		s.switchToIP = map[string]string{}
	}
	if s.relayAllowlist == nil {
		s.relayAllowlist = map[string]struct{}{}
	}

	vip, err := s.Config.ControlVIPAddr()
	if err != nil {
		return fmt.Errorf("parsing control VIP: %w", err)
	}
	if vip.IsValid() {
		s.controlVIP = net.IP(vip.AsSlice())
	} else {
		slog.Warn("Control VIP isn't configured, looking up the DHCP server identifier for every request")
	}

	kube, cache, err := newKube(ctx, &s.Config, &s.budget)
	if err != nil {
		return err
	}
	s.kube = kube

	synced, err := s.addInformerHandlers(ctx, cache)
	if err != nil {
		return err
	}

	// Capture the ifindex of the listen interface so the health check can detect
	// the socket being left bound to a stale interface after a teardown/recreate.
	// When listening on an IP rather than a named interface there is no
	// SO_BINDTODEVICE binding to go stale, so the check is skipped (boundIfIndex 0).
	listenHost := listenInterfaceName(s.Config.ListenInterface)
	if net.ParseIP(listenHost) == nil {
		iface, err := net.InterfaceByName(listenHost)
		if err != nil {
			return fmt.Errorf("resolving listen interface %s: %w", listenHost, err)
		}
		s.boundIfIndex = iface.Index
	}

	wg := sync.WaitGroup{}

	wg.Go(func() {
		if err := cache.Start(ctx); err != nil {
			slog.Error("Kube cache failed", "err", err)
			os.Exit(1) // TODO graceful handling
		}
	})

	wg.Go(func() {
		if err := s.startHealthz(ctx, s.Config.HealthListen); err != nil {
			slog.Error("Healthz server failed", "err", err)
			os.Exit(4) // TODO graceful handling
		}
	})

	slog.Info("Waiting for DHCPSubnets and Switches to sync")
	if !toolscache.WaitForCacheSync(ctx.Done(), synced...) {
		return fmt.Errorf("waiting for DHCPSubnets and Switches to sync: %w", ctx.Err())
	}
	slog.Info("DHCPSubnets and Switches synced")

	wg.Go(func() {
		if err := s.startCoreDHCP(ctx); err != nil {
			s.serveStopped.Store(true)
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

	wg.Go(func() { s.reportBudget(ctx) })

	wg.Wait()

	return nil
}
