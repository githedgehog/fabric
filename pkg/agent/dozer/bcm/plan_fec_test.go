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

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

// TestPlanPortFECs verifies that an explicit FEC entry is applied to the correct NOS-keyed
// interface regardless of whether the user (or the connection) used the base name or the
// breakout sub-port name for a non-broken-out QSFP port — both resolve to the same NOS name.
func TestPlanPortFECs(t *testing.T) {
	profile := &wiringapi.SwitchProfileSpec{
		Ports: map[string]wiringapi.SwitchProfilePort{
			"M1":    {NOSName: "Management0", Management: true},
			"E1/53": {NOSName: "1/53", BaseNOSName: "Ethernet64", Profile: "QSFP28-100G"},
		},
		PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
			"QSFP28-100G": {
				Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
					Default: "1x100G",
					Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
						"1x100G": {Offsets: []string{"0"}},
					},
				},
			},
		},
	}

	for _, tt := range []struct {
		name   string
		fecKey string
		want   string // expected FEC on Ethernet64; "" means nil
	}{
		{"base name resolves to NOS interface", "E1/53", "rs"},
		{"sub-port name resolves to NOS interface", "E1/53/1", "rs"},
		{"unknown port is ignored", "E9/9", ""},
		{"management port is ignored", "M1", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent := &agentapi.Agent{
				Spec: agentapi.AgentSpec{
					SwitchProfile: profile,
					Switch: wiringapi.SwitchSpec{
						PortFECs: map[string]wiringapi.PortFECMode{
							tt.fecKey: wiringapi.PortFECModeRS,
						},
					},
				},
			}
			// spec.Interfaces is NOS-keyed at this point (planPortFECs runs after translation)
			spec := &dozer.Spec{
				Interfaces: map[string]*dozer.SpecInterface{
					"Ethernet64": {},
				},
			}

			err := planPortFECs(agent, spec)
			require.NoError(t, err)

			if tt.want == "" {
				require.Nil(t, spec.Interfaces["Ethernet64"].FEC)
			} else {
				require.NotNil(t, spec.Interfaces["Ethernet64"].FEC)
				require.Equal(t, tt.want, *spec.Interfaces["Ethernet64"].FEC)
			}
		})
	}
}
