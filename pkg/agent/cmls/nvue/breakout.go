// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"fmt"
	"net/url"
)

// BreakoutDisabled is the breakout mode of a port disabled by the breakout of its neighbor
const BreakoutDisabled = "disabled"

// Breakouts returns the breakout mode of every port with one configured, such as "4x" or BreakoutDisabled, by name.
// The operational state doesn't have the ports that are broken out, only their subports, so it's the applied config.
func (c *Client) Breakouts(ctx context.Context) (map[string]string, error) {
	ifaces := map[string]struct {
		Link struct {
			Breakout map[string]struct{} `json:"breakout"`
		} `json:"link"`
	}{}
	query := url.Values{"rev": {"applied"}, "filled": {"false"}}
	if err := c.get(ctx, pathOf("interface"), query, &ifaces); err != nil {
		return nil, err
	}

	breakouts := map[string]string{}
	for name, iface := range ifaces {
		if len(iface.Link.Breakout) > 1 {
			return nil, fmt.Errorf("port %s has more than one breakout mode: %v", name, iface.Link.Breakout) //nolint:err113
		}
		for mode := range iface.Link.Breakout {
			breakouts[name] = mode
		}
	}

	return breakouts, nil
}
