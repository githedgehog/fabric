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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	kyaml "sigs.k8s.io/yaml"
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
				ErrDisableInterfaces: map[string]*dozer.SpecErrDisable{},
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

const testdataDir = "testdata"

func TestPlan(t *testing.T) {
	for _, tt := range []struct {
		name string
	}{
		// group: reg
		// regular vs vlab with l2vni vpcs, 2 spines, 2 mclag leafs, 2 eslag leafs, 1 standalone
		// virt ext connected to leaf-3 only, 2 subnets per vpc
		// peers: 1+2 1+3:gw:vpc1=subnet-01:vpc2=subnet-01 3+4 1~external-01:subnets=subnet-01 3~external-01:subnets=subnet-01
		{name: "reg-leaf-3"},  // eslag, external connected to it, vpc peering and ext peering
		{name: "reg-leaf-4"},  // eslag, no external connected to it, vpc peering and no ext peering
		{name: "reg-spine-1"}, // spine
		// group: l3vni
		// vs vlab with l3vni vpcs, 2 spines, 2 standalone leaves with multihomed servers, 2 hostbgp vpcs (1 and 2) and one regular vpc
		// one static proxied external and one static non-proxied external attached to leaf-01 only
		// peers: 1+3 2~ext-snp-02 3~ext-sp-01:gw:vpc-nat=masquerade:vpc-as=100.1.10.3/32
		{name: "l3vni-leaf-01"},  // standalone, static externals connected to it
		{name: "l3vni-leaf-02"},  // standalone, no externals
		{name: "l3vni-spine-01"}, // spine
		// group: mesh
		// vs lab with 2 eslag leaves and 1 orphan connected via mesh, 3 vpcs with 2 servers each
		{name: "mesh-leaf-01"}, // eslag, gateway connected to it
		{name: "mesh-leaf-02"}, // same as above
		{name: "mesh-leaf-03"}, // standalone, bgp externals connected to it
	} {
		t.Run(tt.name, func(t *testing.T) {
			updateGoldens := os.Getenv("UPDATE") == "true"

			agData, err := os.ReadFile(filepath.Join(testdataDir, tt.name+".in.agent.yaml"))
			require.NoError(t, err, "reading agent file")

			ag := &agentapi.Agent{}
			err = kyaml.Unmarshal(agData, ag)
			require.NoError(t, err, "unmarshalling agent data")

			bp := &BroadcomProcessor{}
			actualSpec, err := bp.PlanDesiredState(t.Context(), ag)
			require.NoError(t, err, "planning for agent")
			actualSpec.Normalize()

			actualSpecData, err := kyaml.Marshal(actualSpec)
			require.NoError(t, err, "marshalling spec")

			expectedFileName := filepath.Join(testdataDir, tt.name+".out.spec.expected.yaml")
			actualFileName := filepath.Join(testdataDir, tt.name+".out.spec.actual.yaml")

			err = os.WriteFile(actualFileName, actualSpecData, 0o600)
			require.NoError(t, err, "writing actual spec file")

			if updateGoldens {
				err = os.WriteFile(expectedFileName, actualSpecData, 0o600)
				require.NoError(t, err, "writing expected spec file")
			}

			expectedSpecData, err := os.ReadFile(expectedFileName)
			require.NoError(t, err, "reading expected spec file")

			require.Equal(t, string(expectedSpecData), string(actualSpecData),
				"spec mismatch, you can compare expected and actual spec files in testdata dir or re-generate expected by running just test-update")

			actions, err := bp.CalculateActions(t.Context(), &dozer.Spec{}, actualSpec)
			require.NoError(t, err, "calculating actions")

			golden := make([]goldenAction, 0, len(actions))
			for _, a := range actions {
				act := a.(*Action)

				var val map[string]any
				if act.Value != nil && !(reflect.ValueOf(act.Value).Kind() == reflect.Pointer && reflect.ValueOf(act.Value).IsNil()) {
					val, err = gnmi.Marshal(act.Value)
					require.NoError(t, err, "marshalling action value")
				}

				golden = append(golden, goldenAction{
					Weight:     act.Weight,
					Summary:    act.ASummary,
					Type:       act.Type,
					Path:       act.Path,
					Value:      val,
					CustomFunc: act.CustomFunc != nil,
				})
			}

			actualActionsData, err := kyaml.Marshal(golden)
			require.NoError(t, err, "marshalling actions")

			expectedActionsFileName := filepath.Join(testdataDir, tt.name+".out.actions.expected.yaml")
			actualActionsFileName := filepath.Join(testdataDir, tt.name+".out.actions.actual.yaml")

			err = os.WriteFile(actualActionsFileName, actualActionsData, 0o600)
			require.NoError(t, err, "writing actual actions file")

			if updateGoldens {
				err = os.WriteFile(expectedActionsFileName, actualActionsData, 0o600)
				require.NoError(t, err, "writing expected actions file")
			}

			expectedActionsData, err := os.ReadFile(expectedActionsFileName)
			require.NoError(t, err, "reading expected actions file")

			require.Equal(t, string(expectedActionsData), string(actualActionsData),
				"actions mismatch, you can compare expected and actual actions files in testdata dir or re-generate expected by running just test-update")

			mock := newGNMIMock()
			bp.client = mock
			bp.skipCustomFuncs = true
			_, err = bp.ApplyActions(t.Context(), actions)
			require.NoError(t, err, "applying actions to mock gnmi client")

			state, err := mock.StateMap()
			require.NoError(t, err, "marshalling mock gnmi state")

			actualGNMIData, err := kyaml.Marshal(state)
			require.NoError(t, err, "marshalling gnmi state to yaml")

			expectedGNMIFileName := filepath.Join(testdataDir, tt.name+".out.gnmi.expected.yaml")
			actualGNMIFileName := filepath.Join(testdataDir, tt.name+".out.gnmi.actual.yaml")

			err = os.WriteFile(actualGNMIFileName, actualGNMIData, 0o600)
			require.NoError(t, err, "writing actual gnmi state file")

			if updateGoldens {
				err = os.WriteFile(expectedGNMIFileName, actualGNMIData, 0o600)
				require.NoError(t, err, "writing expected gnmi state file")
			}

			expectedGNMIData, err := os.ReadFile(expectedGNMIFileName)
			require.NoError(t, err, "reading expected gnmi state file")

			require.Equal(t, string(expectedGNMIData), string(actualGNMIData),
				"gnmi state mismatch, you can compare expected and actual gnmi state files in testdata dir or re-generate expected by running just test-update")

			loadedSpec, err := bp.LoadActualState(t.Context(), ag)
			require.NoError(t, err, "loading actual state from mock gnmi client")

			// Strip fields that production loadActual* deliberately doesn't
			// reconstruct from gNMI (so the round-trip is fair). Mirror the
			// stripping on the planned spec so the comparison is symmetric.
			roundTripStrippedSpec, err := stripNonRoundTrippable(actualSpec)
			require.NoError(t, err, "stripping non-round-trippable fields from actual spec")

			roundTripData, err := kyaml.Marshal(roundTripStrippedSpec)
			require.NoError(t, err, "marshalling stripped planned spec")

			loadedSpecData, err := kyaml.Marshal(loadedSpec)
			require.NoError(t, err, "marshalling loaded spec")

			// Compare via YAML to normalise nil-vs-empty-map differences that
			// arise from the OC marshal/unmarshal round-trip (semantically
			// identical, but trip up reflect.DeepEqual).
			require.Equal(t, string(roundTripData), string(loadedSpecData),
				"round-trip mismatch: spec loaded back from gnmi state differs from the planned spec")
		})
	}
}

