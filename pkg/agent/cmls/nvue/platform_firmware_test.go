// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

var firmwareCaptures = map[string][]string{
	modelSN5610: {"/platform/firmware"},
	modelSN4700: {"/platform/firmware"},
	modelSN4600: {"/platform/firmware"},
	modelSN2700: {"/platform/firmware"},
	modelSN2201: {"/platform/firmware"},
}

func TestFirmware(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		firmware, err := client.Firmware(t.Context())
		require.NoError(t, err)
		require.Contains(t, firmware, "BIOS")
		require.Contains(t, firmware, "CPLD1")

		for name, fw := range firmware {
			require.NotEmpty(t, fw.ActualFirmware, name)
		}

		requireGolden(t, model, "firmware", firmware)

		// the ASIC is named after its generation
		switch model {
		case modelSN5610:
			require.ElementsMatch(t, []string{"BIOS", "CPLD1", "CPLD2", "CPLD3", "CPLD4", "SSD", "Spectrum-4"},
				slices.Collect(maps.Keys(firmware)))
		case modelSN2700:
			require.Contains(t, firmware, "Spectrum")
		}
	})
}
