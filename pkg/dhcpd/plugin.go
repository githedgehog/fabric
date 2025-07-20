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

	"github.com/coredhcp/coredhcp/handler"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

var pluginHdl *pluginState

func setup(svc *Service) func(args ...string) (handler.Handler4, error) {
	return func(_ /* args */ ...string) (handler.Handler4, error) {
		pluginHdl = &pluginState{
			dhcpSubnets: &DHCPSubnets{
				subnets: map[string]*ManagedSubnet{},
			},
			svcHdl: svc,
		}
		go func() {
			for event := range svc.kubeUpdates {
				switch event.Type {
				case EventTypeAdded:
					pluginHdl.dhcpSubnets.Lock()

					key := event.Subnet.Spec.VRF + event.Subnet.Spec.CircuitID
					if event.Subnet.Name == dhcpapi.ManagementSubnet {
						key = dhcpapi.ManagementSubnet
					}

					if val, ok := pluginHdl.dhcpSubnets.subnets[key]; ok {
						slog.Warn("Received Add event for already existing subnet", "subnet", key, "cidrblock", val.dhcpSubnet.Spec.CIDRBlock)
						if event.Subnet.Spec.StartIP != val.dhcpSubnet.Spec.StartIP ||
							event.Subnet.Spec.CIDRBlock != val.dhcpSubnet.Spec.CIDRBlock ||
							event.Subnet.Spec.EndIP != val.dhcpSubnet.Spec.EndIP { //
							// seems like things have changed since we last synced We delete what we have cached and remove our cached copy and process the add event here
							for mac := range val.allocations.allocation {
								delete(val.allocations.allocation, mac)
							}

							delete(pluginHdl.dhcpSubnets.subnets, key)
						} else {
							pluginHdl.dhcpSubnets.Unlock()

							continue
						}
					}
					_, cidr, err := net.ParseCIDR(event.Subnet.Spec.CIDRBlock)
					if err != nil {
						slog.Warn("Invalid CIDR block on DHCP", "subnet", key, "cidrblock", event.Subnet.Spec.CIDRBlock, "error", err)
						pluginHdl.dhcpSubnets.Unlock()

						continue
					}
					prefixLen, _ := cidr.Mask.Size()
					net.ParseIP(event.Subnet.Spec.StartIP)
					pool, err := newIPv4Range(
						net.ParseIP(event.Subnet.Spec.StartIP),
						net.ParseIP(event.Subnet.Spec.EndIP),
						net.ParseIP(event.Subnet.Spec.Gateway),
						binary.BigEndian.Uint32(net.ParseIP(event.Subnet.Spec.EndIP).To4())-binary.BigEndian.Uint32(net.ParseIP(event.Subnet.Spec.StartIP).To4())+1,
						uint32(prefixLen), //nolint:gosec
					)
					if err != nil {
						slog.Warn("Unable to create ip pool for subnet", "subnet", key, "cidrblock", event.Subnet.Spec.CIDRBlock, "error", err)
						pluginHdl.dhcpSubnets.Unlock()

						continue
					}

					// Sync existing allocations from backend
					allocation := make(map[string]*ipreservation, len(event.Subnet.Status.Allocated))
					for k, v := range event.Subnet.Status.Allocated {
						if _, err := pool.AllocateIP(net.IPNet{IP: net.ParseIP(v.IP), Mask: cidr.Mask}); err != nil {
							slog.Warn("Failed to allocate IP", "ip", v.IP, "error", err)

							continue
						}

						allocation[k] = &ipreservation{
							address:    net.IPNet{IP: net.ParseIP(v.IP), Mask: cidr.Mask},
							macAddress: k,
							expiry:     v.Expiry.Time,
							hostname:   v.Hostname,
							state:      committed,
						}
					}
					slog.Info("Received Add event", "subnet", key, "cidrblock", event.Subnet.Spec.CIDRBlock)
					// Create a new managed subnet.
					if len(event.Subnet.Status.Allocated) == 0 {
						event.Subnet.Status.Allocated = make(map[string]dhcpapi.DHCPAllocated)
					}
					pluginHdl.dhcpSubnets.subnets[key] = &ManagedSubnet{
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

					key := event.Subnet.Spec.VRF + event.Subnet.Spec.CircuitID
					if event.Subnet.Name == dhcpapi.ManagementSubnet {
						key = dhcpapi.ManagementSubnet
					}

					val, ok := pluginHdl.dhcpSubnets.subnets[key]
					if !ok {
						slog.Warn("Received modify event for dhcp subnet that does not exist", "subnet", key, "cidrblock", event.Subnet.Spec.CIDRBlock)
						pluginHdl.dhcpSubnets.Unlock()

						continue
					}

					if val.dhcpSubnet.Spec.StartIP != event.Subnet.Spec.StartIP {
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
						slog.Warn("Can't reduce CIDR block size", "subnet", key, "from", cachedprefixLen, "to", recprefixLen)
						pluginHdl.dhcpSubnets.Unlock()

						continue
					}
					// now we know we are increasing the cidr block size without increasing start ip update the cached copy of dhcp subnets
					val.dhcpSubnet = event.Subnet
					pluginHdl.dhcpSubnets.Unlock()
				case EventTypeDeleted:
					pluginHdl.dhcpSubnets.Lock()

					key := event.Subnet.Spec.VRF + event.Subnet.Spec.CircuitID
					if event.Subnet.Name == dhcpapi.ManagementSubnet {
						key = dhcpapi.ManagementSubnet
					}

					val, ok := pluginHdl.dhcpSubnets.subnets[key]
					if !ok {
						slog.Warn("Received Delete event for non existing subnet", "subnet", key, "cidrblock", val.dhcpSubnet.Spec.CIDRBlock)
						pluginHdl.dhcpSubnets.Unlock()

						continue
					}
					// delete the mapping
					// Does this mean the dhcp status object is gone or do i need to do something else here?
					// Delete all reservations
					for mac := range val.allocations.allocation {
						delete(val.allocations.allocation, mac)
					}

					delete(pluginHdl.dhcpSubnets.subnets, key)
					pluginHdl.dhcpSubnets.Unlock()
				}
			}
		}()

		return handlerDHCP4, nil
	}
}

func handlerDHCP4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	switch req.MessageType() { //nolint:exhaustive
	case dhcpv4.MessageTypeDiscover:
		slog.Debug("Received DHCP Discover4 from client", "mac", req.ClientHWAddr.String())
		if err := handleDiscover4(req, resp); err != nil {
			slog.Error("handleDiscover4", "error", err)
		}
	case dhcpv4.MessageTypeRequest:
		slog.Debug("Received DHCP Request4 from client", "mac", req.ClientHWAddr.String())
		if err := handleRequest4(req, resp); err != nil {
			slog.Error("handleRequest4", "error", err)
		}
	case dhcpv4.MessageTypeRelease:
		slog.Debug("Received DHCP Release4 from client", "mac", req.ClientHWAddr.String())
		if err := handleRelease4(req, resp); err != nil {
			slog.Error("handleRelease4", "error", err)
		}
	case dhcpv4.MessageTypeDecline:
		slog.Debug("Received DHCP Decline4 from client", "mac", req.ClientHWAddr.String())
		if err := handleDecline4(req, resp); err != nil {
			slog.Error("handleDecline4", "error", err)
		}
	default:
		slog.Error("Unknown DHCP message type from client", "mac", req.ClientHWAddr.String())
	}

	return resp, false
}

// This is called with a subnet lock held. We will use the last copy we cached.
func updateBackend4(dhcpsubnet *dhcpapi.DHCPSubnet) error {
	// Do i need entire dhcpsubnet object or only status
	// Locks for subnet are already held by the caller
	// ignore error for time being
	if pluginHdl.svcHdl == nil {
		return errors.New("SvcHdl is not initialized")
	}
	if err := pluginHdl.svcHdl.updateStatus(*dhcpsubnet); err != nil {
		slog.Error("Update to dhcpsubnet failed", "error", err)
	}

	return nil
}
