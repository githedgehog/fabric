// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/dhcp/config"
)

func TestLoad(t *testing.T) {
	defaults := &config.ServerConfig{
		ListenInterface: config.DefaultListenInterface,
		HealthListen:    config.DefaultHealthListen,
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
				ListenInterface: "enp2s1",
				AnyDeviceOnMgmt: true,
				HealthListen:    config.DefaultHealthListen,
			},
		},
		{
			name: "all fields set",
			data: new("listenInterface: enp2s1:67\nanyDeviceOnMgmt: true\nhealthListen: 127.0.0.1:9090\n"),
			want: &config.ServerConfig{
				ListenInterface: "enp2s1:67",
				AnyDeviceOnMgmt: true,
				HealthListen:    "127.0.0.1:9090",
			},
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
