// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"maps"
	"slices"
)

const (
	VRFDefault = "default"
	VRFMgmt    = "mgmt"
)

// VRFs returns the names of the VRFs, including the default and management ones
func (c *Client) VRFs(ctx context.Context) ([]string, error) {
	vrfs := map[string]struct{}{}
	if err := c.get(ctx, pathOf("vrf"), nil, &vrfs); err != nil {
		return nil, err
	}

	return slices.Sorted(maps.Keys(vrfs)), nil
}
