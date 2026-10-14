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

package inspect

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
)

func TestBGPPrefixLimitColumn(t *testing.T) {
	neigh := func(state agentapi.BGPNeighborSessionState, prefixes agentapi.SwitchStateBGPNeighborPrefixes) apiutil.BGPNeighborStatus {
		return apiutil.BGPNeighborStatus{
			Expected: true,
			SwitchStateBGPNeighbor: agentapi.SwitchStateBGPNeighbor{
				SessionState: state,
				Prefixes:     map[string]agentapi.SwitchStateBGPNeighborPrefixes{ipv4UnicastAFISafi: prefixes},
			},
		}
	}
	out := &BGPOut{Neighbors: map[string]map[string]map[string]apiutil.BGPNeighborStatus{
		"leaf-01": {"VrfEext": {
			"10.0.0.1": neigh(agentapi.BGPNeighborSessionStateIdle, agentapi.SwitchStateBGPNeighborPrefixes{Limit: 3}),
			"10.0.0.2": neigh(agentapi.BGPNeighborSessionStateEstablished, agentapi.SwitchStateBGPNeighborPrefixes{Received: 5, Limit: 3, LimitWarnOnly: true}),
			"10.0.0.3": neigh(agentapi.BGPNeighborSessionStateEstablished, agentapi.SwitchStateBGPNeighborPrefixes{Received: 2, Limit: 1000}),
			"10.0.0.4": neigh(agentapi.BGPNeighborSessionStateEstablished, agentapi.SwitchStateBGPNeighborPrefixes{Received: 2}),
		}},
	}}

	text, err := out.MarshalText(BGPIn{}, time.Now())
	require.NoError(t, err)

	lines := strings.Split(text, "\n")
	col := strings.Index(lines[1], "PREFIX LIMIT")
	require.Positive(t, col, lines[1])

	limits := map[string]string{}
	for _, line := range lines[2:] {
		if fields := strings.Fields(line); len(fields) > 1 {
			limits[fields[1]] = strings.TrimSpace(line[col:])
		}
	}
	require.Equal(t, map[string]string{
		"10.0.0.1": "0/3",
		"10.0.0.2": "5/3 warn-only",
		"10.0.0.3": "2/1000",
		"10.0.0.4": "-",
	}, limits)
}
