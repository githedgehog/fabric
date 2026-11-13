// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kyaml "sigs.k8s.io/yaml"
)

// newFixtureServer serves the NVUE responses recorded from a SN5610 in testdata/nvue: "/interface?view=rates" is
// read from interface_rates.json, "/interface/swp1/counters" from interface_swp1_counters.json and so on, anything
// without a file gets the 404 nvued answers with
func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.ReplaceAll(strings.Trim(r.URL.Path, "/"), "/", "_")
		if view := r.URL.Query().Get("view"); view != "" {
			name += "_" + view
		}

		data, err := os.ReadFile(filepath.Join("testdata", "nvue", name+".json")) //nolint:gosec // the test client picks the fixture
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail": "The requested item does not exist.", "status": 404, "title": "The requested item does not exist.", "type": "/errors/ItemDoesNotExist"}`))

			return
		}

		_, _ = w.Write(data) //nolint:gosec // recorded fixtures served to the test client
	}))
	t.Cleanup(server.Close)

	return server
}

func sn5610Agent() *agentapi.Agent {
	return cumulusAgent(&switchprofile.CmlsNvidiaSN5610)
}

func cumulusAgent(sp *wiringapi.SwitchProfile) *agentapi.Agent {
	return &agentapi.Agent{
		Spec: agentapi.AgentSpec{
			Switch: wiringapi.SwitchSpec{
				Profile: sp.Name,
				ASN:     65101,
			},
			SwitchProfile: &sp.Spec,
		},
	}
}

