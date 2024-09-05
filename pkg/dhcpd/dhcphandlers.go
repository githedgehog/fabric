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
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/netlinkutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	vrfOob              = "oobVrf"
	circuitIDoob        = "oobCircuitID"
	onieClassIdentifier = "onie_vendor:"
)

func handleDiscover4(req, resp *dhcpv4.DHCPv4) error {
	var circuitID []byte
	var vrfName []byte
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID = relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName = relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
	} else {
		vrfName = []byte(vrfOob)
		circuitID = []byte(circuitIDoob)
	}
	// Get subnet for this vrf and circuitID
	subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
	}
	routes, err := netlinkutil.RouteGet(req.GatewayIPAddr)
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get route")
	}

	subnet.Lock()
	defer func() {
		addPxeInfo(req, resp, subnet)
		subnet.Unlock()
	}()
	if reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]; ok {
		// We have a reservation for this populate the response and send back
		resp.YourIPAddr = reservation.address.IP
		resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
		resp.Options.Update(dhcpv4.OptSubnetMask(reservation.address.Mask))

		resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))

		resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))

		if len(subnet.dhcpSubnet.Spec.DNSServers) > 0 {
			ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.DNSServers))
			for index, dnsserver := range subnet.dhcpSubnet.Spec.DNSServers {
				ips[index] = net.ParseIP(dnsserver)
			}
			resp.Options.Update(dhcpv4.OptDNS(ips...))
		}

		if len(subnet.dhcpSubnet.Spec.TimeServers) > 0 {
			ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.TimeServers))
			for index, timeserver := range subnet.dhcpSubnet.Spec.TimeServers {
				ips[index] = net.ParseIP(timeserver)
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
		if strings.HasPrefix(req.ClassIdentifier(), onieClassIdentifier) {
			if len(subnet.dhcpSubnet.Spec.DefaultOnieURL) == 0 {
				log.Errorf("onie subnet %s configured without default URL", subnet.dhcpSubnet.Spec.CIDRBlock)
			} else {
				resp.Options.Update(dhcpv4.Option{
					Code: dhcpv4.OptionURL,
					Value: dhcpv4.OptionGeneric{
						Data: []byte(subnet.dhcpSubnet.Spec.DefaultOnieURL),
					},
				})
			}
		}
		return nil
	}
	// This is not  a know reservation
	ipnet, err := subnet.pool.Allocate()
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to allocate ip")
	}
	resp.YourIPAddr = ipnet.IP
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
	resp.Options.Update(dhcpv4.OptSubnetMask(ipnet.Mask))
	resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))
	resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))
	subnet.allocations.allocation[req.ClientHWAddr.String()] = &ipreservation{
		address:    ipnet,
		macAddress: req.ClientHWAddr.String(),
		expiry:     time.Now().Add(leaseTime),
		state:      pending,
		hostname:   req.HostName(),
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

	if strings.HasPrefix(req.ClassIdentifier(), onieClassIdentifier) {
		if len(subnet.dhcpSubnet.Spec.DefaultOnieURL) == 0 {
			log.Errorf("onie subnet configured without default URL", subnet.dhcpSubnet.Spec.CIDRBlock)
		} else {
			resp.Options.Update(dhcpv4.Option{
				Code: dhcpv4.OptionURL,
				Value: dhcpv4.OptionGeneric{
					Data: []byte(subnet.dhcpSubnet.Spec.DefaultOnieURL),
				},
			})
		}
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
				log.Errorf("Failed to free reservation %s:%s with error: %v", req.ClientHWAddr.String(), reservation.address.String(), err)
			}
		}
	})

	return nil
}

