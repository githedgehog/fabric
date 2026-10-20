// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
)

const (
	onieClassIDPrefix     = "onie_vendor:"
	cumulusClassID        = "cumulus-linux  x86_64"
	cumulusRemoteIDPrefix = "cumulus:"
	vrfPrefix             = "vrfv"
)

func msgTypeString(req *dhcpv4.DHCPv4) string {
	switch req.MessageType() {
	case dhcpv4.MessageTypeAck:
		return "ack"
	case dhcpv4.MessageTypeDecline:
		return "decline"
	case dhcpv4.MessageTypeDiscover:
		return "discover"
	case dhcpv4.MessageTypeInform:
		return "inform"
	case dhcpv4.MessageTypeNak:
		return "nak"
	case dhcpv4.MessageTypeNone:
		return "none"
	case dhcpv4.MessageTypeOffer:
		return "offer"
	case dhcpv4.MessageTypeRelease:
		return "release"
	case dhcpv4.MessageTypeRequest:
		return "request"
	}

	return "unexpected"
}

func reqSummary(req *dhcpv4.DHCPv4, vrf, circuitID, remoteID string) []any {
	res := []any{"type", msgTypeString(req), "mac", req.ClientHWAddr.String()}

	if req.HostName() != "" {
		res = append(res, "hostname", req.HostName())
	}

	if vrf != "" {
		res = append(res, "vrf", vrf)
	}

	if circuitID != "" {
		res = append(res, "circuitID", circuitID)
	}

	if req.RequestedIPAddress() != nil {
		res = append(res, "requested", req.RequestedIPAddress().String())
	}

	if remoteID != "" {
		res = append(res, "remoteID", remoteID)
	}

	return res
}

// checkRelayedRequest returns an error when a relayed DHCP request should be
// dropped. Returns nil for unrelayed packets (GIADDR 0.0.0.0) and for packets
// whose GIADDR belongs to a known leaf switch.
func (s *Server) checkRelayedRequest(giaddr net.IP) error {
	if giaddr.IsUnspecified() {
		return nil
	}
	str := giaddr.String()
	s.mu.RLock()
	_, known := s.relayAllowlist[str]
	s.mu.RUnlock()
	if !known {
		return fmt.Errorf("unknown GIADDR %s", str) //nolint:goerr113
	}

	return nil
}

// setupDHCP4Plugin sets up the coredhcp handler. Any non-nil response is sent to the client, so every path that can't
// produce a proper reply returns nil to drop the request instead of sending an empty OFFER/ACK (yiaddr 0.0.0.0)
func (s *Server) setupDHCP4Plugin(ctx context.Context) plugins.SetupFunc4 {
	return func(args ...string) (handler.Handler4, error) {
		return func(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
			vrf, circuitID, remoteID := "", "", ""
			if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
				vrf = strings.ToLower(strings.TrimSpace(string(relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption))))
				if len(vrf) > 1 {
					vrf = vrf[1:]
				}
				vrf = strings.TrimPrefix(vrf, vrfPrefix)
				if vrf == "" {
					vrf = defaultVRF
				}

				circuitID = strings.ToLower(strings.TrimSpace(string(relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption))))
				remoteID = strings.ToLower(strings.TrimSpace(string(relayAgentInfo.Get(dhcpv4.AgentRemoteIDSubOption))))
			}

			if violation := s.checkRelayedRequest(req.GatewayIPAddr); violation != nil {
				slog.Error("DHCP relay validation failed, dropping",
					append(reqSummary(req, vrf, circuitID, remoteID),
						"giaddr", req.GatewayIPAddr.String(), "reason", violation)...)

				return nil, true
			}

			s.mu.RLock()

			var sn *subnet

			// Cumulus doesn't support vrf-select so fallback to just checking the circuit ID (vlan interface name)
			if strings.HasPrefix(remoteID, cumulusRemoteIDPrefix) && circuitID != "" {
				for _, some := range s.subnets {
					if strings.EqualFold(some.obj.Load().Spec.CircuitID, circuitID) {
						if sn != nil {
							slog.Warn("Duplicate subnet found for Cumulus dhcp-relay, ignoring request",
								append([]any{"subnet", some.obj.Load().Name, "other", sn.obj.Load().Name}, reqSummary(req, vrf, circuitID, remoteID)...)...)

							sn = nil

							break
						}

						sn = some
					}
				}
			} else {
				sn = s.subnets[subnetKey(vrf, circuitID)]
			}

			s.mu.RUnlock()

			// only use dhcp for onie and cumulus in the management subnet unless all devices are allowed
			mgmtSkip := false
			if !s.Config.AnyDeviceOnMgmt && sn != nil && sn.obj.Load().Name == dhcpapi.ManagementSubnet {
				classID := req.ClassIdentifier()
				if !strings.HasPrefix(classID, onieClassIDPrefix) && classID != cumulusClassID {
					mgmtSkip = true
					sn = nil
				}
			}

			if sn == nil {
				// We aren't printing anything if mgmtSkip here because we don't want to spam the logs with unnecessary information
				if !mgmtSkip {
					slog.Info("No subnet found", reqSummary(req, vrf, circuitID, remoteID)...)
				}

				return nil, true
			}

			slog.Info("Handling", reqSummary(req, vrf, circuitID, remoteID)...)
			if err := s.handleDHCP4(ctx, sn, req, resp, vrf, circuitID, remoteID); err != nil {
				slog.Error("Error handling, dropping", append(reqSummary(req, vrf, circuitID, remoteID), "err", err.Error())...)

				return nil, true
			}

			return resp, false
		}, nil
	}
}

func (s *Server) handleDHCP4(ctx context.Context, sn *subnet, req, resp *dhcpv4.DHCPv4, vrf, circuitID, remoteID string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("Panicked", append(reqSummary(req, vrf, circuitID, remoteID), "err", r)...)
			err = fmt.Errorf("panicked: %v", r) //nolint:err113
		}
	}()

	switch req.MessageType() { //nolint:exhaustive
	case dhcpv4.MessageTypeDiscover, dhcpv4.MessageTypeRequest:
		ip, subnet, err := sn.lease(ctx, s.kube, req)
		if err != nil {
			return fmt.Errorf("updating subnet %s to allocate: %w", sn.obj.Load().Name, err)
		}

		slog.Info("Allocated", append(reqSummary(req, vrf, circuitID, remoteID), "ip", ip)...)

		_, ipNet, err := net.ParseCIDR(subnet.Spec.CIDRBlock)
		if err != nil {
			return fmt.Errorf("parsing subnet %s: %w", subnet.Name, err)
		}
		ipNet.IP = net.IP(ip.AsSlice())

		if err := updateResponse(req, resp, subnet, ipNet); err != nil {
			return fmt.Errorf("updating response: %w", err)
		}
	case dhcpv4.MessageTypeRelease, dhcpv4.MessageTypeDecline:
		if _, err := sn.persist(ctx, s.kube, func(obj *dhcpapi.DHCPSubnet) error {
			delete(obj.Status.Allocated, req.ClientHWAddr.String())

			return nil
		}); err != nil {
			return fmt.Errorf("updating subnet %s to release: %w", sn.obj.Load().Name, err)
		}

		slog.Info("Released", reqSummary(req, vrf, circuitID, remoteID)...)

		// TODO update response? wasn't done in a previous implementation
	default:
		return fmt.Errorf("unsupported DHCP request type") //nolint:err113
	}

	return nil
}
