// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

// EnvironmentStateOK is the state of a healthy fan, PSU or sensor
const EnvironmentStateOK = "ok"

type Fan struct {
	// the speeds are in RPM
	CurrentSpeed Number `json:"current-speed"`
	MinSpeed     Number `json:"min-speed"`
	MaxSpeed     Number `json:"max-speed"`
	// Direction is "F2B" or "B2F"
	Direction string `json:"direction"`
	State     string `json:"state"`
}

// Fans returns the fans by name, such as "FAN1/1", including the ones of the PSUs
func (c *Client) Fans(ctx context.Context) (map[string]Fan, error) {
	fans := map[string]Fan{}
	if err := c.get(ctx, pathOf("platform", "environment", "fan"), nil, &fans); err != nil {
		return nil, err
	}

	return fans, nil
}
