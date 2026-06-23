// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

var errServeStopped = errors.New("dhcp serve loop has stopped")

// listenInterfaceName strips an optional ":port" (or "[host]:port") suffix from
// a --listen value, returning the bare interface name or IP. coredhcp accepts
// values like "eth0:67"; the ifindex lookup needs just the interface name.
func listenInterfaceName(listen string) string {
	if host, _, err := net.SplitHostPort(listen); err == nil {
		return host
	}

	return listen
}

// checkHealth reports whether the DHCP server is still able to serve requests.
//
// It is intentionally conservative about false positives (an idle server is
// healthy) and targets the failure mode seen in the field: the coredhcp
// listener binds its UDP socket to the --listen interface via SO_BINDTODEVICE,
// resolved once at startup. If that interface is torn down and recreated (e.g.
// CNI/multus churn), the socket is left bound to a now-stale interface, ReadFrom
// blocks forever with no error, and the process keeps running while serving
// nothing. We detect that by comparing the listen interface's current ifindex
// against the one captured when coredhcp started; any change (or the interface
// disappearing / going down) means the socket can no longer receive and the
// container must be restarted to re-bind.
func (s *Server) checkHealth() error {
	if s.serveStopped.Load() {
		return errServeStopped
	}

	// boundIfIndex is 0 when listening on an IP rather than a named interface;
	// there is no SO_BINDTODEVICE binding to go stale in that case.
	if s.boundIfIndex != 0 {
		listenHost := listenInterfaceName(s.ListenInterface)
		iface, err := net.InterfaceByName(listenHost)
		if err != nil {
			return fmt.Errorf("listen interface %s not found: %w", listenHost, err)
		}
		if iface.Index != s.boundIfIndex {
			return fmt.Errorf("listen interface %s ifindex changed (%d -> %d): socket is bound to a stale interface and can no longer receive, restart required", //nolint:err113
				listenHost, s.boundIfIndex, iface.Index)
		}
		if iface.Flags&net.FlagUp == 0 {
			return fmt.Errorf("listen interface %s is down", listenHost) //nolint:err113
		}
	}

	return nil
}

// healthzHandler responds 200 when checkHealth passes and 503 otherwise. It
// backs both /healthz and /readyz, so a wedged instance is restarted by the
// kubelet.
func (s *Server) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.checkHealth(); err != nil {
		slog.Warn("Health check failed", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, "unhealthy: %v\n", err)

		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}

// startHealthz serves /healthz and /readyz for Kubernetes liveness/readiness
// probes.
func (s *Server) startHealthz(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.HandleFunc("/readyz", s.healthzHandler)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		// ctx is already done here; use a fresh context to bound the shutdown.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx) //nolint:contextcheck
	}()

	slog.Info("Starting healthz server", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("healthz server on %s: %w", addr, err)
	}

	return nil
}
