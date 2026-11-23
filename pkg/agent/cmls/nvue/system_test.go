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

		switch model {
		case modelSN5610:
			require.Equal(t, &System{
				Hostname:    "sn5610-01",
				ProductName: "Cumulus Linux",
				Platform:    "x86_64-nvidia_sn5610-r0",
				Uptime:      Number{Value: 66213, OK: true, Raw: "66213"},
				Version: SystemVersion{
					ProductRelease: "5.18.3",
					BaseOS:         "Debian GNU/Linux 12.15",
					Kernel:         "6.1.0-cl-1-amd64",
					ONIE:           "2025.05-5.3.0017-115200",
					Image:          SystemVersionImage{BuildID: "5.18.3.0001", BuildDate: "Sun Oct 04 17:27:31 UTC 2026"},
				},
				DateTime: SystemDateTime{Timezone: "Etc/UTC", LocalTime: "2026-11-23 19:26:59"},
				Health: SystemHealth{
					Status:    "Not OK",
					StatusLED: "amber",
					Issues: map[string]SystemHealthIssue{
						"PSU2":              {Issue: "bad"},
						"PSU2-Temp-Sensor":  {Issue: "absent"},
						"PSU2/FAN":          {Issue: "absent"},
						"PSU4":              {Issue: "bad"},
						"PSU4-Temp-Sensor":  {Issue: "absent"},
						"PSU4/FAN":          {Issue: "absent"},
						"ps_r_policy_check": {Issue: "bad : Good PSUs - 2 :: PSU redundancy Required PSUs - 4"},
					},
				},
			}, system)
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