// stripNonRoundTrippable returns a deep copy of spec with fields cleared that
// production loadActual* functions deliberately don't reconstruct from gNMI.
// These are real gaps in the round-trip — by design, not bugs:
//
//   - User Password / AuthorizedKeys: the password is hashed and write-only on
//     the device; authorized keys are installed by a CustomFunc to the local
//     filesystem, never sent to gNMI. unmarshalOCUsers (spec_system.go) only
//     reads Role.
//   - Interface AutoNegotiate on management interfaces: PlanDesiredState sets
//     AutoNegotiate=true for Management0 (plan.go), but
//     specInterfaceEthernetBaseEnforcer skips non-physical interfaces, so it
//     never reaches gNMI.
//   - VRF AttachedHosts: written under
//     /protocols/protocol[ATTACHED_HOST]/attached-host but loadActualVRFs
//     does not currently parse them back.
func stripNonRoundTrippable(spec *dozer.Spec) (*dozer.Spec, error) {
	// Deep copy via YAML round-trip so we don't mutate the input spec.
	data, err := kyaml.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshaling spec: %w", err)
	}
	out := &dozer.Spec{}
	if err := kyaml.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("unmarshaling spec: %w", err)
	}

	for _, user := range out.Users {
		user.Password = ""
		user.AuthorizedKeys = nil
	}
	for name, iface := range out.Interfaces {
		if isManagement(name) {
			iface.AutoNegotiate = nil
		}
	}
	for _, vrf := range out.VRFs {
		vrf.AttachedHosts = nil
	}

	return out, nil
}

type goldenAction struct {
	Weight     ActionWeight   `json:"weight"`
	Summary    string         `json:"summary"`
	Type       ActionType     `json:"type"`
	Path       string         `json:"path"`
	Value      map[string]any `json:"value,omitempty"`
	CustomFunc bool           `json:"customFunc,omitempty"`
}
