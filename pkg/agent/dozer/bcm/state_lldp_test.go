// Copyright 2026 Hedgehog
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
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ocNeighbor = oc.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor

// ocNeighborOpts is a single LLDP neighbor as reported by the switch, id is the neighbor list key (the source MAC).
type ocNeighborOpts struct {
	id         string
	chassisID  string
	sysName    string
	sysDescr   string
	portID     string
	portDescr  string
	portIDType oc.E_OpenconfigLldp_PortIdType
	ttl        *uint16
	lastUpdate *int64
	manuf      string
}

func ocNeighbors(opts ...ocNeighborOpts) map[string]*ocNeighbor {
	neighbors := map[string]*ocNeighbor{}

	for _, o := range opts {
		st := &oc.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_State{
			PortIdType: o.portIDType,
			Ttl:        o.ttl,
			LastUpdate: o.lastUpdate,
		}

		for val, dst := range map[string]**string{
			o.chassisID: &st.ChassisId,
			o.sysName:   &st.SystemName,
			o.sysDescr:  &st.SystemDescription,
			o.portID:    &st.PortId,
			o.portDescr: &st.PortDescription,
		} {
			if val != "" {
				*dst = pointer.To(val)
			}
		}

		neighbor := &ocNeighbor{Id: pointer.To(o.id), State: st}

		if o.manuf != "" {
			neighbor.Med = &oc.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_Med{
				State: &oc.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_Med_State{
					Inventory: &oc.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_Med_State_Inventory{
						Manufacturer: pointer.To(o.manuf),
					},
				},
			}
		}

		neighbors[o.id] = neighbor
	}

	return neighbors
}

func TestIsMACAddress(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want bool
	}{
		{in: "b4:db:91:9b:60:27", want: true},
		{in: "0A:00:00:00:1A:01", want: true},
		{in: "Ethernet480"},
		{in: "enp2s2"},
		{in: ""},
		// chassis IDs aren't always MACs
		{in: "77924ab4a93b41d4928e000000000003"},
		// ParseMAC accepts these, we only want the colon-separated 6 octet form
		{in: "b4-db-91-9b-60-27"},
		{in: "b4db.919b.6027"},
		{in: "b4db919b6027"},
		{in: "aa:bb:cc:dd:ee:ff:00:11"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isMACAddress(tt.in))
		})
	}
}

