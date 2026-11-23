// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var interfaceRatesCaptures = map[string][]string{
	modelSN5610: {"/interface?view=rates"},
	modelSN2700: {"/interface?view=rates"},
}

func TestInterfaceRates(t *testing.T) {
	forEachModel(t, []string{modelSN5610, modelSN2700}, func(t *testing.T, client *Client, model string) {
		rates, err := client.InterfaceRates(t.Context())
		require.NoError(t, err)

		ifaces, err := client.Interfaces(t.Context())
		require.NoError(t, err)
		for name, iface := range ifaces {
			if iface.Type != InterfaceTypeSwp || iface.Link.OperStatus != StatusUp {
				continue
			}

			require.Contains(t, rates, name)
			require.True(t, rates[name].InBitsRate.OK, name)
			require.True(t, rates[name].OutUtilization.OK, name)
			require.InDelta(t, 60, rates[name].LoadInterval.Value, 0, name)
		}

		switch model {
		case modelSN5610:
			require.Equal(t, InterfaceRates{
				InBitsRate:     Number{Value: 113, OK: true, Raw: "113"},
				InPktsRate:     Number{Value: 0, OK: true, Raw: "0"},
				InUtilization:  Number{Value: 2.8259607184702132e-08, OK: true, Raw: "2.8259607184702132e-08"},
				OutBitsRate:    Number{Value: 456, OK: true, Raw: "456"},
				OutPktsRate:    Number{Value: 0, OK: true, Raw: "0"},
				OutUtilization: Number{Value: 1.1387714155917655e-07, OK: true, Raw: "1.1387714155917655e-07"},
				LoadInterval:   Number{Value: 60, OK: true, Raw: "60"},
			}, rates["swp29"])
		case modelSN2700:
			// the base ports of a breakout and the ones it disables have none
			require.NotContains(t, rates, "swp15")
			require.NotContains(t, rates, "swp16")
			require.Contains(t, rates, "swp15s0")
		}
	})
}
