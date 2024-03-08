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
// +build linux

package dhcpd

import (
	"encoding/binary"
	"net"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var pluginHdl *pluginState
var log = logger.GetLogger("plugins/dhcpd")

func setup(svc *Service) func(args ...string) (handler.Handler4, error) {
	return func(args ...string) (handler.Handler4, error) {
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
							if event.Subnet.Spec.StartIP != val.dhcpSubnet.Spec.StartIP ||
								event.Subnet.Spec.CIDRBlock != val.dhcpSubnet.Spec.CIDRBlock ||
								event.Subnet.Spec.EndIP != val.dhcpSubnet.Spec.EndIP { //
								// seems like things have changed since we last synced We delete what we have cached and remove our cached copy and process the add event here
								for mac, _ := range val.allocations.allocation {
									delete(val.allocations.allocation, mac)
								}

								delete(pluginHdl.dhcpSubnets.subnets, event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID)

							} else {
								pluginHdl.dhcpSubnets.Unlock()
								continue
							}

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
							if _, err := pool.AllocateIP(net.IPNet{IP: net.ParseIP(v.IP), Mask: cidr.Mask}); err != nil {
								log.Errorf("Failed to allocate IP %s with error %s", v.IP, err)
								continue
							}

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
						if len(event.Subnet.Status.Allocated) == 0 {
							event.Subnet.Status.Allocated = make(map[string]dhcpapi.DHCPAllocated)
						}
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
						pluginHdl.dhcpSubnets.Lock()
						val, ok := pluginHdl.dhcpSubnets.subnets[event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID]
						if !ok {
							log.Errorf("Received modify event for dhcp subnet that does not exist: %s:%s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						if val.dhcpSubnet.Spec.StartIP != val.dhcpSubnet.Spec.StartIP {
							// ignore this event.
							// Can't modify the start ip
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						_, received, _ := net.ParseCIDR(event.Subnet.Spec.CIDRBlock)
						_, cached, _ := net.ParseCIDR(val.dhcpSubnet.Spec.CIDRBlock)
						recprefixLen, _ := received.Mask.Size()
						cachedprefixLen, _ := cached.Mask.Size()
						if recprefixLen < cachedprefixLen {
							// can't reduce CIDR block size
							log.Errorf("Can't reduce CIDR block size for %s:%s from %d to %d", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, cachedprefixLen, recprefixLen)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						// now we know we are increasing the cidr block size without increasing start ip update the cached copy of dhcp subnets
						val.dhcpSubnet = event.Subnet
						pluginHdl.dhcpSubnets.Unlock()
					case EventTypeDeleted:
						pluginHdl.dhcpSubnets.Lock()
						val, ok := pluginHdl.dhcpSubnets.subnets[event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID]
						if !ok {
							log.Errorf("Received Delete event for non existing subnet %s:%s with cidrblock %s", event.Subnet.Spec.VRF, event.Subnet.Spec.CircuitID, val.dhcpSubnet.Spec.CIDRBlock)
							pluginHdl.dhcpSubnets.Unlock()
							continue
						}
						// delete the mapping
						// Does this mean the dhcp status object is gone or do i need to do something else here?
						// Delete all reservations
						for mac, _ := range val.allocations.allocation {
							delete(val.allocations.allocation, mac)
						}

						delete(pluginHdl.dhcpSubnets.subnets, event.Subnet.Spec.VRF+event.Subnet.Spec.CircuitID)
						pluginHdl.dhcpSubnets.Unlock()
					}
				}

			}
		}()

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
func updateBackend4(dhcpsubnet *dhcpapi.DHCPSubnet) error {
	// Do i need entire dhcpsubnet object or only status
	// Locks for subnet are already held by the caller
	// ignore error for time being
	if err := pluginHdl.svcHdl.updateStatus(*dhcpsubnet); err != nil {
		log.Errorf("Update to dhcpsubnet failed: %v", err)
	}
	return nil
}
