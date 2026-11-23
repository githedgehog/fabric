// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue/nvuetest"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
)

// stateModel has the fixtures of everything the switch state is collected from, see nvue/testdata/README.md
const stateModel = "sn4600"

// stateAgent is the SN4600 of env-7 the fixtures are captured from
func stateAgent() *agentapi.Agent {
	return &agentapi.Agent{
		Spec: agentapi.AgentSpec{
			SwitchProfile: switchprofile.CumulusNvidiaSN4600.Spec.DeepCopy(),
			Switch: wiringapi.SwitchSpec{
				Profile: switchprofile.CumulusNvidiaSN4600.Name,
				ASN:     65102,
				PortBreakouts: map[string]string{
					"E1/1": "1x100G", "E1/2": "1x100G", "E1/3": "1x100G", "E1/17": "4x10G", "E1/20": "1x200G",
				},
			},
		},
	}
}

func collectState(t *testing.T, proc *CumulusProcessor, agent *agentapi.Agent) *agentapi.SwitchState {
	t.Helper()

	reg := switchstate.NewRegistry()
	require.NoError(t, proc.UpdateSwitchState(t.Context(), agent, reg))
	st := reg.GetSwitchState()
	require.NotNil(t, st)

	return st
}

func TestUpdateSwitchState(t *testing.T) {
	st := collectState(t, fixtureProcessor(t, stateModel), stateAgent())
	nvuetest.RequireGolden(t, filepath.Join("testdata", "state"), stateModel, st)

	require.False(t, st.RoCE)

	require.Equal(t, "5.18.3", st.NOS.SoftwareVersion)
	require.Equal(t, "Cumulus Linux", st.NOS.ProductDescription)
	require.Equal(t, "Mellanox", st.NOS.MfgName)
	require.Equal(t, "MSN4600", st.NOS.HwskuVersion)
	require.Equal(t, "Spectrum-3", st.NOS.AsicVersion)
	require.Equal(t, "x86_64-mlnx_msn4600-r0", st.NOS.PlatformName)
	require.NotEmpty(t, st.NOS.Uptime)
	require.Contains(t, st.Firmware, "Spectrum-3")
	require.Contains(t, st.Firmware, "BIOS")

	require.NotEmpty(t, st.Platform.Fans)
	require.NotEmpty(t, st.Platform.PSUs)
	for name, psu := range st.Platform.PSUs {
		if psu.Status {
			require.Positive(t, psu.OutputPower, name)
		}
	}
	require.Contains(t, st.Platform.Temperatures, "Asic-Temp-Sensor")

	// the leaf uplinks, all ports in breakout modes are named E1/N/M
	for name, peer := range map[string]string{"E1/1/1": "sn3700c-01", "E1/2/1": "sn3700c-01", "E1/3/1": "sn2700-01"} {
		require.Contains(t, st.Interfaces, name)
		iface := st.Interfaces[name]
		require.True(t, iface.Enabled, name)
		require.Equal(t, agentapi.OperStatusUp, iface.OperStatus, name)
		require.Equal(t, "100G", iface.Speed, name)
		require.False(t, iface.LastChange.IsZero(), name)
		require.NotNil(t, iface.Counters, name)
		require.Positive(t, iface.Counters.InBits, name)
		require.Positive(t, iface.Counters.OutBits, name)
		require.NotEmpty(t, iface.Counters.Queues, name)
		require.Len(t, iface.LLDPNeighbors, 1, name)
		require.Equal(t, peer, iface.LLDPNeighbors[0].SystemName, name)
	}

	// the 4x10G DAC breakout to the servers
	for _, name := range []string{"E1/17/1", "E1/17/2", "E1/17/3", "E1/17/4"} {
		require.Contains(t, st.Interfaces, name)
		require.Equal(t, "10G", st.Interfaces[name].Speed, name)
		require.NotNil(t, st.Interfaces[name].Counters, name)
	}
	require.Equal(t, "server-05", st.Interfaces["E1/17/3"].LLDPNeighbors[0].SystemName)

	require.Contains(t, st.Interfaces, "E1/20/1")
	require.False(t, st.Interfaces["E1/20/1"].Enabled)
	require.Nil(t, st.Interfaces["E1/20/1"].Counters)

	// bonds keep their NOS names, the management port has no LLDP neighbors reported
	require.Contains(t, st.Interfaces, "bond1")
	require.Contains(t, st.Interfaces, "M1")
	require.Empty(t, st.Interfaces["M1"].LLDPNeighbors)
	for name := range st.Interfaces {
		require.False(t, strings.HasPrefix(name, "swp"), name)
	}

	// every cage of the switch, empty or not
	require.Len(t, st.Transceivers, 64)
	require.Len(t, st.Breakouts, 64)
	for _, name := range []string{"E1/1", "E1/2", "E1/3", "E1/17"} {
		require.Equal(t, "PRESENT", st.Transceivers[name].Present, name)
		require.Equal(t, "active", st.Transceivers[name].OperStatus, name)
		require.Equal(t, "QSFP28", st.Transceivers[name].FormFactor, name)
		require.Equal(t, "DAC", st.Transceivers[name].CableClass, name)
		require.InDelta(t, 1, st.Transceivers[name].CableLength, 0, name)
	}
	require.Equal(t, agentapi.SwitchStateTransceiver{}, st.Transceivers["E1/20"])
	require.Equal(t, "1x100G", st.Breakouts["E1/1"].Mode)
	require.Equal(t, "4x10G", st.Breakouts["E1/17"].Mode)
	require.Equal(t, "disabled", st.Breakouts["E1/18"].Mode)
	require.Equal(t, "1x200G", st.Breakouts["E1/20"].Mode)
}

