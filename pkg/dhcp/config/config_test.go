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
	defaults := &config.ServerConfig{
		ListenInterface:    config.DefaultListenInterface,
		HealthListen:       config.DefaultHealthListen,
		LeaseExpiryPadding: kmetav1.Duration{Duration: config.DefaultLeaseExpiryPadding},
		OfferTTL:           kmetav1.Duration{Duration: config.DefaultOfferTTL},
		RequestTimeout:     kmetav1.Duration{Duration: config.DefaultRequestTimeout},
		UpdateQPS:          config.DefaultUpdateQPS,
		UpdateBurst:        config.DefaultUpdateQPS,
	}

	for _, tt := range []struct {
		name    string
		data    *string // nil means no file
		want    *config.ServerConfig
		wantErr bool
	}{
		{
			name: "no file means defaults",
			want: defaults,
		},
		{
			name: "empty file means defaults",
			data: new(""),
			want: defaults,
		},
		{
			name: "missing fields fall back to defaults",
			data: new("listenInterface: enp2s1\nanyDeviceOnMgmt: true\n"),
			want: &config.ServerConfig{
				ListenInterface:    "enp2s1",
				AnyDeviceOnMgmt:    true,
				HealthListen:       config.DefaultHealthListen,
				LeaseExpiryPadding: kmetav1.Duration{Duration: config.DefaultLeaseExpiryPadding},
				OfferTTL:           kmetav1.Duration{Duration: config.DefaultOfferTTL},
				RequestTimeout:     kmetav1.Duration{Duration: config.DefaultRequestTimeout},
				UpdateQPS:          config.DefaultUpdateQPS,
				UpdateBurst:        config.DefaultUpdateQPS,
			},
		},
		{
			name: "all fields set",
			data: new("listenInterface: enp2s1:67\nanyDeviceOnMgmt: true\nhealthListen: 127.0.0.1:9090\ncontrolVIP: 172.30.0.1/32\n" +
				"leaseExpiryPadding: 10m\nofferTTL: 1m30s\nrequestTimeout: 3s\nupdateQPS: 75\nupdateBurst: 100\n"),
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
			data: new("updateQPS: 2.5\n"),
			want: func() *config.ServerConfig {
				cfg := *defaults
				cfg.UpdateQPS = 2.5
				cfg.UpdateBurst = 3

				return &cfg
			}(),
		},
		{
			name:    "negative request timeout",
			data:    new("requestTimeout: -1s\n"),
			wantErr: true,
		},
		{
			name:    "negative update qps",
			data:    new("updateQPS: -1\n"),
			wantErr: true,
		},
		{
			name:    "negative update burst",
			data:    new("updateBurst: -1\n"),
			wantErr: true,
		},
		{
			name:    "negative lease expiry padding",
			data:    new("leaseExpiryPadding: -1m\n"),
			wantErr: true,
		},
		{
			name:    "negative offer ttl",
			data:    new("offerTTL: -1s\n"),
			wantErr: true,
		},
		{
			name:    "bad duration",
			data:    new("offerTTL: 5\n"),
			wantErr: true,
		},
		{
			name:    "control vip without prefix length",
			data:    new("controlVIP: 172.30.0.1\n"),
			wantErr: true,
		},
		{
			name:    "control vip ipv6",
			data:    new("controlVIP: fd00::1/128\n"),
			wantErr: true,
		},
		{
			name:    "unknown field",
			data:    new("listen: enp2s1\n"),
			wantErr: true,
		},
		{
			name:    "garbage",
			data:    new("{not yaml"),
			wantErr: true,
		},
		{
			name:    "health listen without port",
			data:    new("healthListen: 127.0.0.1\n"),
			wantErr: true,
		},
		{
			name:    "health listen with bad port",
			data:    new("healthListen: :http\n"),
			wantErr: true,
		},
		{
			name:    "health listen with zero port",
			data:    new("healthListen: :0\n"),
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.data != nil {
				require.NoError(t, os.WriteFile(path, []byte(*tt.data), 0o600))
			}

			got, err := config.Load(path)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
