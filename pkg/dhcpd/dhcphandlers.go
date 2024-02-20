package dhcpd

import (
	"fmt"
	"net"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		routes, err := netlink.RouteGet(req.GatewayIPAddr)
		if err != nil {
			return errors.Wrapf(err, "handleDiscover4: failed to get route")
		}

		subnet.Lock()
		defer subnet.Unlock()
		if reservation, ok := subnet.allocations.allocation[req.ClientHWAddr.String()]; ok {
			// We have a reservation for this populate the response and send back
			resp.YourIPAddr = reservation.address.IP
			resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leaseTime))
			resp.Options.Update(dhcpv4.OptSubnetMask(reservation.address.Mask))

			resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))

			resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))
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
	log.Debug("Entering handleRequest4")
	defer log.Debug("Leave handleRequest4")
	if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {

		circuitID := relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption)
		vrfName := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption)
		if len(vrfName) > 1 {
			vrfName = vrfName[1:]
		}
		routes, err := netlink.RouteGet(req.GatewayIPAddr)
		if err != nil {
			log.Errorf("Error getting route %v", err)
		}
		subnet, err := getSubnetInfo(string(vrfName), string(circuitID))
		if err != nil {
			return errors.Wrapf(err, "handleRequest4: failed to get subnet info")
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
			resp.Options.Update(dhcpv4.OptServerIdentifier(routes[0].Src))
			resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.dhcpSubnet.Spec.Gateway)))
			subnet.dhcpSubnet.Status.Allocated[req.ClientHWAddr.String()] = v1alpha2.DHCPAllocated{
				IP:       reservation.address.IP.String(),
				Expiry:   metav1.NewTime(reservation.expiry),
				Hostname: reservation.Hostname,
			}
			updateBackend4(subnet.dhcpSubnet)
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
			MacAddress: req.ClientHWAddr.String(),
			expiry:     time.Now().Add(leaseTime),
			state:      committed,
			Hostname:   req.HostName(),
		}
		subnet.dhcpSubnet.Status.Allocated[req.ClientHWAddr.String()] = v1alpha2.DHCPAllocated{
			IP:       ipnet.IP.String(),
			Expiry:   metav1.NewTime(time.Now().Add(leaseTime)),
			Hostname: req.HostName(),
		}
		updateBackend4(subnet.dhcpSubnet)

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
		delete(subnet.dhcpSubnet.Status.Allocated, req.ClientHWAddr.String())
		updateBackend4(subnet.dhcpSubnet)
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
		delete(subnet.dhcpSubnet.Status.Allocated, req.ClientHWAddr.String())
		updateBackend4(subnet.dhcpSubnet)
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

	// wake up every 2 min and try looking for expired leases
	// This is a long loop we migh want to break this so we don't spend too much time here
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
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
				}
				updateBackend4(v.dhcpSubnet)
			}
			pluginHdl.dhcpSubnets.Unlock()
		}
	}

}