func TestLLDPNeighbors(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name string
		in   map[string]*ocNeighbor
		want []agentapi.SwitchStateLLDPNeighbor
	}{
		{
			name: "no neighbors",
			in:   ocNeighbors(),
		},
		{
			// fabric neighbor: NOS port name as the port ID, source MAC only in the list key
			name: "fabric switch",
			in: ocNeighbors(ocNeighborOpts{
				id:        "b4:db:91:9b:60:27",
				chassisID: "b4:db:91:9b:60:24",
				sysName:   "ds5000-03",
				sysDescr:  "Hedgehog Fabric",
				portID:    "Ethernet480",
				portDescr: "Fabric ds5000-01/E1/61/1 ds5000-03--fabric--ds5000-01",
				manuf:     "Celestica",
			}),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					ChassisID:         "b4:db:91:9b:60:24",
					SystemName:        "ds5000-03",
					SystemDescription: "Hedgehog Fabric",
					PortID:            "Ethernet480",
					PortDescription:   "Fabric ds5000-01/E1/61/1 ds5000-03--fabric--ds5000-01",
					MAC:               "b4:db:91:9b:60:27",
					Port:              "Ethernet480",
					Manufacturer:      "Celestica",
				},
			},
		},
		{
			// DPU: port ID is a MAC, the port name is only in the description
			name: "MAC port ID with description",
			in: ocNeighbors(ocNeighborOpts{
				id:        "0a:00:00:00:1a:01",
				chassisID: "0a:00:00:00:1a:00",
				sysName:   "dpu-1",
				portID:    "0a:00:00:00:1a:01",
				portDescr: "p0",
			}),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					ChassisID:       "0a:00:00:00:1a:00",
					SystemName:      "dpu-1",
					PortID:          "0a:00:00:00:1a:01",
					PortDescription: "p0",
					MAC:             "0a:00:00:00:1a:01",
					Port:            "p0",
				},
			},
		},
		{
			name: "MAC port ID without description",
			in: ocNeighbors(ocNeighborOpts{
				id:      "0c:48:c6:97:3b:12",
				sysName: "ds3000-06",
				portID:  "0C:48:C6:97:3B:12",
			}),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					SystemName: "ds3000-06",
					PortID:     "0C:48:C6:97:3B:12",
					MAC:        "0c:48:c6:97:3b:12",
					Port:       "0C:48:C6:97:3B:12",
				},
			},
		},
		{
			// only the reported type tells us it's a MAC, but it's not in a form we can use, so no MAC is reported
			name: "MAC port ID by type only",
			in: ocNeighbors(ocNeighborOpts{
				id:         "6e:27:d4:e2:6b:f7",
				sysName:    "server-1",
				portID:     "6e27d4e26bf7",
				portDescr:  "enp2s2",
				portIDType: oc.OpenconfigLldp_PortIdType_MAC_ADDRESS,
			}),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					SystemName:      "server-1",
					PortID:          "6e27d4e26bf7",
					PortDescription: "enp2s2",
					Port:            "enp2s2",
				},
			},
		},
		{
			// interface name as the port ID, source MAC only in the list key
			name: "interface name port ID",
			in: ocNeighbors(ocNeighborOpts{
				id:        "6e:27:d4:e2:6b:f7",
				chassisID: "77924ab4a93b41d4928e000000000003",
				sysName:   "server-1",
				portID:    "enp2s2",
			}),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					ChassisID:  "77924ab4a93b41d4928e000000000003",
					SystemName: "server-1",
					PortID:     "enp2s2",
					MAC:        "6e:27:d4:e2:6b:f7",
					Port:       "enp2s2",
				},
			},
		},
		{
			// same system on two ports of the same interface, both are real neighbors
			name: "multiple neighbors sorted",
			in: ocNeighbors(
				ocNeighborOpts{
					id:        "4c:bb:47:e8:ef:fa",
					chassisID: "4c:bb:47:e8:ef:f9",
					sysName:   "spark-1.lan",
					portID:    "4c:bb:47:e8:ef:fa",
					portDescr: "enp1s0f0np0",
				},
				ocNeighborOpts{
					id:        "4c:bb:47:e8:ef:fe",
					chassisID: "4c:bb:47:e8:ef:f9",
					sysName:   "spark-1.lan",
					portID:    "4c:bb:47:e8:ef:fe",
					portDescr: "enP2p1s0f0np0",
				},
			),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					ChassisID:       "4c:bb:47:e8:ef:f9",
					SystemName:      "spark-1.lan",
					PortID:          "4c:bb:47:e8:ef:fe",
					PortDescription: "enP2p1s0f0np0",
					MAC:             "4c:bb:47:e8:ef:fe",
					Port:            "enP2p1s0f0np0",
				},
				{
					ChassisID:       "4c:bb:47:e8:ef:f9",
					SystemName:      "spark-1.lan",
					PortID:          "4c:bb:47:e8:ef:fa",
					PortDescription: "enp1s0f0np0",
					MAC:             "4c:bb:47:e8:ef:fa",
					Port:            "enp1s0f0np0",
				},
			},
		},
		{
			// nothing but sysDescr sets these apart, the tie breakers have to keep the order stable
			name: "neighbors differing only in a tie breaker",
			in: ocNeighbors(
				ocNeighborOpts{
					id:        "0a:00:00:00:1a:01",
					sysName:   "dpu-1",
					sysDescr:  "Ubuntu 24.04",
					portID:    "0a:00:00:00:1a:01",
					portDescr: "p0",
				},
				ocNeighborOpts{
					id:        "0A:00:00:00:1A:01",
					sysName:   "dpu-1",
					sysDescr:  "Debian 13",
					portID:    "0a:00:00:00:1a:01",
					portDescr: "p0",
				},
			),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					SystemName:        "dpu-1",
					SystemDescription: "Debian 13",
					PortID:            "0a:00:00:00:1a:01",
					PortDescription:   "p0",
					MAC:               "0a:00:00:00:1a:01",
					Port:              "p0",
				},
				{
					SystemName:        "dpu-1",
					SystemDescription: "Ubuntu 24.04",
					PortID:            "0a:00:00:00:1a:01",
					PortDescription:   "p0",
					MAC:               "0a:00:00:00:1a:01",
					Port:              "p0",
				},
			},
		},
		{
			// the switch can report the same neighbor twice, both entries are kept as they're indistinguishable
			name: "same neighbor reported twice",
			in: ocNeighbors(
				ocNeighborOpts{
					id:        "0a:00:00:00:1a:01",
					chassisID: "0a:00:00:00:1a:00",
					sysName:   "dpu-1",
					portID:    "0a:00:00:00:1a:01",
					portDescr: "p0",
					ttl:       pointer.To(uint16(120)),
				},
				ocNeighborOpts{
					id:        "0A:00:00:00:1A:01",
					chassisID: "0a:00:00:00:1a:00",
					sysName:   "dpu-1",
					portID:    "0a:00:00:00:1a:01",
					portDescr: "p0",
					ttl:       pointer.To(uint16(120)),
				},
			),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					ChassisID:       "0a:00:00:00:1a:00",
					SystemName:      "dpu-1",
					PortID:          "0a:00:00:00:1a:01",
					PortDescription: "p0",
					MAC:             "0a:00:00:00:1a:01",
					Port:            "p0",
					TTL:             120,
				},
				{
					ChassisID:       "0a:00:00:00:1a:00",
					SystemName:      "dpu-1",
					PortID:          "0a:00:00:00:1a:01",
					PortDescription: "p0",
					MAC:             "0a:00:00:00:1a:01",
					Port:            "p0",
					TTL:             120,
				},
			},
		},
		{
			name: "incomplete entries skipped",
			in: map[string]*ocNeighbor{
				"0a:00:00:00:1a:01": nil,
				"0a:00:00:00:1a:02": {},
				"0a:00:00:00:1a:03": ocNeighbors(ocNeighborOpts{
					id:      "0a:00:00:00:1a:03",
					sysName: "dpu-1",
					portID:  "Ethernet0",
				})["0a:00:00:00:1a:03"],
			},
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					SystemName: "dpu-1",
					PortID:     "Ethernet0",
					MAC:        "0a:00:00:00:1a:03",
					Port:       "Ethernet0",
				},
			},
		},
		{
			name: "ttl and last update",
			in: ocNeighbors(
				ocNeighborOpts{
					id:         "0a:00:00:00:1a:01",
					sysName:    "a-just-updated",
					portID:     "Ethernet0",
					ttl:        pointer.To(uint16(120)),
					lastUpdate: pointer.To(int64(0)),
				},
				ocNeighborOpts{
					id:         "0a:00:00:00:1a:02",
					sysName:    "b-stale",
					portID:     "Ethernet1",
					ttl:        pointer.To(uint16(120)),
					lastUpdate: pointer.To(int64(4200)),
				},
				ocNeighborOpts{
					id:      "0a:00:00:00:1a:03",
					sysName: "c-unset",
					portID:  "Ethernet2",
				},
			),
			want: []agentapi.SwitchStateLLDPNeighbor{
				{
					SystemName: "a-just-updated",
					PortID:     "Ethernet0",
					MAC:        "0a:00:00:00:1a:01",
					Port:       "Ethernet0",
					TTL:        120,
					LastUpdate: &kmetav1.Time{Time: now},
				},
				{
					SystemName: "b-stale",
					PortID:     "Ethernet1",
					MAC:        "0a:00:00:00:1a:02",
					Port:       "Ethernet1",
					TTL:        120,
					LastUpdate: &kmetav1.Time{Time: now.Add(-70 * time.Minute)},
				},
				{
					SystemName: "c-unset",
					PortID:     "Ethernet2",
					MAC:        "0a:00:00:00:1a:03",
					Port:       "Ethernet2",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lldpNeighbors(tt.in, now)
			require.Equal(t, tt.want, got)

			if len(got) > 0 {
				// unset timestamps should be omitted, not reported as null
				data, err := json.Marshal(got)
				require.NoError(t, err)
				require.NotContains(t, string(data), "null")
			}
		})
	}
}