func handleRequest4(req, resp *dhcpv4.DHCPv4) error {
	var vrfName []byte
	var circuitID []byte
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID = relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName = relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
	} else {
		vrfName = []byte(vrfOob)
		circuitID = []byte(circuitIDoob)
	}
	routes, err := netlinkutil.RouteGet(req.GatewayIPAddr)
	if err != nil {
		log.Errorf("Error getting route %v", err)
	}
	subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
	if err != nil {
		return errors.Wrapf(err, "handleRequest4: failed to get subnet info")
	}
	subnet.Lock()
	defer func() {
		addPxeInfo(req, resp, subnet)
		subnet.Unlock()
	}()
	reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]
	if ok {
		reservation.state = committed
		reservation.expiry = time.Now().Add(leaseTime)
		resp.YourIPAddr = reservation.address.IP
		resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
		resp.Options.Update(dhcpv4.OptSubnetMask(reservation.address.Mask))
		resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))
		resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))
		subnet.dhcpSubnet.Status.Allocated[req.ClientHWAddr.String()] = v1alpha2.DHCPAllocated{
			IP:       reservation.address.IP.String(),
			Expiry:   metav1.NewTime(reservation.expiry),
			Hostname: reservation.hostname,
		}
		if err := updateBackend4(subnet.dhcpSubnet); err != nil {
			log.Warnf("Update Backend failed for record with Mac Address: %s IP %s", req.ClientHWAddr.String(), reservation.address.IP.String())
		}

		if len(subnet.dhcpSubnet.Spec.DNSServers) > 0 {
			ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.DNSServers))
			for index, dnsserver := range subnet.dhcpSubnet.Spec.DNSServers {
				ips[index] = net.ParseIP(dnsserver)
			}
			resp.Options.Update(dhcpv4.OptDNS(ips...))
		}

		if len(subnet.dhcpSubnet.Spec.TimeServers) > 0 {
			ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.TimeServers))
			for index, timeserver := range subnet.dhcpSubnet.Spec.TimeServers {
				ips[index] = net.ParseIP(timeserver)
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
		if strings.HasPrefix(req.ClassIdentifier(), onieClassIdentifier) {
			if len(subnet.dhcpSubnet.Spec.DefaultOnieURL) == 0 {
				log.Errorf("onie subnet configured without default URL", subnet.dhcpSubnet.Spec.CIDRBlock)
			} else {
				resp.Options.Update(dhcpv4.Option{
					Code: dhcpv4.OptionURL,
					Value: dhcpv4.OptionGeneric{
						Data: []byte(subnet.dhcpSubnet.Spec.DefaultOnieURL),
					},
				})
			}
		}

		return nil
	}
	ipnet, err := subnet.pool.Allocate()
	if err != nil {
		return errors.Wrapf(err, "handleRequest4: failed to allocate ip")
	}
	resp.YourIPAddr = ipnet.IP
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
	resp.Options.Update(dhcpv4.OptSubnetMask(ipnet.Mask))
	resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))
	resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))
	subnet.allocations.allocation[req.ClientHWAddr.String()] = &ipreservation{
		address:    ipnet,
		macAddress: req.ClientHWAddr.String(),
		expiry:     time.Now().Add(leaseTime),
		state:      committed,
		hostname:   req.HostName(),
	}
	subnet.dhcpSubnet.Status.Allocated[req.ClientHWAddr.String()] = v1alpha2.DHCPAllocated{
		IP:       ipnet.IP.String(),
		Expiry:   metav1.NewTime(time.Now().Add(leaseTime)),
		Hostname: req.HostName(),
	}
	if len(subnet.dhcpSubnet.Spec.DNSServers) > 0 {
		ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.DNSServers))
		for index, dnsserver := range subnet.dhcpSubnet.Spec.DNSServers {
			ips[index] = net.ParseIP(dnsserver)
		}
		resp.Options.Update(dhcpv4.OptDNS(ips...))
	}

	if len(subnet.dhcpSubnet.Spec.TimeServers) > 0 {
		ips := make([]net.IP, len(subnet.dhcpSubnet.Spec.TimeServers))
		for index, timeserver := range subnet.dhcpSubnet.Spec.TimeServers {
			ips[index] = net.ParseIP(timeserver)
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
	if strings.HasPrefix(req.ClassIdentifier(), onieClassIdentifier) {
		if len(subnet.dhcpSubnet.Spec.DefaultOnieURL) == 0 {
			log.Errorf("onie subnet configured without default URL", subnet.dhcpSubnet.Spec.CIDRBlock)
		} else {
			resp.Options.Update(dhcpv4.Option{
				Code: dhcpv4.OptionURL,
				Value: dhcpv4.OptionGeneric{
					Data: []byte(subnet.dhcpSubnet.Spec.DefaultOnieURL),
				},
			})
		}
	}
	if err := updateBackend4(subnet.dhcpSubnet); err != nil {
		log.Warnf("Update Backend failed for record with Mac Address: %s IP %s", req.ClientHWAddr.String(), ipnet.String())
	}

	return nil
}

func handleDecline4(req, _ /* resp */ *dhcpv4.DHCPv4) error {
	var vrfName []byte
	var circuitID []byte
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID = relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName = relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
	} else {
		vrfName = []byte(vrfOob)
		circuitID = []byte(circuitIDoob)
	}
	subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
	}
	subnet.Lock()
	defer subnet.Unlock()
	reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]
	if !ok {
		log.Debugf("No reservation found for mac %s ip %s", req.ClientHWAddr.String(), req.ClientIPAddr.String())
	}
	delete(subnet.allocations.allocation, req.ClientHWAddr.String())
	if err := subnet.pool.Free(reservation.address); err != nil {
		log.Errorf("IP address %s could not be released", reservation.address.String())
	}
	delete(subnet.dhcpSubnet.Status.Allocated, req.ClientHWAddr.String())
	if err := updateBackend4(subnet.dhcpSubnet); err != nil {
		log.Warnf("Update Backend failed for record with Mac Address: %s IP %s", req.ClientHWAddr.String(), reservation.address.IP.String())
	}

	return nil
}

