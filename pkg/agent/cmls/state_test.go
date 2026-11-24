// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue/nvuetest"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
)

// stateModel has the fixtures of everything the switch state is collected from, see nvue/testdata/README.md
const stateModel = "sn4600"

// stateAgent is the SN4600 the fixtures are captured from
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

	// the spines over the unnumbered uplinks and their loopbacks for the overlay, the VPCs have no BGP, the external
	// keeps its native VRF name
	require.Equal(t, []string{"172.30.8.0", "172.30.8.1", "E1/1/1", "E1/2/1", "E1/3/1"},
		slices.Sorted(maps.Keys(st.BGPNeighbors["default"])))
	for name, neighbor := range st.BGPNeighbors["default"] {
		require.Equal(t, agentapi.BGPNeighborSessionStateEstablished, neighbor.SessionState, name)
		require.Equal(t, agentapi.BGPPeerTypeExternal, neighbor.PeerType, name)
		require.Equal(t, uint32(65102), neighbor.LocalAS, name)
		require.Equal(t, uint32(65100), neighbor.PeerAS, name)
		require.Less(t, neighbor.LastEstablished.Time, fixtureTime, name)
	}
	require.Contains(t, st.BGPNeighbors["default"]["E1/1/1"].Prefixes, "IPV4_UNICAST")
	require.Contains(t, st.BGPNeighbors["default"]["172.30.8.0"].Prefixes, "L2VPN_EVPN")
	require.Equal(t, "overlay", st.BGPNeighbors["default"]["172.30.8.0"].PeerGroup)
	require.Empty(t, st.BGPNeighbors["vpc-vpc-05"])
	require.NotContains(t, st.BGPNeighbors, "mgmt")
	require.Contains(t, st.BGPNeighbors["ext-ext-bgp"], "100.1.10.6")
	require.Empty(t, st.BGPNeighbors["ext-ext-bgp"]["100.1.10.6"].PeerGroup)

	// the single hop BFD sessions of the unnumbered uplinks and the multihop ones of the overlay
	require.Equal(t, map[string][]string{"default": {"172.30.8.0", "172.30.8.1", "E1/1/1", "E1/2/1", "E1/3/1"}},
		bfdPeerNames(st))
	for name, peer := range st.BFDPeers["default"] {
		require.Equal(t, agentapi.BFDSessionStateUp, peer.SessionState, name)
		require.Less(t, peer.LastUpTime.Time, fixtureTime, name)
	}
	require.Equal(t, "underlay", st.BFDPeers["default"]["E1/1/1"].Profile)
	require.Equal(t, "overlay", st.BFDPeers["default"]["172.30.8.0"].Profile)
}

func bfdPeerNames(st *agentapi.SwitchState) map[string][]string {
	res := map[string][]string{}
	for vrf, peers := range st.BFDPeers {
		res[vrf] = slices.Sorted(maps.Keys(peers))
	}

	return res
}

