// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var temperatureCaptures = map[string][]string{
	modelSN5610: {"/platform/environment/temperature"},
	modelSN4700: {"/platform/environment/temperature"},
	modelSN4600: {"/platform/environment/temperature"},
	modelSN2700: {"/platform/environment/temperature"},
	modelSN2201: {"/platform/environment/temperature"},
}

func TestTemperatures(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		temperatures, err := client.Temperatures(t.Context())
		require.NoError(t, err)
		require.Contains(t, temperatures, "Asic-Temp-Sensor")

		for name, temp := range temperatures {
			if temp.State != EnvironmentStateOK {
				continue
			}

			require.True(t, temp.Current.OK, name)
			require.InDelta(t, 50, temp.Current.Value, 50, name)
			if temp.Crit.OK {
				require.Greater(t, temp.Crit.Value, temp.Current.Value, name)
			}
		}

		requireGolden(t, model, "temperatures", temperatures)

		if model == modelSN5610 {
			asic := temperatures["Asic-Temp-Sensor"]
			require.Equal(t, Number{Value: 105, OK: true, Raw: "105.0"}, asic.Max)
			require.Equal(t, Number{Value: 120, OK: true, Raw: "120.0"}, asic.Crit)
			// the sensor of a PSU without power
			notAvailable := Number{Raw: "N/A"}
			require.Equal(t, Temperature{
				Current: notAvailable, Max: notAvailable, Crit: notAvailable, Min: notAvailable, State: "absent",
			}, temperatures["PSU2-Temp-Sensor"])
		}
	})
}
