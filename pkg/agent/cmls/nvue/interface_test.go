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

		requireGolden(t, model, "interfaces", ifaces)

		// what the switches are set up with for the fixtures, see testdata/README.md
		switch model {
		case modelSN5610:
			swp29 := ifaces["swp29"]
			require.Equal(t, StatusUp, swp29.Link.OperStatus)
			require.Equal(t, "400G", swp29.Link.Speed)
			require.Contains(t, swp29.LLDP.Neighbor, "server-01")

			spine := ifaces["swp63s0"].LLDP.Neighbor["sn3700c-01"]
			require.Equal(t, "Hedgehog Fabric", spine.Chassis.SystemDescription)
			require.Equal(t, "ifname", spine.Port.Type)

			// the neighbor of the management port has its inventory in LLDP-MED
			for _, neighbor := range ifaces["eth0"].LLDP.Neighbor {
				require.NotEmpty(t, neighbor.MED.Inventory.Model)
			}

			unused := ifaces["swp4"]
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
			require.Equal(t, "lacp", bond.Bond.Mode)
			require.Equal(t, map[string]InterfaceBondMember{
				"swp1s0": {BondingState: "active", MIIStatus: StatusUp},
			}, bond.Bond.Member)
		}
	})
}

func TestInterfaceLinkLastChange(t *testing.T) {
	link := &InterfaceLink{OperStatusLastChange: "2026/11/23 15:04:20.245"}

	loc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	lastChange, err := link.LastChange(loc)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 11, 23, 23, 4, 20, 245_000_000, time.UTC), lastChange.UTC())

	link.OperStatusLastChange = "N/A"
	_, err = link.LastChange(time.UTC)
	require.ErrorContains(t, err, `parsing last change "N/A"`)
}