// the BGP neighbors and BFD peers of the switches with other kinds of them than the SN4600 has
func TestUpdateBGPNeighborsAndBFDPeers(t *testing.T) {
	for _, tc := range []struct {
		model  string
		agent  *agentapi.Agent
		bgp    map[string][]string
		bfd    map[string][]string
		peerAS map[string]map[string]uint32
	}{
		{
			// a leaf with 2x breakout uplinks and a host BGP VPC
			model: "sn5610",
			agent: &agentapi.Agent{Spec: agentapi.AgentSpec{
				SwitchProfile: switchprofile.CumulusNvidiaSN5610.Spec.DeepCopy(),
				Switch: wiringapi.SwitchSpec{
					Profile:       switchprofile.CumulusNvidiaSN5610.Name,
					PortBreakouts: map[string]string{"E1/63": "2x100G", "E1/64": "2x100G"},
				},
			}},
			bgp: map[string][]string{
				"default":       {"172.30.8.0", "172.30.8.1", "E1/63/1", "E1/63/2", "E1/64/1", "E1/64/2"},
				"vpc-hbgp-nvue": {"E1/66"},
				// the VPCs without BGP are there, but empty
				"vpc-vpc-01": nil,
				"vpc-vpc-02": nil,
			},
			bfd: map[string][]string{"default": {
				"172.30.8.0", "172.30.8.1", "E1/63/1", "E1/63/2", "E1/64/1", "E1/64/2",
			}},
			peerAS: map[string]map[string]uint32{"default": {"E1/63/1": 65100}, "vpc-hbgp-nvue": {"E1/66": 64999}},
		},
		{
			// a spine with the leaves over unnumbered links and the gateways over numbered ones on a 4x breakout
			model: "sn3700c",
			agent: &agentapi.Agent{Spec: agentapi.AgentSpec{
				SwitchProfile: switchprofile.CumulusNvidiaSN3700c.Spec.DeepCopy(),
				Switch: wiringapi.SwitchSpec{
					Profile:       switchprofile.CumulusNvidiaSN3700c.Name,
					PortBreakouts: map[string]string{"E1/10": "1x100G", "E1/15": "4x25G", "E1/24": "1x100G"},
				},
			}},
			bgp: map[string][]string{
				"default": {
					"172.30.128.5", "172.30.128.7", "172.30.8.2", "172.30.8.3", "172.30.8.4", "172.30.8.5",
					"E1/1/1", "E1/2/1", "E1/29/1", "E1/3/1", "E1/30/1", "E1/31/1", "E1/32/1", "E1/4/1",
				},
			},
			bfd: map[string][]string{"default": {
				"172.30.128.5", "172.30.128.7", "172.30.8.2", "172.30.8.3", "172.30.8.4", "172.30.8.5",
				"E1/1/1", "E1/2/1", "E1/29/1", "E1/3/1", "E1/30/1", "E1/31/1", "E1/32/1", "E1/4/1",
			}},
			peerAS: map[string]map[string]uint32{"default": {"172.30.128.5": 65534, "172.30.8.2": 65101, "E1/1/1": 65102}},
		},
	} {
		t.Run(tc.model, func(t *testing.T) {
			proc := fixtureProcessor(t, tc.model)
			portMap, err := tc.agent.Spec.SwitchProfile.GetNOS2APIPortsFor(&tc.agent.Spec.Switch)
			require.NoError(t, err)

			reg := switchstate.NewRegistry()
			st := &agentapi.SwitchState{
				BGPNeighbors: map[string]map[string]agentapi.SwitchStateBGPNeighbor{},
				BFDPeers:     map[string]map[string]agentapi.SwitchStateBFDPeer{},
			}

			vrfs, err := proc.routedVRFs(t.Context())
			require.NoError(t, err)
			require.NoError(t, proc.updateBGPNeighbors(t.Context(), reg, st, vrfs, portMap, false))
			require.NoError(t, proc.updateBFDPeers(t.Context(), reg, st, vrfs, portMap, false))
			nvuetest.RequireGolden(t, filepath.Join("testdata", "state"), tc.model+"-routing", st)

			bgp := map[string][]string{}
			for vrf, neighbors := range st.BGPNeighbors {
				bgp[vrf] = slices.Sorted(maps.Keys(neighbors))
				for name, neighbor := range neighbors {
					require.Equal(t, agentapi.BGPNeighborSessionStateEstablished, neighbor.SessionState, name)
				}
			}
			require.Equal(t, tc.bgp, bgp)
			require.Equal(t, tc.bfd, bfdPeerNames(st))
			for vrf, peers := range tc.peerAS {
				for name, peerAS := range peers {
					require.Equal(t, peerAS, st.BGPNeighbors[vrf][name].PeerAS, "%s in %s", name, vrf)
				}
			}

			bgpSeries := 0
			for _, neighbors := range tc.bgp {
				bgpSeries += len(neighbors)
			}
			require.Len(t, metricSeries(t, reg, "bgp_neighbor_session_state"), bgpSeries)
			require.Len(t, metricSeries(t, reg, "bfd_peer_session_state"), len(tc.bfd["default"]))
		})
	}
}

