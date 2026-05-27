// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

// TestPlanServerConnectionsFallback checks fallback placement: ESLAG on every
// leaf, MCLAG first-only.
func TestPlanServerConnectionsFallback(t *testing.T) {
	const (
		connName = "server-01--eslag--leaf-01--leaf-02"
		pcName   = "PortChannel42"
	)

	// agentName is the leaf we plan for; first is the alphabetically-first leaf
	// of the redundancy group (the one IsFirstInRedundancyGroup() returns true for).
	const (
		firstLeaf  = "leaf-01"
		secondLeaf = "leaf-02"
	)

	newAgent := func(agentName string, redType meta.RedundancyType, conn wiringapi.ConnectionSpec) *agentapi.Agent {
		ag := &agentapi.Agent{}
		ag.Name = agentName
		ag.Spec.RedundancyGroupPeers = []string{firstLeaf, secondLeaf}
		ag.Spec.Switch.Redundancy = wiringapi.SwitchRedundancy{
			Type:  redType,
			Group: "rg-1",
		}
		ag.Spec.Connections = map[string]wiringapi.ConnectionSpec{connName: conn}
		ag.Spec.Catalog.PortChannelIDs = map[string]uint16{connName: 42}
		ag.Spec.Catalog.ConnectionIDs = map[string]uint32{connName: 1}
		ag.Spec.Config.ESLAGMACBase = "f2:00:00:00:00:00"
		ag.Spec.Config.ESLAGESIPrefix = "00:f2:00:00:"
		ag.Spec.Config.FabricMTU = 9100
		ag.Spec.Config.ServerFacingMTUOffset = 64

		return ag
	}

	newSpec := func() *dozer.Spec {
		return &dozer.Spec{
			Interfaces:         map[string]*dozer.SpecInterface{},
			MCLAGInterfaces:    map[string]*dozer.SpecMCLAGInterface{},
			PortChannelConfigs: map[string]*dozer.SpecPortChannelConfig{},
			VRFs: map[string]*dozer.SpecVRF{
				VRFDefault: {EthernetSegments: map[string]*dozer.SpecVRFEthernetSegment{}},
			},
		}
	}

	link := func(leaf string) wiringapi.ServerToSwitchLink {
		return wiringapi.ServerToSwitchLink{
			Server: wiringapi.BasePortName{Port: "server-01/enp0s1"},
			Switch: wiringapi.BasePortName{Port: leaf + "/E1/1"},
		}
	}

	fallbackOf := func(t *testing.T, spec *dozer.Spec) bool {
		t.Helper()
		pc, ok := spec.PortChannelConfigs[pcName]
		require.True(t, ok, "expected port channel config %s", pcName)
		require.NotNil(t, pc.Fallback, "expected fallback to be set")

		return *pc.Fallback
	}

	planFor := func(t *testing.T, agentName string, redType meta.RedundancyType, conn wiringapi.ConnectionSpec) *dozer.Spec {
		t.Helper()
		ag := newAgent(agentName, redType, conn)
		spec := newSpec()
		require.NoError(t, planServerConnections(ag, spec))

		return spec
	}

	eslagConn := func(fallback bool) wiringapi.ConnectionSpec {
		return wiringapi.ConnectionSpec{
			ESLAG: &wiringapi.ConnESLAG{
				Links:    []wiringapi.ServerToSwitchLink{link(firstLeaf), link(secondLeaf)},
				Fallback: fallback,
			},
		}
	}

	mclagConn := func(fallback bool) wiringapi.ConnectionSpec {
		return wiringapi.ConnectionSpec{
			MCLAG: &wiringapi.ConnMCLAG{
				Links:    []wiringapi.ServerToSwitchLink{link(firstLeaf), link(secondLeaf)},
				Fallback: fallback,
			},
		}
	}

	t.Run("eslag fallback programmed on first leaf", func(t *testing.T) {
		require.True(t, fallbackOf(t, planFor(t, firstLeaf, meta.RedundancyTypeESLAG, eslagConn(true))))
	})

	t.Run("eslag fallback programmed on second leaf", func(t *testing.T) {
		// the fix: a non-first ESLAG leaf must also program fallback
		require.True(t, fallbackOf(t, planFor(t, secondLeaf, meta.RedundancyTypeESLAG, eslagConn(true))))
	})

	t.Run("eslag fallback off stays off on both leaves", func(t *testing.T) {
		require.False(t, fallbackOf(t, planFor(t, firstLeaf, meta.RedundancyTypeESLAG, eslagConn(false))))
		require.False(t, fallbackOf(t, planFor(t, secondLeaf, meta.RedundancyTypeESLAG, eslagConn(false))))
	})

	t.Run("mclag fallback only on first leaf", func(t *testing.T) {
		require.True(t, fallbackOf(t, planFor(t, firstLeaf, meta.RedundancyTypeMCLAG, mclagConn(true))))
		require.False(t, fallbackOf(t, planFor(t, secondLeaf, meta.RedundancyTypeMCLAG, mclagConn(true))))
	})
}
