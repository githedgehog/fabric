// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

var bgpCaptures = map[string][]string{
	modelSN5610: {
		"/vrf/default/router/bgp/neighbor?view=detail",
		"/vrf/vpc-hbgp-nvue/router/bgp/neighbor?view=detail",
		"/vrf/vpc-vpc-01/router/bgp/neighbor?view=detail",
		"/vrf/mgmt/router/bgp/neighbor?view=detail",
	},
	modelSN4600: {
		"/vrf/default/router/bgp/neighbor?view=detail",
		"/vrf/ext-ext-bgp/router/bgp/neighbor?view=detail",
	},
	modelSN3700C: {"/vrf/default/router/bgp/neighbor?view=detail"},
}

// the VRFs with fixtures, all of them when running live
var bgpVRFs = map[string][]string{
	modelSN5610:  {VRFDefault, "vpc-hbgp-nvue", "vpc-vpc-01", VRFMgmt},
	modelSN4600:  {VRFDefault, "ext-ext-bgp"},
	modelSN3700C: {VRFDefault},
}

func routingVRFs(t *testing.T, client *Client, model string) []string {
	t.Helper()

	if !isLive() {
		return bgpVRFs[model]
	}

	vrfs, err := client.VRFs(t.Context())
	require.NoError(t, err)

	return vrfs
}

func TestBGPNeighbors(t *testing.T) {
	forEachModel(t, routingModels, func(t *testing.T, client *Client, model string) {
		all := map[string]map[string]BGPNeighbor{}
		for _, vrf := range routingVRFs(t, client, model) {
			neighbors, err := client.BGPNeighbors(t.Context(), vrf)
			if errors.Is(err, ErrNotFound) {
				// the management VRF has no BGP and the VPCs without host BGP no neighbors
				require.NotEqual(t, VRFDefault, vrf)

				continue
			}
			require.NoError(t, err, vrf)
			require.NotEmpty(t, neighbors, vrf)
			all[vrf] = neighbors

			for id, neighbor := range neighbors {
				require.Contains(t, []string{BGPNeighborNumbered, BGPNeighborUnnumbered}, neighbor.Type, "%s %s", vrf, id)
				require.True(t, neighbor.LocalAS.ASN.OK, "%s %s", vrf, id)
				if neighbor.SessionState != BGPSessionEstablished {
					continue
				}

				require.True(t, neighbor.RemoteAS.OK, "%s %s", vrf, id)
				require.True(t, neighbor.Uptime.OK, "%s %s", vrf, id)
				require.Positive(t, neighbor.ConnectionsEstablished.Value, "%s %s", vrf, id)
				require.Positive(t, neighbor.MessageStats.RxKeepalives.Value, "%s %s", vrf, id)
				require.NotEmpty(t, neighbor.AddressFamily, "%s %s", vrf, id)
				for af, prefixes := range neighbor.AddressFamily {
					require.True(t, prefixes.RxPrefix.OK, "%s %s %s", vrf, id, af)
					require.True(t, prefixes.TxPrefix.OK, "%s %s %s", vrf, id, af)
				}
			}
		}
		requireGolden(t, model, "bgp-neighbors", all)

		// the neighbors the switches have for the fixtures, see testdata/README.md
		switch model {
		case modelSN5610:
			_, ok := all["vpc-vpc-01"]
			require.False(t, ok, "a VPC without host BGP has no neighbors")

			fabric := all[VRFDefault]["swp63s0"]
			require.Equal(t, BGPNeighborUnnumbered, fabric.Type)
			require.Equal(t, "underlay_spine", fabric.PeerGroup)
			require.Equal(t, "sn3700c-01", fabric.RemoteHostname)
			require.Contains(t, fabric.AddressFamily, "ipv4-unicast")
			require.Equal(t, &BGPNeighborBFD{Profile: "underlay", Status: StatusUp}, fabric.BFD)

			overlay := all[VRFDefault]["172.30.8.0"]
			require.Equal(t, BGPNeighborNumbered, overlay.Type)
			require.Equal(t, "overlay", overlay.PeerGroup)
			require.Contains(t, overlay.AddressFamily, "l2vpn-evpn")

			// a server announcing its address over host BGP
			host := all["vpc-hbgp-nvue"]["swp66"]
			require.Equal(t, BGPNeighborUnnumbered, host.Type)
			require.Equal(t, BGPPeerGroupNone, host.PeerGroup)
			require.Equal(t, BGPSessionEstablished, host.SessionState)
			require.Equal(t, Number{Value: 64999, OK: true, Raw: "64999"}, host.RemoteAS)
			require.Equal(t, "server-03", host.RemoteHostname)
			require.Equal(t, Counter{Value: 1, OK: true}, host.AddressFamily["ipv4-unicast"].RxPrefix)
			require.Nil(t, host.BFD)
		case modelSN4600:
			external := all["ext-ext-bgp"]["100.1.10.6"]
			require.Equal(t, BGPNeighborNumbered, external.Type)
			require.Equal(t, BGPSessionEstablished, external.SessionState)
			require.Contains(t, external.AddressFamily, "ipv4-unicast")
		case modelSN3700C:
			// the leaves over unnumbered links and the gateways over numbered ones
			require.Equal(t, BGPNeighborUnnumbered, all[VRFDefault]["swp31"].Type)
			require.Equal(t, "sn5610-01", all[VRFDefault]["swp31"].RemoteHostname)
			require.Equal(t, BGPNeighborNumbered, all[VRFDefault]["172.30.128.5"].Type)
		}
	})
}
