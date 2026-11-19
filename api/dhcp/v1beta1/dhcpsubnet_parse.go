// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"slices"

	"go.githedgehog.com/fabric/pkg/util/iputil"
)

const (
	MinMTU = 96
	MaxMTU = 9036
)

// DHCPSubnetParsed is a DHCPSubnetSpec with all addresses and URLs parsed and validated
// +kubebuilder:object:generate=false
type DHCPSubnetParsed struct {
	Prefix           netip.Prefix
	Gateway          netip.Addr
	Start            netip.Addr
	End              netip.Addr
	DNSServers       []netip.Addr
	TimeServers      []netip.Addr
	AdvertisedRoutes []DHCPRouteParsed
	PXEURL           *url.URL // nil if not set
}

// +kubebuilder:object:generate=false
type DHCPRouteParsed struct {
	Destination netip.Prefix
	Gateway     netip.Addr
}

// Validate checks everything the DHCP server needs, including the static assignments
func (spec *DHCPSubnetSpec) Validate() error {
	parsed, err := spec.Parse()
	if err != nil {
		return err
	}

	if _, errs := parsed.ParseStatics(spec.Static); len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// Parse checks the whole spec except for the static assignments, which ParseStatics checks one by one so the DHCP
// server can skip an invalid one without dropping the whole subnet
func (spec *DHCPSubnetSpec) Parse() (*DHCPSubnetParsed, error) {
	prefix, err := netip.ParsePrefix(spec.CIDRBlock)
	if err != nil {
		return nil, fmt.Errorf("parsing cidr block %q: %w", spec.CIDRBlock, err)
	}
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("cidr block %s is not IPv4", spec.CIDRBlock) //nolint:err113
	}

	res := &DHCPSubnetParsed{Prefix: prefix.Masked()}

	if res.Gateway, err = parseAddrIn(res.Prefix, "gateway", spec.Gateway); err != nil {
		return nil, err
	}
	if res.Start, err = parseAddrIn(res.Prefix, "start IP", spec.StartIP); err != nil {
		return nil, err
	}
	if res.End, err = parseAddrIn(res.Prefix, "end IP", spec.EndIP); err != nil {
		return nil, err
	}
	if res.End.Less(res.Start) {
		return nil, fmt.Errorf("start IP %s is after end IP %s", res.Start, res.End) //nolint:err113
	}

	if res.DNSServers, err = parseAddrs("dns server", spec.DNSServers); err != nil {
		return nil, err
	}
	if res.TimeServers, err = parseAddrs("time server", spec.TimeServers); err != nil {
		return nil, err
	}

	if spec.InterfaceMTU != 0 && (spec.InterfaceMTU < MinMTU || spec.InterfaceMTU > MaxMTU) {
		return nil, fmt.Errorf("interface MTU %d is not in %d-%d", spec.InterfaceMTU, MinMTU, MaxMTU) //nolint:err113
	}

	for _, route := range spec.AdvertisedRoutes {
		dst, err := netip.ParsePrefix(route.Destination)
		if err != nil {
			return nil, fmt.Errorf("parsing advertised route destination %q: %w", route.Destination, err)
		}
		if !dst.Addr().Is4() {
			return nil, fmt.Errorf("advertised route destination %s is not IPv4", route.Destination) //nolint:err113
		}
		gw, err := parseAddrIn(res.Prefix, "advertised route gateway", route.Gateway)
		if err != nil {
			return nil, err
		}
		res.AdvertisedRoutes = append(res.AdvertisedRoutes, DHCPRouteParsed{Destination: dst.Masked(), Gateway: gw})
	}

	if spec.PXEURL != "" {
		if res.PXEURL, err = parseURL("pxe url", spec.PXEURL); err != nil {
			return nil, err
		}
	}
	if spec.ZTPBaseURL != "" {
		if _, err := parseURL("ztp base url", spec.ZTPBaseURL); err != nil {
			return nil, err
		}
	}

	return res, nil
}

// ParseStatics returns the valid static IPs by normalized MAC and an error for each invalid or duplicate assignment
func (p *DHCPSubnetParsed) ParseStatics(statics map[string]DHCPSubnetStatic) (map[string]netip.Addr, []error) {
	res := make(map[string]netip.Addr, len(statics))
	owners := make(map[netip.Addr]string, len(statics))
	var errs []error

	// sorted, so it's always the same one of the duplicates that is skipped
	for _, mac := range slices.Sorted(maps.Keys(statics)) {
		normMAC, ip, err := p.parseStatic(mac, statics[mac].IP)
		if err != nil {
			errs = append(errs, err)

			continue
		}
		if _, ok := res[normMAC]; ok {
			errs = append(errs, fmt.Errorf("static MAC %s is assigned more than once", mac)) //nolint:err113

			continue
		}
		if owner, ok := owners[ip]; ok {
			errs = append(errs, fmt.Errorf("static IP %s is assigned to both %s and %s", ip, owner, normMAC)) //nolint:err113

			continue
		}

		res[normMAC] = ip
		owners[ip] = normMAC
	}

	return res, errs
}

func (p *DHCPSubnetParsed) parseStatic(mac, ipStr string) (string, netip.Addr, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return "", netip.Addr{}, fmt.Errorf("parsing static MAC %q: %w", mac, err)
	}

	ip, err := parseAddrIn(p.Prefix, "static IP for "+mac, ipStr)
	if err != nil {
		return "", netip.Addr{}, err
	}

	switch ip {
	case p.Prefix.Addr():
		return "", netip.Addr{}, fmt.Errorf("static IP %s for %s is the subnet address", ip, mac) //nolint:err113
	case p.Gateway:
		return "", netip.Addr{}, fmt.Errorf("static IP %s for %s is the gateway", ip, mac) //nolint:err113
	case iputil.LastIPNetip(p.Prefix):
		return "", netip.Addr{}, fmt.Errorf("static IP %s for %s is the broadcast address", ip, mac) //nolint:err113
	}

	return hw.String(), ip, nil
}

func parseAddrIn(prefix netip.Prefix, what, s string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parsing %s %q: %w", what, s, err)
	}
	if !prefix.Contains(addr) {
		return netip.Addr{}, fmt.Errorf("%s %s is not in %s", what, s, prefix) //nolint:err113
	}

	return addr, nil
}

func parseAddrs(what string, ss []string) ([]netip.Addr, error) {
	res := make([]netip.Addr, 0, len(ss))
	for _, s := range ss {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("parsing %s %q: %w", what, s, err)
		}
		if !addr.Is4() {
			return nil, fmt.Errorf("%s %s is not IPv4", what, s) //nolint:err113
		}
		res = append(res, addr)
	}

	return res, nil
}

func parseURL(what, s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parsing %s %q: %w", what, s, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%s %q has no scheme or host", what, s) //nolint:err113
	}

	return u, nil
}