func TestUpdateSwitchState(t *testing.T) {
	server := newFixtureServer(t)

	processor := Processor()
	processor.SetClient(nvue.New(nvue.Config{Address: server.URL, Username: "admin", Password: "secret"}))
	reg := switchstate.NewRegistry()

	require.NoError(t, processor.UpdateSwitchState(t.Context(), sn5610Agent(), reg))

	st := reg.GetSwitchState()
	require.NotNil(t, st)

	t.Run("nos", func(t *testing.T) {
		require.Equal(t, "5.18.0", st.NOS.SoftwareVersion)
		require.Equal(t, "5.18.0.0045", st.NOS.ProductVersion)
		require.Equal(t, "Debian GNU/Linux 12.12", st.NOS.DistributionVersion)
		require.Equal(t, "6.1.0-cl-1-amd64", st.NOS.KernelVersion)
		require.Equal(t, "x86_64-nvidia_sn5610-r0", st.NOS.PlatformName)
		require.Equal(t, "SN5610", st.NOS.HwskuVersion)
		require.Equal(t, "Nvidia", st.NOS.MfgName)
		require.Equal(t, "MT26186005CD", st.NOS.SerialNumber)
		require.Equal(t, "Spectrum-4", st.NOS.AsicVersion)
		require.Equal(t, "up 16 days, 6:15", st.NOS.Uptime)
		require.Equal(t, "34.2020.1086", st.Firmware["Spectrum-4"])
		require.Equal(t, "0ACTV_00.01.012", st.Firmware["BIOS"])
	})

	t.Run("interfaces", func(t *testing.T) {
		// 64 front panel ports, 2 SFP28 ports and the management port, nothing else
		require.Len(t, st.Interfaces, 67)
		require.NotContains(t, st.Interfaces, "lo")
		require.NotContains(t, st.Interfaces, "mgmt")

		swp1 := st.Interfaces["E1/1/1"]
		require.True(t, swp1.Enabled)
		require.Equal(t, agentapi.AdminStatusUp, swp1.AdminStatus)
		require.Equal(t, agentapi.OperStatusUp, swp1.OperStatus)
		require.Equal(t, "f4:20:4d:ad:bf:5e", swp1.MAC)
		require.Equal(t, "400G", swp1.Speed)
		require.Equal(t, "rs", swp1.FEC)
		require.True(t, swp1.AutoNegotiate)
		require.False(t, swp1.ErrDisabled)
		// the recorded switch runs on UTC
		require.Equal(t, time.Date(2026, 10, 14, 14, 46, 35, 336000000, time.UTC), swp1.LastChange.Time.UTC())

		require.NotNil(t, swp1.Counters)
		require.Equal(t, uint64(69746325*8), swp1.Counters.InBits)
		require.Equal(t, uint64(69130474*8), swp1.Counters.OutBits)
		require.Equal(t, uint64(33110), swp1.Counters.InDiscards)
		require.Equal(t, uint64(0), swp1.Counters.InErrors)
		require.InDelta(t, 360, swp1.Counters.InBitsPerSecond, 0.001)
		require.InDelta(t, 357, swp1.Counters.OutBitsPerSecond, 0.001)
		require.Equal(t, uint8(0), swp1.Counters.InUtilization)
		// only the queues that saw traffic are kept
		require.Equal(t, map[string]agentapi.SwitchStateInterfaceCountersQueue{
			"UC7": {TransmitBits: 69130474 * 8, TransmitPkts: 313914},
		}, swp1.Counters.Queues)

		require.Len(t, swp1.LLDPNeighbors, 1)
		require.Equal(t, agentapi.SwitchStateLLDPNeighbor{
			ChassisID:         "f4:20:4d:9d:0e:1e",
			SystemName:        "env-7-sn5610-01",
			SystemDescription: "Cumulus Linux version 5.18.0 running on Nvidia SN5610",
			PortID:            "swp62",
			PortDescription:   "swp62",
			TTL:               120,
			MAC:               "f4:20:4d:9d:0e:1e",
			Port:              "swp62",
		}, swp1.LLDPNeighbors[0])

		swp65 := st.Interfaces["E1/65"]
		require.True(t, swp65.Enabled)
		require.Equal(t, "10G", swp65.Speed)
		require.NotNil(t, swp65.Counters)

		// a port that is down still reports its (idle) rates, like on SONiC, but keeps no traffic counters
		swp2 := st.Interfaces["E1/2/1"]
		require.Equal(t, agentapi.OperStatusDown, swp2.OperStatus)
		require.NotNil(t, swp2.Counters)
		require.Zero(t, swp2.Counters.InBitsPerSecond)
		require.Zero(t, swp2.Counters.InBits)
		require.Empty(t, swp2.Counters.Queues)

		// the management port doesn't keep counters and its LLDP neighbors are noise
		mgmt := st.Interfaces["M1"]
		require.Equal(t, agentapi.OperStatusUp, mgmt.OperStatus)
		require.Equal(t, "1G", mgmt.Speed)
		require.Nil(t, mgmt.Counters)
		require.Empty(t, mgmt.LLDPNeighbors)
	})

	t.Run("transceivers", func(t *testing.T) {
		// every cage is reported, with or without a module
		require.Len(t, st.Transceivers, 66)

		dac := st.Transceivers["E1/1"]
		require.Equal(t, transceiverPresent, dac.Present)
		require.Equal(t, transceiverOperActive, dac.OperStatus)
		require.Equal(t, "OSFP", dac.FormFactor)
		require.Equal(t, "ETH_400GBASE_CR4", dac.Description)
		require.Equal(t, "Passive copper cable", dac.CableClass)
		require.InDelta(t, 2, dac.CableLength, 0.001)
		require.Equal(t, "FS", dac.Vendor)
		require.Equal(t, "OSFP-800G2OPC015", dac.VendorPart)
		require.Equal(t, "C2504422334", dac.SerialNumber)
		require.Equal(t, "64:9d:99", dac.VendorOUI)
		require.Equal(t, "Rev. 5.0", dac.CMISRev)
		require.Empty(t, dac.Channels)

		optic := st.Transceivers["E1/65"]
		require.Equal(t, transceiverPresent, optic.Present)
		require.Equal(t, "Edgecore", optic.Vendor)
		require.InDelta(t, 28.68, optic.Temperature, 0.001)
		require.InDelta(t, 3.2512, optic.Voltage, 0.0001)
		require.Len(t, optic.Channels, 1)
		require.NotNil(t, optic.Channels["1"].In)
		require.InDelta(t, -2.16, *optic.Channels["1"].In, 0.001)
		require.InDelta(t, -2.49, *optic.Channels["1"].Out, 0.001)
		require.InDelta(t, 5.54, optic.Channels["1"].Bias, 0.001)

		require.Equal(t, agentapi.SwitchStateTransceiver{Present: transceiverNotPresent}, st.Transceivers["E1/2"])
	})

	t.Run("platform", func(t *testing.T) {
		// 5 trays of 2 plus one per PSU slot, two of which are empty
		require.Len(t, st.Platform.Fans, 14)
		require.Equal(t, agentapi.SwitchStatePlatformFan{Direction: "F2B", Speed: 6974, Presence: true, Status: true}, st.Platform.Fans["FAN1/1"])
		require.Equal(t, agentapi.SwitchStatePlatformFan{}, st.Platform.Fans["PSU2/FAN"])

		require.Len(t, st.Platform.PSUs, 4)
		require.Equal(t, agentapi.SwitchStatePlatformPSU{
			OutputCurrent: 11.671, OutputPower: 145, OutputVoltage: 12.429, Presence: true, Status: true,
		}, st.Platform.PSUs["PSU1"])
		require.Equal(t, agentapi.SwitchStatePlatformPSU{Presence: true}, st.Platform.PSUs["PSU2"])

		require.Len(t, st.Platform.Temperatures, 8)
		require.Equal(t, agentapi.SwitchStatePlatformTemperature{
			Temperature: 54, HighThreshold: 105, CriticalHighThreshold: 120, LowThreshold: 5,
		}, st.Platform.Temperatures["Asic-Temp-Sensor"])
		require.Equal(t, agentapi.SwitchStatePlatformTemperature{Alarms: "absent"}, st.Platform.Temperatures["PSU2-Temp-Sensor"])
	})

	t.Run("bgp", func(t *testing.T) {
		// the BGP fixtures come from a SN4600 leaf (AS 65102) peering with two spines, the SN5610 itself runs none
		require.Len(t, st.BGPNeighbors, 1)
		neighbors := st.BGPNeighbors["default"]
		require.Len(t, neighbors, 5)

		underlay := neighbors["172.30.128.14"]
		require.True(t, underlay.Enabled)
		require.Equal(t, agentapi.BGPNeighborSessionStateEstablished, underlay.SessionState)
		require.Equal(t, agentapi.BGPPeerTypeExternal, underlay.PeerType)
		require.Equal(t, uint32(65100), underlay.PeerAS)
		require.Equal(t, uint32(65102), underlay.LocalAS)
		require.Equal(t, "underlay_spine", underlay.PeerGroup)
		require.Equal(t, "172.30.8.1", underlay.RemoteRouterID)
		require.Equal(t, uint16(179), underlay.PeerPort)
		require.Equal(t, uint64(1), underlay.ConnectionsDropped)
		require.Equal(t, uint64(2), underlay.EstablishedTransitions)
		require.Equal(t, "No AFI/SAFI activated for peer", underlay.LastResetReason)
		require.False(t, underlay.LastEstablished.IsZero())
		require.False(t, underlay.LastResetTime.IsZero())
		require.Equal(t, agentapi.BGPMessagesCounters{Open: 2, Keepalive: 688002}, underlay.Messages.Received)
		require.Equal(t, agentapi.BGPMessagesCounters{Open: 2, Keepalive: 687998}, underlay.Messages.Sent)
		require.Equal(t, map[string]agentapi.SwitchStateBGPNeighborPrefixes{
			"IPV4_UNICAST": {Received: 1, Sent: 4},
		}, underlay.Prefixes)

		overlay := neighbors["172.30.8.1"]
		require.Equal(t, "overlay", overlay.PeerGroup)
		require.Equal(t, map[string]agentapi.SwitchStateBGPNeighborPrefixes{"L2VPN_EVPN": {}}, overlay.Prefixes)
	})

	t.Run("metrics", func(t *testing.T) {
		families, err := reg.Gather()
		require.NoError(t, err)

		names := map[string]bool{}
		for _, family := range families {
			names[family.GetName()] = true
		}
		for _, name := range []string{
			"fabric_agent_interface_oper_status",
			"fabric_agent_interface_in_octets",
			"fabric_agent_interface_in_bits_per_second",
			"fabric_agent_interface_queue_transmit_pkts",
			"fabric_agent_transceiver_temperature",
			"fabric_agent_transceiver_in_power",
			"fabric_agent_platform_fan_speed",
			"fabric_agent_platform_psu_output_voltage",
			"fabric_agent_platform_sensor_temperature",
			"fabric_agent_lldp_neighbors",
			"fabric_agent_bgp_neighbor_session_state",
		} {
			require.True(t, names[name], "metric %s is missing", name)
		}
	})
}

