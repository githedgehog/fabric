// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"

	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
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

func reqSummary(req *dhcpv4.DHCPv4, vrf, circuitID string) []any {
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

	return res
}

func (s *Server) setupDHCP4Plugin(ctx context.Context) plugins.SetupFunc4 {
	return func(args ...string) (handler.Handler4, error) {
		return func(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
			vrf, circuitID := "", ""
			if relayAgentInfo := req.RelayAgentInfo(); relayAgentInfo != nil {
				vrf = string(relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption))
				if len(vrf) > 1 {
					vrf = vrf[1:]
				}
				if vrf == "" {
					vrf = defaultVRF
				}

				circuitID = string(relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption))
			}

			s.m.RLock()
			subnet, ok := s.subnets[subnetKey(vrf, circuitID)]
			if ok {
				subnet = subnet.DeepCopy()
			}
			s.m.RUnlock()

			if !ok {
				slog.Info("No subnet found", reqSummary(req, vrf, circuitID)...)
			} else {
				slog.Info("Handling", reqSummary(req, vrf, circuitID)...)
				if err := s.handleDHCP4(ctx, subnet, req, resp, vrf, circuitID); err != nil {
					slog.Error("Error handling", append(reqSummary(req, vrf, circuitID), "err", err.Error())...)
				}
			}

			return resp, false
		}, nil
	}
}

func (s *Server) handleDHCP4(ctx context.Context, subnet *dhcpapi.DHCPSubnet, req, resp *dhcpv4.DHCPv4, vrf, circuitID string) error {
	defer func() {
		if err := recover(); err != nil {
			slog.Warn("Panicked", append(reqSummary(req, vrf, circuitID), "err", err)...)
		}
	}()

	switch req.MessageType() { //nolint:exhaustive
	case dhcpv4.MessageTypeDiscover, dhcpv4.MessageTypeRequest:
		var ip netip.Addr
		if err := s.updateSubnet(ctx, subnet, func(subnet *dhcpapi.DHCPSubnet) error {
			var err error
			ip, err = allocate(subnet, req)
			if err != nil {
				return fmt.Errorf("allocating ip: %w", err)
			}

			return nil
		}); err != nil {
			return fmt.Errorf("updating subnet %s to allocate: %w", subnet.Name, err)
		}

		slog.Info("Allocated", append(reqSummary(req, vrf, circuitID), "ip", ip)...)

		_, ipNet, err := net.ParseCIDR(subnet.Spec.CIDRBlock)
		if err != nil {
			return fmt.Errorf("parsing subnet %s: %w", subnet.Name, err)
		}
		ipNet.IP = net.IP(ip.AsSlice())

		if err := updateResponse(req, resp, subnet, ipNet); err != nil {
			return fmt.Errorf("updating response: %w", err)
		}
	case dhcpv4.MessageTypeRelease, dhcpv4.MessageTypeDecline:
		if err := s.updateSubnet(ctx, subnet, func(subnet *dhcpapi.DHCPSubnet) error {
			delete(subnet.Status.Allocated, req.ClientHWAddr.String())

			return nil
		}); err != nil {
			return fmt.Errorf("updating subnet %s to release: %w", subnet.Name, err)
		}

		slog.Info("Released", reqSummary(req, vrf, circuitID)...)

		// TODO update response? wasn't done in a previous implementation
	default:
		return fmt.Errorf("unsupported DHCP request type") //nolint:err113
	}

	return nil
}