// TestUpdateLLDPNeighbors covers both halves of the management port rule: neighbors of the switch own management
// interface aren't collected, while a neighbor of a data port is kept even if it reports a management looking port.
func TestUpdateLLDPNeighbors(t *testing.T) {
	t.Parallel()

	ifaceWithNeighbor := func(iface, sysName string) *oc.OpenconfigLldp_Lldp_Interfaces_Interface {
		return &oc.OpenconfigLldp_Lldp_Interfaces_Interface{
			Name: pointer.To(iface),
			Neighbors: &oc.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors{
				Neighbor: ocNeighbors(ocNeighborOpts{
					id:        "0c:48:c6:97:3b:12",
					sysName:   sysName,
					portID:    "0c:48:c6:97:3b:12",
					portDescr: "eth0",
				}),
			},
		}
	}

	client := newGNMIMock()
	client.root = &oc.Device{
		Lldp: &oc.OpenconfigLldp_Lldp{
			Interfaces: &oc.OpenconfigLldp_Lldp_Interfaces{
				Interface: map[string]*oc.OpenconfigLldp_Lldp_Interfaces_Interface{
					"Ethernet0":   ifaceWithNeighbor("Ethernet0", "ds3000-06"),
					"Management0": ifaceWithNeighbor("Management0", "ds3000-07"),
				},
			},
		},
	}

	p := &BroadcomProcessor{client: client}
	swState := &agentapi.SwitchState{Interfaces: map[string]agentapi.SwitchStateInterface{}}
	portMap := map[string]string{"Ethernet0": "E1/1", "Management0": "M1"}

	require.NoError(t, p.updateLLDPNeighbors(context.Background(), swState, portMap))

	require.Equal(t, map[string]agentapi.SwitchStateInterface{
		"E1/1": {
			LLDPNeighbors: []agentapi.SwitchStateLLDPNeighbor{
				{
					SystemName:      "ds3000-06",
					PortID:          "0c:48:c6:97:3b:12",
					PortDescription: "eth0",
					MAC:             "0c:48:c6:97:3b:12",
					Port:            "eth0",
				},
			},
		},
	}, swState.Interfaces)
}
