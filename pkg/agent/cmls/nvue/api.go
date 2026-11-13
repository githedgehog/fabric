// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
)

// Interfaces lists every interface with its link state, LLDP neighbors and link counters, but not the traffic
// counters or rates, which have their own calls
func (c *Client) Interfaces(ctx context.Context) (map[string]Interface, error) {
	ifaces := map[string]Interface{}
	if err := c.Get(ctx, "/interface", nil, &ifaces); err != nil {
		return nil, err
	}

	return ifaces, nil
}

// InterfaceRates returns the traffic rates of every interface that keeps them, in one call
func (c *Client) InterfaceRates(ctx context.Context) (map[string]InterfaceRates, error) {
	ifaces := map[string]Interface{}
	if err := c.Get(ctx, "/interface", url.Values{"view": {"rates"}}, &ifaces); err != nil {
		return nil, err
	}

	rates := make(map[string]InterfaceRates, len(ifaces))
	for name, iface := range ifaces {
		if iface.Rates != nil {
			rates[name] = *iface.Rates
		}
	}

	return rates, nil
}

// InterfaceCounters returns the traffic and queue counters of one interface, ErrNotFound for the ones without
func (c *Client) InterfaceCounters(ctx context.Context, name string) (*InterfaceCounters, error) {
	counters := &InterfaceCounters{}
	if err := c.Get(ctx, interfacePath(name, "counters"), nil, counters); err != nil {
		return nil, err
	}

	return counters, nil
}

// Transceivers lists the inserted modules with their vendor fields only, see Transceiver for the rest
func (c *Client) Transceivers(ctx context.Context) (map[string]Transceiver, error) {
	transceivers := map[string]Transceiver{}
	if err := c.Get(ctx, "/platform/transceiver", nil, &transceivers); err != nil {
		return nil, err
	}

	return transceivers, nil
}

// Transceiver returns everything the module in the named port reports, including diagnostics when it has them
func (c *Client) Transceiver(ctx context.Context, name string) (*Transceiver, error) {
	transceiver := &Transceiver{}
	if err := c.Get(ctx, "/platform/transceiver/"+url.PathEscape(name), nil, transceiver); err != nil {
		return nil, err
	}

	return transceiver, nil
}

func (c *Client) Platform(ctx context.Context) (*Platform, error) {
	platform := &Platform{}
	if err := c.Get(ctx, "/platform", nil, platform); err != nil {
		return nil, err
	}

	return platform, nil
}

func (c *Client) Fans(ctx context.Context) (map[string]Fan, error) {
	fans := map[string]Fan{}
	if err := c.Get(ctx, "/platform/environment/fan", nil, &fans); err != nil {
		return nil, err
	}

	return fans, nil
}

func (c *Client) PSUs(ctx context.Context) (map[string]PSU, error) {
	psus := map[string]PSU{}
	if err := c.Get(ctx, "/platform/environment/psu", nil, &psus); err != nil {
		return nil, err
	}

	return psus, nil
}

func (c *Client) Temperatures(ctx context.Context) (map[string]Temperature, error) {
	temperatures := map[string]Temperature{}
	if err := c.Get(ctx, "/platform/environment/temperature", nil, &temperatures); err != nil {
		return nil, err
	}

	return temperatures, nil
}

// Firmware returns the firmware of every platform component (BIOS, CPLDs, SSD, ASIC) keyed by component name
func (c *Client) Firmware(ctx context.Context) (map[string]Firmware, error) {
	firmware := map[string]Firmware{}
	if err := c.Get(ctx, "/platform/firmware", nil, &firmware); err != nil {
		return nil, err
	}

	return firmware, nil
}

func (c *Client) System(ctx context.Context) (*System, error) {
	system := &System{}
	if err := c.Get(ctx, "/system", nil, system); err != nil {
		return nil, err
	}

	return system, nil
}

// VRFs returns the VRF names, sorted, "default" and "mgmt" are always among them
func (c *Client) VRFs(ctx context.Context) ([]string, error) {
	vrfs := map[string]json.RawMessage{}
	if err := c.Get(ctx, "/vrf", nil, &vrfs); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(vrfs))
	for name := range vrfs {
		names = append(names, name)
	}
	slices.Sort(names)

	return names, nil
}

// BGPInstance returns the BGP instance of a VRF with a summary of its neighbors, ErrNotFound when the VRF has no BGP
func (c *Client) BGPInstance(ctx context.Context, vrf string) (*BGPInstance, error) {
	instance := &BGPInstance{}
	if err := c.Get(ctx, fmt.Sprintf("/vrf/%s/router/bgp", url.PathEscape(vrf)), nil, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// BGPNeighbor returns everything about one neighbor, ErrNotFound once it's gone
func (c *Client) BGPNeighbor(ctx context.Context, vrf, id string) (*BGPNeighbor, error) {
	neighbor := &BGPNeighbor{}
	path := fmt.Sprintf("/vrf/%s/router/bgp/neighbor/%s", url.PathEscape(vrf), url.PathEscape(id))
	if err := c.Get(ctx, path, nil, neighbor); err != nil {
		return nil, err
	}

	return neighbor, nil
}

// BGPPeerGroup returns a peer group of the BGP instance in a VRF, ErrNotFound when there's no such group
func (c *Client) BGPPeerGroup(ctx context.Context, vrf, id string) (*BGPPeerGroup, error) {
	group := &BGPPeerGroup{}
	path := fmt.Sprintf("/vrf/%s/router/bgp/peer-group/%s", url.PathEscape(vrf), url.PathEscape(id))
	if err := c.Get(ctx, path, nil, group); err != nil {
		return nil, err
	}

	return group, nil
}
