// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.githedgehog.com/fabric/pkg/util/netlinkutil"
)

// serverIdentifier returns the DHCP server identifier (option 54) for a request relayed via giaddr: the control VIP if
// configured, otherwise the source address of the route to the relay (the control node management IP)
func (s *Server) serverIdentifier(giaddr net.IP) (net.IP, error) {
	if s.controlVIP != nil {
		return s.controlVIP, nil
	}

	routes, err := netlinkutil.RouteGet(giaddr)
	if err != nil {
		return nil, fmt.Errorf("getting route to %s: %w", giaddr, err)
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no route found to %s", giaddr) //nolint:err113
	}

	return routes[0].Src, nil
}

// nakResponse turns resp into a DHCPNAK (RFC 2131 4.3.2), the client then restarts with a DISCOVER
func nakResponse(req, resp *dhcpv4.DHCPv4, serverID net.IP) {
	resp.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeNak))
	resp.UpdateOption(dhcpv4.OptServerIdentifier(serverID))
	resp.UpdateOption(dhcpv4.OptMessage("requested address not available"))
	resp.YourIPAddr = net.IPv4zero
	resp.ClientIPAddr = net.IPv4zero

	// the relay has to broadcast it as the client may not have the address anymore (RFC 2131 4.1)
	if !req.GatewayIPAddr.IsUnspecified() {
		resp.SetBroadcast()
	}
}

func (sp *subnetSpec) respond(req, resp *dhcpv4.DHCPv4, ip netip.Addr, serverID net.IP) {
	// With l3vni VPCs, send a short lease time on the first request to trigger
	// a renewal, which will carry the IP address and MAC of the server and make the
	// leaf learn it. On renewals (and in other VPC modes), send the configured lease time.
	leaseTime := sp.leaseTime
	if sp.l3 && req.ClientIPAddr.IsUnspecified() {
		leaseTime = l3FirstLeaseTime
	}

	resp.YourIPAddr = addrIP(ip)
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))

	wantsRoutes := req.IsOptionRequested(dhcpv4.OptionClasslessStaticRoute)
	// From RFC3442: When a DHCP client requests the Classless Static Routes option and
	// also requests either or both of the Router option and the Static
	// Routes option, and the DHCP server is sending Classless Static Routes
	// options to that client, the server SHOULD NOT include the Router or
	// Static Routes options.
	if !sp.disableDefaultRoute && (!wantsRoutes || !sp.hasAdvertisedRoutes) {
		resp.Options.Update(sp.router)
	}
	resp.Options.Update(dhcpv4.OptServerIdentifier(serverID))
	resp.Options.Update(sp.mask)

	for _, opt := range []*dhcpv4.Option{sp.dns, sp.ntp, sp.mtu} {
		if opt != nil {
			resp.Options.Update(*opt)
		}
	}

	classID := req.ClassIdentifier()
	switch {
	case strings.HasPrefix(classID, onieClassIDPrefix):
		resp.Options.Update(sp.onie)
	case classID == cumulusClassID:
		resp.Options.Update(sp.cumulus)
	}

	if wantsRoutes && sp.routes != nil {
		resp.Options.Update(*sp.routes)
	}

	sp.addPXE(req, resp)
}

func (sp *subnetSpec) addPXE(req, resp *dhcpv4.DHCPv4) {
	if req.RelayAgentInfo() == nil || sp.pxe == nil {
		return
	}

	if req.IsOptionRequested(dhcpv4.OptionTFTPServerName) {
		resp.Options.Update(sp.pxe.serverName)
	}
	if req.IsOptionRequested(dhcpv4.OptionBootfileName) {
		if sp.pxe.httpURL != "" {
			resp.BootFileName = sp.pxe.httpURL
			resp.Options.Update(dhcpv4.OptClassIdentifier(string(req.Options.Get(dhcpv4.OptionClassIdentifier))))
		} else {
			resp.Options.Update(*sp.pxe.bootFile)
			// clients take the TFTP server from siaddr, option 54 stays the DHCP server identifier
			if sp.pxe.nextServer != nil {
				resp.ServerIPAddr = sp.pxe.nextServer
			}
		}
	}
}
