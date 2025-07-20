// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package dhcpd

import (
	"encoding/binary"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/netlinkutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	onieClassIdentifier = "onie_vendor:"
)

func handleDiscover4(req, resp *dhcpv4.DHCPv4) error {
	subnet, err := getSubnetInfo(req)
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
	}

	subnet.Lock()
	defer func() {
		subnet.Unlock()
	}()

	if reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]; ok {
		// We have a reservation for this populate the response and send back
		if err := updateResponse(req, resp, subnet, reservation.address); err != nil {
			return errors.Wrapf(err, "handleDiscover4: failed to update response")
		}

		return nil
	}

	// This is not  a know reservation
	ipnet, err := subnet.pool.Allocate()
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to allocate ip")
	}
	leaseTime := time.Duration(subnet.dhcpSubnet.Spec.LeaseTimeSeconds) * time.Second

	subnet.allocations.allocation[req.ClientHWAddr.String()] = &ipreservation{
		address:    ipnet,
		macAddress: req.ClientHWAddr.String(),
		expiry:     time.Now().Add(leaseTime),
		state:      pending,
		hostname:   req.HostName(),
	}

	if err := updateResponse(req, resp, subnet, ipnet); err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to update response")
	}

	time.AfterFunc(pendingDiscoverTimeout, func() {
		subnet.Lock()
		defer subnet.Unlock()
		if reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]; ok {
			if reservation.state == committed {
				// The IP already committed. We saw request that follows and allocated
				return
			}
			// We did not see the request that follows. We need to release the IP
			delete(subnet.allocations.allocation, req.ClientHWAddr.String())
			if err := subnet.pool.Free(reservation.address); err != nil {
				slog.Warn("Failed to free reservation", "mac", req.ClientHWAddr.String(), "ip", reservation.address.String(), "error", err)
			}
		}
	})

	return nil
}

func handleRequest4(req, resp *dhcpv4.DHCPv4) error {
	subnet, err := getSubnetInfo(req)
	if err != nil {
		return errors.Wrapf(err, "handleRequest4: failed to get subnet info")
	}

	subnet.Lock()
	defer func() {
		subnet.Unlock()
	}()

	leaseTime := time.Duration(subnet.dhcpSubnet.Spec.LeaseTimeSeconds) * time.Second

	if reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]; ok {
		reservation.state = committed
		reservation.expiry = time.Now().Add(leaseTime)

		subnet.dhcpSubnet.Status.Allocated[req.ClientHWAddr.String()] = dhcpapi.DHCPAllocated{
			IP:       reservation.address.IP.String(),
			Expiry:   kmetav1.NewTime(reservation.expiry),
			Hostname: reservation.hostname,
		}

		if err := updateBackend4(subnet.dhcpSubnet); err != nil {
			slog.Warn("Update Backend failed for record", "mac", req.ClientHWAddr.String(), "ip", reservation.address.IP.String(), "error", err)
		}

		if err := updateResponse(req, resp, subnet, reservation.address); err != nil {
			return errors.Wrapf(err, "handleRequest4: failed to update response")
		}

		return nil
	}

	ipnet, err := subnet.pool.Allocate()
	if err != nil {
		return errors.Wrapf(err, "handleRequest4: failed to allocate ip")
	}

	subnet.allocations.allocation[req.ClientHWAddr.String()] = &ipreservation{
		address:    ipnet,
		macAddress: req.ClientHWAddr.String(),
		expiry:     time.Now().Add(leaseTime),
		state:      committed,
		hostname:   req.HostName(),
	}

	subnet.dhcpSubnet.Status.Allocated[req.ClientHWAddr.String()] = dhcpapi.DHCPAllocated{
		IP:       ipnet.IP.String(),
		Expiry:   kmetav1.NewTime(time.Now().Add(leaseTime)),
		Hostname: req.HostName(),
	}

	if err := updateBackend4(subnet.dhcpSubnet); err != nil {
		slog.Warn("Update Backend failed for record", "mac", req.ClientHWAddr.String(), "ip", ipnet.String(), "error", err)
	}

	if err := updateResponse(req, resp, subnet, ipnet); err != nil {
		return errors.Wrapf(err, "handleRequest4: failed to update response")
	}

	return nil
}

