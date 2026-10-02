// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SwitchFilter selects the switches to report on: the named ones, the ones of a Fabric (optionally only of one of its
// domains) or all of them if nothing is set
type SwitchFilter struct {
	Names  []string
	Fabric string
	Domain string
}

func (f SwitchFilter) Validate() error {
	if len(f.Names) > 0 && (f.Fabric != "" || f.Domain != "") {
		return fmt.Errorf("fabric and domain can't be used together with switch names") //nolint:err113
	}

	// domain names are only unique within a fabric
	if f.Domain != "" && f.Fabric == "" {
		return fmt.Errorf("domain can only be used together with fabric") //nolint:err113
	}

	return nil
}

// fabricLabels selects the objects of the filtered fabric, and of its domain if requested and set
func (f SwitchFilter) fabricLabels(withDomain bool) labels.Set {
	set := labels.Set{}
	if f.Fabric != "" {
		set[wiringapi.ListLabelFabric(f.Fabric)] = wiringapi.ListLabelValue
	}
	if withDomain && f.Domain != "" {
		set[wiringapi.ListLabelDomain(f.Domain)] = wiringapi.ListLabelValue
	}

	return set
}

// check validates the filter and makes sure the filtered fabric and domain exist, listing the ones that do otherwise
func (f SwitchFilter) check(ctx context.Context, kube kclient.Reader) error {
	if err := f.Validate(); err != nil {
		return err
	}

	if f.Fabric == "" {
		return nil
	}

	fabList := &wiringapi.FabricList{}
	if err := kube.List(ctx, fabList, kclient.InNamespace(kmetav1.NamespaceDefault)); err != nil {
		return fmt.Errorf("listing fabrics: %w", err)
	}

	fabrics := map[string]*wiringapi.Fabric{}
	for idx := range fabList.Items {
		fabrics[fabList.Items[idx].Name] = &fabList.Items[idx]
	}

	fab, ok := fabrics[f.Fabric]
	if !ok {
		return fmt.Errorf("fabric %s not found, available: %s", f.Fabric, availableNames(fabrics)) //nolint:err113
	}

	if _, ok := fab.Spec.Domains[f.Domain]; f.Domain != "" && !ok {
		return fmt.Errorf("domain %s not found in fabric %s, available: %s", f.Domain, f.Fabric, availableNames(fab.Spec.Domains)) //nolint:err113
	}

	return nil
}

func availableNames[V any](m map[string]V) string {
	if len(m) == 0 {
		return "none"
	}

	return strings.Join(slices.Sorted(maps.Keys(m)), ", ")
}

// switchInput is everything about a single switch the per-switch status helpers need. Switches and profiles always
// come from their own objects through the cache, never from the copies in the agent spec.
type switchInput struct {
	sw    *wiringapi.Switch
	ag    *agentapi.Agent
	conns []*wiringapi.Connection
	cache *switchCache
}

// switchCache resolves switches (e.g. peers) and switch profiles by name: from the switches loaded for the filter or
// by fetching and remembering the rest
type switchCache struct {
	kube     kclient.Reader
	switches map[string]*wiringapi.Switch
	profiles map[string]*wiringapi.SwitchProfile
}

func (c *switchCache) switchByName(ctx context.Context, name string) (*wiringapi.Switch, error) {
	if sw, ok := c.switches[name]; ok {
		return sw, nil
	}

	sw := &wiringapi.Switch{}
	if err := c.kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
		if kapierrors.IsNotFound(err) {
			return nil, fmt.Errorf("switch %s not found", name) //nolint:err113
		}

		return nil, fmt.Errorf("getting switch %s: %w", name, err)
	}
	c.switches[name] = sw

	return sw, nil
}

func (c *switchCache) profile(ctx context.Context, name string) (*wiringapi.SwitchProfile, error) {
	if sp, ok := c.profiles[name]; ok {
		return sp, nil
	}

	sp := &wiringapi.SwitchProfile{}
	if err := c.kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sp); err != nil {
		return nil, fmt.Errorf("getting switch profile %s: %w", name, err)
	}
	c.profiles[name] = sp

	return sp, nil
}

// switchProfile returns the switch together with its profile
func (c *switchCache) switchProfile(ctx context.Context, name string) (*wiringapi.Switch, *wiringapi.SwitchProfile, error) {
	sw, err := c.switchByName(ctx, name)
	if err != nil {
		return nil, nil, err
	}

	if sw.Spec.Profile == "" {
		return nil, nil, fmt.Errorf("switch profile is not set for %s", name) //nolint:err113
	}

	sp, err := c.profile(ctx, sw.Spec.Profile)
	if err != nil {
		return nil, nil, err
	}

	return sw, sp, nil
}

