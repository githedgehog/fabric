// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var platformCaptures = map[string][]string{
	modelSN5610: {"/platform"},
	modelSN4700: {"/platform"},
	modelSN4600: {"/platform"},
	modelSN2700: {"/platform"},
	modelSN2201: {"/platform"},
}

func TestPlatform(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		platform, err := client.Platform(t.Context())
		require.NoError(t, err)

		require.Regexp(t, `^M?SN\d+`, platform.SystemType)
		require.Contains(t, []string{"Nvidia", "Mellanox"}, platform.Manufacturer)
		require.NotEmpty(t, platform.PartNumber)
		require.NotEmpty(t, platform.SerialNumber)
		require.Regexp(t, `^Spectrum`, platform.ASICModel)
		require.Regexp(t, macAddressRe, platform.SystemMAC)
		require.NotEmpty(t, platform.PortLayout)

		if model != "" {
			require.Equal(t, model, strings.ToLower(strings.TrimPrefix(platform.SystemType, "M")))
		}

		requireGolden(t, model, "platform", platform)

		// the hardware doesn't change, only the scrubbed serial number and MAC between captures
		switch model {
		case modelSN5610:
			require.Equal(t, "SN5610", platform.SystemType)
			require.Equal(t, "Nvidia", platform.Manufacturer)
			require.Equal(t, "Spectrum-4", platform.ASICModel)
			require.Equal(t, "64 x 800G-OSFP & 2 x 25G-SFP28", platform.PortLayout)
		case modelSN4700:
			// the ones that came out as Mellanox are still named so
			require.Equal(t, "MSN4700", platform.SystemType)
			require.Equal(t, "Mellanox", platform.Manufacturer)
			require.Equal(t, "Spectrum-3", platform.ASICModel)
		case modelSN2201:
			require.Equal(t, "Spectrum", platform.ASICModel)
			require.Equal(t, "48 x RJ45 & 4 x 100G-QSFP28", platform.PortLayout)
		}
	})
}
