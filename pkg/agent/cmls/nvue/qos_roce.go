// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

const RoCEModeLossless = "lossless"

// RoCE has no state or mode when it isn't configured
type RoCE struct {
	State string `json:"state"`
	Mode  string `json:"mode"`
}

// Enabled is whether lossless RoCE is on
func (r *RoCE) Enabled() bool {
	return r.State == StateEnabled && r.Mode == RoCEModeLossless
}

func (c *Client) RoCE(ctx context.Context) (*RoCE, error) {
	roce := &RoCE{}
	if err := c.get(ctx, pathOf("qos", "roce"), nil, roce); err != nil {
		return nil, err
	}

	return roce, nil
}
