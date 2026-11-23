// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

// Temperature is a sensor reading with its thresholds, all in Celsius
type Temperature struct {
	Current Number `json:"current"`
	Max     Number `json:"max"`
	Crit    Number `json:"crit"`
	Min     Number `json:"min"`
	State   string `json:"state"`
}

// Temperatures returns the temperature sensors by name, such as "Asic-Temp-Sensor"
func (c *Client) Temperatures(ctx context.Context) (map[string]Temperature, error) {
	temperatures := map[string]Temperature{}
	if err := c.get(ctx, pathOf("platform", "environment", "temperature"), nil, &temperatures); err != nil {
		return nil, err
	}

	return temperatures, nil
}
