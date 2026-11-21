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
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var testValidators = meta.ExtraValidators{
	Peering:    func(context.Context, kclient.Reader, kclient.Object) error { return nil },
	Connection: func(context.Context, kclient.Reader, kclient.Object) error { return nil },
}

func testFabricConfig(base string, gateway ...string) *meta.FabricConfig {
	cfg := &meta.FabricConfig{
		ControlVIP:             "172.30.0.1/32",
		APIServer:              "172.30.0.1:6443",
		AgentRepo:              "agent",
		VPCIRBVLANRanges:       []meta.VLANRange{{From: 1000, To: 1999}},
		TH5WorkaroundVLANRange: []meta.VLANRange{{From: 3000, To: 3999}},
		ReservedSubnets:        []string{"172.30.0.0/16"},
		FabricMode:             meta.FabricModeSpineLeaf,
		BaseVPCCommunity:       base,
		FabricMTU:              9100,
		ServerFacingMTUOffset:  64,
		ESLAGMACBase:           "f2:00:00:00:00:00",
		ESLAGESIPrefix:         "00:f2:00:00:",
		DefaultMaxPathsEBGP:    64,
		ManagementSubnet:       "172.30.0.0/21",
		ManagementDHCPStart:    "172.30.4.0",
		ManagementDHCPEnd:      "172.30.7.254",
		GatewayCommunities:     map[uint32]string{},
	}
	for idx, comm := range gateway {
		cfg.GatewayCommunities[uint32(idx)] = comm
	}

	return cfg
}

func TestFabricConfigExtraValidators(t *testing.T) {
	_, err := testFabricConfig("50000:50000").Init(meta.ExtraValidators{Connection: testValidators.Connection})
	require.ErrorContains(t, err, "ExtraValidators.Peering is required")

	_, err = testFabricConfig("50000:50000").Init(meta.ExtraValidators{Peering: testValidators.Peering})
	require.ErrorContains(t, err, "ExtraValidators.Connection is required")

	_, err = testFabricConfig("50000:50000").Init(testValidators)
	require.NoError(t, err)
}

