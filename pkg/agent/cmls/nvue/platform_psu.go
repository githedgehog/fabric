// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

// PSU readings are on the output side, they're placeholders for a PSU without power
type PSU struct {
	// Capacity and Power are in W
	Capacity Number `json:"capacity"`
	Power    Number `json:"power"`
	Current  Number `json:"current"`
	Voltage  Number `json:"voltage"`
	State    string `json:"state"`
}

// PSUs returns the power supplies by name, such as "PSU1"
func (c *Client) PSUs(ctx context.Context) (map[string]PSU, error) {
	psus := map[string]PSU{}
	if err := c.get(ctx, pathOf("platform", "environment", "psu"), nil, &psus); err != nil {
		return nil, err
	}

	return psus, nil
}
