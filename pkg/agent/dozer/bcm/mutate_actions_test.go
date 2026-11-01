// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestMutateRulesActions(t *testing.T) {
	acceptEntry := func(src string) *dozer.SpecACLEntry {
		return &dozer.SpecACLEntry{Action: dozer.SpecACLEntryActionAccept, SourceAddress: new(src)}
	}
	dropEntry := &dozer.SpecACLEntry{Action: dozer.SpecACLEntryActionDrop, SourceAddress: new("10.99.0.0/16")}

	fabric := func() *dozer.Spec {
		return &dozer.Spec{
			Interfaces: map[string]*dozer.SpecInterface{
				"Ethernet0": {Subinterfaces: map[uint32]*dozer.SpecSubinterface{10: {VLAN: new(uint16(10))}}},
			},
			ACLs: map[string]*dozer.SpecACL{
				"fabric": {Description: new("fabric"), Entries: map[uint32]*dozer.SpecACLEntry{10: acceptEntry("10.0.0.0/24")}},
			},
			ACLInterfaces: map[string]*dozer.SpecACLInterface{"Ethernet0.10": {Egress: new("fabric")}},
			RouteMaps: map[string]*dozer.SpecRouteMap{
				"ext-in": {Statements: map[string]*dozer.SpecRouteMapStatement{"10": {Result: dozer.SpecRouteMapResultAccept}}},
			},
			PrefixLists:    map[string]*dozer.SpecPrefixList{},
			VRFs:           map[string]*dozer.SpecVRF{"default": {}},
			BFDProfiles:    map[string]*dozer.SpecBFDProfile{"fabric": {DetectionMultiplier: new(uint8(3))}},
			CommunityLists: map[string]*dozer.SpecCommunityList{"all-externals": {Members: []string{"65000:1"}}},
		}
	}

	// the fabric configuration with the manual changes on top of it, as it's on the switch after the rules are applied
	onSwitch := func() *dozer.Spec {
		spec := fabric()
		spec.ACLs["fabric"].Entries[65000] = dropEntry
		spec.ACLs["manual"] = &dozer.SpecACL{Description: new("manual"), Entries: map[uint32]*dozer.SpecACLEntry{10: dropEntry}}
		spec.ACLInterfaces["Ethernet0.10"].Ingress = new("manual")
		spec.RouteMaps["ext-in"].Statements["1"] = &dozer.SpecRouteMapStatement{Conditions: dozer.SpecRouteMapConditions{Call: new("manual-in")}}
		spec.RouteMaps["manual-in"] = &dozer.SpecRouteMap{Statements: map[string]*dozer.SpecRouteMapStatement{
			"10": {Conditions: dozer.SpecRouteMapConditions{MatchPrefixList: new("manual-pl")}, Result: dozer.SpecRouteMapResultAccept},
		}}
		spec.PrefixLists["manual-pl"] = &dozer.SpecPrefixList{Prefixes: map[uint32]*dozer.SpecPrefixListEntry{
			10: {Prefix: dozer.SpecPrefixListPrefix{Prefix: "10.99.0.0/16"}, Action: dozer.SpecPrefixListActionPermit},
		}}
		spec.VRFs["default"].StaticRoutes = map[string]*dozer.SpecVRFStaticRoute{
			"10.99.0.0/16": {NextHops: []dozer.SpecVRFStaticRouteNextHop{{IP: "172.30.0.1"}}},
		}
		spec.BFDProfiles["fabric"].DetectionMultiplier = new(uint8(5))
		spec.CommunityLists["all-externals"].Members = []string{"65000:1", "65000:100"}

		return spec
	}

	rules, err := wiringapi.ParseBCMMutateRules([]string{
		"keep@acl/fabric/65000", "keep@acl/manual", "keep@aclInterface/Ethernet0.10/ingress",
		"keep@routeMap/ext-in/1", "keep@routeMap/manual-in", "keep@prefixList/manual-pl",
		"keep@staticRoute/default/10.99.0.0/16", "override@bfdProfile/fabric", "add@communityList/all-externals/65000:100",
	})
	require.NoError(t, err)

	summaries := func(actions []dozer.Action) []string {
		res := make([]string, 0, len(actions))
		for _, action := range actions {
			res = append(res, action.Summary())
		}

		return res
	}

	t.Run("without rules", func(t *testing.T) {
		actions, err := Processor().CalculateActions(t.Context(), onSwitch(), fabric())
		require.NoError(t, err)
		require.Subset(t, summaries(actions), []string{
			"Delete ACL entry 65000", "Delete ACL manual base", "Update ACL interface Ethernet0.10",
			"Delete Route Map Statement 1", "Delete Route Maps Base manual-in", "Delete Prefix List Base manual-pl",
			"Delete VRF static route 10.99.0.0/16", "Update BFD Profile fabric", "Update Community Lists all-externals",
		})
	})

	t.Run("with rules", func(t *testing.T) {
		actual, desired := onSwitch(), fabric()
		applied, warnings := dozer.ApplyMutateRules(actual, desired, rules)
		require.Len(t, applied, len(rules))
		require.Empty(t, warnings)

		actions, err := Processor().CalculateActions(t.Context(), actual, desired)
		require.NoError(t, err)
		require.Empty(t, summaries(actions))
	})

	t.Run("fabric changes next to the kept configuration", func(t *testing.T) {
		actual, desired := onSwitch(), fabric()
		desired.ACLs["fabric"].Entries[20] = acceptEntry("10.0.1.0/24")
		desired.CommunityLists["all-externals"].Members = []string{"65000:1", "65000:2"}
		_, warnings := dozer.ApplyMutateRules(actual, desired, rules)
		require.Empty(t, warnings)

		actions, err := Processor().CalculateActions(t.Context(), actual, desired)
		require.NoError(t, err)
		require.Equal(t, []string{"Update Community Lists all-externals", "Create ACL entry 20"}, summaries(actions))
		require.Equal(t, []string{"65000:1", "65000:100", "65000:2"}, desired.CommunityLists["all-externals"].Members)
	})

	t.Run("fabric plans the kept element", func(t *testing.T) {
		actual, desired := onSwitch(), fabric()
		desired.ACLs["fabric"].Entries[65000] = acceptEntry("10.0.2.0/24")
		_, warnings := dozer.ApplyMutateRules(actual, desired, rules)
		require.Equal(t, []string{"keep@acl/fabric/65000: planned by the fabric"}, warnings)

		actions, err := Processor().CalculateActions(t.Context(), actual, desired)
		require.NoError(t, err)
		require.Equal(t, []string{"Delete ACL entry 65000", "Create ACL entry 65000"}, summaries(actions))
	})
}
