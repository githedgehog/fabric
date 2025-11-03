// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultVRF = "default"
)

func subnetKey(vrf, circuitID string) string {
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

var ErrNoAvailableIP = fmt.Errorf("no available IP address")

func allocate(subnet *dhcpapi.DHCPSubnet, req *dhcpv4.DHCPv4) (netip.Addr, error) {
	if subnet.Status.Allocated == nil {
		subnet.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
	}

	expiry := time.Now()
	if req.MessageType() == dhcpv4.MessageTypeDiscover {
		expiry = expiry.Add(time.Minute) // TODO const
	} else {
		expiry = expiry.Add(time.Duration(subnet.Spec.LeaseTimeSeconds) * time.Second)
	}

	var res netip.Addr
	mac := req.ClientHWAddr.String()

	// if static IP is valid and not a gateway, use it
	if static, ok := subnet.Spec.Static[mac]; ok {
		ip, err := netip.ParseAddr(static.IP)
		switch {
		case err != nil:
			slog.Warn("Invalid static IP address, ignoring", "ip", static.IP)
		case static.IP == subnet.Spec.Gateway:
			slog.Warn("Static IP address is gateway, ignoring", "ip", static.IP)
		default:
			// ignore if already allocated for different MAC
			inUse := false
			for allocatedMAC, allocated := range subnet.Status.Allocated {
				if allocatedMAC == mac {
					continue
				}

				if allocated.IP == static.IP {
					slog.Warn("Static IP is already allocated for different MAC, ignoring", "ip", ip.String(), "mac", mac, "usedBy", allocatedMAC)
					inUse = true
				}
			}

			if !inUse {
				res = ip
				expiry = time.Time{}
			}
		}
	}

	used := map[string]string{
		subnet.Spec.Gateway: "reserved",
	}
	for allocatedMAC, allocated := range subnet.Status.Allocated {
		used[allocated.IP] = allocatedMAC
	}
	for staticMAC, static := range subnet.Spec.Static {
		// skip static IPs that are already in use by another MAC
		if used[static.IP] != "" && used[static.IP] != staticMAC {
			continue
		}
		used[static.IP] = staticMAC
	}

	startIP, err := netip.ParseAddr(subnet.Spec.StartIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parsing subnet start ip: %s: %w", subnet.Spec.StartIP, err)
	}

	endIP, err := netip.ParseAddr(subnet.Spec.EndIP)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parsing subnet end ip: %s: %w", subnet.Spec.EndIP, err)
	}

	// if requested IP is valid, not in use, and within the subnet start/end range, use it
	if !res.Is4() {
		if requested := req.RequestedIPAddress(); requested != nil {
			ip, ok := netip.AddrFromSlice(requested)
			switch {
			case !ok:
				slog.Warn("Invalid requested IP address, ignoring", "ip", requested.String())
			case used[ip.String()] != "" && used[ip.String()] != mac:
				slog.Warn("Requested IP is already used, ignoring", "ip", requested.String(), "mac", mac, "usedBy", used[ip.String()])
			case ip.Compare(startIP) < 0 || ip.Compare(endIP) > 0:
				slog.Warn("Requested IP is outside start-end range, ignoring", "ip", requested.String(), "mac", mac)
			default:
				res = ip
			}
		}
	}

	// if allocated IP is valid, not in use, and within the start/end subnet range, use it
	if !res.Is4() {
		if allocated, ok := subnet.Status.Allocated[mac]; ok {
			ip, err := netip.ParseAddr(allocated.IP)
			switch {
			case err != nil:
				slog.Warn("Invalid allocated IP address, ignoring", "ip", allocated.IP, "mac", mac)
			case used[allocated.IP] != "" && used[allocated.IP] != mac:
				slog.Warn("Allocated IP is already used, ignoring", "ip", allocated.IP, "mac", mac, "usedBy", used[allocated.IP])
			case ip.Compare(startIP) < 0 || ip.Compare(endIP) > 0:
				slog.Warn("Allocated IP is outside start-end range, ignoring", "ip", allocated.IP, "mac", mac)
			default:
				res = ip
			}
		}
	}

	// if able to find unused IP within the start/end subnet range, use it
	if !res.Is4() {
		ip := startIP
		for ip.Compare(endIP) <= 0 {
			if used[ip.String()] != "" && used[ip.String()] != mac {
				ip = ip.Next()

				continue
			}

			res = ip

			break
		}
	}

	// if no IP is found, error
	if !res.Is4() {
		return netip.Addr{}, ErrNoAvailableIP
	}

	subnet.Status.Allocated[mac] = dhcpapi.DHCPAllocated{
		IP:       res.String(),
		Expiry:   kmetav1.Time{Time: expiry},
		Hostname: req.HostName(),
		Discover: req.MessageType() == dhcpv4.MessageTypeDiscover,
	}

	return res, nil
}

func cleanup(subnet *dhcpapi.DHCPSubnet) error {
	now := time.Now()

	for mac, allocated := range subnet.Status.Allocated {
		if !allocated.Expiry.Time.IsZero() && allocated.Expiry.Time.Before(now) {
			slog.Info("Removing entry for expired lease", "subnet", subnet.Name, "ip", allocated.IP, "mac", mac)
			delete(subnet.Status.Allocated, mac)
		}
	}

	return nil
}