func handleDecline4(req, _ /* resp */ *dhcpv4.DHCPv4) error {
	subnet, err := getSubnetInfo(req)
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
	}

	subnet.Lock()
	defer subnet.Unlock()

	reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]
	if !ok {
		slog.Debug("No reservation found for mac", "mac", req.ClientHWAddr.String(), "ip", req.ClientIPAddr.String())
	}

	delete(subnet.allocations.allocation, req.ClientHWAddr.String())
	if err := subnet.pool.Free(reservation.address); err != nil {
		slog.Error("IP address could not be released", "ip", reservation.address.String(), "error", err)
	}
	delete(subnet.dhcpSubnet.Status.Allocated, req.ClientHWAddr.String())
	if err := updateBackend4(subnet.dhcpSubnet); err != nil {
		slog.Warn("Update Backend failed for record", "mac", req.ClientHWAddr.String(), "ip", reservation.address.IP.String(), "error", err)
	}

	return nil
}

func handleRelease4(req, _ /* resp */ *dhcpv4.DHCPv4) error {
	subnet, err := getSubnetInfo(req)
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
	}

	subnet.Lock()
	defer subnet.Unlock()

	reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]
	if !ok {
		slog.Debug("No reservation found for mac", "mac", req.ClientHWAddr.String(), "ip", req.ClientIPAddr.String())
	}

	delete(subnet.allocations.allocation, req.ClientHWAddr.String())
	if err := subnet.pool.Free(reservation.address); err != nil {
		slog.Error("IP address could not be released", "ip", reservation.address.String(), "error", err)
	}
	delete(subnet.dhcpSubnet.Status.Allocated, req.ClientHWAddr.String())
	if err := updateBackend4(subnet.dhcpSubnet); err != nil {
		slog.Warn("Update Backend failed for record", "mac", req.ClientHWAddr.String(), "ip", reservation.address.IP.String(), "error", err)
	}

	return nil
}

func getSubnetInfo(req *dhcpv4.DHCPv4) (*ManagedSubnet, error) {
	circuitID, vrfName := "", ""
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID = string(relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption))
		vrfName = string(relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption))
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
		if vrfName == "" {
			vrfName = "default"
		}
	}

	pluginHdl.dhcpSubnets.Lock()
	defer pluginHdl.dhcpSubnets.Unlock()

	if circuitID != "" && vrfName != "" {
		subnet, ok := pluginHdl.dhcpSubnets.subnets[vrfName+circuitID]
		if !ok {
			return nil, errors.Errorf("No subnet found for vrf %s and circuitID %s", vrfName, circuitID)
		}

		return subnet, nil
	} else if strings.HasPrefix(req.ClassIdentifier(), onieClassIdentifier) {
		subnet, ok := pluginHdl.dhcpSubnets.subnets[dhcpapi.ManagementSubnet]
		if !ok {
			return nil, errors.Errorf("management subnet is missing")
		}

		return subnet, nil
	}

	return nil, errors.Errorf("No subnet found for request from %s", req.ClientHWAddr.String())
}

func handleExpiredLeases() {
	// wake up every 2 min and try looking for expired leases
	// This is a long loop we migh want to break this so we don't spend too much time here
	ticker := time.NewTicker(120 * time.Second)
	for range ticker.C {
		if pluginHdl.dhcpSubnets == nil {
			continue
		}
		pluginHdl.dhcpSubnets.Lock()
		for _, v := range pluginHdl.dhcpSubnets.subnets {
			for hwmacaddress, reservation := range v.allocations.allocation {
				if time.Now().After(reservation.expiry) {
					// lease expired
					delete(v.allocations.allocation, hwmacaddress)
					delete(v.dhcpSubnet.Status.Allocated, hwmacaddress)
				}
				if err := updateBackend4(v.dhcpSubnet); err != nil {
					slog.Warn("Update Backend failed for record", "mac", hwmacaddress, "ip", reservation.address.String(), "error", err)
				}
			}
		}
		pluginHdl.dhcpSubnets.Unlock()
	}
}

