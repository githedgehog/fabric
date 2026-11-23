// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var fanCaptures = map[string][]string{
	modelSN5610: {"/platform/environment/fan"},
	modelSN4700: {"/platform/environment/fan"},
	modelSN4600: {"/platform/environment/fan"},
	modelSN2700: {"/platform/environment/fan"},
	modelSN2201: {"/platform/environment/fan"},
}

func TestFans(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		fans, err := client.Fans(t.Context())
		require.NoError(t, err)
		require.NotEmpty(t, fans)

		for name, fan := range fans {
			require.NotEmpty(t, fan.State, name)
			if fan.State != EnvironmentStateOK {
				continue
			}

			require.True(t, fan.CurrentSpeed.OK, name)
			require.Positive(t, fan.CurrentSpeed.Value, name)
			require.Contains(t, []string{"F2B", "B2F"}, fan.Direction, name)
		}

		requireGolden(t, model, "fans", fans)

		if model == modelSN5610 {
			require.Len(t, fans, 14)
			// the fan of a PSU without power
			notAvailable := Number{Raw: "N/A"}
			require.Equal(t, Fan{
				CurrentSpeed: notAvailable, MinSpeed: notAvailable, MaxSpeed: notAvailable, Direction: "N/A", State: "absent",
			}, fans["PSU2/FAN"])
		}
	})
}
