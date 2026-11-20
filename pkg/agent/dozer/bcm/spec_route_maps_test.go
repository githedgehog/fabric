// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestSpecRouteMapStatementRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   *dozer.SpecRouteMapStatement
	}{
		{
			name: "delete communities",
			in: &dozer.SpecRouteMapStatement{
				Conditions:        dozer.SpecRouteMapConditions{MatchPrefixList: new("pl")},
				DeleteCommunities: new("any-community"),
				Result:            dozer.SpecRouteMapResultAccept,
			},
		},
		{
			name: "delete and set communities",
			in: &dozer.SpecRouteMapStatement{
				SetCommunities:     []string{"50000:0"},
				DeleteCommunities:  new("any-community"),
				SetLocalPreference: new(uint32(150)),
				Result:             dozer.SpecRouteMapResultAccept,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out, err := specRouteMapStatementEnforcer.Marshal("10", tt.in)
			require.NoError(t, err)

			routeMaps, err := unmarshalOCRouteMaps(&oc.OpenconfigRoutingPolicy_RoutingPolicy{
				PolicyDefinitions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions{
					PolicyDefinition: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition{
						"rm": {
							Name:       new("rm"),
							Statements: out.(*oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements),
						},
					},
				},
			})
			require.NoError(t, err)
			require.Equal(t, tt.in, routeMaps["rm"].Statements["10"])
		})
	}
}
