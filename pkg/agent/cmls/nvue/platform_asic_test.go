// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var asicCaptures = map[string][]string{
	modelSN5610: {"/platform/asic", "/platform/asic/ASIC1/resource"},
	modelSN4700: {"/platform/asic", "/platform/asic/ASIC1/resource"},
	modelSN4600: {"/platform/asic", "/platform/asic/ASIC1/resource"},
	modelSN2700: {"/platform/asic", "/platform/asic/ASIC1/resource"},
	modelSN2201: {"/platform/asic", "/platform/asic/ASIC1/resource"},
}

func TestASICResources(t *testing.T) {
	forEachModel(t, allModels, func(t *testing.T, client *Client, model string) {
		asics, err := client.ASICs(t.Context())
		require.NoError(t, err)
		require.Equal(t, []string{"ASIC1"}, asics)

		resources, err := client.ASICResources(t.Context(), asics[0])
		require.NoError(t, err)

		for _, name := range []string{"IPv4-Routes", "IPv6-Routes", "ECMP-nexthops", "MAC-entries"} {
			resource, ok := resources.Global[name]
			require.True(t, ok, name)
			require.True(t, resource.Used.OK, name)
			require.True(t, resource.Max.OK, name)
			require.InDelta(t, resource.Max.Value, resource.Used.Value+resource.Free.Value, 0, name)
		}
		for _, name := range []string{"IPv4-neighbors", "IPv6-neighbors", "ECMP-entries"} {
			resource, ok := resources.Global[name]
			require.True(t, ok, name)
			require.True(t, resource.Used.OK, name)
			require.False(t, resource.Max.OK, name)
		}

		require.Contains(t, resources.ACL, "Ingress-ACL-ipv4-filter-table")
		for name, table := range resources.ACL {
			require.True(t, table.RuleCount.OK, name)
		}

		requireGolden(t, model, "asic-resources", resources)
	})
}
