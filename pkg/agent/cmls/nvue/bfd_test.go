// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// the BFD peers are checked against the BGP neighbors, their fixtures are captured for the BGP tests
var bfdCaptures = map[string][]string{
	modelSN5610: {
		"/vrf/default/router/bfd/peers",
		"/vrf/vpc-hbgp-nvue/router/bfd/peers",
		"/vrf/vpc-vpc-01/router/bfd/peers",
		"/vrf/mgmt/router/bfd/peers",
	},
	modelSN4600: {
		"/vrf/default/router/bfd/peers",
		"/vrf/ext-ext-bgp/router/bfd/peers",
	},
	modelSN3700C: {"/vrf/default/router/bfd/peers"},
}

func TestBFDPeers(t *testing.T) {
	forEachModel(t, routingModels, func(t *testing.T, client *Client, model string) {
		all := map[string]map[string]BFDPeer{}
		for _, vrf := range routingVRFs(t, client, model) {
			peers, err := client.BFDPeers(t.Context(), vrf)
			require.NoError(t, err, vrf)
			all[vrf] = peers

			for id, peer := range peers {
				require.NotEmpty(t, peer.Peer, "%s %s", vrf, id)
				require.NotEmpty(t, peer.State, "%s %s", vrf, id)
				require.True(t, peer.SessionUp.OK, "%s %s", vrf, id)
				require.True(t, peer.DetectMultiplier.OK, "%s %s", vrf, id)
			}

			// every BGP neighbor with BFD up has its session, unnumbered ones by interface
			neighbors, err := client.BGPNeighbors(t.Context(), vrf)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			require.NoError(t, err, vrf)
			for id, neighbor := range neighbors {
				if neighbor.BFD == nil || neighbor.BFD.Status != StatusUp {
					continue
				}

				found := false
				for _, peer := range peers {
					if (neighbor.Type == BGPNeighborUnnumbered && peer.Interface == id) || peer.Peer == id {
						require.Equal(t, StatusUp, peer.State, "%s %s", vrf, id)
						require.Equal(t, neighbor.BFD.Profile, peer.Profile, "%s %s", vrf, id)
						found = true
					}
				}
				require.True(t, found, "no BFD session of %s neighbor %s", vrf, id)
			}
		}
		requireGolden(t, model, "bfd-peers", all)

		switch model {
		case modelSN5610:
			// the host BGP sessions have no BFD
			require.Empty(t, all["vpc-hbgp-nvue"])
			require.NotEmpty(t, all[VRFDefault])
		case modelSN3700C:
			require.NotEmpty(t, all[VRFDefault])
		}
	})
}