func updateResponse(req, resp *dhcpv4.DHCPv4, subnet *ManagedSubnet, ipnet net.IPNet) error {
	routes, err := netlinkutil.RouteGet(req.GatewayIPAddr)
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get route")
	}
	if len(routes) == 0 {
		return errors.New("handleDiscover4: no route found")
	}

	// With l3vni VPCs, send a short lease time on the first request to trigger
	// a renewal, which will carry the IP address and MAC of the server and make the
	// leaf learn it. On renewals (and in other VPC modes), send the configured lease time.
	leaseTime := time.Duration(subnet.dhcpSubnet.Spec.LeaseTimeSeconds) * time.Second
	if subnet.dhcpSubnet.Spec.L3Mode && req.ClientIPAddr.IsUnspecified() {
		leaseTime = 10 * time.Second
	}

	resp.YourIPAddr = ipnet.IP
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
	// From RFC3442: When a DHCP client requests the Classless Static Routes option and
	// also requests either or both of the Router option and the Static
	// Routes option, and the DHCP server is sending Classless Static Routes
	// options to that client, the server SHOULD NOT include the Router or
	// Static Routes options.
	if !subnet.dhcpSubnet.Spec.DisableDefaultRoute && (!req.IsOptionRequested(dhcpv4.OptionClasslessStaticRoute) ||
		len(subnet.dhcpSubnet.Spec.AdvertisedRoutes) == 0) {
		resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))
	}
	resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))

	switch {
	case subnet.dhcpSubnet.Spec.L3Mode:
		resp.Options.Update(dhcpv4.OptSubnetMask(net.IPv4Mask(255, 255, 255, 255)))
	default:
		resp.Options.Update(dhcpv4.OptSubnetMask(ipnet.Mask))
	}

	if len(subnet.dhcpSubnet.Spec.DNSServers) > 0 {
		ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.DNSServers))
		for index, dnsServer := range subnet.dhcpSubnet.Spec.DNSServers {
			ips[index] = net.ParseIP(dnsServer)
		}
		resp.Options.Update(dhcpv4.OptDNS(ips...))
	}

	if len(subnet.dhcpSubnet.Spec.TimeServers) > 0 {
		ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.TimeServers))
		for index, timeServer := range subnet.dhcpSubnet.Spec.TimeServers {
			ips[index] = net.ParseIP(timeServer)
		}
		resp.Options.Update(dhcpv4.OptNTPServers(ips...))
	}

	if subnet.dhcpSubnet.Spec.InterfaceMTU > 0 {
		mtu := make([]byte, 2)
		binary.BigEndian.PutUint16(mtu, subnet.dhcpSubnet.Spec.InterfaceMTU)
		resp.Options.Update(dhcpv4.Option{
			Code: dhcpv4.OptionInterfaceMTU,
			Value: dhcpv4.OptionGeneric{
				Data: mtu,
			},
		})
	}

	if subnet.dhcpSubnet.Spec.DefaultURL != "" {
		resp.Options.Update(dhcpv4.Option{
			Code: dhcpv4.OptionURL,
			Value: dhcpv4.OptionGeneric{
				Data: []byte(subnet.dhcpSubnet.Spec.DefaultURL),
			},
		})
	}

	// we want to advertise classless static routes:
	// - if the dhcp client requested them (prerequisite) AND
	// - EITHER the user has configured some routes to advertise
	// - OR the user disabled the default route for an L3VNI mode VPC, as we need to advertise at least the VPC subnet
	if req.IsOptionRequested(dhcpv4.OptionClasslessStaticRoute) && (len(subnet.dhcpSubnet.Spec.AdvertisedRoutes) > 0 ||
		(subnet.dhcpSubnet.Spec.DisableDefaultRoute && subnet.dhcpSubnet.Spec.L3Mode)) {
		routes := dhcpv4.Routes{}
		// advertise a default route here unless it was disabled
		if !subnet.dhcpSubnet.Spec.DisableDefaultRoute {
			routes = append(routes, &dhcpv4.Route{
				Dest: &net.IPNet{
					IP:   net.IPv4zero,
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
				Router: net.ParseIP(subnet.dhcpSubnet.Spec.Gateway),
			})
		} else if subnet.dhcpSubnet.Spec.L3Mode {
			// in L3 mode, we need to advertise the VPC subnet as a classless static route
			_, prefix, err := net.ParseCIDR(subnet.dhcpSubnet.Spec.CIDRBlock)
			if err != nil {
				return errors.Wrapf(err, "handleDiscover4: failed to parse VPC subnet CIDR block %s", subnet.dhcpSubnet.Spec.CIDRBlock)
			}
			routes = append(routes, &dhcpv4.Route{
				Dest:   prefix,
				Router: net.ParseIP(subnet.dhcpSubnet.Spec.Gateway),
			})
		}
		for _, advertisedRoute := range subnet.dhcpSubnet.Spec.AdvertisedRoutes {
			_, prefix, err := net.ParseCIDR(advertisedRoute.Destination)
			if err != nil {
				return errors.Wrapf(err, "handleDiscover4: failed to parse advertised route prefix %s", advertisedRoute.Destination)
			}
			gateway := net.ParseIP(advertisedRoute.Gateway)
			if gateway == nil {
				return errors.Errorf("handleDiscover4: failed to parse advertised route gateway %s", advertisedRoute.Gateway)
			}
			routes = append(routes, &dhcpv4.Route{
				Dest:   prefix,
				Router: gateway,
			})
		}
		resp.Options.Update(dhcpv4.OptClasslessStaticRoute(routes...))
	}

	addPxeInfo(req, resp, subnet)

	return nil
}

