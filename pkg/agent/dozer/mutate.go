// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dozer

import (
	"slices"
	"strconv"
	"strings"

	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

type mutator struct {
	actual   *Spec
	desired  *Spec
	applied  []string
	warnings []string
}

// ApplyMutateRules changes desired so that it takes the switch content selected by the rules and returns the rules that
// took effect (sorted) and the warnings for the ones skipped because of what the fabric plans or the switch has
func ApplyMutateRules(actual, desired *Spec, rules []wiringapi.BCMMutateRule) ([]string, []string) {
	m := &mutator{actual: actual, desired: desired, applied: []string{}, warnings: []string{}}

	copies := func(rule wiringapi.BCMMutateRule) bool {
		return rule.Mode == wiringapi.BCMMutateKeep || rule.Mode == wiringapi.BCMMutateOverride
	}

	// whole objects first, so the bindings and statements can reference the objects kept by other rules
	for _, rule := range rules {
		if copies(rule) && rule.Element == "" {
			m.copy(rule)
		}
	}

	// then elements of the objects: ACL rules, route map statements, prefix list entries, ACL bindings, static routes
	for _, rule := range rules {
		if copies(rule) && rule.Element != "" {
			m.copy(rule)
		}
	}

	// list members are added to the lists planned by Fabric
	for _, rule := range rules {
		if rule.Mode == wiringapi.BCMMutateAdd {
			m.members(rule)
		}
	}

	// and removed at the end, after all of them are added
	for _, rule := range rules {
		if rule.Mode == wiringapi.BCMMutateRemove {
			m.members(rule)
		}
	}

	if len(rules) > 0 {
		for _, spec := range []*Spec{actual, desired} {
			for _, list := range spec.CommunityLists {
				list.Members = sortedMembers(list.Members)
			}
			for _, list := range spec.AsPathLists {
				list.Members = sortedMembers(list.Members)
			}
		}

		slices.Sort(m.applied)
	}

	return m.applied, m.warnings
}

func (m *mutator) copy(rule wiringapi.BCMMutateRule) {
	a, d := m.actual, m.desired

	switch rule.Kind {
	case wiringapi.BCMMutateKindACL:
		if rule.Element == "" {
			copyObject(m, rule, a.ACLs, &d.ACLs)
		} else if seq, ok := m.seq(rule); ok && m.planned(rule, d.ACLs[rule.Name] != nil) && m.onSwitch(rule, a.ACLs[rule.Name] != nil) {
			d.ACLs[rule.Name].Entries = copyElement(m, rule, seq, a.ACLs[rule.Name].Entries, d.ACLs[rule.Name].Entries)
		}
	case wiringapi.BCMMutateKindRouteMap:
		if rule.Element == "" {
			copyObject(m, rule, a.RouteMaps, &d.RouteMaps)
		} else if m.planned(rule, d.RouteMaps[rule.Name] != nil) && m.onSwitch(rule, a.RouteMaps[rule.Name] != nil) &&
			m.statementRefsKept(rule, a.RouteMaps[rule.Name].Statements[rule.Element]) {
			d.RouteMaps[rule.Name].Statements = copyElement(m, rule, rule.Element, a.RouteMaps[rule.Name].Statements, d.RouteMaps[rule.Name].Statements)
		}
	case wiringapi.BCMMutateKindPrefixList:
		if rule.Element == "" {
			copyObject(m, rule, a.PrefixLists, &d.PrefixLists)
		} else if seq, ok := m.seq(rule); ok && m.planned(rule, d.PrefixLists[rule.Name] != nil) && m.onSwitch(rule, a.PrefixLists[rule.Name] != nil) {
			d.PrefixLists[rule.Name].Prefixes = copyElement(m, rule, seq, a.PrefixLists[rule.Name].Prefixes, d.PrefixLists[rule.Name].Prefixes)
		}
	case wiringapi.BCMMutateKindACLInterface:
		m.copyACLInterface(rule)
	case wiringapi.BCMMutateKindStaticRoute:
		if m.planned(rule, d.VRFs[rule.Name] != nil) && m.onSwitch(rule, a.VRFs[rule.Name] != nil) {
			d.VRFs[rule.Name].StaticRoutes = copyElement(m, rule, rule.Element, a.VRFs[rule.Name].StaticRoutes, d.VRFs[rule.Name].StaticRoutes)
		}
	case wiringapi.BCMMutateKindBFDProfile:
		copyObject(m, rule, a.BFDProfiles, &d.BFDProfiles)
	case wiringapi.BCMMutateKindCommunityList:
		copyObject(m, rule, a.CommunityLists, &d.CommunityLists)
	case wiringapi.BCMMutateKindASPathList:
		copyObject(m, rule, a.AsPathLists, &d.AsPathLists)
	default:
		m.warn(rule, "unknown kind")
	}
}

func (m *mutator) copyACLInterface(rule wiringapi.BCMMutateRule) {
	direction := func(iface *SpecACLInterface) **string {
		if rule.Element == wiringapi.BCMMutateIngress {
			return &iface.Ingress
		}

		return &iface.Egress
	}

	// a binding of an interface Fabric deletes would make the switch refuse the deletion
	if !m.planned(rule, m.interfacePlanned(rule.Name)) {
		return
	}

	actual := m.actual.ACLInterfaces[rule.Name]
	if !m.onSwitch(rule, actual != nil && *direction(actual) != nil) {
		return
	}
	// an ACL Fabric deletes while it's still bound would make the switch refuse the deletion
	if acl := **direction(actual); m.desired.ACLs[acl] == nil {
		m.warn(rule, "references acl/"+acl+", not planned or kept")

		return
	}

	desired := m.desired.ACLInterfaces[rule.Name]
	if desired != nil && *direction(desired) != nil && rule.Mode == wiringapi.BCMMutateKeep {
		m.warn(rule, "planned by the fabric")

		return
	}
	if desired == nil {
		if m.desired.ACLInterfaces == nil {
			m.desired.ACLInterfaces = map[string]*SpecACLInterface{}
		}
		desired = &SpecACLInterface{}
		m.desired.ACLInterfaces[rule.Name] = desired
	}

	*direction(desired) = *direction(actual)
	m.applied = append(m.applied, rule.String())
}

// statementRefsKept checks that everything the statement uses is planned by Fabric or kept by a rule, as Fabric would
// otherwise delete it while the statement still uses it and the switch would refuse that
func (m *mutator) statementRefsKept(rule wiringapi.BCMMutateRule, statement *SpecRouteMapStatement) bool {
	// copyElement skips and reports the statements missing on the switch and the kept ones planned by Fabric
	if statement == nil {
		return true
	}
	if routeMap := m.desired.RouteMaps[rule.Name]; routeMap != nil && rule.Mode == wiringapi.BCMMutateKeep {
		if _, planned := routeMap.Statements[rule.Element]; planned {
			return true
		}
	}

	missing := []string{}
	cond := statement.Conditions
	if cond.Call != nil && m.desired.RouteMaps[*cond.Call] == nil {
		missing = append(missing, "routeMap/"+*cond.Call)
	}
	if cond.MatchPrefixList != nil && m.desired.PrefixLists[*cond.MatchPrefixList] == nil {
		missing = append(missing, "prefixList/"+*cond.MatchPrefixList)
	}
	if cond.MatchNextHopPrefixList != nil && m.desired.PrefixLists[*cond.MatchNextHopPrefixList] == nil {
		missing = append(missing, "prefixList/"+*cond.MatchNextHopPrefixList)
	}
	if cond.MatchCommunityList != nil && m.desired.CommunityLists[*cond.MatchCommunityList] == nil {
		missing = append(missing, "communityList/"+*cond.MatchCommunityList)
	}
	if cond.MatchAsPathList != nil && m.desired.AsPathLists[*cond.MatchAsPathList] == nil {
		missing = append(missing, "asPathList/"+*cond.MatchAsPathList)
	}
	if cond.MatchSourceVRF != nil && m.desired.VRFs[*cond.MatchSourceVRF] == nil {
		missing = append(missing, "vrf/"+*cond.MatchSourceVRF)
	}

	if len(missing) > 0 {
		m.warn(rule, "references "+strings.Join(missing, ", ")+", not planned or kept")
	}

	return len(missing) == 0
}

// interfacePlanned checks the interface or subinterface (<port>.<vlan>) is planned by Fabric
func (m *mutator) interfacePlanned(name string) bool {
	port, vlan, isSub := strings.Cut(name, ".")
	iface := m.desired.Interfaces[port]
	if iface == nil {
		return false
	}
	if !isSub {
		return true
	}

	id, err := strconv.ParseUint(vlan, 10, 32)

	return err == nil && iface.Subinterfaces[uint32(id)] != nil
}

func (m *mutator) members(rule wiringapi.BCMMutateRule) {
	var list *[]string
	switch rule.Kind {
	case wiringapi.BCMMutateKindCommunityList:
		if l := m.desired.CommunityLists[rule.Name]; l != nil {
			list = &l.Members
		}
	case wiringapi.BCMMutateKindASPathList:
		if l := m.desired.AsPathLists[rule.Name]; l != nil {
			list = &l.Members
		}
	default:
		m.warn(rule, "add and remove are only for community and as-path lists")

		return
	}
	if !m.planned(rule, list != nil) {
		return
	}

	// rules that wouldn't change the list or would leave it empty are skipped
	inList := slices.Contains(*list, rule.Element)
	switch {
	case rule.Mode == wiringapi.BCMMutateAdd && inList:
		m.warn(rule, "member already in the list")

		return
	case rule.Mode == wiringapi.BCMMutateAdd:
		*list = append(*list, rule.Element)
	case rule.Mode == wiringapi.BCMMutateRemove && !inList:
		m.warn(rule, "member not in the list")

		return
	case rule.Mode == wiringapi.BCMMutateRemove && len(*list) == 1:
		m.warn(rule, "last member of the list")

		return
	case rule.Mode == wiringapi.BCMMutateRemove:
		*list = slices.DeleteFunc(*list, func(member string) bool { return member == rule.Element })
	}
	m.applied = append(m.applied, rule.String())
}

func (m *mutator) planned(rule wiringapi.BCMMutateRule, planned bool) bool {
	if !planned {
		m.warn(rule, "not planned by the fabric")
	}

	return planned
}

func (m *mutator) onSwitch(rule wiringapi.BCMMutateRule, onSwitch bool) bool {
	if !onSwitch {
		m.warn(rule, "not on the switch")
	}

	return onSwitch
}

func (m *mutator) seq(rule wiringapi.BCMMutateRule) (uint32, bool) {
	seq, err := strconv.ParseUint(rule.Element, 10, 32)
	if err != nil {
		m.warn(rule, "parsing seq: "+err.Error())

		return 0, false
	}

	return uint32(seq), true
}

func (m *mutator) warn(rule wiringapi.BCMMutateRule, reason string) {
	m.warnings = append(m.warnings, rule.String()+": "+reason)
}

func copyObject[V any](m *mutator, rule wiringapi.BCMMutateRule, actual map[string]*V, desired *map[string]*V) {
	value := actual[rule.Name]
	if !m.onSwitch(rule, value != nil) {
		return
	}
	if _, planned := (*desired)[rule.Name]; planned && rule.Mode == wiringapi.BCMMutateKeep {
		m.warn(rule, "planned by the fabric")

		return
	}

	if *desired == nil {
		*desired = map[string]*V{}
	}
	// a shallow copy: replacing fields of the desired one doesn't change the actual one, but the nested maps, slices and
	// pointers are shared, so they must not be changed in place (elements and members of the same object can't have
	// rules of their own)
	cp := *value
	(*desired)[rule.Name] = &cp
	m.applied = append(m.applied, rule.String())
}

func copyElement[K comparable, V any](m *mutator, rule wiringapi.BCMMutateRule, key K, actual, desired map[K]V) map[K]V {
	value, ok := actual[key]
	if !m.onSwitch(rule, ok) {
		return desired
	}
	if _, planned := desired[key]; planned && rule.Mode == wiringapi.BCMMutateKeep {
		m.warn(rule, "planned by the fabric")

		return desired
	}

	if desired == nil {
		desired = map[K]V{}
	}
	desired[key] = value
	m.applied = append(m.applied, rule.String())

	return desired
}

func sortedMembers(members []string) []string {
	if len(members) == 0 {
		return members
	}

	return slices.Compact(slices.Sorted(slices.Values(members)))
}
