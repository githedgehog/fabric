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
