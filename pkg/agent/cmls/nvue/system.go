// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"fmt"
	"time"
)

type System struct {
	Hostname    string `json:"hostname"`
	ProductName string `json:"product-name"`
	// Platform is the ONIE platform, such as "x86_64-nvidia_sn5610-r0" or "x86_64-mlnx_msn4700-r0"
	Platform string `json:"platform"`
	// Uptime is in seconds
	Uptime   Number         `json:"uptime"`
	Version  SystemVersion  `json:"version"`
	DateTime SystemDateTime `json:"date-time"`
	Health   SystemHealth   `json:"health"`
}

type SystemVersion struct {
	ProductRelease string             `json:"product-release"`
	BaseOS         string             `json:"base-os"`
	Kernel         string             `json:"kernel"`
	ONIE           string             `json:"onie"`
	Image          SystemVersionImage `json:"image"`
}

type SystemVersionImage struct {
	BuildID   string `json:"build-id"`
	BuildDate string `json:"build-date"`
}

type SystemDateTime struct {
	// Timezone is the one the switch reports its times in, such as "Etc/UTC"
	Timezone  string `json:"timezone"`
	LocalTime string `json:"local-time"`
}

type SystemHealth struct {
	// Status is "OK" or "Not OK"
	Status    string                       `json:"status"`
	StatusLED string                       `json:"status-led"`
	Issues    map[string]SystemHealthIssue `json:"issues"`
}

type SystemHealthIssue struct {
	Issue string `json:"issue"`
}

// Location is the timezone the switch reports its times in
func (s *System) Location() (*time.Location, error) {
	loc, err := time.LoadLocation(s.DateTime.Timezone)
	if err != nil {
		return nil, fmt.Errorf("loading timezone %q: %w", s.DateTime.Timezone, err)
	}

	return loc, nil
}

// System returns the identity, versions and health of the switch, it takes close to a second
func (c *Client) System(ctx context.Context) (*System, error) {
	system := &System{}
	if err := c.get(ctx, pathOf("system"), nil, system); err != nil {
		return nil, err
	}

	return system, nil
}
