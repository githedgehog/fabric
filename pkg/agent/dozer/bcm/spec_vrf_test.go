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
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

// The switch refuses a neighbor replace that swaps the restart timer for warning-only or back, so the prefix limit
// has to be deleted first. Both directions crash-looped the agent on a VLAB.
func TestSpecVRFBGPPrefixLimitModeSwitch(t *testing.T) {
	restart := &dozer.SpecVRFBGPNeighbor{IPv4MaxPrefixes: pointer.To[uint32](3), IPv4MaxPrefixesRestart: pointer.To[uint16](5)}
	restartHigher := &dozer.SpecVRFBGPNeighbor{IPv4MaxPrefixes: pointer.To[uint32](10), IPv4MaxPrefixesRestart: pointer.To[uint16](5)}
	warnOnly := &dozer.SpecVRFBGPNeighbor{IPv4MaxPrefixes: pointer.To[uint32](3), IPv4MaxPrefixesWarnOnly: pointer.To(true)}
	noLimit := &dozer.SpecVRFBGPNeighbor{}

	deletePath := "/network-instances/network-instance[name=VrfEext]/protocols/protocol[identifier=BGP][name=bgp]/bgp" +
		"/neighbors/neighbor[neighbor-address=1.2.3.4]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/ipv4-unicast/prefix-limit"

	for _, tt := range []struct {
		name            string
		actual, desired *dozer.SpecVRFBGPNeighbor
		wantDelete      bool
	}{
		{name: "restart to warning-only", actual: restart, desired: warnOnly, wantDelete: true},
		{name: "warning-only to restart", actual: warnOnly, desired: restart, wantDelete: true},
		{name: "restart with another max", actual: restart, desired: restartHigher},
		{name: "added", actual: noLimit, desired: warnOnly},
		{name: "removed", actual: warnOnly, desired: noLimit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{"1.2.3.4": tt.actual}}
			desired := &dozer.SpecVRFBGP{Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{"1.2.3.4": tt.desired}}

			actions := &ActionQueue{}
			require.NoError(t, specVRFBGPEnforcer.Handle("/network-instances/network-instance[name=VrfEext]", "VrfEext", actual, desired, actions))

			deletes := []string{}
			for _, a := range actions.actions {
				if act := a.(*Action); act.Type == ActionTypeDelete {
					deletes = append(deletes, act.Path)
				}
			}
			if tt.wantDelete {
				require.Equal(t, []string{deletePath}, deletes)
			} else {
				require.Empty(t, deletes)
			}
		})
	}

	require.Less(t, ActionWeightVRFBGPNeighborDelete, ActionWeightVRFBGPNeighborUpdate)
}
