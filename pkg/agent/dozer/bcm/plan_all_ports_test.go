// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var allPortsTestProfile = &wiringapi.SwitchProfileSpec{
	Ports: map[string]wiringapi.SwitchProfilePort{
		"M1":    {NOSName: "Management0", Management: true},
		"E1/1":  {NOSName: "Ethernet0", Profile: "SFP28-25G"},
		"E1/53": {NOSName: "1/53", BaseNOSName: "Ethernet52", Profile: "QSFP28-100G"},
		"E1/54": {NOSName: "1/54", BaseNOSName: "Ethernet56", Profile: "QSFP28-100G"},
	},
	PortProfiles: map[string]wiringapi.SwitchProfilePortProfile{
		"SFP28-25G": {
			Speed: &wiringapi.SwitchProfilePortProfileSpeed{
				Default:   "25G",
				Supported: []string{"25G"},
			},
		},
		"QSFP28-100G": {
			Breakout: &wiringapi.SwitchProfilePortProfileBreakout{
				Default: "1x100G",
				Supported: map[string]wiringapi.SwitchProfilePortProfileBreakoutMode{
					"1x100G": {Offsets: []string{"0"}},
					"4x25G":  {Offsets: []string{"0", "1", "2", "3"}},
				},
			},
		},
	},
}

// A fabric link planned on "E1/53" must not be overwritten by the "Unused" entry planAllPortsUp
// adds for "E1/53/1": both map to Ethernet52 and used to flip randomly on every reconcile.
func TestPlanAllPortsUpBreakoutBaseName(t *testing.T) {
	agent := &agentapi.Agent{
		Spec: agentapi.AgentSpec{
			SwitchProfile: allPortsTestProfile,
			Switch:        wiringapi.SwitchSpec{EnableAllPorts: true},
		},
	}
	spec := &dozer.Spec{
		Interfaces: map[string]*dozer.SpecInterface{
			"E1/53":   {Enabled: pointer.To(true), Description: pointer.To("Fabric E1/53 spine-1/E1/3")},
			"E1/54/1": {Enabled: pointer.To(true), Description: pointer.To("Fabric E1/54 spine-2/E1/3")},
		},
	}

	require.NoError(t, planAllPortsUp(agent, spec))
	require.NoError(t, translatePortNames(agent, spec))

	require.Len(t, spec.Interfaces, 3)
	require.Equal(t, "Fabric E1/53 spine-1/E1/3", *spec.Interfaces["Ethernet52"].Description)
	require.Equal(t, "Fabric E1/54 spine-2/E1/3", *spec.Interfaces["Ethernet56"].Description)
	require.Equal(t, "Unused", *spec.Interfaces["Ethernet0"].Description)
}
