// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var vrfCaptures = map[string][]string{
	modelSN5610:  {"/vrf"},
	modelSN4600:  {"/vrf"},
	modelSN3700C: {"/vrf"},
}

var routingModels = []string{modelSN5610, modelSN4600, modelSN3700C}

func TestVRFs(t *testing.T) {
	forEachModel(t, routingModels, func(t *testing.T, client *Client, model string) {
		vrfs, err := client.VRFs(t.Context())
		require.NoError(t, err)
		require.Contains(t, vrfs, VRFDefault)
		require.Contains(t, vrfs, VRFMgmt)

		// the VPC and external VRFs the switches have for the fixtures, see testdata/README.md
		switch model {
		case modelSN5610:
			require.Contains(t, vrfs, "vpc-hbgp-nvue")
		case modelSN4600:
			require.Contains(t, vrfs, "ext-ext-bgp")
		case modelSN3700C:
			require.Equal(t, []string{VRFDefault, VRFMgmt}, vrfs)
		}
	})
}
