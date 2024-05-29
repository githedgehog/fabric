// Copyright 2023 Hedgehog
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
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

func TestTranslatePortNames(t *testing.T) {
	for _, tt := range []struct {
		name  string
		agent *agentapi.Agent
		spec  *dozer.Spec
		want  *dozer.Spec
		err   bool
	}{
		{
			name: "simple",
			agent: &agentapi.Agent{
				Spec: agentapi.AgentSpec{
					Switch: wiringapi.SwitchSpec{
						PortBreakouts: map[string]string{
							"E1/8": "4x25G",
							"E1/9": "2x50G",
						},
					},
					SwitchProfile: &wiringapi.SwitchProfileSpec{
						DisplayName: "Test",
						Ports: map[string]wiringapi.SwitchProfilePort{
							"M1":   {NOSName: "Management0", Management: true, OniePortName: "eth0"},
							"E1/1": {NOSName: "Ethernet0", Label: "1", Group: "1"},
							"E1/2": {NOSName: "Ethernet4", Label: "2", Group: "1"},
							"E1/3": {NOSName: "Ethernet8", Label: "3", Group: "2"},
							"E1/4": {NOSName: "Ethernet12", Label: "4", Group: "2"},
							"E1/5": {NOSName: "Ethernet16", Label: "5", Profile: "SFP28-25G"},
							"E1/6": {NOSName: "Ethernet17", Label: "6", Profile: "SFP28-25G"},
							"E1/7": {NOSName: "1/7", BaseNOSName: "Ethernet20", Label: "7", Profile: "QSFP28-100G"},
							"E1/8": {NOSName: "1/8", BaseNOSName: "Ethernet24", Label: "8", Profile: "QSFP28-100G"},
							"E1/9": {NOSName: "1/9", BaseNOSName: "Ethernet28", Label: "8", Profile: "QSFP28-100G"},
						},
						PortGroups: map[string]wiringapi.SwitchProfilePortGroup{
							"1": {
								NOSName: "G1",
								Profile: "SFP28-25G",
							},
							"2": {
								NOSName: "G2",
								Profile: "SFP28-25G",
							},
						},
						PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
							"SFP28-25G": {
								Speed: &wiringapi.SwitchProfilePortProfileSpeed{
									Default:   "25G",
									Supported: []string{"10G", "25G"},
								},
							},
							"QSFP28-100G": {
								Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
									Default: "1x100G",
									Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
										"1x100G": {Offsets: []string{"0"}},
										"1x40G":  {Offsets: []string{"0"}},
										"2x50G":  {Offsets: []string{"0", "2"}},
										"1x50G":  {Offsets: []string{"0"}},
										"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
										"4x10G":  {Offsets: []string{"0", "1", "2", "3"}},
										"1x25G":  {Offsets: []string{"0"}},
										"1x10G":  {Offsets: []string{"0"}},
									},
								},
							},
						},
					},
				},
			},
			spec: &dozer.Spec{
				LLDPInterfaces: map[string]*dozer.SpecLLDPInterface{
					"E1/1":   {Enabled: pointer.To(true)},
					"E1/8/3": {Enabled: pointer.To(true)},
				},
				PortGroups: map[string]*dozer.SpecPortGroup{
					"1": {Speed: pointer.To("10G")},
					"2": {Speed: pointer.To("10G")},
				},
				PortBreakouts: map[string]*dozer.SpecPortBreakout{
					"E1/8": {Mode: "4x25G"},
				},
				Interfaces: map[string]*dozer.SpecInterface{
					"E1/1":   {Enabled: pointer.To(true)},
					"E1/8/3": {Enabled: pointer.To(true)},
				},
				ACLInterfaces: map[string]*dozer.SpecACLInterface{
					"E1/1":   {Ingress: pointer.To("ingress")},
					"E1/8/3": {Ingress: pointer.To("ingress")},
				},
				LSTInterfaces: map[string]*dozer.SpecLSTInterface{
					"E1/1":   {Groups: []string{"group1"}},
					"E1/8/3": {Groups: []string{"group1"}},
				},
			},
			want: &dozer.Spec{
				LLDPInterfaces: map[string]*dozer.SpecLLDPInterface{
					"Ethernet0":  {Enabled: pointer.To(true)},
					"Ethernet26": {Enabled: pointer.To(true)},
				},
				PortGroups: map[string]*dozer.SpecPortGroup{
					"G1": {Speed: pointer.To("10G")},
					"G2": {Speed: pointer.To("10G")},
				},
				PortBreakouts: map[string]*dozer.SpecPortBreakout{
					"1/8": {Mode: "4x25G"},
				},
				Interfaces: map[string]*dozer.SpecInterface{
					"Ethernet0":  {Enabled: pointer.To(true)},
					"Ethernet26": {Enabled: pointer.To(true)},
				},
				ACLInterfaces: map[string]*dozer.SpecACLInterface{
					"Ethernet0":  {Ingress: pointer.To("ingress")},
					"Ethernet26": {Ingress: pointer.To("ingress")},
				},
				LSTInterfaces: map[string]*dozer.SpecLSTInterface{
					"Ethernet0":  {Groups: []string{"group1"}},
					"Ethernet26": {Groups: []string{"group1"}},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := translatePortNames(tt.agent, tt.spec)

			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, tt.spec)
		})
	}
}
