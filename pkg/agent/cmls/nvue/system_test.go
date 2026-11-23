// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var systemCaptures = map[string][]string{
	modelSN5610: {"/system"},
	modelSN4700: {"/system"},
	modelSN4600: {"/system"},
	modelSN2700: {"/system"},
	modelSN2201: {"/system"},
}

func TestSystem(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		system, err := client.System(t.Context())
		require.NoError(t, err)

		require.NotEmpty(t, system.Hostname)
		require.Equal(t, "Cumulus Linux", system.ProductName)
		require.Regexp(t, `^x86_64-(nvidia|mlnx)_.+-r\d+$`, system.Platform)
		require.True(t, system.Uptime.OK)
		require.Positive(t, system.Uptime.Value)
		require.Regexp(t, `^\d+\.\d+\.\d+$`, system.Version.ProductRelease)
		require.NotEmpty(t, system.Version.Kernel)
		require.NotEmpty(t, system.Version.BaseOS)
		require.NotEmpty(t, system.Version.Image.BuildID)
		require.Contains(t, []string{"OK", "Not OK"}, system.Health.Status)

		_, err = system.Location()
		require.NoError(t, err)

		requireGolden(t, model, "system", system)

		switch model {
		case modelSN5610:
			require.Equal(t, "sn5610-01", system.Hostname)
			require.Equal(t, "x86_64-nvidia_sn5610-r0", system.Platform)
			// two of its PSUs have no power
			require.Equal(t, "Not OK", system.Health.Status)
			require.Equal(t, SystemHealthIssue{Issue: "bad"}, system.Health.Issues["PSU2"])
		case modelSN4700:
			// the older Mellanox ones have a different platform name
			require.Equal(t, "x86_64-mlnx_msn4700-r0", system.Platform)
		case modelSN2700:
			require.Equal(t, "x86_64-mlnx_x86-r0", system.Platform)
		}
	})
}

func TestSystemLocation(t *testing.T) {
	system := &System{DateTime: SystemDateTime{Timezone: "Etc/UTC"}}
	loc, err := system.Location()
	require.NoError(t, err)
	require.Equal(t, "Etc/UTC", loc.String())

	system.DateTime.Timezone = "Nowhere/Special"
	_, err = system.Location()
	require.ErrorContains(t, err, `loading timezone "Nowhere/Special"`)
}
