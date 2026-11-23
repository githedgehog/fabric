// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// the applied interface config of the SN5610 is captured for the client tests already
var breakoutCaptures = map[string][]string{
	modelSN4600: {"/interface?filled=false&rev=applied"},
	modelSN2700: {"/interface?filled=false&rev=applied"},
}

func TestBreakouts(t *testing.T) {
	forEachModel(t, []string{modelSN5610, modelSN4600, modelSN2700}, func(t *testing.T, client *Client, model string) {
		breakouts, err := client.Breakouts(t.Context())
		require.NoError(t, err)

		ifaces, err := client.Interfaces(t.Context())
		require.NoError(t, err)

		for port, mode := range breakouts {
			if mode == BreakoutDisabled {
				require.NotContains(t, ifaces, port)

				continue
			}

			count, err := strconv.Atoi(strings.TrimSuffix(mode, "x"))
			require.NoError(t, err, "port %s mode %s", port, mode)
			if count == 1 {
				require.Contains(t, ifaces, port)

				continue
			}

			require.NotContains(t, ifaces, port)
			for i := range count {
				require.Contains(t, ifaces, fmt.Sprintf("%ss%d", port, i))
			}
		}

		switch model {
		case modelSN5610:
			require.Equal(t, map[string]string{
				"swp1": "8x", "swp2": BreakoutDisabled, "swp3": "4x", "swp63": "2x", "swp64": "2x",
			}, breakouts)
		case modelSN4600:
			require.Equal(t, map[string]string{"swp17": "4x", "swp18": BreakoutDisabled}, breakouts)
		case modelSN2700:
			require.Equal(t, map[string]string{"swp15": "4x", "swp16": BreakoutDisabled}, breakouts)
		}
	})
}