func TestFabricConfigHugePages(t *testing.T) {
	for _, tt := range []struct {
		name   string
		pg1G   int64
		pg2M   int64
		errStr string
	}{
		{name: "none"},
		{name: "default", pg1G: 4},
		{name: "max", pg1G: 1 << 10, pg2M: 1 << 19},
		{name: "negative-1G", pg1G: -1, errStr: "gatewayHugePages1G"},
		{name: "negative-2M", pg2M: -1, errStr: "gatewayHugePages2M"},
		{name: "bytes-1G", pg1G: 4 << 30, errStr: "gatewayHugePages1G"},
		{name: "bytes-2M", pg2M: 4 << 30, errStr: "gatewayHugePages2M"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testFabricConfig("50000:50000")
			cfg.GatewayHugePages1G = tt.pg1G
			cfg.GatewayHugePages2M = tt.pg2M

			_, err := cfg.Init(testValidators)
			if tt.errStr != "" {
				require.ErrorContains(t, err, tt.errStr)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestFabricConfigCumulusSVIWorkaroundSubnet(t *testing.T) {
	for _, tt := range []struct {
		name      string
		subnet    string
		noCumulus bool
		reserved  []string // replaces the default 172.30.0.0/16
		mutate    func(cfg *meta.FabricConfig)
		expected  []string // reserved subnets after init
		err       string
	}{
		{name: "default-covered", expected: []string{"172.30.0.0/16"}},
		{name: "default", reserved: []string{"10.0.0.0/8"}, expected: []string{"10.0.0.0/8", meta.DefaultCumulusSVIWorkaroundSubnet}},
		{name: "no-cumulus", noCumulus: true, reserved: []string{"10.0.0.0/8"}, expected: []string{"10.0.0.0/8"}},
		{name: "no-cumulus-still-validated", noCumulus: true, subnet: "10.99.0.0/21", err: "cumulusSVIWorkaroundSubnet must be IPv4 /20 or bigger"},
		{name: "custom", subnet: "10.99.0.0/16", expected: []string{"172.30.0.0/16", "10.99.0.0/16"}},
		{name: "min", subnet: "10.99.0.0/20", expected: []string{"172.30.0.0/16", "10.99.0.0/20"}},
		{name: "unmasked", subnet: "10.99.1.2/20", expected: []string{"172.30.0.0/16", "10.99.0.0/20"}},
		{
			name: "partial-overlap", subnet: "10.99.0.0/16", reserved: []string{"10.99.1.0/24"},
			err: "cumulusSVIWorkaroundSubnet 10.99.0.0/16 partially overlaps reserved subnet 10.99.1.0/24",
		},
		{name: "management", subnet: "172.30.0.0/20", err: "cumulusSVIWorkaroundSubnet 172.30.0.0/20 overlaps managementSubnet 172.30.0.0/21"},
		{
			name: "fabric", subnet: "172.30.128.0/20",
			mutate: func(cfg *meta.FabricConfig) { cfg.FabricSubnet = "172.30.128.0/17" },
			err:    "cumulusSVIWorkaroundSubnet 172.30.128.0/20 overlaps fabricSubnet 172.30.128.0/17",
		},
		{
			name: "l2-proxy-external", subnet: "10.99.0.0/16",
			mutate: func(cfg *meta.FabricConfig) { cfg.L2ProxyExternalSubnet = "10.99.16.0/24" },
			err:    "cumulusSVIWorkaroundSubnet 10.99.0.0/16 overlaps l2ProxyExternalSubnet 10.99.16.0/24",
		},
		{name: "too-small", subnet: "10.99.0.0/21", err: "cumulusSVIWorkaroundSubnet must be IPv4 /20 or bigger"},
		{name: "ipv6", subnet: "fd00::/64", err: "cumulusSVIWorkaroundSubnet must be IPv4 /20 or bigger"},
		{name: "invalid", subnet: "10.99.0.0", err: "config: cumulusSVIWorkaroundSubnet"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testFabricConfig("50000:50000")
			cfg.IncludeCumulus = !tt.noCumulus
			cfg.CumulusSVIWorkaroundSubnet = tt.subnet
			if tt.reserved != nil {
				cfg.ReservedSubnets = tt.reserved
			}
			if tt.mutate != nil {
				tt.mutate(cfg)
			}

			cfg, err := cfg.Init(testValidators)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)

				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.subnet, cfg.CumulusSVIWorkaroundSubnet)
			expected := []netip.Prefix{}
			for _, subnet := range tt.expected {
				expected = append(expected, netip.MustParsePrefix(subnet))
			}
			require.Equal(t, expected, cfg.ParsedReservedSubnets())
		})
	}
}

func TestFabricConfigCommunities(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cfg     *meta.FabricConfig
		base    string
		gateway map[uint32]string
		err     string
	}{
		{
			name:    "canonical",
			cfg:     testFabricConfig("50000:50000", "50001:0", "50001:1"),
			base:    "50000:50000",
			gateway: map[uint32]string{0: "50001:0", 1: "50001:1"},
		},
		{
			name:    "leading-zeros",
			cfg:     testFabricConfig("050000:0050000", "50001:00", "050001:01"),
			base:    "50000:50000",
			gateway: map[uint32]string{0: "50001:0", 1: "50001:1"},
		},
		{
			name: "gateway-same-as-base",
			cfg:  testFabricConfig("50001:0", "50001:00"),
			err:  "same as baseVPCCommunity",
		},
		{
			name: "base-same-as-gateway",
			cfg:  testFabricConfig("50001:00", "50001:0"),
			err:  "same as baseVPCCommunity",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := tt.cfg.Init(testValidators)
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