func TestUpdateSwitchStateOffline(t *testing.T) {
	reg := switchstate.NewRegistry()
	require.NoError(t, Processor().UpdateSwitchState(t.Context(), sn5610Agent(), reg))

	st := reg.GetSwitchState()
	require.NotNil(t, st)
	require.Empty(t, st.Interfaces)
}

// TestUpdateSwitchStateLive runs against a real switch when the HH_NVUE_* environment points at one, HH_NVUE_PROFILE
// picks the switch profile and defaults to the SN5610, e.g.
//
//	HH_NVUE_ADDRESS=https://127.0.0.1:8765/nvue_v1 HH_NVUE_USERNAME=admin HH_NVUE_PASSWORD=... HH_NVUE_PROFILE=nvidia-sn4600 \
//	  go test ./pkg/agent/cmls -run Live -v
func TestUpdateSwitchStateLive(t *testing.T) {
	cfg, err := nvue.AgentConfig("")
	if errors.Is(err, nvue.ErrNoCredentials) {
		t.Skip("no live switch configured")
	}
	require.NoError(t, err)
	client := nvue.New(cfg)

	agent := sn5610Agent()
	if name := os.Getenv("HH_NVUE_PROFILE"); name != "" {
		profiles := map[string]*wiringapi.SwitchProfile{}
		for _, sp := range []*wiringapi.SwitchProfile{
			&switchprofile.CmlsNvidiaSN2201, &switchprofile.CmlsNvidiaSN2700, &switchprofile.CmlsNvidiaSN3700c,
			&switchprofile.CmlsNvidiaSN4600, &switchprofile.CmlsNvidiaSN5610,
		} {
			profiles[sp.Name] = sp
		}

		sp, ok := profiles[name]
		require.True(t, ok, "unknown cumulus switch profile %q", name)
		agent = cumulusAgent(sp)
	}

	processor := Processor()
	processor.SetClient(client)
	reg := switchstate.NewRegistry()

	start := time.Now()
	require.NoError(t, processor.UpdateSwitchState(t.Context(), agent, reg))
	t.Logf("collected in %s", time.Since(start))

	st := reg.GetSwitchState()
	require.NotNil(t, st)
	require.NotEmpty(t, st.NOS.SoftwareVersion)
	require.NotEmpty(t, st.Interfaces)

	data, err := kyaml.Marshal(st)
	require.NoError(t, err)
	t.Logf("switch state:\n%s", data)
}

