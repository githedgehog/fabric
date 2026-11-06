// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientGet(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() + "?" + r.URL.RawQuery {
		case "/nvue_v1/interface/swp1%2F1?rev=applied":
			_, _ = w.Write([]byte(`{"link": {"speed": "25G", "mtu": 9216, "lanes": "1"}}`))
		case "/nvue_v1/interface/eth0/counters?":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail": "The requested item does not exist.", "status": 404, "title": "The requested item does not exist.", "type": "/errors/ItemDoesNotExist"}`))
		case "/nvue_v1/broken?":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`upstream went away`))
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer server.Close()

	client := New(server.URL+"/nvue_v1/", "admin", "secret")

	t.Run("decodes and escapes the path", func(t *testing.T) {
		var iface Interface
		err := client.Get(t.Context(), interfacePath("swp1/1"), url.Values{"rev": {"applied"}}, &iface)
		require.NoError(t, err)
		require.NotNil(t, iface.Link)
		require.Equal(t, "25G", iface.Link.Speed)
		require.Equal(t, Number{Value: 9216, OK: true, Raw: "9216"}, iface.Link.MTU)
		require.Equal(t, Number{Value: 1, OK: true, Raw: "1"}, iface.Link.Lanes)
	})

	t.Run("404 is ErrNotFound", func(t *testing.T) {
		_, err := client.InterfaceCounters(t.Context(), "eth0")
		require.ErrorIs(t, err, ErrNotFound)

		var apiErr *Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.Status)
		require.Equal(t, "/errors/ItemDoesNotExist", apiErr.Type)
	})

	t.Run("non-json failure keeps the body", func(t *testing.T) {
		var out map[string]any
		err := client.Get(t.Context(), "/broken", nil, &out)
		require.NotErrorIs(t, err, ErrNotFound)

		var apiErr *Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadGateway, apiErr.Status)
		require.Equal(t, "Bad Gateway", apiErr.Title)
		require.Equal(t, "upstream went away", apiErr.Detail)
	})

	t.Run("bad credentials", func(t *testing.T) {
		var out map[string]any
		err := New(server.URL+"/nvue_v1", "admin", "wrong").Get(t.Context(), "/broken", nil, &out)

		var apiErr *Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.Status)
	})
}

func TestBGPInstanceWithoutBGP(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail": "The requested item does not exist.", "status": 404, "title": "The requested item does not exist.", "type": "/errors/ItemDoesNotExist"}`))
	}))
	defer server.Close()

	instance, err := New(server.URL, "admin", "secret").BGPInstance(t.Context(), "default")
	require.NoError(t, err)
	require.Nil(t, instance)
}

func TestNewForAgent(t *testing.T) {
	t.Run("nothing configured", func(t *testing.T) {
		t.Setenv(EnvPassword, "")

		_, err := NewForAgent("")
		require.ErrorIs(t, err, ErrNoCredentials)
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv(EnvAddress, "https://10.0.0.1:8765/nvue_v1/")
		t.Setenv(EnvUsername, "admin")
		t.Setenv(EnvPassword, "secret")

		client, err := NewForAgent("")
		require.NoError(t, err)
		require.Equal(t, "https://10.0.0.1:8765/nvue_v1", client.address)
		require.Equal(t, "admin", client.username)
		require.Equal(t, "secret", client.password)
	})

	t.Run("agent account in basedir", func(t *testing.T) {
		t.Setenv(EnvAddress, "")
		t.Setenv(EnvUsername, "")
		t.Setenv(EnvPassword, "")

		basedir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(basedir, AgentPasswordFile), []byte("s3cret\n"), 0o600))

		client, err := NewForAgent(basedir)
		require.NoError(t, err)
		require.Equal(t, DefaultAddress, client.address)
		require.Equal(t, AgentUser, client.username)
		require.Equal(t, "s3cret", client.password)
	})
}

func TestErrorIs(t *testing.T) {
	require.True(t, errors.Is(&Error{Status: http.StatusNotFound}, ErrNotFound))
	require.False(t, errors.Is(&Error{Status: http.StatusForbidden}, ErrNotFound))
}
