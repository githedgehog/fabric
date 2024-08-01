// Copyright 2023 Hedgehog
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
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestSpecCommunityListEnforcerMarshal(t *testing.T) {
	commListName := "name"

	for _, tc := range []struct {
		name string
		in   *dozer.SpecCommunityList
		out  []string
	}{
		{
			name: "empty",
			in:   &dozer.SpecCommunityList{},
			out:  []string{},
		},
		{
			name: "single",
			in:   &dozer.SpecCommunityList{Members: []string{"1"}},
			out:  []string{"1"},
		},
		{
			name: "dup",
			in:   &dozer.SpecCommunityList{Members: []string{"1", "1"}},
			out:  []string{"1"},
		},
		{
			name: "unsorted-dup",
			in:   &dozer.SpecCommunityList{Members: []string{"2", "1", "3", "1"}},
			out:  []string{"1", "2", "3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := specCommunityListEnforcer.Marshal(commListName, tc.in)

			require.NoError(t, err)

			commLists := out.(*oc.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets)
			require.NotNil(t, commLists)

			commList, ok := commLists.CommunitySet[commListName]
			require.True(t, ok)
			require.NotNil(t, commList)
			require.NotNil(t, commList, commList.Config)

			members := []string{}

			for _, member := range commList.Config.CommunityMember {
				str, ok := member.(oc.UnionString)
				require.True(t, ok)

				members = append(members, string(str))
			}

			require.Equal(t, tc.out, members)
		})
	}
}
