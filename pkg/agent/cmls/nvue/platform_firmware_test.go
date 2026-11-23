// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
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

		switch model {
		case modelSN5610:
			// the ASIC is named after its generation
			require.Equal(t, map[string]Firmware{
				"BIOS":       {ActualFirmware: "0ACTV_00.01.012", PartNumber: "American Megatrends Inc."},
				"CPLD1":      {ActualFirmware: "3.0", PartNumber: "370"},
				"CPLD2":      {ActualFirmware: "7.0", PartNumber: "398"},
				"CPLD3":      {ActualFirmware: "5.0", PartNumber: "399"},
				"CPLD4":      {ActualFirmware: "4.0", PartNumber: "400"},
				"SSD":        {ActualFirmware: "CE00A400", PartNumber: "Virtium VTPM24CEXI080-BM110006"},
				"Spectrum-4": {ActualFirmware: "34.2020.1148", PartNumber: "920-9N42F-00RI-3C1_Ax"},
			}, firmware)
		case modelSN2700:
			require.Contains(t, firmware, "Spectrum")
		}
	})
}
