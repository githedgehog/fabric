// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue/nvuetest"
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
		requireGolden(t, model, "breakouts", breakouts)

		ifaces, err := client.Interfaces(t.Context())
		require.NoError(t, err)

		broken := map[string]Breakout{}
		for port, breakout := range breakouts {
			require.True(t, strings.HasPrefix(port, InterfaceTypeSwp), port)

			switch breakout.Mode {
			case "":
				require.Contains(t, ifaces, port)
				require.NotEmpty(t, breakout.Speed, port)
			case BreakoutDisabled:
				require.NotContains(t, ifaces, port)
				require.Empty(t, breakout.Speed, port)
				broken[port] = breakout
			default:
				count, err := strconv.Atoi(strings.TrimSuffix(breakout.Mode, "x"))
				require.NoError(t, err, "port %s mode %s", port, breakout.Mode)
				require.Greater(t, count, 1, port)
				require.NotContains(t, ifaces, port)
				for i := range count {
					sub := fmt.Sprintf("%ss%d", port, i)
					require.Contains(t, ifaces, sub)
					// the configured speed is the one the subports run at
					if ifaces[sub].Link.OperStatus == StatusUp {
						require.Equal(t, breakout.Speed, ifaces[sub].Link.Speed, sub)
					}
				}
				broken[port] = breakout
			}
		}

		switch model {
		case modelSN5610:
			require.Equal(t, map[string]Breakout{
				"swp1":  {Mode: "8x", Speed: "100G"},
				"swp2":  {Mode: BreakoutDisabled},
				"swp3":  {Mode: "4x", Speed: "200G"},
				"swp63": {Mode: "2x", Speed: "100G"},
				"swp64": {Mode: "2x", Speed: "100G"},
			}, broken)
			require.Equal(t, Breakout{Speed: "800G"}, breakouts["swp4"])
			require.Equal(t, Breakout{Speed: "10G"}, breakouts["swp65"])
		case modelSN4600:
			require.Equal(t, map[string]Breakout{
				"swp17": {Mode: "4x", Speed: "10G"},
				"swp18": {Mode: BreakoutDisabled},
			}, broken)
			require.Equal(t, Breakout{Speed: "100G"}, breakouts["swp1"])
			require.Equal(t, Breakout{Speed: "200G"}, breakouts["swp20"])
		case modelSN2700:
			require.Equal(t, map[string]Breakout{
				"swp15": {Mode: "4x", Speed: "25G"},
				"swp16": {Mode: BreakoutDisabled},
			}, broken)
			require.Equal(t, Breakout{Speed: "100G"}, breakouts["swp1"])
		}
	})
}

func TestBreakoutsInvalid(t *testing.T) {
	for name, applied := range map[string]string{
		"two modes":    `{"swp1": {"link": {"breakout": {"2x": {}, "4x": {}}}}}`,
		"no count":     `{"swp1": {"link": {"breakout": {"x": {}}}}}`,
		"zero count":   `{"swp1": {"link": {"breakout": {"0x": {}}}}}`,
		"unknown mode": `{"swp1": {"link": {"breakout": {"loopback": {}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, nvuetest.FixtureName("/interface?filled=false&rev=applied")+".json")
			require.NoError(t, os.WriteFile(file, []byte(applied), 0o600))

			client, err := New(WithSocket(nvuetest.Serve(t, dir)))
			require.NoError(t, err)
			_, err = client.Breakouts(t.Context())
			require.ErrorContains(t, err, "port swp1 has")
		})
	}
}
