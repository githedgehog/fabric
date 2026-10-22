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

package meta_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestFabricConfigCommunities(t *testing.T) {
	cfgGen := func(base string, gateway ...string) *meta.FabricConfig {
		cfg := &meta.FabricConfig{
			ControlVIP:             "172.30.0.1/32",
			APIServer:              "172.30.0.1:6443",
			AgentRepo:              "agent",
			VPCIRBVLANRanges:       []meta.VLANRange{{From: 1000, To: 1999}},
			VPCPeeringVLANRanges:   []meta.VLANRange{{From: 100, To: 999}},
			TH5WorkaroundVLANRange: []meta.VLANRange{{From: 3000, To: 3999}},
			ReservedSubnets:        []string{"172.30.0.0/16"},
			FabricMode:             meta.FabricModeSpineLeaf,
			BaseVPCCommunity:       base,
			VPCLoopbackSubnet:      "100.64.0.0/16",
			FabricMTU:              9100,
			ServerFacingMTUOffset:  64,
			ESLAGMACBase:           "f2:00:00:00:00:00",
			ESLAGESIPrefix:         "00:f2:00:00:",
			DefaultMaxPathsEBGP:    64,
			GatewayASN:             65534,
			SpineASN:               65100,
			LeafASNStart:           65101,
			LeafASNEnd:             65199,
			ManagementSubnet:       "172.30.0.0/21",
			ManagementDHCPStart:    "172.30.4.0",
			ManagementDHCPEnd:      "172.30.7.254",
			GatewayCommunities:     map[uint32]string{},
		}
		for idx, comm := range gateway {
			cfg.GatewayCommunities[uint32(idx)] = comm //nolint:gosec
		}

		return cfg
	}
	validators := meta.ExtraValidators{
		Peering: func(context.Context, kclient.Reader, kclient.Object) error { return nil },
	}

	for _, tt := range []struct {
		name    string
		cfg     *meta.FabricConfig
		base    string
		gateway map[uint32]string
		err     string
	}{
		{
			name:    "canonical",
			cfg:     cfgGen("50000:50000", "50001:0", "50001:1"),
			base:    "50000:50000",
			gateway: map[uint32]string{0: "50001:0", 1: "50001:1"},
		},
		{
			name:    "leading-zeros",
			cfg:     cfgGen("050000:0050000", "50001:00", "050001:01"),
			base:    "50000:50000",
			gateway: map[uint32]string{0: "50001:0", 1: "50001:1"},
		},
		{
			name: "gateway-same-as-base",
			cfg:  cfgGen("50001:0", "50001:00"),
			err:  "same as baseVPCCommunity",
		},
		{
			name: "base-same-as-gateway",
			cfg:  cfgGen("50001:00", "50001:0"),
			err:  "same as baseVPCCommunity",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := tt.cfg.Init(validators)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.base, cfg.BaseVPCCommunity)
			require.Equal(t, tt.gateway, cfg.GatewayCommunities)
		})
	}
}
