// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var interfaceCountersCaptures = map[string][]string{
	modelSN5610: {
		"/interface/swp29/counters",
		"/interface/swp63s0/counters",
		"/interface/swp1s0/counters",
		"/interface/swp3s0/counters",
		"/interface/swp65/counters",
		"/interface/swp4/counters",
		"/interface/eth0/counters",
	},
	modelSN4700: {"/interface/swp1s0/counters", "/interface/bond1/counters"},
	modelSN4600: {"/interface/swp17s0/counters"},
	modelSN2700: {"/interface/swp15s0/counters"},
	modelSN2201: {"/interface/swp1/counters"},
}

func TestInterfaceCounters(t *testing.T) {
	// the ports with fixtures, all of them when running live
	ports := map[string][]string{
		modelSN5610: {"swp29", "swp63s0", "swp1s0", "swp3s0", "swp65", "swp4"},
		modelSN4700: {"swp1s0"},
		modelSN4600: {"swp17s0"},
		modelSN2700: {"swp15s0"},
		modelSN2201: {"swp1"},
	}
	others := map[string][]string{
		modelSN5610: {"eth0"},
		modelSN4700: {"bond1"},
	}

	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		if isLive() {
			ifaces, err := client.Interfaces(t.Context())
			require.NoError(t, err)
			for name, iface := range ifaces {
				switch iface.Type {
				case InterfaceTypeSwp:
					ports[model] = append(ports[model], name)
				case InterfaceTypeEth, InterfaceTypeBond:
					others[model] = append(others[model], name)
				}
			}
		}

		for _, name := range ports[model] {
			counters, err := client.InterfaceCounters(t.Context(), name)
			require.NoError(t, err, name)
			require.True(t, counters.InBytes.OK, name)
			require.True(t, counters.OutPkts.OK, name)
			require.NotNil(t, counters.QoS, name)
			require.Len(t, counters.QoS.EgressQueueStats, 8, name)
		}

		// only the switch ports have hardware counters
		for _, name := range others[model] {
			_, err := client.InterfaceCounters(t.Context(), name)
			require.ErrorIs(t, err, ErrNotFound, name)
		}

		switch model {
		case modelSN5610:
			counters, err := client.InterfaceCounters(t.Context(), "swp29")
			require.NoError(t, err)
			require.Equal(t, &InterfaceCounters{
				InBytes:          Counter{Value: 16362, OK: true},
				InPkts:           Counter{Value: 106, OK: true},
				InUnicastPkts:    Counter{Value: 18, OK: true},
				InMulticastPkts:  Counter{Value: 60, OK: true},
				InBroadcastPkts:  Counter{Value: 28, OK: true},
				InDrops:          Counter{Value: 46, OK: true},
				InErrors:         Counter{Value: 0, OK: true},
				OutBytes:         Counter{Value: 79364, OK: true},
				OutPkts:          Counter{Value: 407, OK: true},
				OutUnicastPkts:   Counter{Value: 6, OK: true},
				OutMulticastPkts: Counter{Value: 374, OK: true},
				OutBroadcastPkts: Counter{Value: 27, OK: true},
				OutDrops:         Counter{Value: 0, OK: true},
				OutErrors:        Counter{Value: 0, OK: true},
				QoS:              counters.QoS,
			}, counters)
			zero := Counter{OK: true}
			require.Equal(t, EgressQueueStats{
				TxBytes: zero, TxFrames: zero, TxUCBufferDiscards: zero, WREDDiscards: zero, ECNMarkedFrames: zero,
			}, counters.QoS.EgressQueueStats["0"])
		case modelSN4600:
			counters, err := client.InterfaceCounters(t.Context(), "swp17s0")
			require.NoError(t, err)
			require.Equal(t, Counter{Value: 183598564681, OK: true}, counters.InBytes)
		}
	})
}