// forEachSwitch loads the switches selected by the already checked filter with their agents and connections, and
// calls fn for each of them in name order. Connections are the ones with the switch list label, narrowed by connReqs.
func forEachSwitch(ctx context.Context, kube kclient.Reader, filter SwitchFilter, connReqs []labels.Requirement, fn func(in *switchInput) error) error {
	switches := map[string]*wiringapi.Switch{}
	agents := map[string]*agentapi.Agent{}
	conns := map[string][]*wiringapi.Connection{}

	if len(filter.Names) > 0 {
		for _, name := range filter.Names {
			if _, ok := switches[name]; ok {
				continue
			}

			sw := &wiringapi.Switch{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, sw); err != nil {
				if kapierrors.IsNotFound(err) {
					return fmt.Errorf("switch %s not found", name) //nolint:err113
				}

				return fmt.Errorf("getting switch %s: %w", name, err)
			}
			switches[name] = sw

			ag := &agentapi.Agent{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, ag); err != nil {
				if kapierrors.IsNotFound(err) {
					return fmt.Errorf("agent %s not found", name) //nolint:err113
				}

				return fmt.Errorf("getting agent %s: %w", name, err)
			}
			agents[name] = ag

			connList := &wiringapi.ConnectionList{}
			sel := labels.SelectorFromSet(labels.Set{wiringapi.ListLabelSwitch(name): wiringapi.ListLabelValue}).Add(connReqs...)
			if err := kube.List(ctx, connList, kclient.InNamespace(kmetav1.NamespaceDefault), kclient.MatchingLabelsSelector{Selector: sel}); err != nil {
				return fmt.Errorf("listing connections of switch %s: %w", name, err)
			}
			for idx := range connList.Items {
				conns[name] = append(conns[name], &connList.Items[idx])
			}
		}
	} else {
		// agents carry the labels of their switches
		swSel := kclient.MatchingLabels(filter.fabricLabels(true))

		swList := &wiringapi.SwitchList{}
		if err := kube.List(ctx, swList, kclient.InNamespace(kmetav1.NamespaceDefault), swSel); err != nil {
			return fmt.Errorf("listing switches: %w", err)
		}
		for idx := range swList.Items {
			switches[swList.Items[idx].Name] = &swList.Items[idx]
		}

		agList := &agentapi.AgentList{}
		if err := kube.List(ctx, agList, kclient.InNamespace(kmetav1.NamespaceDefault), swSel); err != nil {
			return fmt.Errorf("listing agents: %w", err)
		}
		for idx := range agList.Items {
			agents[agList.Items[idx].Name] = &agList.Items[idx]
		}

		// connections have no domain label, the switches they're grouped by narrow them down to the domain
		connList := &wiringapi.ConnectionList{}
		sel := labels.SelectorFromSet(filter.fabricLabels(false)).Add(connReqs...)
		if err := kube.List(ctx, connList, kclient.InNamespace(kmetav1.NamespaceDefault), kclient.MatchingLabelsSelector{Selector: sel}); err != nil {
			return fmt.Errorf("listing connections: %w", err)
		}

		switchPrefix := wiringapi.ListLabelPrefix(wiringapi.ConnectionLabelTypeSwitch)
		for idx := range connList.Items {
			conn := &connList.Items[idx]
			for label, value := range conn.Labels {
				name, ok := strings.CutPrefix(label, switchPrefix)
				if !ok || value != wiringapi.ListLabelValue {
					continue
				}

				if _, ok := switches[name]; ok {
					conns[name] = append(conns[name], conn)
				}
			}
		}
	}

	cache := &switchCache{
		kube:     kube,
		switches: maps.Clone(switches),
		profiles: map[string]*wiringapi.SwitchProfile{},
	}

	for _, name := range slices.Sorted(maps.Keys(switches)) {
		ag, ok := agents[name]
		if !ok {
			return fmt.Errorf("agent %s not found", name) //nolint:err113
		}

		if err := fn(&switchInput{
			sw:    switches[name],
			ag:    ag,
			conns: conns[name],
			cache: cache,
		}); err != nil {
			return err
		}
	}

	return nil
}