func TestBGPNeighborState(t *testing.T) {
	for name, tc := range map[string]struct {
		neighbor nvue.BGPNeighbor
		state    agentapi.BGPNeighborSessionState
		peerType agentapi.BGPPeerType
		err      bool
	}{
		"internal by AS": {
			neighbor: nvue.BGPNeighbor{SessionState: "established", RemoteAS: nvue.Number{Value: 65102, OK: true}},
			state:    agentapi.BGPNeighborSessionStateEstablished,
			peerType: agentapi.BGPPeerTypeInternal,
		},
		"external by kind": {
			neighbor: nvue.BGPNeighbor{SessionState: "active", RemoteAS: nvue.Number{Raw: "external"}},
			state:    agentapi.BGPNeighborSessionStateActive,
			peerType: agentapi.BGPPeerTypeExternal,
		},
		"internal by kind": {
			neighbor: nvue.BGPNeighbor{SessionState: "opensent", RemoteAS: nvue.Number{Raw: "internal"}},
			state:    agentapi.BGPNeighborSessionStateOpenSent,
			peerType: agentapi.BGPPeerTypeInternal,
		},
		"openconfirm": {
			neighbor: nvue.BGPNeighbor{SessionState: "openconfirm", RemoteAS: nvue.Number{Value: 65100, OK: true}},
			state:    agentapi.BGPNeighborSessionStateOpenConfirm,
			peerType: agentapi.BGPPeerTypeExternal,
		},
		"clearing": {
			neighbor: nvue.BGPNeighbor{SessionState: "clearing", RemoteAS: nvue.Number{Value: 65100, OK: true}},
			state:    agentapi.BGPNeighborSessionStateIdle,
			peerType: agentapi.BGPPeerTypeExternal,
		},
		"unknown state": {
			neighbor: nvue.BGPNeighbor{SessionState: "bogus", RemoteAS: nvue.Number{Value: 65100, OK: true}},
			err:      true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			tc.neighbor.LocalAS.ASN = nvue.Number{Value: 65102, OK: true}
			tc.neighbor.PeerGroup = nvue.BGPPeerGroupNone
			// not established, so there's no uptime to count back
			tc.neighbor.Uptime = nvue.Number{Value: 1000, OK: true}

			st, err := bgpNeighborState(switchstate.NewRegistry(), "default", "n", &tc.neighbor, fixtureTime)
			if tc.err {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.state, st.SessionState)
			require.Equal(t, tc.peerType, st.PeerType)
			require.Empty(t, st.PeerGroup)
			require.Equal(t, tc.state == agentapi.BGPNeighborSessionStateEstablished, !st.LastEstablished.IsZero())
		})
	}
}

// captureWarnings collects what's logged at the warning level and above until the end of the test
func captureWarnings(t *testing.T) *strings.Builder {
	t.Helper()

	logs := &strings.Builder{}
	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(defaultLogger) })

	return logs
}

func TestReportedInterface(t *testing.T) {
	sn2700 := switchprofile.CumulusNvidiaSN2700
	portMap, err := sn2700.Spec.GetNOS2APIPortsFor(&wiringapi.SwitchSpec{Profile: sn2700.Name})
	require.NoError(t, err)
	link := &nvue.InterfaceLink{}

	for name, tc := range map[string]struct {
		nosName string
		iface   nvue.Interface
		profile string
		api     string
		warn    bool
	}{
		"port":                    {nosName: "swp1", iface: nvue.Interface{Type: "swp", Link: link}, api: "E1/1/1"},
		"management port":         {nosName: "eth0", iface: nvue.Interface{Type: "eth", Link: link}, api: "M1"},
		"bond":                    {nosName: "bond1", iface: nvue.Interface{Type: "bond", Link: link}, api: "bond1"},
		"svi":                     {nosName: "vlan1001", iface: nvue.Interface{Type: "svi", Link: link}},
		"unknown port":            {nosName: "swp99", iface: nvue.Interface{Type: "swp", Link: link}, warn: true},
		"no link":                 {nosName: "swp1", iface: nvue.Interface{Type: "swp"}, warn: true},
		"second management port":  {nosName: "eth1", iface: nvue.Interface{Type: "eth", Link: link}},
		"other profile with eth1": {nosName: "eth1", iface: nvue.Interface{Type: "eth", Link: link}, profile: "other", warn: true},
	} {
		t.Run(name, func(t *testing.T) {
			logs := captureWarnings(t)

			profile := sn2700.Name
			if tc.profile != "" {
				profile = tc.profile
			}

			api, ok := reportedInterface(tc.nosName, &tc.iface, profile, portMap)
			require.Equal(t, tc.api, api)
			require.Equal(t, tc.api != "", ok)
			require.Equal(t, tc.warn, logs.Len() > 0, logs.String())
		})
	}
}

