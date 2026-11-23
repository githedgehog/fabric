// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var psuCaptures = map[string][]string{
	modelSN5610: {"/platform/environment/psu"},
	modelSN4700: {"/platform/environment/psu"},
	modelSN4600: {"/platform/environment/psu"},
	modelSN2700: {"/platform/environment/psu"},
	modelSN2201: {"/platform/environment/psu"},
}

func TestPSUs(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		psus, err := client.PSUs(t.Context())
		require.NoError(t, err)
		require.NotEmpty(t, psus)

		powered := 0
		for name, psu := range psus {
			require.NotEmpty(t, psu.State, name)
			if psu.State != EnvironmentStateOK || !psu.Power.OK {
				continue
			}

			powered++
			require.Positive(t, psu.Power.Value, name)
			require.InDelta(t, 12, psu.Voltage.Value, 1, name)
		}
		require.Positive(t, powered, "no PSU with power")

		requireGolden(t, model, "psus", psus)

		if model == modelSN5610 {
			// two of the four PSUs have no power
			notAvailable := Number{Raw: "N/A"}
			for _, name := range []string{"PSU2", "PSU4"} {
				require.Equal(t, PSU{
					Capacity: notAvailable, Power: notAvailable, Current: notAvailable, Voltage: notAvailable, State: "bad",
				}, psus[name], name)
			}
			for _, name := range []string{"PSU1", "PSU3"} {
				require.Equal(t, EnvironmentStateOK, psus[name].State, name)
				require.Equal(t, Number{Value: 2200, OK: true, Raw: "2200.0"}, psus[name].Capacity, name)
			}
		}
	})
}
