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

		switch model {
		case modelSN5610:
			require.Equal(t, &Platform{
				SystemType:   "SN5610",
				Manufacturer: "Nvidia",
				PartNumber:   "920-9N42F-00RI-3C1",
				SerialNumber: "SN00000013",
				ASICModel:    "Spectrum-4",
				SystemMAC:    "0a:1b:2c:00:00:01",
				PortLayout:   "64 x 800G-OSFP & 2 x 25G-SFP28",
			}, platform)
		case modelSN4700:
			require.Equal(t, &Platform{
				SystemType:   "MSN4700",
				Manufacturer: "Mellanox",
				PartNumber:   "MSN4700-WS2FC",
				SerialNumber: "SN00000008",
				ASICModel:    "Spectrum-3",
				SystemMAC:    "0a:1b:2c:00:00:03",
				PortLayout:   "32 x 400G-QSFPDD",
			}, platform)
		case modelSN2201:
			require.Equal(t, "Spectrum", platform.ASICModel)
			require.Equal(t, "48 x RJ45 & 4 x 100G-QSFP28", platform.PortLayout)
		}
	})
}
