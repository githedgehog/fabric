// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var clientCaptures = map[string][]string{
	fixtureModel: {
		"/system/version",
		"/interface?filled=false&rev=applied",
		"/platform/environment/fan/FAN1%2F1",
		"/interface/swp999",
		"/vrf/mgmt/router/bgp",
		"/system/forwarding",
		"/nosuchthing",
		"/interface?view=nosuchview",
	},
}

func TestNewInvalidOptions(t *testing.T) {
	for _, tt := range []struct {
		name string
		opt  Option
	}{
		{name: "empty socket", opt: WithSocket("")},
		{name: "zero timeout", opt: WithTimeout(0)},
		{name: "negative timeout", opt: WithTimeout(-time.Second)},
		{name: "zero max response size", opt: WithMaxResponseSize(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opt)
			require.ErrorIs(t, err, ErrInvalidOption)
		})
	}
}

func TestPathOf(t *testing.T) {
	require.Equal(t, "/interface/swp1s0/counters", pathOf("interface", "swp1s0", "counters"))
	require.Equal(t, "/platform/environment/fan/FAN1%2F1", pathOf("platform", "environment", "fan", "FAN1/1"))
	require.Equal(t, "/interface/ACL%20Drops", pathOf("interface", "ACL Drops"))
}

func TestGet(t *testing.T) {
	client := testClient(t, fixtureModel)

	version := map[string]any{}
	require.NoError(t, client.get(t.Context(), pathOf("system", "version"), nil, &version))
	require.Contains(t, version, "kernel")
	require.Contains(t, version, "image")
}

func TestGetQuery(t *testing.T) {
	client := testClient(t, fixtureModel)

	// the applied config of the management port has its address, the operational state the link
	ifaces := map[string]map[string]any{}
	err := client.get(t.Context(), pathOf("interface"), url.Values{"rev": {"applied"}, "filled": {"false"}}, &ifaces)
	require.NoError(t, err)
	require.Contains(t, ifaces["eth0"], "ipv4")
	require.NotContains(t, ifaces["eth0"], "counters")
}

func TestGetEscapedSegment(t *testing.T) {
	client := testClient(t, fixtureModel)

	// fan ids such as "FAN1/1" only work with the slash escaped
	fan := map[string]any{}
	require.NoError(t, client.get(t.Context(), pathOf("platform", "environment", "fan", "FAN1/1"), nil, &fan))
	require.Contains(t, fan, "current-speed")
}

func TestGetNotFound(t *testing.T) {
	client := testClient(t, fixtureModel)

	for _, tt := range []struct {
		name     string
		path     string
		probType string
	}{
		{name: "item", path: pathOf("interface", "swp999"), probType: "/errors/ItemDoesNotExist"},
		{name: "no data", path: pathOf("vrf", "mgmt", "router", "bgp"), probType: "about:blank"},
		{name: "not supported", path: pathOf("system", "forwarding"), probType: "/errors/OperationalNotSupported"},
		{name: "unknown url", path: pathOf("nosuchthing"), probType: "about:blank"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := client.get(t.Context(), tt.path, nil, &map[string]any{})
			require.ErrorIs(t, err, ErrNotFound)

			apiErr := &Error{}
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusNotFound, apiErr.Status)
			require.Equal(t, tt.probType, apiErr.Type)
			require.NotEmpty(t, apiErr.Title)
			require.NotEmpty(t, apiErr.Detail)
		})
	}
}

func TestGetBadRequest(t *testing.T) {
	client := testClient(t, fixtureModel)

	err := client.get(t.Context(), pathOf("interface"), url.Values{"view": {"nosuchview"}}, &map[string]any{})
	require.NotErrorIs(t, err, ErrNotFound)

	apiErr := &Error{}
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.Status)
	require.Equal(t, "Bad Request", apiErr.Title)
	require.Contains(t, apiErr.Detail, "'nosuchview' is not one of")
	require.Contains(t, err.Error(), "nvue: 400 Bad Request: 'nosuchview' is not one of")
}

// TestGetForbidden needs a user without an NVUE role when running live, as root gets everything
func TestGetForbidden(t *testing.T) {
	if isLive() && os.Geteuid() == 0 {
		t.Skip("root has every NVUE role")
	}

	client := testClient(t, fixtureModel+"/nobody")

	err := client.get(t.Context(), pathOf("system", "version"), nil, &map[string]any{})
	apiErr := &Error{}
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusForbidden, apiErr.Status)
	require.Equal(t, "Forbidden", apiErr.Title)
	require.Contains(t, apiErr.Detail, "You don't have the permission")
}

// slowRequest is a request the client won't get an answer to in time: /system/cpu takes over 3s on a switch as nvued
// samples the CPU load, the fake nvued never answers
func slowRequest(t *testing.T, opts ...Option) (*Client, string) {
	t.Helper()

	if !isLive() {
		opts = append([]Option{WithSocket(silentSocket(t))}, opts...)
	}

	client, err := New(opts...)
	require.NoError(t, err)

	return client, pathOf("system", "cpu")
}

func TestGetTimeout(t *testing.T) {
	client, path := slowRequest(t, WithTimeout(100*time.Millisecond))

	start := time.Now()
	err := client.get(t.Context(), path, nil, &map[string]any{})
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second)

	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout(), "expected a timeout, got %v", err)
}

func TestGetContextDeadline(t *testing.T) {
	client, path := slowRequest(t)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := client.get(ctx, path, nil, &map[string]any{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
}

func TestGetResponseTooLarge(t *testing.T) {
	client := testClient(t, fixtureModel, WithMaxResponseSize(64))

	err := client.get(t.Context(), pathOf("system", "version"), nil, &map[string]any{})
	require.ErrorIs(t, err, ErrResponseTooLarge)
}

func TestGetMissingSocket(t *testing.T) {
	client, err := New(WithSocket(socketPath(t)))
	require.NoError(t, err)

	err = client.get(t.Context(), pathOf("system", "version"), nil, &map[string]any{})
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestGetConcurrent(t *testing.T) {
	client := testClient(t, fixtureModel)

	wg := sync.WaitGroup{}
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			errs <- client.get(t.Context(), pathOf("system", "version"), nil, &map[string]any{})
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}

// the error bodies nvued doesn't send on its own: the legacy message only one, and anything from a proxy in between
func TestReadError(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   Error
	}{
		{
			name:   "legacy message",
			status: http.StatusInternalServerError,
			body:   `{"message": "something went wrong"}`,
			want:   Error{Status: 500, Title: "Internal Server Error", Detail: "something went wrong"},
		},
		{
			name:   "not json",
			status: http.StatusBadGateway,
			body:   "upstream went away\n",
			want:   Error{Status: 502, Title: "Bad Gateway", Detail: "upstream went away"},
		},
		{
			name:   "empty",
			status: http.StatusServiceUnavailable,
			want:   Error{Status: 503, Title: "Service Unavailable"},
		},
		{
			name:   "huge",
			status: http.StatusInternalServerError,
			body:   strings.Repeat("x", 3*maxErrorBodySize),
			want:   Error{Status: 500, Title: "Internal Server Error", Detail: strings.Repeat("x", maxErrorBodySize)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}
			require.Equal(t, &tt.want, readError(resp))
		})
	}
}
