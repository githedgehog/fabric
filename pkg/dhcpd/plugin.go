//go:build linux
// +build linux

package dhcpd

import (
	"encoding/binary"
	"net"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/insomniacslk/dhcp/dhcpv4"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var pluginHdl *pluginState
var log = logger.GetLogger("plugins/dhcpd")

func setup(svc *Service) func(args ...string) (handler.Handler4, error) {
	return func(args ...string) (handler.Handler4, error) {
		// TODO
		// you can use params from svc here, like
		// svc.kubeUpdates channel to listen for updates from k8s
		// svc.updateStatus to update status of a subnets in k8s

		// for {
		// 	switch <-svc.kubeUpdates {
		// 	//..
		// 	}
		// }
		pluginHdl = &pluginState{
			dhcpSubnets: &DHCPSubnets{
				subnets: map[string]*ManagedSubnet{},
			},
			svcHdl: svc,
		}
		go func() {
			for {
				select {
				case event := <-svc.kubeUpdates:
					switch event.Type {
					case EventTypeAdded:

						pluginHdl.dhcpSubnets.Lock()
						if val, ok := pluginHdl.dhcpSubnets.subnets[event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID]; ok {
							log.Errorf("Received Add event for already existing subnet %s:%s with cidrblock %s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, val.dhcpSubnet.Spec.CIDRBlock)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						_, cidr, err := net.ParseCIDR(event.Subnet.Spec.CIDRBlock)
						if err != nil {
							log.Errorf("Invalid CIDR block on DHCP subnet %s:%s with cidrblock %s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, event.Subnet.Spec.CIDRBlock)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						prefixLen, _ := cidr.Mask.Size()
						net.ParseIP(event.Subnet.Spec.StartIP)
						pool, err := NewIPv4Range(
							net.ParseIP(event.Subnet.Spec.StartIP),
							net.ParseIP(event.Subnet.Spec.EndIP),
							net.ParseIP(event.Subnet.Spec.Gateway),
							binary.BigEndian.Uint32(net.ParseIP(event.Subnet.Spec.EndIP).To4())-binary.BigEndian.Uint32(net.ParseIP(event.Subnet.Spec.StartIP).To4())+1,
							uint32(prefixLen),
						)
						if err != nil {
							log.Errorf("Unable to create ip pool for subnet %s:%s with cidrblock %s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, event.Subnet.Spec.CIDRBlock)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}

						// Sync existing allocations from backend
						allocation := make(map[string]*ipreservation, len(event.Subnet.Status.Allocated))
						for k, v := range event.Subnet.Status.Allocated {
							allocation[k] = &ipreservation{
								address:    net.IPNet{IP: net.ParseIP(v.IP), Mask: cidr.Mask},
								MacAddress: k,
								expiry:     v.Expiry.Time,
								Hostname:   v.Hostname,
								state:      committed,
							}
						}
						log.Infof("Received Add event for subnet %s:%s with cidrblock %s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, event.Subnet.Spec.CIDRBlock)
						// Create a new managed subnet.
						pluginHdl.dhcpSubnets.subnets[event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID] = &ManagedSubnet{
							dhcpSubnet: event.Subnet,
							pool:       pool,
							allocations: &ipallocations{
								allocation: allocation,
							},
						}

						pluginHdl.dhcpSubnets.Unlock()
					case EventTypeModified:
						// Maybe we have a new allocation for this subnet.
						// Lets handle this later
						// Will require merge
					case EventTypeDeleted:
						pluginHdl.dhcpSubnets.Lock()
						if val, ok := pluginHdl.dhcpSubnets.subnets[event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID]; !ok {
							log.Errorf("Received Delete event for non existing subnet %s:%s with cidrblock %s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, val.dhcpSubnet.Spec.CIDRBlock)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						// delete the mapping
						// Does this mean the dhcp status object is gone or do i need to do something else here?
						delete(pluginHdl.dhcpSubnets.subnets, event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID)
						pluginHdl.dhcpSubnets.Unlock()
					}
				}

			}
		}()
		// subnet := dhcpapi.DHCPSubnet{}
		// subnet.Status.Allocated["asdasd"] = dhcpapi.DHCPAllocated{
		// 	IP:       "",
		// 	Expiry:   metav1.Time{},
		// 	Hostname: "",
		// }
		// err := svc.updateStatus(subnet)

		return handlerDHCP4, nil
	}
}

func handlerDHCP4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	// TODO

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		if err := handleDiscover4(req, resp); err != nil {
			log.Errorf("handleDiscover4 error: %s", err)
		}
	case dhcpv4.MessageTypeRequest:
		if err := handleRequest4(req, resp); err != nil {
			log.Errorf("handle DHCP Request4 error: %s", err)
		}
	case dhcpv4.MessageTypeRelease:
		if err := handleRelease4(req, resp); err != nil {
			log.Errorf("handle DHCP Release4 error: %s", err)
		}
	case dhcpv4.MessageTypeDecline:
		if err := handleDecline4(req, resp); err != nil {
			log.Errorf("handle DHCP Decline4 error: %s", err)
		}
	default:
		log.Errorf("Unknown DHCP message type from client: %s", req.ClientHWAddr.String())
	}
	return resp, false
}

// This is called with a subnet lock held. We will use the last copy we cached.
func updateBackend4(info *updateBackend) error {
	// Do i need entire dhcpsubnet object or only status
	return nil
}
