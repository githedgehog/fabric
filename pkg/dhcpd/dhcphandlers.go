package dhcpd

import (
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/pkg/errors"
)

func handleDiscover4(req, resp *dhcpv4.DHCPv4) error {
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
		// Get subnet for this vrf and circuitID
		subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
		if err != nil {
			return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
		}
		subnet.Lock()
		defer subnet.Unlock()
		if reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]; ok {
			// We have a reservation for this populate the response and send back
			resp.YourIPAddr = reservation.address.IP
			//net.ParseCIDR(reservation.address.String())
			resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
			resp.Options.Update(dhcpv4.OptSubnetMask(reservation.address.Mask))
			resp.Options.Update(dhcpv4.OptRouter(net.IP(subnet.dhcpSubnet.Spec.Gateway)))
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
		resp.Options.Update(dhcpv4.OptRouter(net.IP(subnet.dhcpSubnet.Spec.Gateway)))
		subnet.allocations.allocation[req.ClientHWAddr.String()] = &ipreservation{
			address:    ipnet,
			MacAddress: req.ClientHWAddr.String(),
			expiry:     time.Now().Add(leaseTime),
			state:      pending,
			Hostname:   req.HostName(),
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
				subnet.pool.Free(reservation.address)
			}
		})

	}
	return nil
}

func handleRequest4(req, resp *dhcpv4.DHCPv4) error {
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
		subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
		if err != nil {
			return errors.Wrapf(err, "handleDiscover4: failed to get subnet info")
		}
		subnet.Lock()
		defer subnet.Unlock()
		reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]
		if ok {
			reservation.state = committed
			reservation.expiry = time.Now().Add(leaseTime)
			resp.YourIPAddr = reservation.address.IP
			resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
			resp.Options.Update(dhcpv4.OptSubnetMask(reservation.address.Mask))
			resp.Options.Update(dhcpv4.OptRouter(net.IP(subnet.dhcpSubnet.Spec.Gateway)))
			updateBackend4(&updateBackend{
				IP:         reservation.address.String(),
				MacAddress: req.ClientHWAddr.String(),
				Expiry:     reservation.expiry,
				Hostname:   req.HostName(),
				Vrf:        string(vrfName),
				circuitID:  string(circuitID),
			})
			return nil
		}
		ipnet, err := subnet.pool.Allocate()
		if err != nil {
			return errors.Wrapf(err, "handleDiscover4: failed to allocate ip")
		}
		resp.YourIPAddr = ipnet.IP
		resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
		resp.Options.Update(dhcpv4.OptSubnetMask(ipnet.Mask))
		resp.Options.Update(dhcpv4.OptRouter(net.IP(subnet.dhcpSubnet.Spec.Gateway)))
		subnet.allocations.allocation[req.ClientHWAddr.String()] = &ipreservation{
			address:    ipnet,
			MacAddress: req.ClientHWAddr.String(),
			expiry:     time.Now().Add(leaseTime),
			state:      committed,
			Hostname:   req.HostName(),
		}
		updateBackend4(&updateBackend{
			IP:         ipnet.String(),
			MacAddress: req.ClientHWAddr.String(),
			Expiry:     time.Now().Add(leaseTime),
			Hostname:   req.HostName(),
			Vrf:        string(vrfName),
			circuitID:  string(circuitID),
		})
	}
	return nil
}

func handleDecline4(req, resp *dhcpv4.DHCPv4) error {
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
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
		updateBackend4(&updateBackend{
			IP:         reservation.address.String(),
			MacAddress: req.ClientHWAddr.String(),
			Expiry:     reservation.expiry,
			Hostname:   req.HostName(),
			Vrf:        string(vrfName),
			circuitID:  string(circuitID),
		})
	}
	return nil
}

func handleRelease4(req, resp *dhcpv4.DHCPv4) error {
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
		circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
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

		updateBackend4(&updateBackend{
			IP:         reservation.address.String(),
			MacAddress: req.ClientHWAddr.String(),
			Expiry:     reservation.expiry,
			Hostname:   req.HostName(),
			Vrf:        string(vrfName),
			circuitID:  string(circuitID),
		})

	}
	return nil
}

func getSubnetInfo(vrfName string, circuitID string) (*ManagedSubnet, error) {
	pluginHdl.dhcpSubnets.Lock()
	defer pluginHdl.dhcpSubnets.Unlock()
	subnet, ok := pluginHdl.dhcpSubnets.subnets[string(vrfName)+string(circuitID)]
	if !ok {
		return nil, fmt.Errorf("No subnet found for vrf %s and circuitID %s", vrfName, circuitID)
	}
	return subnet, nil
}

func handleExpiredLeases() {
	pluginHdl.dhcpSubnets.Lock()
	defer pluginHdl.dhcpSubnets.Unlock()

	// for k, v := range pluginHdl.dhcpSubnets.subnets {

	// }
}
