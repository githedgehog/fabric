// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dozer_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func TestApplyMutateRules(t *testing.T) {
	drop := &dozer.SpecACLEntry{Action: dozer.SpecACLEntryActionDrop}
	accept := &dozer.SpecACLEntry{Action: dozer.SpecACLEntryActionAccept}
	acceptStmt := &dozer.SpecRouteMapStatement{Result: dozer.SpecRouteMapResultAccept}
	rejectStmt := &dozer.SpecRouteMapStatement{Result: dozer.SpecRouteMapResultReject}
	permit := &dozer.SpecPrefixListEntry{Prefix: dozer.SpecPrefixListPrefix{Prefix: "10.0.0.0/8"}, Action: dozer.SpecPrefixListActionPermit}
	viaA := &dozer.SpecVRFStaticRoute{NextHops: []dozer.SpecVRFStaticRouteNextHop{{IP: "172.30.0.1"}}}
	viaB := &dozer.SpecVRFStaticRoute{NextHops: []dozer.SpecVRFStaticRouteNextHop{{IP: "172.30.0.2"}}}
	interfaces := map[string]*dozer.SpecInterface{
		"Ethernet0": {Subinterfaces: map[uint32]*dozer.SpecSubinterface{0: {}, 10: {}}},
		"Vlan1000":  {},
		"Vlan1001":  {},
		"Vlan1003":  {},
	}

	for _, tt := range []struct {
		name     string
		rules    []string
		actual   *dozer.Spec
		desired  *dozer.Spec
		expected *dozer.Spec
		applied  []string
		warnings []string
	}{
		{
			name:     "no rules",
			actual:   &dozer.Spec{ACLs: map[string]*dozer.SpecACL{"x": {}}},
			desired:  &dozer.Spec{},
			expected: &dozer.Spec{},
			applied:  []string{},
			warnings: []string{},
		},
		{
			name:     "acl objects",
			rules:    []string{"keep@acl/manual", "keep@acl/fabric", "override@acl/tuned", "keep@acl/missing"},
			actual:   &dozer.Spec{ACLs: map[string]*dozer.SpecACL{"manual": {}, "fabric": {}, "tuned": {Entries: map[uint32]*dozer.SpecACLEntry{1: drop}}}},
			desired:  &dozer.Spec{ACLs: map[string]*dozer.SpecACL{"fabric": {Entries: map[uint32]*dozer.SpecACLEntry{1: accept}}, "tuned": {}}},
			expected: &dozer.Spec{ACLs: map[string]*dozer.SpecACL{"manual": {}, "fabric": {Entries: map[uint32]*dozer.SpecACLEntry{1: accept}}, "tuned": {Entries: map[uint32]*dozer.SpecACLEntry{1: drop}}}},
			applied:  []string{"keep@acl/manual", "override@acl/tuned"},
			warnings: []string{"keep@acl/fabric: planned by the fabric", "keep@acl/missing: not on the switch"},
		},
		{
			name:  "acl rules",
			rules: []string{"keep@acl/x/65000", "keep@acl/x/1", "override@acl/x/2", "keep@acl/x/3", "keep@acl/y/1", "keep@acl/z/1"},
			actual: &dozer.Spec{ACLs: map[string]*dozer.SpecACL{
				"x": {Entries: map[uint32]*dozer.SpecACLEntry{1: drop, 2: drop, 65000: drop}},
				"y": {Entries: map[uint32]*dozer.SpecACLEntry{1: drop}},
			}},
			desired: &dozer.Spec{ACLs: map[string]*dozer.SpecACL{
				"x": {Entries: map[uint32]*dozer.SpecACLEntry{1: accept, 2: accept}},
				"z": {},
			}},
			expected: &dozer.Spec{ACLs: map[string]*dozer.SpecACL{
				"x": {Entries: map[uint32]*dozer.SpecACLEntry{1: accept, 2: drop, 65000: drop}},
				"z": {},
			}},
			applied: []string{"keep@acl/x/65000", "override@acl/x/2"},
			warnings: []string{
				"keep@acl/x/1: planned by the fabric", "keep@acl/x/3: not on the switch",
				"keep@acl/y/1: not planned by the fabric", "keep@acl/z/1: not on the switch",
			},
		},
		{
			name:     "route map statements",
			rules:    []string{"keep@routeMap/x/1", "override@routeMap/x/10"},
			actual:   &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{"1": rejectStmt, "10": rejectStmt}}}},
			desired:  &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{"10": acceptStmt}}}},
			expected: &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{"1": rejectStmt, "10": rejectStmt}}}},
			applied:  []string{"keep@routeMap/x/1", "override@routeMap/x/10"},
			warnings: []string{},
		},
		{
			name:  "route map statement references",
			rules: []string{"keep@routeMap/ext-in/1", "keep@routeMap/ext-in/2", "keep@routeMap/ext-in/3", "keep@routeMap/manual", "keep@prefixList/fabric-pl/1"},
			actual: &dozer.Spec{
				RouteMaps: map[string]*dozer.SpecRouteMap{
					"ext-in": {Statements: map[string]*dozer.SpecRouteMapStatement{
						"1": {Conditions: dozer.SpecRouteMapConditions{Call: new("manual"), MatchPrefixList: new("fabric-pl"), MatchSourceVRF: new("default")}},
						"2": {Conditions: dozer.SpecRouteMapConditions{Call: new("other"), MatchCommunityList: new("missing")}},
						"3": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("VrfVgone")}},
					}},
					"manual": {},
					"other":  {},
				},
				PrefixLists: map[string]*dozer.SpecPrefixList{"fabric-pl": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{1: permit}}},
			},
			desired: &dozer.Spec{
				RouteMaps:   map[string]*dozer.SpecRouteMap{"ext-in": {Statements: map[string]*dozer.SpecRouteMapStatement{"10": acceptStmt}}},
				PrefixLists: map[string]*dozer.SpecPrefixList{"fabric-pl": {}},
				VRFs:        map[string]*dozer.SpecVRF{"default": {}},
			},
			expected: &dozer.Spec{
				RouteMaps: map[string]*dozer.SpecRouteMap{
					"ext-in": {Statements: map[string]*dozer.SpecRouteMapStatement{
						"1":  {Conditions: dozer.SpecRouteMapConditions{Call: new("manual"), MatchPrefixList: new("fabric-pl"), MatchSourceVRF: new("default")}},
						"10": acceptStmt,
					}},
					"manual": {},
				},
				PrefixLists: map[string]*dozer.SpecPrefixList{"fabric-pl": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{1: permit}}},
				VRFs:        map[string]*dozer.SpecVRF{"default": {}},
			},
			applied: []string{"keep@prefixList/fabric-pl/1", "keep@routeMap/ext-in/1", "keep@routeMap/manual"},
			warnings: []string{
				"keep@routeMap/ext-in/2: references routeMap/other, communityList/missing, not planned or kept",
				"keep@routeMap/ext-in/3: references vrf/VrfVgone, not planned or kept",
			},
		},
		{
			name:  "route map statement source vrfs",
			rules: []string{"override@routeMap/peering/10", "override@routeMap/peering/20", "keep@routeMap/peering/30", "keep@routeMap/manual"},
			actual: &dozer.Spec{
				RouteMaps: map[string]*dozer.SpecRouteMap{
					"peering": {Statements: map[string]*dozer.SpecRouteMapStatement{
						"10": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("VrfVvpc-01")}, Result: dozer.SpecRouteMapResultAccept},
						"20": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("VrfVvpc-02")}, Result: dozer.SpecRouteMapResultAccept},
						"30": {Conditions: dozer.SpecRouteMapConditions{MatchPrefixList: new("gone"), MatchSourceVRF: new("VrfVvpc-02")}},
					}},
					"manual": {Statements: map[string]*dozer.SpecRouteMapStatement{
						"1": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("VrfVvpc-02")}},
					}},
				},
			},
			desired: &dozer.Spec{
				RouteMaps: map[string]*dozer.SpecRouteMap{"peering": {Statements: map[string]*dozer.SpecRouteMapStatement{"10": rejectStmt, "20": rejectStmt}}},
				VRFs:      map[string]*dozer.SpecVRF{"VrfVvpc-01": {}},
			},
			expected: &dozer.Spec{
				RouteMaps: map[string]*dozer.SpecRouteMap{
					"peering": {Statements: map[string]*dozer.SpecRouteMapStatement{
						"10": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("VrfVvpc-01")}, Result: dozer.SpecRouteMapResultAccept},
						"20": rejectStmt,
					}},
					// statements of whole route maps aren't checked
					"manual": {Statements: map[string]*dozer.SpecRouteMapStatement{
						"1": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("VrfVvpc-02")}},
					}},
				},
				VRFs: map[string]*dozer.SpecVRF{"VrfVvpc-01": {}},
			},
			applied: []string{"keep@routeMap/manual", "override@routeMap/peering/10"},
			warnings: []string{
				"override@routeMap/peering/20: references vrf/VrfVvpc-02, not planned or kept",
				"keep@routeMap/peering/30: references prefixList/gone, vrf/VrfVvpc-02, not planned or kept",
			},
		},
		{
			name:  "route map statement source vrf without planned vrfs",
			rules: []string{"keep@routeMap/x/1"},
			actual: &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{
				"1": {Conditions: dozer.SpecRouteMapConditions{MatchSourceVRF: new("default")}},
			}}}},
			desired:  &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {}}},
			expected: &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {}}},
			applied:  []string{},
			warnings: []string{"keep@routeMap/x/1: references vrf/default, not planned or kept"},
		},
		{
			name:     "prefix list entry into an empty list",
			rules:    []string{"keep@prefixList/x/16777216"},
			actual:   &dozer.Spec{PrefixLists: map[string]*dozer.SpecPrefixList{"x": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{16777216: permit}}}},
			desired:  &dozer.Spec{PrefixLists: map[string]*dozer.SpecPrefixList{"x": {}}},
			expected: &dozer.Spec{PrefixLists: map[string]*dozer.SpecPrefixList{"x": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{16777216: permit}}}},
			applied:  []string{"keep@prefixList/x/16777216"},
			warnings: []string{},
		},
		{
			name:  "prefix list objects",
			rules: []string{"keep@prefixList/manual", "keep@prefixList/fabric", "override@prefixList/tuned"},
			actual: &dozer.Spec{PrefixLists: map[string]*dozer.SpecPrefixList{
				"manual": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{10: permit}},
				"fabric": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{10: permit}},
				"tuned":  {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{10: permit}},
			}},
			desired: &dozer.Spec{PrefixLists: map[string]*dozer.SpecPrefixList{"fabric": {}, "tuned": {}}},
			expected: &dozer.Spec{PrefixLists: map[string]*dozer.SpecPrefixList{
				"manual": {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{10: permit}},
				"fabric": {},
				"tuned":  {Prefixes: map[uint32]*dozer.SpecPrefixListEntry{10: permit}},
			}},
			applied:  []string{"keep@prefixList/manual", "override@prefixList/tuned"},
			warnings: []string{"keep@prefixList/fabric: planned by the fabric"},
		},
		{
			name:  "route map statement other references, missing and planned statements",
			rules: []string{"keep@routeMap/x/1", "keep@routeMap/x/2", "keep@routeMap/x/3", "keep@routeMap/x/10"},
			actual: &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{
				"1":  {Conditions: dozer.SpecRouteMapConditions{MatchNextHopPrefixList: new("gone")}},
				"2":  {Conditions: dozer.SpecRouteMapConditions{MatchAsPathList: new("gone")}},
				"10": {Conditions: dozer.SpecRouteMapConditions{MatchPrefixList: new("gone")}},
			}}}},
			desired:  &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{"10": acceptStmt}}}},
			expected: &dozer.Spec{RouteMaps: map[string]*dozer.SpecRouteMap{"x": {Statements: map[string]*dozer.SpecRouteMapStatement{"10": acceptStmt}}}},
			applied:  []string{},
			warnings: []string{
				"keep@routeMap/x/1: references prefixList/gone, not planned or kept",
				"keep@routeMap/x/2: references asPathList/gone, not planned or kept",
				"keep@routeMap/x/3: not on the switch",
				"keep@routeMap/x/10: planned by the fabric",
			},
		},
		{
			name:     "acl interface without planned bindings",
			rules:    []string{"keep@acl/manual", "keep@aclInterface/Ethernet0/ingress"},
			actual:   &dozer.Spec{ACLs: map[string]*dozer.SpecACL{"manual": {}}, ACLInterfaces: map[string]*dozer.SpecACLInterface{"Ethernet0": {Ingress: new("manual")}}},
			desired:  &dozer.Spec{Interfaces: interfaces},
			expected: &dozer.Spec{Interfaces: interfaces, ACLs: map[string]*dozer.SpecACL{"manual": {}}, ACLInterfaces: map[string]*dozer.SpecACLInterface{"Ethernet0": {Ingress: new("manual")}}},
			applied:  []string{"keep@acl/manual", "keep@aclInterface/Ethernet0/ingress"},
			warnings: []string{},
		},
		{
			name: "acl interfaces",
			rules: []string{
				"keep@aclInterface/Ethernet0/ingress", "keep@aclInterface/Ethernet0.10/ingress", "keep@aclInterface/Ethernet0.10/egress",
				"override@aclInterface/Vlan1000/egress", "keep@aclInterface/Vlan1001/ingress",
				"keep@aclInterface/Ethernet0.20/ingress", "keep@aclInterface/Vlan1002/ingress", "keep@aclInterface/Vlan1003/ingress",
				"keep@acl/manual",
			},
			actual: &dozer.Spec{
				ACLs: map[string]*dozer.SpecACL{"manual": {}, "other": {}},
				ACLInterfaces: map[string]*dozer.SpecACLInterface{
					"Ethernet0":    {Ingress: new("manual")},
					"Ethernet0.10": {Ingress: new("manual"), Egress: new("manual")},
					"Ethernet0.20": {Ingress: new("manual")},
					"Vlan1000":     {Egress: new("manual")},
					"Vlan1002":     {Ingress: new("manual")},
					"Vlan1003":     {Ingress: new("other")},
				},
			},
			desired: &dozer.Spec{
				Interfaces: interfaces,
				ACLs:       map[string]*dozer.SpecACL{"fabric": {}},
				ACLInterfaces: map[string]*dozer.SpecACLInterface{
					"Ethernet0.10": {Egress: new("fabric")},
					"Vlan1000":     {Ingress: new("fabric"), Egress: new("fabric")},
				},
			},
			expected: &dozer.Spec{
				Interfaces: interfaces,
				ACLs:       map[string]*dozer.SpecACL{"fabric": {}, "manual": {}},
				ACLInterfaces: map[string]*dozer.SpecACLInterface{
					"Ethernet0":    {Ingress: new("manual")},
					"Ethernet0.10": {Ingress: new("manual"), Egress: new("fabric")},
					"Vlan1000":     {Ingress: new("fabric"), Egress: new("manual")},
				},
			},
			applied: []string{
				"keep@acl/manual", "keep@aclInterface/Ethernet0.10/ingress", "keep@aclInterface/Ethernet0/ingress",
				"override@aclInterface/Vlan1000/egress",
			},
			warnings: []string{
				"keep@aclInterface/Ethernet0.10/egress: planned by the fabric", "keep@aclInterface/Vlan1001/ingress: not on the switch",
				"keep@aclInterface/Ethernet0.20/ingress: not planned by the fabric", "keep@aclInterface/Vlan1002/ingress: not planned by the fabric",
				"keep@aclInterface/Vlan1003/ingress: references acl/other, not planned or kept",
			},
		},
		{
			name:  "static routes",
			rules: []string{"keep@staticRoute/default/10.99.0.0/16", "override@staticRoute/default/0.0.0.0/0", "keep@staticRoute/VrfVmanual/10.0.0.0/8"},
			actual: &dozer.Spec{VRFs: map[string]*dozer.SpecVRF{
				"default":    {StaticRoutes: map[string]*dozer.SpecVRFStaticRoute{"10.99.0.0/16": viaA, "0.0.0.0/0": viaA}},
				"VrfVmanual": {StaticRoutes: map[string]*dozer.SpecVRFStaticRoute{"10.0.0.0/8": viaA}},
			}},
			desired: &dozer.Spec{VRFs: map[string]*dozer.SpecVRF{
				"default": {StaticRoutes: map[string]*dozer.SpecVRFStaticRoute{"0.0.0.0/0": viaB}},
			}},
			expected: &dozer.Spec{VRFs: map[string]*dozer.SpecVRF{
				"default": {StaticRoutes: map[string]*dozer.SpecVRFStaticRoute{"10.99.0.0/16": viaA, "0.0.0.0/0": viaA}},
			}},
			applied:  []string{"keep@staticRoute/default/10.99.0.0/16", "override@staticRoute/default/0.0.0.0/0"},
			warnings: []string{"keep@staticRoute/VrfVmanual/10.0.0.0/8: not planned by the fabric"},
		},
		{
			name:     "bfd profile",
			rules:    []string{"override@bfdProfile/fabric"},
			actual:   &dozer.Spec{BFDProfiles: map[string]*dozer.SpecBFDProfile{"fabric": {DetectionMultiplier: new(uint8(5))}}},
			desired:  &dozer.Spec{BFDProfiles: map[string]*dozer.SpecBFDProfile{"fabric": {DetectionMultiplier: new(uint8(3))}}},
			expected: &dozer.Spec{BFDProfiles: map[string]*dozer.SpecBFDProfile{"fabric": {DetectionMultiplier: new(uint8(5))}}},
			applied:  []string{"override@bfdProfile/fabric"},
			warnings: []string{},
		},
		{
			name: "community and as-path members",
			rules: []string{
				"add@communityList/x/65000:100", "add@communityList/x/50000:0", "remove@communityList/x/50000:1",
				"add@communityList/y/1:1", "override@asPathList/manual",
				"remove@asPathList/fabric/_65100_", "remove@asPathList/fabric/_65099_", "remove@communityList/z/1:1",
			},
			actual: &dozer.Spec{
				CommunityLists: map[string]*dozer.SpecCommunityList{"x": {Members: []string{"65000:100", "50000:1", "50000:0"}}},
				AsPathLists:    map[string]*dozer.SpecAsPathList{"manual": {Members: []string{"_2_", "_1_", "_2_"}}},
			},
			desired: &dozer.Spec{
				CommunityLists: map[string]*dozer.SpecCommunityList{
					"x": {Members: []string{"50000:1", "50000:0"}},
					"z": {Members: []string{"1:1"}},
				},
				AsPathLists: map[string]*dozer.SpecAsPathList{"fabric": {Members: []string{"_65534_", "_65100_"}}, "empty": {}},
			},
			expected: &dozer.Spec{
				CommunityLists: map[string]*dozer.SpecCommunityList{
					"x": {Members: []string{"50000:0", "65000:100"}},
					"z": {Members: []string{"1:1"}},
				},
				AsPathLists: map[string]*dozer.SpecAsPathList{
					"fabric": {Members: []string{"_65534_"}},
					"manual": {Members: []string{"_1_", "_2_"}},
					"empty":  {},
				},
			},
			applied: []string{
				"add@communityList/x/65000:100",
				"override@asPathList/manual", "remove@asPathList/fabric/_65100_", "remove@communityList/x/50000:1",
			},
			warnings: []string{
				"add@communityList/x/50000:0: member already in the list", "add@communityList/y/1:1: not planned by the fabric",
				"remove@asPathList/fabric/_65099_: member not in the list", "remove@communityList/z/1:1: last member of the list",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := wiringapi.ParseBCMMutateRules(tt.rules)
			require.NoError(t, err)

			applied, warnings := dozer.ApplyMutateRules(tt.actual, tt.desired, rules)
			require.Equal(t, tt.expected, tt.desired)
			require.Equal(t, tt.applied, applied)
			require.Equal(t, tt.warnings, warnings)
		})
	}

	t.Run("actual members are sorted and stay separate", func(t *testing.T) {
		actual := &dozer.Spec{CommunityLists: map[string]*dozer.SpecCommunityList{"x": {Members: []string{"2:2", "1:1", "2:2"}}}}
		desired := &dozer.Spec{}

		rules, err := wiringapi.ParseBCMMutateRules([]string{"keep@communityList/x"})
		require.NoError(t, err)

		dozer.ApplyMutateRules(actual, desired, rules)
		require.Equal(t, []string{"1:1", "2:2"}, actual.CommunityLists["x"].Members)
		require.Equal(t, []string{"1:1", "2:2"}, desired.CommunityLists["x"].Members)
		desired.CommunityLists["x"].Members[0] = "3:3"
		require.Equal(t, []string{"1:1", "2:2"}, actual.CommunityLists["x"].Members)
	})
}
