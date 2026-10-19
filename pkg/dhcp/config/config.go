// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"strconv"

	kyaml "sigs.k8s.io/yaml"
)

const (
	Path = "/config/config.yaml"

	DefaultListenInterface = "eth0"
	DefaultHealthListen    = ":8080"
)

// ServerConfig is the fabric-dhcpd configuration, fabricator renders it into the fabric-dhcpd-config ConfigMap. Any
// missing field falls back to its default, so an older or missing ConfigMap keeps working.
type ServerConfig struct {
	// ListenInterface is the interface (optionally with ":port") to listen for DHCP requests on, "eth0" if not set
	ListenInterface string `json:"listenInterface,omitempty"`
	// AnyDeviceOnMgmt allows any device, not only ONIE and Cumulus, to get an IP from the management subnet
	AnyDeviceOnMgmt bool `json:"anyDeviceOnMgmt,omitempty"`
	// HealthListen is the address for the liveness/readiness HTTP server, ":8080" if not set
	HealthListen string `json:"healthListen,omitempty"`
}

// Load reads the config from path. A missing file means all defaults, while a file that can't be read or parsed, or
// an invalid value, is an error.
func Load(path string) (*ServerConfig, error) {
	cfg := &ServerConfig{}

	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		slog.Info("Config file not found, using defaults", "path", path)
	case err != nil:
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	default:
		if err := kyaml.UnmarshalStrict(data, cfg); err != nil {
			return nil, fmt.Errorf("unmarshalling config file %s: %w", path, err)
		}
	}

	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config file %s: %w", path, err)
	}

	return cfg, nil
}

// Default sets the defaults for all fields that aren't set
func (cfg *ServerConfig) Default() {
	if cfg.ListenInterface == "" {
		cfg.ListenInterface = DefaultListenInterface
	}
	if cfg.HealthListen == "" {
		cfg.HealthListen = DefaultHealthListen
	}
}

var ErrInvalidConfig = errors.New("invalid config")

// Validate checks the config, it's expected to be called after Default
func (cfg *ServerConfig) Validate() error {
	if cfg.ListenInterface == "" {
		return fmt.Errorf("%w: listenInterface is required", ErrInvalidConfig)
	}

	_, port, err := net.SplitHostPort(cfg.HealthListen)
	if err != nil {
		return fmt.Errorf("%w: healthListen %q: %w", ErrInvalidConfig, cfg.HealthListen, err)
	}
	if portNum, err := strconv.ParseUint(port, 10, 16); err != nil || portNum == 0 {
		return fmt.Errorf("%w: healthListen %q: port must be 1-65535", ErrInvalidConfig, cfg.HealthListen)
	}

	return nil
}
