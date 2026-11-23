// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

type Firmware struct {
	ActualFirmware string `json:"actual-firmware"`
	PartNumber     string `json:"part-number"`
}

// Firmware returns the firmware of the platform components, such as the BIOS, the CPLDs, the SSD and the ASIC, by
// name. nvued queries all of them for it, which takes about a second and several on a cold cache, so it's to be read
// once.
func (c *Client) Firmware(ctx context.Context) (map[string]Firmware, error) {
	firmware := map[string]Firmware{}
	if err := c.get(ctx, pathOf("platform", "firmware"), nil, &firmware); err != nil {
		return nil, err
	}

	return firmware, nil
}
