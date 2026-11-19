// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/dhcp/config"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLoad(t *testing.T) {
	const required = "listenInterface: enp2s1\ncontrolVIP: 172.30.0.1/32\n"
	withRequired := func(data string) *string {
		return new(required + data)
	}

	defaults := &config.ServerConfig{
		ListenInterface:    "enp2s1",
		HealthListen:       config.DefaultHealthListen,
		ControlVIP:         "172.30.0.1/32",
		LeaseExpiryPadding: kmetav1.Duration{Duration: config.DefaultLeaseExpiryPadding},
		OfferTTL:           kmetav1.Duration{Duration: config.DefaultOfferTTL},
		RequestTimeout:     kmetav1.Duration{Duration: config.DefaultRequestTimeout},
		UpdateQPS:          config.DefaultUpdateQPS,
		UpdateBurst:        config.DefaultUpdateQPS,
	}

	for _, tt := range []struct {
		name         string
		data         *string // nil means no file
		healthListen string
		want         *config.ServerConfig
		wantErr      bool
	}{
		{
			name:    "no file",
			wantErr: true,
		},
		{
			name:    "empty file",
			data:    new(""),
			wantErr: true,
		},
		{
			name:    "no listen interface",
			data:    new("controlVIP: 172.30.0.1/32\n"),
			wantErr: true,
		},
		{
			name:    "no control vip",
			data:    new("listenInterface: enp2s1\n"),
			wantErr: true,
		},
		{
			name: "only required fields means defaults",
			data: withRequired(""),
			want: defaults,
		},
		{
			name: "missing fields fall back to defaults",
			data: withRequired("anyDeviceOnMgmt: true\n"),
			want: func() *config.ServerConfig {
				cfg := *defaults
				cfg.AnyDeviceOnMgmt = true

				return &cfg
			}(),
		},
		{
			name: "all fields set",
			data: new("listenInterface: enp2s1:67\nanyDeviceOnMgmt: true\ncontrolVIP: 172.30.0.1/32\n" +
				"leaseExpiryPadding: 10m\nofferTTL: 1m30s\nrequestTimeout: 3s\nupdateQPS: 75\nupdateBurst: 100\n"),
			healthListen: "127.0.0.1:9090",
			want: &config.ServerConfig{
				ListenInterface:    "enp2s1:67",
				AnyDeviceOnMgmt:    true,
				HealthListen:       "127.0.0.1:9090",
				ControlVIP:         "172.30.0.1/32",
				LeaseExpiryPadding: kmetav1.Duration{Duration: 10 * time.Minute},
				OfferTTL:           kmetav1.Duration{Duration: 90 * time.Second},
				RequestTimeout:     kmetav1.Duration{Duration: 3 * time.Second},
				UpdateQPS:          75,
				UpdateBurst:        100,
			},
		},
		{
			name: "update burst defaults to update qps rounded up",
			data: withRequired("updateQPS: 2.5\n"),
			want: func() *config.ServerConfig {
				cfg := *defaults
				cfg.UpdateQPS = 2.5
				cfg.UpdateBurst = 3

				return &cfg
			}(),
		},
		{
			name:    "negative request timeout",
			data:    withRequired("requestTimeout: -1s\n"),
			wantErr: true,
		},
		{
			name:    "negative update qps",
			data:    withRequired("updateQPS: -1\n"),
			wantErr: true,
		},
		{
			name:    "negative update burst",
			data:    withRequired("updateBurst: -1\n"),
			wantErr: true,
		},
		{
			name:    "negative lease expiry padding",
			data:    withRequired("leaseExpiryPadding: -1m\n"),
			wantErr: true,
		},
		{
			name:    "negative offer ttl",
			data:    withRequired("offerTTL: -1s\n"),
			wantErr: true,
		},
		{
			name:    "bad duration",
			data:    withRequired("offerTTL: 5\n"),
			wantErr: true,
		},
		{
			name:    "control vip without prefix length",
			data:    new("listenInterface: enp2s1\ncontrolVIP: 172.30.0.1\n"),
			wantErr: true,
		},
		{
			name:    "control vip ipv6",
			data:    new("listenInterface: enp2s1\ncontrolVIP: fd00::1/128\n"),
			wantErr: true,
		},
		{
			name:    "unknown field",
			data:    withRequired("listen: enp2s1\n"),
			wantErr: true,
		},
		{
			name:    "garbage",
			data:    new("{not yaml"),
			wantErr: true,
		},
		{
			name:    "health listen in the file",
			data:    withRequired("healthListen: :9090\n"),
			wantErr: true,
		},
		{
			name:         "health listen without port",
			data:         withRequired(""),
			healthListen: "127.0.0.1",
			wantErr:      true,
		},
		{
			name:         "health listen with bad port",
			data:         withRequired(""),
			healthListen: ":http",
			wantErr:      true,
		},
		{
			name:         "health listen with zero port",
			data:         withRequired(""),
			healthListen: ":0",
			wantErr:      true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.data != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.data), 0o600))
			}

			got, err := config.Load(path, tt.healthListen)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
