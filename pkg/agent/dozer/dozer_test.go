// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dozer_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestSpecPruneUnmanaged(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		spec := &dozer.Spec{}
		require.Empty(t, spec.PruneUnmanaged())
		require.Equal(t, &dozer.Spec{}, spec)
	})

	t.Run("mixed", func(t *testing.T) {
		spec := &dozer.Spec{
			ACLs:           map[string]*dozer.SpecACL{"vpc-filtering--a--b": {}, "nohh-b": {}, "nohh-a": {}},
			AsPathLists:    map[string]*dozer.SpecAsPathList{"nohh-x": {}},
			BFDProfiles:    map[string]*dozer.SpecBFDProfile{"fabric": {}, "nohh-x": {}},
			CommunityLists: map[string]*dozer.SpecCommunityList{"nohh-x": {}},
			LSTGroups:      map[string]*dozer.SpecLSTGroup{"spinelink": {}, "nohh-x": {}},
			PrefixLists:    map[string]*dozer.SpecPrefixList{"any-prefix": {}, "nohh-x": {}},
			RouteMaps: map[string]*dozer.SpecRouteMap{
				"loopback-vtep": {Statements: map[string]*dozer.SpecRouteMapStatement{"nohh-10": {}}},
				"nohh-x":        {},
				"xnohh-":        {},
			},
			// not covered by the unmanaged objects
			Users: map[string]*dozer.SpecUser{"nohh-x": {}},
			VRFs:  map[string]*dozer.SpecVRF{"nohh-x": {}},
		}

		require.Equal(t, []string{
			"acl/nohh-a",
			"acl/nohh-b",
			"asPathList/nohh-x",
			"bfdProfile/nohh-x",
			"communityList/nohh-x",
			"lstGroup/nohh-x",
			"prefixList/nohh-x",
			"routeMap/nohh-x",
		}, spec.PruneUnmanaged())

		require.Equal(t, &dozer.Spec{
			ACLs:           map[string]*dozer.SpecACL{"vpc-filtering--a--b": {}},
			AsPathLists:    map[string]*dozer.SpecAsPathList{},
			BFDProfiles:    map[string]*dozer.SpecBFDProfile{"fabric": {}},
			CommunityLists: map[string]*dozer.SpecCommunityList{},
			LSTGroups:      map[string]*dozer.SpecLSTGroup{"spinelink": {}},
			PrefixLists:    map[string]*dozer.SpecPrefixList{"any-prefix": {}},
			RouteMaps: map[string]*dozer.SpecRouteMap{
				"loopback-vtep": {Statements: map[string]*dozer.SpecRouteMapStatement{"nohh-10": {}}},
				"xnohh-":        {},
			},
			Users: map[string]*dozer.SpecUser{"nohh-x": {}},
			VRFs:  map[string]*dozer.SpecVRF{"nohh-x": {}},
		}, spec)
	})
}