func addPxeInfo(req, resp *dhcpv4.DHCPv4, subnet *ManagedSubnet) {
	relayAgentInfo := req.RelayAgentInfo()
	if relayAgentInfo == nil || subnet.dhcpSubnet == nil {
		return
	}
	circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
	vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)

	// Add TFTP server Option Name
	if len(subnet.dhcpSubnet.Spec.PXEURL) == 0 &&
		(req.IsOptionRequested(dhcpv4.OptionTFTPServerName) || req.IsOptionRequested(dhcpv4.OptionBootfileName)) { // PxeURL is not specified return early with an error message
		slog.Error("Client Requested pxe but it is not configured", "circuitID", circuitID, "vrfName", vrfName, "macAddress", req.ClientHWAddr.String())

		return
	}
	u, err := url.Parse(subnet.dhcpSubnet.Spec.PXEURL)
	if err != nil {
		slog.Error("Invalid Pxe URL", "url", subnet.dhcpSubnet.Spec.PXEURL, "error", err)

		return
	}
	if req.IsOptionRequested(dhcpv4.OptionTFTPServerName) {
		resp.Options.Update(dhcpv4.OptTFTPServerName(u.Host))
	}
	if req.IsOptionRequested(dhcpv4.OptionBootfileName) {
		switch u.Scheme {
		case "http", "https", "ftp":
			vendorClassIdentifer := req.Options.Get(dhcpv4.OptionClassIdentifier)
			resp.BootFileName = u.String()
			resp.Options.Update(dhcpv4.OptClassIdentifier(string(vendorClassIdentifer)))
		default:
			resp.Options.Update(dhcpv4.OptBootFileName(strings.TrimPrefix(u.Path, "/")))
			resp.Options.Update(dhcpv4.OptServerIdentifier(net.ParseIP(u.Host)))
		}
	}
}
