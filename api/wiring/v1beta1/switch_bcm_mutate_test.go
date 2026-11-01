// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

func TestParseBCMMutateRule(t *testing.T) {
	for _, tt := range []struct {
		rule     string
		expected wiringapi.BCMMutateRule
		err      string
	}{
		{rule: "keep@acl/x", expected: wiringapi.BCMMutateRule{Mode: "keep", Kind: "acl", Name: "x"}},
		{rule: "keep@acl/x/65000", expected: wiringapi.BCMMutateRule{Mode: "keep", Kind: "acl", Name: "x", Element: "65000"}},
		{rule: "override@routeMap/ext-inbound--ext-01/1", expected: wiringapi.BCMMutateRule{Mode: "override", Kind: "routeMap", Name: "ext-inbound--ext-01", Element: "1"}},
		{rule: "keep@prefixList/x/16777216", expected: wiringapi.BCMMutateRule{Mode: "keep", Kind: "prefixList", Name: "x", Element: "16777216"}},
		{rule: "keep@aclInterface/Ethernet0.10/ingress", expected: wiringapi.BCMMutateRule{Mode: "keep", Kind: "aclInterface", Name: "Ethernet0.10", Element: "ingress"}},
		{rule: "override@staticRoute/default/10.0.0.0/8", expected: wiringapi.BCMMutateRule{Mode: "override", Kind: "staticRoute", Name: "default", Element: "10.0.0.0/8"}},
		{rule: "keep@bfdProfile/x", expected: wiringapi.BCMMutateRule{Mode: "keep", Kind: "bfdProfile", Name: "x"}},
		{rule: "override@communityList/x", expected: wiringapi.BCMMutateRule{Mode: "override", Kind: "communityList", Name: "x"}},
		{rule: "add@communityList/all-externals/65000:100", expected: wiringapi.BCMMutateRule{Mode: "add", Kind: "communityList", Name: "all-externals", Element: "65000:100"}},
		{rule: "remove@asPathList/x/^65100_.*", expected: wiringapi.BCMMutateRule{Mode: "remove", Kind: "asPathList", Name: "x", Element: "^65100_.*"}},
		{rule: "acl/x", err: "has no mode"},
		{rule: "keep:acl/x", err: "has no mode"},
		{rule: "drop@acl/x", err: `unknown mode "drop"`},
		{rule: "keep@vrf/x", err: `unknown kind "vrf"`},
		{rule: "keep@acl", err: "has no name"},
		{rule: "keep@acl/", err: "has no name"},
		{rule: "keep@acl/x/", err: "has an empty element"},
		{rule: "keep@acl/x/0", err: "seq must be a number from 1 to 65535"},
		{rule: "keep@acl/x/65536", err: "seq must be a number from 1 to 65535"},
		{rule: "keep@routeMap/x/010", err: "seq must be a number from 1 to 65535"},
		{rule: "keep@acl/x/ten", err: "parsing seq"},
		{rule: "keep@prefixList/x/4294967296", err: "seq must be a number from 1 to 4294967295"},
		{rule: "keep@aclInterface/Ethernet0", err: "direction must be ingress or egress"},
		{rule: "keep@aclInterface/Ethernet0/both", err: "direction must be ingress or egress"},
		{rule: "keep@staticRoute/default", err: "parsing prefix"},
		{rule: "keep@staticRoute/default/10.0.0.1/8", err: "prefix must be 10.0.0.0/8"},
		{rule: "keep@bfdProfile/x/1", err: "bfd profile has no elements"},
		{rule: "add@acl/x/10", err: "add and remove are only for community and as-path list members"},
		{rule: "add@communityList/x", err: "add and remove are only for community and as-path list members"},
		{rule: "keep@communityList/x/65000:100", err: "add and remove are only for community and as-path list members"},
		{rule: "add@communityList/x/65000", err: "community must be ASN:value"},
		{rule: "add@communityList/x/70000:1", err: "parsing community ASN"},
		{rule: "add@communityList/x/1:70000", err: "parsing community value"},
		{rule: "add@asPathList/x/_1 2_", err: "as-path member can't have spaces"},
	} {
		t.Run(tt.rule, func(t *testing.T) {
			rule, err := wiringapi.ParseBCMMutateRule(tt.rule)
			if tt.err != "" {
				require.ErrorIs(t, err, wiringapi.ErrInvalidBCMMutateRule)
				require.ErrorContains(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, rule)
			require.Equal(t, tt.rule, rule.String())
		})
	}
}

func TestParseBCMMutateRules(t *testing.T) {
	for _, tt := range []struct {
		name  string
		rules []string
		err   string
	}{
		{name: "empty"},
		{name: "elements of the same object", rules: []string{"keep@acl/x/10", "override@acl/x/20"}},
		{name: "same name of different kinds", rules: []string{"keep@acl/x", "keep@routeMap/x", "keep@prefixList/x"}},
		{name: "list and its members", rules: []string{"override@communityList/x", "add@communityList/x/1:1", "remove@communityList/x/1:2"}},
		{name: "duplicate", rules: []string{"keep@acl/x", "keep@acl/x"}, err: `"keep@acl/x" conflicts with "keep@acl/x"`},
		{name: "keep and override", rules: []string{"keep@acl/x/10", "override@acl/x/10"}, err: `"override@acl/x/10" conflicts with "keep@acl/x/10"`},
		{name: "add and remove", rules: []string{"add@asPathList/x/_1_", "remove@asPathList/x/_1_"}, err: `"remove@asPathList/x/_1_" conflicts with "add@asPathList/x/_1_"`},
		{name: "object then element", rules: []string{"keep@routeMap/x", "keep@routeMap/x/1"}, err: `"keep@routeMap/x/1" conflicts with "keep@routeMap/x"`},
		{name: "element then object", rules: []string{"keep@prefixList/x/1", "keep@prefixList/x/2", "override@prefixList/x"}, err: `"override@prefixList/x" conflicts with "keep@prefixList/x/1"`},
		{name: "invalid rule", rules: []string{"keep@acl/x", "keep@acl/y/0"}, err: "seq must be a number from 1 to 65535"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := wiringapi.ParseBCMMutateRules(tt.rules)
			if tt.err != "" {
				require.ErrorIs(t, err, wiringapi.ErrInvalidBCMMutateRule)
				require.ErrorContains(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			require.Len(t, rules, len(tt.rules))
		})
	}
}
