// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/netip"
	"os"
	"strconv"
	"time"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kyaml "sigs.k8s.io/yaml"
)

const (
	Path = "/config/config.yaml"

	DefaultListenInterface    = "eth0"
	DefaultHealthListen       = ":8080"
	DefaultLeaseExpiryPadding = 5 * time.Minute
	DefaultOfferTTL           = 5 * time.Minute
	DefaultRequestTimeout     = 30 * time.Second
	DefaultUpdateQPS          = 50
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
	// ControlVIP is the control VIP prefix (e.g. "172.30.0.1/32"), the relay target on all leaves, used as the DHCP server
	// identifier. If not set, the source address of the route to the relay is looked up for every request.
	ControlVIP string `json:"controlVIP,omitempty"`
	// LeaseExpiryPadding is added to the stored lease expiry, so renewals within it don't need a write, 5m if not set
	LeaseExpiryPadding kmetav1.Duration `json:"leaseExpiryPadding,omitzero"`
	// OfferTTL is how long an IP offered in reply to a DISCOVER is held for the client (in memory), 5m if not set
	OfferTTL kmetav1.Duration `json:"offerTTL,omitzero"`
	// RequestTimeout is how long a DHCP request can wait (e.g. for the update budget) before it's dropped, 30s if not set.
	// Clients keep the transaction ID when retransmitting, so a late reply is still accepted.
	RequestTimeout kmetav1.Duration `json:"requestTimeout,omitzero"`
	// UpdateQPS is the rate of DHCPSubnet status updates (lease writes), 50 if not set
	UpdateQPS float64 `json:"updateQPS,omitempty"`
	// UpdateBurst is the burst of DHCPSubnet status updates, UpdateQPS rounded up if not set
	UpdateBurst int `json:"updateBurst,omitempty"`
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
	if cfg.LeaseExpiryPadding.Duration == 0 {
		cfg.LeaseExpiryPadding.Duration = DefaultLeaseExpiryPadding
	}
	if cfg.OfferTTL.Duration == 0 {
		cfg.OfferTTL.Duration = DefaultOfferTTL
	}
	if cfg.RequestTimeout.Duration == 0 {
		cfg.RequestTimeout.Duration = DefaultRequestTimeout
	}
	if cfg.UpdateQPS == 0 {
		cfg.UpdateQPS = DefaultUpdateQPS
	}
	if cfg.UpdateBurst == 0 {
		cfg.UpdateBurst = int(math.Ceil(cfg.UpdateQPS))
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

	if cfg.ControlVIP != "" {
		if _, err := cfg.ControlVIPAddr(); err != nil {
			return err
		}
	}

	if cfg.LeaseExpiryPadding.Duration < 0 {
		return fmt.Errorf("%w: leaseExpiryPadding %s must not be negative", ErrInvalidConfig, cfg.LeaseExpiryPadding.Duration)
	}
	if cfg.OfferTTL.Duration < 0 {
		return fmt.Errorf("%w: offerTTL %s must not be negative", ErrInvalidConfig, cfg.OfferTTL.Duration)
	}
	if cfg.RequestTimeout.Duration < 0 {
		return fmt.Errorf("%w: requestTimeout %s must not be negative", ErrInvalidConfig, cfg.RequestTimeout.Duration)
	}
	if cfg.UpdateQPS < 0 {
		return fmt.Errorf("%w: updateQPS %g must not be negative", ErrInvalidConfig, cfg.UpdateQPS)
	}
	if cfg.UpdateBurst < 0 {
		return fmt.Errorf("%w: updateBurst %d must not be negative", ErrInvalidConfig, cfg.UpdateBurst)
	}

	return nil
}

// ControlVIPAddr returns the control VIP address, or an invalid netip.Addr if it isn't set
func (cfg *ServerConfig) ControlVIPAddr() (netip.Addr, error) {
	if cfg.ControlVIP == "" {
		return netip.Addr{}, nil
	}

	prefix, err := netip.ParsePrefix(cfg.ControlVIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w: controlVIP %q: %w", ErrInvalidConfig, cfg.ControlVIP, err)
	}
	if !prefix.Addr().Is4() {
		return netip.Addr{}, fmt.Errorf("%w: controlVIP %q: must be IPv4", ErrInvalidConfig, cfg.ControlVIP)
	}

	return prefix.Addr(), nil
}