func TestAPIIfaceName(t *testing.T) {
	portMap := map[string]string{"swp1": "E1/1/1", "swp17s1": "E1/17/2", "swp66": "E1/66"}
	for name, expected := range map[string]string{
		"swp1":         "E1/1/1",
		"swp17s1":      "E1/17/2",
		"swp17s1.1001": "E1/17/2.1001",
		"swp66.1060":   "E1/66.1060",
		"swp99":        "swp99",
		"vlan1005":     "vlan1005",
	} {
		require.Equal(t, expected, apiIfaceName(name, portMap), name)
	}
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
			require.NoError(t, proc.updateTransceivers(t.Context(), reg, st, cages, false))
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

// vxModel has the fixtures of a Cumulus VX leaf on 5.16, see nvue/testdata/README.md
const vxModel = "vx"

func vxAgent() *agentapi.Agent {
	return &agentapi.Agent{
		Spec: agentapi.AgentSpec{
			SwitchProfile: switchprofile.CumulusVX.Spec.DeepCopy(),
			Switch:        wiringapi.SwitchSpec{Profile: switchprofile.CumulusVX.Name},
		},
	}
}

func TestUpdateSwitchStateVX(t *testing.T) {
	logs := captureWarnings(t)
	st := collectState(t, fixtureProcessor(t, vxModel), vxAgent())
	nvuetest.RequireGolden(t, filepath.Join("testdata", "state"), vxModel, st)
	require.Empty(t, logs.String())

	require.Equal(t, "5.16.5", st.NOS.SoftwareVersion)

	// no traffic rates, and the hardware counters of VX are all zeroes as it has no ASIC
	uplink := st.Interfaces["E1/5"]
	require.Equal(t, agentapi.OperStatusUp, uplink.OperStatus)
	require.Equal(t, &agentapi.SwitchStateInterfaceCounters{}, uplink.Counters)
	require.Equal(t, "spine-01", uplink.LLDPNeighbors[0].SystemName)

	// the unnumbered neighbors and their BFD sessions by port, without the type 5.16 doesn't report
	for _, name := range []string{"E1/5", "E1/6", "E1/7", "E1/8"} {
		require.Equal(t, agentapi.BGPNeighborSessionStateEstablished, st.BGPNeighbors["default"][name].SessionState, name)
		require.Equal(t, agentapi.BFDSessionStateUp, st.BFDPeers["default"][name].SessionState, name)
	}
	require.Contains(t, st.BGPNeighbors["default"], "172.30.8.0")
	require.Contains(t, st.BFDPeers["default"], "172.30.8.0")
	require.NotEmpty(t, st.BGPNeighbors["ext-ext-bgp-01"])
	for vrf, neighbors := range st.BGPNeighbors {
		for name := range neighbors {
			require.False(t, strings.HasPrefix(name, "swp"), "%s in %s", name, vrf)
		}
	}
}

// the fallbacks are only for the Cumulus VX on 5.16, anything else reporting the same fails
func TestUpdateSwitchStateVXLegacyOnly(t *testing.T) {
	hardware := vxAgent()
	hardware.Spec.SwitchProfile.NOSType = meta.NOSTypeCumulusMlx
	err := fixtureProcessor(t, vxModel).UpdateSwitchState(t.Context(), hardware, switchstate.NewRegistry())
	require.ErrorContains(t, err, "getting interface rates")
	require.ErrorIs(t, err, nvue.ErrNotFound)

	proc := fixtureProcessor(t, vxModel)
	portMap, err := vxAgent().Spec.SwitchProfile.GetNOS2APIPortsFor(&vxAgent().Spec.Switch)
	require.NoError(t, err)
	st := &agentapi.SwitchState{
		BGPNeighbors: map[string]map[string]agentapi.SwitchStateBGPNeighbor{},
		BFDPeers:     map[string]map[string]agentapi.SwitchStateBFDPeer{},
	}
	err = proc.updateBGPNeighbors(t.Context(), switchstate.NewRegistry(), st, []string{"default"}, portMap, false)
	require.ErrorContains(t, err, "no type")
	err = proc.updateBFDPeers(t.Context(), switchstate.NewRegistry(), st, []string{"default"}, portMap, false)
	require.ErrorContains(t, err, "multihop as a boolean")

	// the placeholders of the transceivers are taken for modules with an unknown status
	logs := captureWarnings(t)
	st.Transceivers = map[string]agentapi.SwitchStateTransceiver{}
	cages := cagesOf(vxAgent().Spec.SwitchProfile)
	require.NoError(t, proc.updateTransceivers(t.Context(), switchstate.NewRegistry(), st, cages, false))
	require.Contains(t, logs.String(), "Unknown transceiver status")
	require.Contains(t, logs.String(), "transceiver=swp1.20")
}

func TestIsVXLegacy(t *testing.T) {
	for _, tc := range []struct {
		nosType meta.NOSType
		release string
		legacy  bool
	}{
		{nosType: meta.NOSTypeCumulusVX, release: "5.16.5", legacy: true},
		{nosType: meta.NOSTypeCumulusVX, release: "5.16.0", legacy: true},
		{nosType: meta.NOSTypeCumulusVX, release: "5.18.3"},
		{nosType: meta.NOSTypeCumulusVX, release: "5.1.0"},
		{nosType: meta.NOSTypeCumulusMlx, release: "5.16.5"},
		{nosType: meta.NOSTypeCumulusMlx, release: "5.18.3"},
	} {
		system := &nvue.System{}
		system.Version.ProductRelease = tc.release
		profile := &wiringapi.SwitchProfileSpec{NOSType: tc.nosType}
		require.Equal(t, tc.legacy, isVXLegacy(profile, system), "%s %s", tc.nosType, tc.release)
	}
}

func TestBGPNeighborUnnumbered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		typ      string
		vxLegacy bool
		want     bool
		err      string
	}{
		{name: "swp1", typ: nvue.BGPNeighborUnnumbered, want: true},
		{name: "172.30.8.0", typ: nvue.BGPNeighborNumbered},
		{name: "swp1", typ: nvue.BGPNeighborUnnumbered, vxLegacy: true, want: true},
		{name: "swp1", vxLegacy: true, want: true},
		{name: "swp1.1001", vxLegacy: true, want: true},
		{name: "172.30.8.0", vxLegacy: true},
		{name: "fd00::1", vxLegacy: true},
		{name: "swp1", err: "no type"},
		{name: "172.30.8.0", err: "no type"},
		{name: "swp1", typ: "bogus", vxLegacy: true, err: `unknown type "bogus"`},
	} {
		unnumbered, err := bgpNeighborUnnumbered(tc.name, &nvue.BGPNeighbor{Type: tc.typ}, tc.vxLegacy)
		if tc.err != "" {
			require.ErrorContains(t, err, tc.err, "%+v", tc)

			continue
		}
		require.NoError(t, err, "%+v", tc)
		require.Equal(t, tc.want, unnumbered, "%+v", tc)
	}
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
