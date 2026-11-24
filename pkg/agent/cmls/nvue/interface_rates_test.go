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
	modelVX:     {"/interface?view=rates"},
}

// Cumulus VX has no traffic rates, its nvued says the service collecting them isn't active
func TestInterfaceRatesVX(t *testing.T) {
	if isLive() {
		t.Skip("only for the fixtures of " + modelVX)
	}

	_, err := testClient(t, modelVX).InterfaceRates(t.Context())
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorContains(t, err, "prometheus-sdk-stats.service is not active")
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

		requireGolden(t, model, "interface-rates", rates)

		if model == modelSN2700 {
			// the base ports of a breakout and the ones it disables have none
			require.NotContains(t, rates, "swp15")
			require.NotContains(t, rates, "swp16")
			require.Contains(t, rates, "swp15s0")
		}
	})
}