func handleRelease4(req, _ /* resp */ *dhcpv4.DHCPv4) error {
	var vrfName []byte
	var circuitID []byte
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID = relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName = relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
	} else {
		vrfName = []byte(vrfOob)
		circuitID = []byte(circuitIDoob)
	}
	subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
	if err != nil {
		return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
	}
	subnet.Lock()
	defer subnet.Unlock()
	reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]
	if !ok {
		log.Debugf("No reservation found for mac %s ip %s", req.ClientHWAddr.String(), req.ClientIPAddr.String())
	}
	delete(subnet.allocations.allocation, req.ClientHWAddr.String())
	if err := subnet.pool.Free(reservation.address); err != nil {
		log.Errorf("IP address %s could not be released", reservation.address.String())
	}
	delete(subnet.dhcpSubnet.Status.Allocated, req.ClientHWAddr.String())
	if err := updateBackend4(subnet.dhcpSubnet); err != nil {
		log.Warnf("Update Backend failed for record with Mac Address: %s IP %s", req.ClientHWAddr.String(), reservation.address.IP.String())
	}

	return nil
}

func getSubnetInfo(vrfName string, circuitID string) (*ManagedSubnet, error) {
	pluginHdl.dhcpSubnets.Lock()
	defer pluginHdl.dhcpSubnets.Unlock()
	subnet, ok := pluginHdl.dhcpSubnets.subnets[vrfName+circuitID]
	if !ok {
		return nil, errors.Errorf("No subnet found for vrf %s and circuitID %s", vrfName, circuitID)
	}

	return subnet, nil
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
					log.Warnf("Update Backend failed for record with Mac Address: %s IP %s", hwmacaddress, reservation.address.String())
				}
			}
		}
		pluginHdl.dhcpSubnets.Unlock()
	}
}

func addPxeInfo(req, resp *dhcpv4.DHCPv4, subnet *ManagedSubnet) {
	relayAgentInfo := req.RelayAgentInfo()
	if relayAgentInfo == nil || subnet.dhcpSubnet == nil {
		return
	}
	circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
	vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)

	// Add TFTP server Option Name
	if len(subnet.dhcpSubnet.Spec.PXEURL) <= 0 &&
		(req.IsOptionRequested(dhcpv4.OptionTFTPServerName) || req.IsOptionRequested(dhcpv4.OptionBootfileName)) { // PxeURL is not specified return early with an error message
		log.Errorf("Client Requested pxe but it is not configured circuitID %s vrfName %s macAddress %s", circuitID, vrfName, req.ClientHWAddr.String())

		return
	}
	u, err := url.Parse(subnet.dhcpSubnet.Spec.PXEURL)
	if err != nil {
		log.Errorf("Invalid Pxe URL %s: %v", subnet.dhcpSubnet.Spec.PXEURL, err)

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
