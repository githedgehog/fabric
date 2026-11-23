// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

type Platform struct {
	// SystemType is the model, such as "SN5610" or "MSN4700" for the ones of Mellanox
	SystemType   string `json:"system-type"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"part-number"`
	SerialNumber string `json:"serial-number"`
	ASICModel    string `json:"asic-model"`
	SystemMAC    string `json:"system-mac"`
	PortLayout   string `json:"port-layout"`
}

// Platform returns the hardware identity of the switch, which doesn't change but takes over 3 seconds to read as
// nvued samples the CPU load for the response, so it's to be read once
func (c *Client) Platform(ctx context.Context) (*Platform, error) {
	platform := &Platform{}
	if err := c.get(ctx, pathOf("platform"), nil, platform); err != nil {
		return nil, err
	}

	return platform, nil
}
