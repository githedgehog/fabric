// Copyright 2024 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package switchprofile

import (
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// TestDellFECConfigurablePorts guards the FEC port-name rule on a real Dell profile:
// Switch.Validate gates PortFECs on GetFECConfigurablePorts, which must accept the base name
// of a NON-broken-out QSFP port (e.g. E1/1 at 1x100G) as well as its sub-port (E1/1/1), but
// once the port is broken out only the sub-port names are valid. Management ports are never
// configurable.
func TestDellFECConfigurablePorts(t *testing.T) {
	t.Run("not broken out: base and first sub-port both valid", func(t *testing.T) {
		ports, err := DellS5232FON.Spec.GetFECConfigurablePorts(&wiringapi.SwitchSpec{})
		require.NoError(t, err)

		require.True(t, ports["E1/1"], "base name of a non-broken-out QSFP port must be configurable")
		require.True(t, ports["E1/1/1"], "sub-port of a non-broken-out QSFP port must be configurable")
		require.False(t, ports["M1"], "management port M1 must not be configurable")
	})

	t.Run("broken out: only sub-ports valid, base rejected", func(t *testing.T) {
		ports, err := DellS5232FON.Spec.GetFECConfigurablePorts(&wiringapi.SwitchSpec{
			PortBreakouts: map[string]string{"E1/1": "4x25G"},
		})
		require.NoError(t, err)

		require.False(t, ports["E1/1"], "base name of a broken-out port must be rejected")
		for _, sub := range []string{"E1/1/1", "E1/1/2", "E1/1/3", "E1/1/4"} {
			require.True(t, ports[sub], "breakout sub-port %s must be configurable", sub)
		}
	})
}
