// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/boot/server"
)

const (
	defaultLeaseTime = 600 * time.Second
	// short lease for the first ACK in L3 mode, so the renewal right after it makes the leaf learn the client
	l3FirstLeaseTime = 10 * time.Second
)

// subnetSpec is a DHCPSubnet spec parsed and validated once per generation, with the reply options prebuilt
type subnetSpec struct {
	gen     int64
	err     error // the subnet is invalid and isn't served
	parsed  *dhcpapi.DHCPSubnetParsed
	statics map[string]netip.Addr // valid static IPs by MAC

	leaseTime           time.Duration
	l3                  bool
	disableDefaultRoute bool
	hasAdvertisedRoutes bool

	router  dhcpv4.Option
	mask    dhcpv4.Option
	dns     *dhcpv4.Option
	ntp     *dhcpv4.Option
	mtu     *dhcpv4.Option
	routes  *dhcpv4.Option // classless static routes, sent only if requested
	onie    dhcpv4.Option
	cumulus dhcpv4.Option

	pxeTFTPServer *dhcpv4.Option
	pxeHTTPURL    string // boot file for http, https and ftp
	pxeBootFile   *dhcpv4.Option
	pxeServerID   *dhcpv4.Option
}

func newSubnetSpec(obj *dhcpapi.DHCPSubnet) *subnetSpec {
	sp := &subnetSpec{gen: obj.Generation}

	parsed, err := obj.Spec.Parse()
	if err != nil {
		slog.Warn("Invalid DHCPSubnet, not serving it", "subnet", obj.Name, "generation", obj.Generation, "err", err)
		sp.err = fmt.Errorf("invalid subnet %s: %w", obj.Name, err)

		return sp
	}
	sp.parsed = parsed

	statics, errs := parsed.ParseStatics(obj.Spec.Static)
	for _, err := range errs {
		slog.Warn("Invalid static IP assignment, ignoring", "subnet", obj.Name, "generation", obj.Generation, "err", err)
	}
	sp.statics = statics

	spec := &obj.Spec
	sp.leaseTime = time.Duration(spec.LeaseTimeSeconds) * time.Second
	if sp.leaseTime == 0 {
		sp.leaseTime = defaultLeaseTime
	}
	sp.l3 = spec.L3Mode
	sp.disableDefaultRoute = spec.DisableDefaultRoute
	sp.hasAdvertisedRoutes = len(parsed.AdvertisedRoutes) > 0

	gateway := addrIP(parsed.Gateway)
	sp.router = dhcpv4.OptRouter(gateway)

	if sp.l3 {
		sp.mask = dhcpv4.OptSubnetMask(net.CIDRMask(32, 32))
	} else {
		sp.mask = dhcpv4.OptSubnetMask(net.CIDRMask(parsed.Prefix.Bits(), 32))
	}

	if len(parsed.DNSServers) > 0 {
		opt := dhcpv4.OptDNS(addrIPs(parsed.DNSServers)...)
		sp.dns = &opt
	}
	if len(parsed.TimeServers) > 0 {
		opt := dhcpv4.OptNTPServers(addrIPs(parsed.TimeServers)...)
		sp.ntp = &opt
	}
	if spec.InterfaceMTU > 0 {
		mtu := make([]byte, 2)
		binary.BigEndian.PutUint16(mtu, spec.InterfaceMTU)
		sp.mtu = &dhcpv4.Option{Code: dhcpv4.OptionInterfaceMTU, Value: dhcpv4.OptionGeneric{Data: mtu}}
	}

	// advertised routes, or the VPC subnet itself if the default route is disabled in L3 mode
	if sp.hasAdvertisedRoutes || (sp.disableDefaultRoute && sp.l3) {
		routes := dhcpv4.Routes{}
		if !sp.disableDefaultRoute {
			routes = append(routes, &dhcpv4.Route{Dest: prefixIPNet(netip.PrefixFrom(netip.IPv4Unspecified(), 0)), Router: gateway})
		} else if sp.l3 {
			routes = append(routes, &dhcpv4.Route{Dest: prefixIPNet(parsed.Prefix), Router: gateway})
		}
		for _, route := range parsed.AdvertisedRoutes {
			routes = append(routes, &dhcpv4.Route{Dest: prefixIPNet(route.Destination), Router: addrIP(route.Gateway)})
		}
		opt := dhcpv4.OptClasslessStaticRoute(routes...)
		sp.routes = &opt
	}

	sp.onie = dhcpv4.Option{Code: dhcpv4.OptionURL, Value: dhcpv4.OptionGeneric{Data: []byte(spec.ZTPBaseURL + nosinstall.OnieURLSuffix)}}
	sp.cumulus = dhcpv4.Option{Code: dhcpv4.GenericOptionCode(239), Value: dhcpv4.OptionGeneric{Data: []byte(spec.ZTPBaseURL + server.CumulusZTPURLSuffix)}}

	if u := parsed.PXEURL; u != nil {
		tftpServer := dhcpv4.OptTFTPServerName(u.Host)
		sp.pxeTFTPServer = &tftpServer

		switch u.Scheme {
		case "http", "https", "ftp":
			sp.pxeHTTPURL = u.String()
		default:
			bootFile := dhcpv4.OptBootFileName(strings.TrimPrefix(u.Path, "/"))
			sp.pxeBootFile = &bootFile
			if ip := net.ParseIP(u.Host); ip != nil {
				serverID := dhcpv4.OptServerIdentifier(ip)
				sp.pxeServerID = &serverID
			}
		}
	}

	return sp
}

func addrIP(addr netip.Addr) net.IP {
	return net.IP(addr.AsSlice())
}

func addrIPs(addrs []netip.Addr) []net.IP {
	res := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		res = append(res, addrIP(addr))
	}

	return res
}

func prefixIPNet(prefix netip.Prefix) *net.IPNet {
	return &net.IPNet{IP: addrIP(prefix.Addr()), Mask: net.CIDRMask(prefix.Bits(), 32)}
}
