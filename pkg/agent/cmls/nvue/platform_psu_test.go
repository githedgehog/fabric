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

		if model == modelSN5610 {
			// two of the four PSUs aren't plugged in
			notAvailable := Number{Raw: "N/A"}
			require.Equal(t, map[string]PSU{
				"PSU1": {
					Capacity: Number{Value: 2200, OK: true, Raw: "2200.0"},
					Power:    Number{Value: 157.75, OK: true, Raw: "157.75"},
					Current:  Number{Value: 12.703, OK: true, Raw: "12.703"},
					Voltage:  Number{Value: 12.429, OK: true, Raw: "12.429"},
					State:    EnvironmentStateOK,
				},
				"PSU2": {Capacity: notAvailable, Power: notAvailable, Current: notAvailable, Voltage: notAvailable, State: "bad"},
				"PSU3": {
					Capacity: Number{Value: 2200, OK: true, Raw: "2200.0"},
					Power:    Number{Value: 164.5, OK: true, Raw: "164.5"},
					Current:  Number{Value: 13.218, OK: true, Raw: "13.218"},
					Voltage:  Number{Value: 12.451, OK: true, Raw: "12.451"},
					State:    EnvironmentStateOK,
				},
				"PSU4": {Capacity: notAvailable, Power: notAvailable, Current: notAvailable, Voltage: notAvailable, State: "bad"},
			}, psus)
		}
	})
}