// metricSeries returns the reported values of a metric keyed by its labels
func metricSeries(t *testing.T, reg *switchstate.Registry, name string) map[string]float64 {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	res := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "fabric_agent_"+name {
			continue
		}

		for _, metric := range family.GetMetric() {
			key := []string{} //nolint:prealloc
			for _, label := range metric.GetLabel() {
				key = append(key, label.GetName()+"="+label.GetValue())
			}
			res[strings.Join(key, ",")] = metric.GetGauge().GetValue()
		}
	}

	return res
}

// the transceivers and breakouts of the switches with optics and other breakouts than the SN4600 has
func TestUpdateTransceiversAndBreakouts(t *testing.T) {
	for _, tc := range []struct {
		model   string
		profile *wiringapi.SwitchProfileSpec
		check   func(t *testing.T, st *agentapi.SwitchState, reg *switchstate.Registry)
	}{
		{
			model:   "sn5610",
			profile: &switchprofile.CumulusNvidiaSN5610.Spec,
			check: func(t *testing.T, st *agentapi.SwitchState, reg *switchstate.Registry) {
				t.Helper()

				require.Len(t, st.Transceivers, 66)
				require.Equal(t, map[string]agentapi.SwitchStateBreakout{
					"E1/1": {Mode: "8x100G"}, "E1/2": {Mode: "disabled"}, "E1/3": {Mode: "4x200G"},
					"E1/63": {Mode: "2x100G"}, "E1/64": {Mode: "2x100G"},
				}, filterBreakouts(st.Breakouts, func(mode string) bool { return !strings.HasPrefix(mode, "1x") }))
				// the SFP28 ports can't be broken out
				require.NotContains(t, st.Breakouts, "E1/65")
				require.Equal(t, "1x800G", st.Breakouts["E1/4"].Mode)

				dac := st.Transceivers["E1/1"]
				require.Equal(t, "OSFP", dac.FormFactor)
				require.Equal(t, "DAC", dac.CableClass)
				require.InDelta(t, 2, dac.CableLength, 0)
				require.Equal(t, "5.0", dac.CMISRev)
				require.Empty(t, dac.Channels)
				require.Zero(t, dac.Temperature)

				for _, name := range []string{"E1/13", "E1/21", "E1/29"} {
					optic := st.Transceivers[name]
					require.Equal(t, "OSFP", optic.FormFactor, name)
					require.Equal(t, "FIBER", optic.CableClass, name)
					require.Positive(t, optic.Temperature, name)
					require.Positive(t, optic.Voltage, name)
					require.Len(t, optic.Channels, 8, name)
				}
				// the lanes in use have light, the ones with none at all have no reading
				for _, ch := range []string{"1", "2", "3", "4"} {
					require.Greater(t, *st.Transceivers["E1/29"].Channels[ch].In, -3.0, ch)
					require.Greater(t, *st.Transceivers["E1/29"].Channels[ch].Out, -3.0, ch)
					require.Positive(t, st.Transceivers["E1/29"].Channels[ch].Bias, ch)
				}
				for _, ch := range []string{"5", "6", "7", "8"} {
					require.Nil(t, st.Transceivers["E1/21"].Channels[ch].In, ch)
					require.Nil(t, st.Transceivers["E1/21"].Channels[ch].Out, ch)
				}

				sfp := st.Transceivers["E1/65"]
				require.Equal(t, "SFP", sfp.FormFactor)
				require.Equal(t, "FIBER", sfp.CableClass)
				require.Len(t, sfp.Channels, 1)
				require.Positive(t, sfp.Channels["1"].Bias)
				require.NotNil(t, sfp.Channels["1"].In)

				present := metricSeries(t, reg, "transceiver_present")
				require.Len(t, present, 66)
				require.InDelta(t, 1, present["transceiver=E1/13"], 0)
				require.InDelta(t, 0, present["transceiver=E1/2"], 0)
				require.Len(t, metricSeries(t, reg, "transceiver_info"), 11)
				// E1/21 has no light on half of its lanes
				require.Len(t, metricSeries(t, reg, "transceiver_in_power"), 3*8-4+2)
			},
		},
		{
			model:   "sn2700",
			profile: &switchprofile.CumulusNvidiaSN2700.Spec,
			check: func(t *testing.T, st *agentapi.SwitchState, _ *switchstate.Registry) {
				t.Helper()

				require.Len(t, st.Transceivers, 32)
				require.Equal(t, "4x25G", st.Breakouts["E1/15"].Mode)
				require.Equal(t, "disabled", st.Breakouts["E1/16"].Mode)
				require.Equal(t, "1x100G", st.Breakouts["E1/1"].Mode)

				// the DACs with diagnostics only have them for the module, the lanes have no light to report
				dac := st.Transceivers["E1/1"]
				require.Equal(t, "QSFP28", dac.Description)
				require.Positive(t, dac.Temperature)
				require.Positive(t, dac.Voltage)
				require.Len(t, dac.Channels, 4)
				for ch, chSt := range dac.Channels {
					require.Equal(t, agentapi.SwitchStateTransceiverChannel{}, chSt, ch)
				}
			},
		},
	} {
		t.Run(tc.model, func(t *testing.T) {
			proc := fixtureProcessor(t, tc.model)
			reg := switchstate.NewRegistry()
			st := &agentapi.SwitchState{
				Transceivers: map[string]agentapi.SwitchStateTransceiver{},
				Breakouts:    map[string]agentapi.SwitchStateBreakout{},
			}

			cages := cagesOf(tc.profile)
			require.NoError(t, proc.updateTransceivers(t.Context(), reg, st, cages))
			require.NoError(t, proc.updateBreakouts(t.Context(), st, cages))
			nvuetest.RequireGolden(t, filepath.Join("testdata", "state"), tc.model+"-ports", st)

			tc.check(t, st, reg)
		})
	}
}

