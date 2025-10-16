// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/netlinkutil"
)

func updateResponse(req, resp *dhcpv4.DHCPv4, subnet *dhcpapi.DHCPSubnet, ipnet *net.IPNet) error {
	routes, err := netlinkutil.RouteGet(req.GatewayIPAddr)
	if err != nil {
		return fmt.Errorf("getting route: %w", err)
	}
	if len(routes) == 0 {
		return fmt.Errorf("no route found") //nolint:err113
	}

	// With l3vni VPCs, send a short lease time on the first request to trigger
	// a renewal, which will carry the IP address and MAC of the server and make the
	// leaf learn it. On renewals (and in other VPC modes), send the configured lease time.
	leaseTime := time.Duration(subnet.Spec.LeaseTimeSeconds) * time.Second
	if subnet.Spec.L3Mode && req.ClientIPAddr.IsUnspecified() {
		leaseTime = 10 * time.Second
	}

	resp.YourIPAddr = ipnet.IP
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
	// From RFC3442: When a DHCP client requests the Classless Static Routes option and
	// also requests either or both of the Router option and the Static
	// Routes option, and the DHCP server is sending Classless Static Routes
	// options to that client, the server SHOULD NOT include the Router or
	// Static Routes options.
	if !subnet.Spec.DisableDefaultRoute && (!req.IsOptionRequested(dhcpv4.OptionClasslessStaticRoute) ||
		len(subnet.Spec.AdvertisedRoutes) == 0) {
		resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.Spec.Gateway)))
	}
	resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))

	switch {
	case subnet.Spec.L3Mode:
		resp.Options.Update(dhcpv4.OptSubnetMask(net.IPv4Mask(255, 255, 255, 255)))
	default:
		resp.Options.Update(dhcpv4.OptSubnetMask(ipnet.Mask))
	}

	if len(subnet.Spec.DNSServers) > 0 {
		ips := make([]net.IP, len(subnet.Spec.DNSServers))
		for index, dnsServer := range subnet.Spec.DNSServers {
			ips[index] = net.ParseIP(dnsServer)
		}
		resp.Options.Update(dhcpv4.OptDNS(ips...))
	}

	if len(subnet.Spec.TimeServers) > 0 {
		ips := make([]net.IP, len(subnet.Spec.TimeServers))
		for index, timeServer := range subnet.Spec.TimeServers {
			ips[index] = net.ParseIP(timeServer)
		}
		resp.Options.Update(dhcpv4.OptNTPServers(ips...))
	}

	if subnet.Spec.InterfaceMTU > 0 {
		mtu := make([]byte, 2)
		binary.BigEndian.PutUint16(mtu, subnet.Spec.InterfaceMTU)
		resp.Options.Update(dhcpv4.Option{
			Code: dhcpv4.OptionInterfaceMTU,
			Value: dhcpv4.OptionGeneric{
				Data: mtu,
			},
		})
	}

	if subnet.Spec.DefaultURL != "" {
		resp.Options.Update(dhcpv4.Option{
			Code: dhcpv4.OptionURL,
			Value: dhcpv4.OptionGeneric{
				Data: []byte(subnet.Spec.DefaultURL),
			},
		})
	}

	// we want to advertise classless static routes:
	// - if the dhcp client requested them (prerequisite) AND
	// - EITHER the user has configured some routes to advertise
	// - OR the user disabled the default route for an L3VNI mode VPC, as we need to advertise at least the VPC subnet
	if req.IsOptionRequested(dhcpv4.OptionClasslessStaticRoute) && (len(subnet.Spec.AdvertisedRoutes) > 0 ||
		(subnet.Spec.DisableDefaultRoute && subnet.Spec.L3Mode)) {
		routes := dhcpv4.Routes{}
		// advertise a default route here unless it was disabled
		if !subnet.Spec.DisableDefaultRoute {
			routes = append(routes, &dhcpv4.Route{
				Dest: &net.IPNet{
					IP:   net.IPv4zero,
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
				Router: net.ParseIP(subnet.Spec.Gateway),
			})
		} else if subnet.Spec.L3Mode {
			// in L3 mode, we need to advertise the VPC subnet as a classless static route
			_, prefix, err := net.ParseCIDR(subnet.Spec.CIDRBlock)
			if err != nil {
				return fmt.Errorf("parsing VPC subnet %s CIDR %s", subnet.Spec.Subnet, subnet.Spec.CIDRBlock) //nolint:err113
			}
			routes = append(routes, &dhcpv4.Route{
				Dest:   prefix,
				Router: net.ParseIP(subnet.Spec.Gateway),
			})
		}
		for _, advertisedRoute := range subnet.Spec.AdvertisedRoutes {
			_, prefix, err := net.ParseCIDR(advertisedRoute.Destination)
			if err != nil {
				return fmt.Errorf("parsing advertised route prefix %s: %w", advertisedRoute.Destination, err)
			}
			gateway := net.ParseIP(advertisedRoute.Gateway)
			if gateway == nil {
				return fmt.Errorf("parsing advertised route gateway %s", advertisedRoute.Gateway) //nolint:err113
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

func addPxeInfo(req, resp *dhcpv4.DHCPv4, subnet *dhcpapi.DHCPSubnet) {
	relayAgentInfo := req.RelayAgentInfo()
	if relayAgentInfo == nil || subnet == nil {
		return
	}
	circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
	vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)

	// Add TFTP server Option Name
	if len(subnet.Spec.PXEURL) == 0 &&
		(req.IsOptionRequested(dhcpv4.OptionTFTPServerName) || req.IsOptionRequested(dhcpv4.OptionBootfileName)) { // PxeURL is not specified return early with an error message
		slog.Error("Client Requested pxe but it is not configured", "circuitID", circuitID, "vrfName", vrfName, "macAddress", req.ClientHWAddr.String())

		return
	}
	u, err := url.Parse(subnet.Spec.PXEURL)
	if err != nil {
		slog.Error("Invalid Pxe URL", "url", subnet.Spec.PXEURL, "err", err)

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
