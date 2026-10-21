// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

const (
	defaultVRF = "default"
	reservedIP = "reserved"
)

func subnetKey(vrf, circuitID string) string {
	vrf = strings.ToLower(vrf)
	circuitID = strings.ToLower(circuitID)

	if vrf == "" {
		vrf = defaultVRF
	}

	res := vrf
	if circuitID != "" {
		res += ":" + circuitID
	}

	return res
}

func subnetKeyFrom(subnet *dhcpapi.DHCPSubnet) string {
	return subnetKey(subnet.Spec.VRF, subnet.Spec.CircuitID)
}

var (
	ErrNoAvailableIP          = fmt.Errorf("no available IP address")
	ErrRequestedIPUnavailable = errors.New("requested IP unavailable")
)

func wantedIP(req *dhcpv4.DHCPv4) (netip.Addr, bool) {
	ip := req.RequestedIPAddress()
	if (ip == nil || ip.IsUnspecified()) && req.MessageType() == dhcpv4.MessageTypeRequest {
		ip = req.ClientIPAddr
	}
	if ip == nil || ip.IsUnspecified() {
		return netip.Addr{}, false
	}

	addr, ok := netip.AddrFromSlice(ip)

	return addr.Unmap(), ok
}

// usedIPs returns the owner MAC (or reserved) of the gateway, leases and statics, and of the leases only
func usedIPs(obj *dhcpapi.DHCPSubnet, sp *subnetSpec) (map[netip.Addr]string, map[netip.Addr]string) {
	used := make(map[netip.Addr]string, len(obj.Status.Allocated)+len(sp.statics)+1)
	allocated := make(map[netip.Addr]string, len(obj.Status.Allocated))

	used[sp.parsed.Gateway] = reservedIP
	for mac, entry := range obj.Status.Allocated {
		// an unparseable IP can't collide with any candidate
		if ip, err := netip.ParseAddr(entry.IP); err == nil {
			used[ip] = mac
			allocated[ip] = mac
		}
	}
	for mac, ip := range sp.statics {
		// static IPs that are already used by another MAC are free for it until that lease expires
		if owner, ok := used[ip]; ok && owner != mac {
			continue
		}
		used[ip] = mac
	}

	return used, allocated
}

// allocate picks an IP for the client without modifying obj and reports whether it's the client's static IP. IPs
// offered to other clients count as used, offers must be unexpired. A DISCOVER gets another IP if the wanted one isn't
// available with the reason why, while a REQUEST gets ErrRequestedIPUnavailable.
func allocate(obj *dhcpapi.DHCPSubnet, sp *subnetSpec, offers map[string]offer, req *dhcpv4.DHCPv4) (netip.Addr, bool, string, error) {
	used, allocatedBy := usedIPs(obj, sp)
	for offerMAC, o := range offers {
		// a stored lease wins over an offer, it could only be written by another writer after the offer was made
		if _, ok := used[o.ip]; !ok {
			used[o.ip] = offerMAC
		}
	}
	usedByOther := func(ip netip.Addr, mac string) (string, bool) {
		owner, ok := used[ip]

		return owner, ok && owner != mac
	}

	var res netip.Addr
	mac := req.ClientHWAddr.String()

	if static, ok := sp.statics[mac]; ok {
		if owner, ok := allocatedBy[static]; ok && owner != mac {
			slog.Warn("Static IP is already allocated for different MAC, ignoring", "ip", static.String(), "mac", mac, "usedBy", owner)
		} else {
			res = static
		}
	}
	isStatic := res.IsValid()

	isRequest := req.MessageType() == dhcpv4.MessageTypeRequest
	wanted, hasWanted := wantedIP(req)

	if isStatic && isRequest && hasWanted && wanted != res {
		return netip.Addr{}, false, "", fmt.Errorf("%w: %s, static IP is %s", ErrRequestedIPUnavailable, wanted, res)
	}

	// if requested IP is valid, not in use, and within the subnet start/end range, use it
	notWanted := ""
	if !res.IsValid() && hasWanted {
		reason := ""
		if !wanted.Is4() {
			reason = "not an IPv4 address"
		} else if owner, isUsed := usedByOther(wanted, mac); isUsed {
			reason = "used by " + owner
		} else if !inRange(sp, wanted) {
			reason = "outside of the start-end range"
		} else {
			res = wanted
		}

		if reason != "" {
			if isRequest {
				return netip.Addr{}, false, "", fmt.Errorf("%w: %s %s", ErrRequestedIPUnavailable, wanted, reason)
			}

			notWanted = fmt.Sprintf("requested %s %s", wanted, reason)
		}
	}

	// a retransmitted DISCOVER gets the same offer
	if !res.IsValid() && !isRequest {
		if o, ok := offers[mac]; ok {
			if _, isUsed := usedByOther(o.ip, mac); !isUsed && inRange(sp, o.ip) {
				res = o.ip
			}
		}
	}

	// if allocated IP is valid, not in use, and within the start/end subnet range, use it
	if !res.IsValid() {
		if allocated, ok := obj.Status.Allocated[mac]; ok {
			ip, err := netip.ParseAddr(allocated.IP)
			if err != nil {
				slog.Warn("Invalid allocated IP address, ignoring", "ip", allocated.IP, "mac", mac)
			} else if owner, isUsed := usedByOther(ip, mac); isUsed {
				slog.Warn("Allocated IP is already used, ignoring", "ip", allocated.IP, "mac", mac, "usedBy", owner)
			} else if !inRange(sp, ip) {
				slog.Warn("Allocated IP is outside start-end range, ignoring", "ip", allocated.IP, "mac", mac)
			} else {
				res = ip
			}
		}
	}

	// if able to find unused IP within the start/end subnet range, use it
	if !res.IsValid() {
		for ip := sp.parsed.Start; ip.IsValid() && ip.Compare(sp.parsed.End) <= 0; ip = ip.Next() {
			if _, isUsed := usedByOther(ip, mac); !isUsed {
				res = ip

				break
			}
		}
	}

	if !res.IsValid() {
		return netip.Addr{}, false, "", ErrNoAvailableIP
	}

	return res, isStatic, notWanted, nil
}

func inRange(sp *subnetSpec, ip netip.Addr) bool {
	return ip.Compare(sp.parsed.Start) >= 0 && ip.Compare(sp.parsed.End) <= 0
}
