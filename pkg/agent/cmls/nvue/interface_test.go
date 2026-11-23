// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var interfaceCaptures = map[string][]string{
	modelSN5610: {"/interface"},
	modelSN4700: {"/interface"},
	modelSN4600: {"/interface"},
	modelSN2700: {"/interface"},
	modelSN2201: {"/interface"},
}

var macAddressRe = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

func isPort(iface Interface) bool {
	return iface.Type == InterfaceTypeSwp || iface.Type == InterfaceTypeEth || iface.Type == InterfaceTypeBond
}

func TestInterfaces(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		ifaces, err := client.Interfaces(t.Context())
		require.NoError(t, err)
		require.Contains(t, ifaces, "eth0")

		for name, iface := range ifaces {
			if !isPort(iface) {
				continue
			}

			require.NotNil(t, iface.Link, name)
			require.Contains(t, []string{StatusUp, StatusDown}, iface.Link.AdminStatus, name)
			require.NotEmpty(t, iface.Link.OperStatus, name)
			require.Regexp(t, macAddressRe, iface.Link.MACAddress, name)
			_, err := iface.Link.LastChange(time.UTC)
			require.NoError(t, err, name)

			if iface.Type == InterfaceTypeBond {
				require.NotNil(t, iface.Bond, name)
				for member := range iface.Bond.Member {
					require.Equal(t, InterfaceTypeSwp, ifaces[member].Type, "member %s of %s", member, name)
				}
			}

			if iface.LLDP != nil {
				for id, neighbor := range iface.LLDP.Neighbor {
					require.NotEmpty(t, neighbor.Chassis.SystemName+neighbor.Chassis.ChassisID, "%s neighbor %s", name, id)
					require.NotEmpty(t, neighbor.Port.Name, "%s neighbor %s", name, id)
					require.True(t, neighbor.Port.TTL.OK, "%s neighbor %s", name, id)
				}
			}
		}

		switch model {
		case modelSN5610:
			swp29 := ifaces["swp29"]
			require.Equal(t, InterfaceTypeSwp, swp29.Type)
			require.Equal(t, "Unbundled server-01 server-01--unbundled--sn5610-01", swp29.Description)
			require.Equal(t, &InterfaceLink{
				AdminStatus:          StatusUp,
				OperStatus:           StatusUp,
				OperStatusLastChange: "2026/11/23 15:04:20.245",
				Speed:                "400G",
				AutoNegotiate:        StateEnabled,
				FEC:                  "rs",
				MACAddress:           "0a:1b:2c:00:00:20",
				MTU:                  Number{Value: 9216, OK: true, Raw: "9216"},
				Protodown:            StateDisabled,
			}, swp29.Link)
			require.Equal(t, Counter{Value: 22, OK: true}, swp29.Counters.Link.CarrierTransitions)
			require.Equal(t, Counter{Value: 11, OK: true}, swp29.Counters.Link.CarrierUpCount)
			require.Equal(t, Counter{Value: 11, OK: true}, swp29.Counters.Link.CarrierDownCount)

			lastChange, err := swp29.Link.LastChange(time.UTC)
			require.NoError(t, err)
			require.Equal(t, time.Date(2026, 11, 23, 15, 4, 20, 245_000_000, time.UTC), lastChange)

			server := swp29.LLDP.Neighbor["server-01"]
			require.Equal(t, LLDPChassis{ChassisID: "00:00:00:00:00:00", SystemName: "server-01"}, server.Chassis)
			require.Equal(t, LLDPPort{Name: "enp2s1", Type: "ifname", TTL: Number{Value: 120, OK: true, Raw: "120"}},
				server.Port)

			spine := ifaces["swp63s0"].LLDP.Neighbor["sn3700c-01"]
			require.Equal(t, LLDPChassis{
				ChassisID:         "0a:1b:2c:00:00:4a",
				SystemName:        "sn3700c-01",
				SystemDescription: "Hedgehog Fabric",
			}, spine.Chassis)
			require.Equal(t, LLDPPort{
				Name:        "swp31",
				Type:        "ifname",
				Description: "Fabric sn5610-01/E1/63/1 sn3700c-01--fabric--sn5610-01",
				TTL:         Number{Value: 20, OK: true, Raw: "20"},
			}, spine.Port)

			mgmt := ifaces["eth0"]
			require.Equal(t, InterfaceTypeEth, mgmt.Type)
			require.Equal(t, "1G", mgmt.Link.Speed)
			require.Equal(t, LLDPMED{
				DeviceType: "Network Connectivity Device",
				Inventory: LLDPInventory{
					Manufacturer:     "Accton",
					Model:            "AS4630-54NPE",
					SerialNumber:     "SN00000001",
					FirmwareRevision: "v47.01.02.02",
					SoftwareRevision: "5.10.0-39-amd64",
				},
			}, mgmt.LLDP.Neighbor["he-f2-1500-05-07-tor"].MED)

			unused := ifaces["swp4"]
			require.Equal(t, StatusDown, unused.Link.AdminStatus)
			require.Equal(t, StatusDown, unused.Link.OperStatus)
			require.Empty(t, unused.Link.Speed)

			// the broken out ports are only there as subports, the one disabled by the 8x breakout not at all
			for _, name := range []string{"swp1", "swp2", "swp3", "swp63", "swp64"} {
				require.NotContains(t, ifaces, name)
			}
			for _, name := range []string{"swp1s0", "swp1s7", "swp3s0", "swp3s3", "swp63s0", "swp63s1"} {
				require.Equal(t, InterfaceTypeSwp, ifaces[name].Type, name)
			}
		case modelSN4700:
			bond := ifaces["bond1"]
			require.Equal(t, InterfaceTypeBond, bond.Type)
			require.Equal(t, "ESLAG server-05 server-05--eslag--sn4600-01--sn4700-01", bond.Description)
			require.Equal(t, "10G", bond.Link.Speed)
			require.Equal(t, &InterfaceBond{
				Mode:       "lacp",
				LACPRate:   "fast",
				LACPBypass: "off",
				Member: map[string]InterfaceBondMember{
					"swp1s0": {BondingState: "active", MIIStatus: StatusUp},
				},
			}, bond.Bond)
		}
	})
}
