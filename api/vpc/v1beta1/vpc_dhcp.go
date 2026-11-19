// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"fmt"
	"strings"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

const (
	DefaultDHCPMTU              = 9036
	DefaultDHCPLeaseTimeSeconds = 3600
)

// DHCPSubnetSpec returns the spec of the DHCPSubnet serving the VPC subnet with DHCP enabled
func (vpc *VPC) DHCPSubnetSpec(subnetName string, subnet *VPCSubnet) dhcpapi.DHCPSubnetSpec {
	pxeURL := ""
	dnsServers := []string{}
	timeServers := []string{}
	mtu := uint16(DefaultDHCPMTU)
	leaseTime := uint32(DefaultDHCPLeaseTimeSeconds)
	advertisedRoutes := []dhcpapi.DHCPRoute{}
	disableDefaultRoute := false

	if subnet.DHCP.Options != nil {
		pxeURL = subnet.DHCP.Options.PXEURL
		dnsServers = subnet.DHCP.Options.DNSServers
		timeServers = subnet.DHCP.Options.TimeServers
		for _, route := range subnet.DHCP.Options.AdvertisedRoutes {
			advertisedRoutes = append(advertisedRoutes, dhcpapi.DHCPRoute{
				Destination: route.Destination,
				Gateway:     route.Gateway,
			})
		}
		disableDefaultRoute = subnet.DHCP.Options.DisableDefaultRoute

		if subnet.DHCP.Options.InterfaceMTU > 0 {
			mtu = subnet.DHCP.Options.InterfaceMTU
		}

		if subnet.DHCP.Options.LeaseTimeSeconds > 0 {
			leaseTime = subnet.DHCP.Options.LeaseTimeSeconds
		}
	}

	startIP, endIP := "", ""
	if subnet.DHCP.Range != nil {
		startIP, endIP = subnet.DHCP.Range.Start, subnet.DHCP.Range.End
	}

	vrf := ""
	switch vpc.Spec.Mode {
	case VPCModeL2VNI, VPCModeL3VNI:
		vrf = strings.ToLower(vpc.Name)
	case VPCModeL3Flat:
		vrf = "default" // default VRF
	}

	statics := map[string]dhcpapi.DHCPSubnetStatic{}
	for mac, static := range subnet.DHCP.Static {
		statics[mac] = dhcpapi.DHCPSubnetStatic{
			IP: static.IP,
		}
	}

	return dhcpapi.DHCPSubnetSpec{
		Subnet:              fmt.Sprintf("%s/%s", vpc.Name, subnetName),
		CIDRBlock:           subnet.Subnet,
		Gateway:             subnet.Gateway,
		StartIP:             startIP,
		EndIP:               endIP,
		LeaseTimeSeconds:    leaseTime,
		VRF:                 vrf,
		CircuitID:           fmt.Sprintf("vlan%d", subnet.VLAN), // TODO move to utils
		PXEURL:              pxeURL,
		DNSServers:          dnsServers,
		TimeServers:         timeServers,
		InterfaceMTU:        mtu,
		L3Mode:              vpc.Spec.Mode == VPCModeL3Flat || vpc.Spec.Mode == VPCModeL3VNI,
		DisableDefaultRoute: disableDefaultRoute,
		AdvertisedRoutes:    advertisedRoutes,
		Static:              statics,
	}
}