func TestInterfaceState(t *testing.T) {
	st := interfaceState("swp3", &nvue.InterfaceLink{
		AdminStatus:          "up",
		OperStatus:           "lower-layer-down",
		OperStatusLastChange: "N/A",
		Protodown:            "enabled",
		AutoNegotiate:        "disabled",
	}, time.UTC)
	require.True(t, st.Enabled)
	require.Equal(t, agentapi.OperStatusLowerLayerDown, st.OperStatus)
	require.True(t, st.ErrDisabled)
	require.False(t, st.AutoNegotiate)
	require.True(t, st.LastChange.IsZero())

	st = interfaceState("swp4", &nvue.InterfaceLink{AdminStatus: "down", OperStatus: "something-new"}, time.UTC)
	require.False(t, st.Enabled)
	require.Equal(t, agentapi.OperStatusUnknown, st.OperStatus)
}

func TestBGPNeighborState(t *testing.T) {
	now := time.Now()

	numbered := bgpNeighborState(&nvue.BGPNeighbor{
		State:                  "enabled",
		Type:                   "numbered",
		PeerGroup:              "spines",
		RemoteAS:               nvue.Number{Value: 65100, OK: true, Raw: "65100"},
		SessionState:           "established",
		Uptime:                 nvue.Number{Value: 60000, OK: true},
		ConnectionsEstablished: nvue.Number{Value: 3, OK: true},
		ConnectionsDropped:     nvue.Number{Value: 2, OK: true},
		LastResetTime:          nvue.Number{Value: 120000, OK: true},
		LastResetReason:        "Admin. shutdown",
		RemoteRouterID:         "10.0.0.1",
		RemotePort:             nvue.Number{Value: 179, OK: true},
		Messages: &nvue.BGPMsgStats{
			RxOpens: nvue.Number{Value: 1, OK: true}, TxOpens: nvue.Number{Value: 2, OK: true},
			RxKeepalives: nvue.Number{Value: 10, OK: true}, TxKeepalives: nvue.Number{Value: 11, OK: true},
		},
		AF: map[string]nvue.BGPAddressFamily{
			"ipv4-unicast": {State: "enabled", TxPrefix: nvue.Number{Value: 5, OK: true}, RxPrefix: nvue.Number{Value: 7, OK: true}},
			"l2vpn-evpn":   {State: "disabled"},
		},
	}, now, 65101)

	require.True(t, numbered.Enabled)
	require.Equal(t, "spines", numbered.PeerGroup)
	require.Equal(t, uint32(65100), numbered.PeerAS)
	require.Equal(t, uint32(65101), numbered.LocalAS)
	require.Equal(t, agentapi.BGPPeerTypeExternal, numbered.PeerType)
	require.Equal(t, agentapi.BGPNeighborSessionStateEstablished, numbered.SessionState)
	require.Equal(t, uint64(3), numbered.EstablishedTransitions)
	require.Equal(t, uint64(2), numbered.ConnectionsDropped)
	require.Equal(t, uint16(179), numbered.PeerPort)
	require.Equal(t, "10.0.0.1", numbered.RemoteRouterID)
	require.WithinDuration(t, now.Add(-time.Minute), numbered.LastEstablished.Time, time.Second)
	require.WithinDuration(t, now.Add(-2*time.Minute), numbered.LastResetTime.Time, time.Second)
	require.Equal(t, agentapi.BGPMessagesCounters{Open: 1, Keepalive: 10}, numbered.Messages.Received)
	require.Equal(t, agentapi.BGPMessagesCounters{Open: 2, Keepalive: 11}, numbered.Messages.Sent)
	require.Equal(t, map[string]agentapi.SwitchStateBGPNeighborPrefixes{
		"IPV4_UNICAST": {Received: 7, Sent: 5},
	}, numbered.Prefixes)

	unnumbered := bgpNeighborState(&nvue.BGPNeighbor{
		Type:           "unnumbered",
		PeerGroup:      "none",
		RemoteAS:       nvue.Number{Raw: "internal"},
		SessionState:   "opensent",
		Uptime:         nvue.Number{Value: 60000, OK: true},
		RemoteRouterID: "0.0.0.0",
	}, now, 65101)

	require.True(t, unnumbered.Enabled)
	require.Empty(t, unnumbered.PeerGroup)
	require.Equal(t, agentapi.BGPPeerTypeInternal, unnumbered.PeerType)
	require.Equal(t, uint32(65101), unnumbered.PeerAS)
	require.Equal(t, agentapi.BGPNeighborSessionStateOpenSent, unnumbered.SessionState)
	// not established, so the uptime isn't one and there's no router ID yet
	require.True(t, unnumbered.LastEstablished.IsZero())
	require.Empty(t, unnumbered.RemoteRouterID)
}

