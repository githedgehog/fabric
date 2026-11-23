// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	// BreakoutDisabled is the breakout mode of a port disabled by the breakout of its neighbor
	BreakoutDisabled = "disabled"

	speedAuto = "auto"
)

// Breakout is how a port is broken out as configured. Mode is such as "4x" or BreakoutDisabled, and empty for a port
// that isn't broken out. Speed is the one of its subports, or of the port itself if it isn't broken out, and empty if
// it's disabled or the speed isn't set.
type Breakout struct {
	Mode  string
	Speed string
}

// Breakouts returns the breakout of every port that can be broken out, by name. The operational state doesn't have the
// ports that are broken out, only their subports, so it's the applied config.
func (c *Client) Breakouts(ctx context.Context) (map[string]Breakout, error) {
	ifaces := map[string]struct {
		Link struct {
			// only the ports that can be broken out have it, empty if they aren't
			Breakout map[string]struct{} `json:"breakout"`
			Speed    string              `json:"speed"`
		} `json:"link"`
	}{}
	query := url.Values{"rev": {"applied"}, "filled": {"false"}}
	if err := c.get(ctx, pathOf("interface"), query, &ifaces); err != nil {
		return nil, err
	}

	breakouts := map[string]Breakout{}
	for name, iface := range ifaces {
		if iface.Link.Breakout == nil {
			continue
		}
		if len(iface.Link.Breakout) > 1 {
			return nil, fmt.Errorf("port %s has more than one breakout mode: %v", name, iface.Link.Breakout) //nolint:err113
		}

		breakout := Breakout{Speed: iface.Link.Speed}
		for mode := range iface.Link.Breakout {
			breakout.Mode = mode
		}

		switch {
		case breakout.Mode == BreakoutDisabled:
			breakout.Speed = ""
		case breakout.Mode != "":
			count, err := strconv.Atoi(strings.TrimSuffix(breakout.Mode, "x"))
			if err != nil || !strings.HasSuffix(breakout.Mode, "x") || count < 1 {
				return nil, fmt.Errorf("port %s has unknown breakout mode %q", name, breakout.Mode) //nolint:err113
			}
			// all subports run at the same speed
			if count > 1 {
				breakout.Speed = ifaces[name+"s0"].Link.Speed
			}
		}
		if breakout.Speed == speedAuto {
			breakout.Speed = ""
		}

		breakouts[name] = breakout
	}

	return breakouts, nil
}
