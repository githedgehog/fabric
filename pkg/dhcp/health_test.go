// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListenInterfaceName(t *testing.T) {
	for _, tt := range []struct {
		name   string
		listen string
		want   string
	}{
		{"bare interface", "eth0", "eth0"},
		{"interface with port", "eth0:67", "eth0"},
		{"bare ipv4", "127.0.0.1", "127.0.0.1"},
		{"ipv4 with port", "127.0.0.1:67", "127.0.0.1"},
		{"bare ipv6", "::1", "::1"},
		{"ipv6 with port", "[::1]:67", "::1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, listenInterfaceName(tt.listen))
		})
	}
}

func TestCheckHealth(t *testing.T) {
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skip("no loopback interface available")
	}
	loUp := lo.Flags&net.FlagUp != 0

	serveStopped := &Server{ListenInterface: "lo", boundIfIndex: lo.Index}
	serveStopped.serveStopped.Store(true)

	for _, tt := range []struct {
		name     string
		server   *Server
		needLoUp bool
		wantErr  bool
	}{
		{
			name:    "serve loop stopped is unhealthy",
			server:  serveStopped,
			wantErr: true,
		},
		{
			name:    "listening on an IP skips the ifindex check",
			server:  &Server{ListenInterface: "127.0.0.1"}, // boundIfIndex 0
			wantErr: false,
		},
		{
			name:     "matching ifindex is healthy",
			server:   &Server{ListenInterface: "lo", boundIfIndex: lo.Index},
			needLoUp: true,
			wantErr:  false,
		},
		{
			name:     "port suffix is normalized away",
			server:   &Server{ListenInterface: "lo:67", boundIfIndex: lo.Index},
			needLoUp: true,
			wantErr:  false,
		},
		{
			name:    "stale ifindex is unhealthy",
			server:  &Server{ListenInterface: "lo", boundIfIndex: lo.Index + 1000},
			wantErr: true,
		},
		{
			name:    "missing interface is unhealthy",
			server:  &Server{ListenInterface: "hh-nonexistent0", boundIfIndex: 1},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needLoUp && !loUp {
				t.Skip("loopback interface is down")
			}
			got := tt.server.checkHealth()
			if tt.wantErr {
				assert.Error(t, got)
			} else {
				assert.NoError(t, got)
			}
		})
	}
}

func TestHealthzHandler(t *testing.T) {
	t.Run("healthy returns 200", func(t *testing.T) {
		s := &Server{ListenInterface: "127.0.0.1"} // boundIfIndex 0 => no ifindex check
		rec := httptest.NewRecorder()
		s.healthzHandler(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "ok")
	})

	t.Run("wedged returns 503", func(t *testing.T) {
		s := &Server{}
		s.serveStopped.Store(true)
		rec := httptest.NewRecorder()
		s.healthzHandler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "unhealthy")
	})
}