func TestBGPPeerTypeFromGroup(t *testing.T) {
	require.Equal(t, agentapi.BGPPeerTypeUnset, bgpPeerTypeFromGroup(nil, 65100))
	require.Equal(t, agentapi.BGPPeerTypeExternal, bgpPeerTypeFromGroup(&nvue.BGPPeerGroup{Type: "external"}, 65100))
	require.Equal(t, agentapi.BGPPeerTypeInternal, bgpPeerTypeFromGroup(&nvue.BGPPeerGroup{Type: "internal"}, 65100))
	require.Equal(t, agentapi.BGPPeerTypeInternal, bgpPeerTypeFromGroup(&nvue.BGPPeerGroup{RemoteAS: nvue.Number{Raw: "internal"}}, 65100))
	require.Equal(t, agentapi.BGPPeerTypeInternal, bgpPeerTypeFromGroup(&nvue.BGPPeerGroup{RemoteAS: nvue.Number{Value: 65100, OK: true}}, 65100))
	require.Equal(t, agentapi.BGPPeerTypeExternal, bgpPeerTypeFromGroup(&nvue.BGPPeerGroup{RemoteAS: nvue.Number{Value: 65101, OK: true}}, 65100))
	require.Equal(t, agentapi.BGPPeerTypeUnset, bgpPeerTypeFromGroup(&nvue.BGPPeerGroup{}, 65100))
}

func TestFormatUptime(t *testing.T) {
	require.Equal(t, "up 0:05", formatUptime(5*60+3))
	require.Equal(t, "up 1 days, 0:00", formatUptime(24*60*60))
	require.Equal(t, "up 16 days, 6:15", formatUptime(1404905))
}