func filterBreakouts(breakouts map[string]agentapi.SwitchStateBreakout, keep func(string) bool) map[string]agentapi.SwitchStateBreakout {
	res := map[string]agentapi.SwitchStateBreakout{}
	for name, breakout := range breakouts {
		if keep(breakout.Mode) {
			res[name] = breakout
		}
	}

	return res
}

func TestFormFactor(t *testing.T) {
	for identifier, expected := range map[string]string{
		"0x03 (SFP)":    "SFP",
		"0x11 (QSFP28)": "QSFP28",
		"0x18 (QSFP-DD Double Density 8X Pluggable Transceiver (INF-8628))": "QSFP-DD",
		"0x19 (OSFP 8X Pluggable Transceiver)":                              "OSFP",
		"unknown":                                                           "unknown",
	} {
		require.Equal(t, expected, formFactor(identifier), identifier)
	}
}

// the platform and the firmware are read once, they take seconds
func TestUpdateSwitchStateStatic(t *testing.T) {
	dir := t.TempDir()
	entries, err := os.ReadDir(filepath.Join(nvueFixtures, stateModel))
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(nvueFixtures, stateModel, entry.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, entry.Name()), data, 0o600)) //nolint:gosec // the fixtures
	}

	client, err := nvue.New(nvue.WithSocket(nvuetest.Serve(t, dir)))
	require.NoError(t, err)
	proc := Processor(client)
	first := collectState(t, proc, stateAgent())

	for _, name := range []string{"platform.json", "platform__firmware.json"} {
		require.NoError(t, os.Remove(filepath.Join(dir, name)))
	}
	second := collectState(t, proc, stateAgent())
	require.Equal(t, first.NOS.SerialNumber, second.NOS.SerialNumber)
	require.Equal(t, first.Firmware, second.Firmware)
}

func TestUpdateSwitchStateErrors(t *testing.T) {
	agent := stateAgent()
	agent.Spec.SwitchProfile = nil
	err := fixtureProcessor(t, stateModel).UpdateSwitchState(t.Context(), agent, switchstate.NewRegistry())
	require.ErrorContains(t, err, "switch profile not found")

	client, err := nvue.New(nvue.WithSocket(nvuetest.SocketPath(t)))
	require.NoError(t, err)
	err = Processor(client).UpdateSwitchState(t.Context(), stateAgent(), switchstate.NewRegistry())
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "getting RoCE"), err.Error())
}
