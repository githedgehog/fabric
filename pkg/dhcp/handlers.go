// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"errors"
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

// relayInfo is what the relay agent tells about the client (option 82)
type relayInfo struct {
	vrf       string
	circuitID string
	remoteID  string
}

func relayInfoFrom(req *dhcpv4.DHCPv4) relayInfo {
	relay := relayInfo{}

	relayAgentInfo := req.RelayAgentInfo()
	if relayAgentInfo == nil {
		return relay
	}

	relay.vrf = strings.ToLower(strings.TrimSpace(string(relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption))))
	if len(relay.vrf) > 1 {
		relay.vrf = relay.vrf[1:]
	}
	relay.vrf = strings.TrimPrefix(relay.vrf, vrfPrefix)
	if relay.vrf == "" {
		relay.vrf = defaultVRF
	}

	relay.circuitID = strings.ToLower(strings.TrimSpace(string(relayAgentInfo.Get(dhcpv4.AgentCircuitIDSubOption))))
	relay.remoteID = strings.ToLower(strings.TrimSpace(string(relayAgentInfo.Get(dhcpv4.AgentRemoteIDSubOption))))

	return relay
}

func reqSummary(req *dhcpv4.DHCPv4, relay relayInfo) []any {
	res := []any{"type", msgTypeString(req), "mac", req.ClientHWAddr.String()}

	if req.HostName() != "" {
		res = append(res, "hostname", req.HostName())
	}

	if relay.vrf != "" {
		res = append(res, "vrf", relay.vrf)
	}

	if relay.circuitID != "" {
		res = append(res, "circuitID", relay.circuitID)
	}

	if req.RequestedIPAddress() != nil {
		res = append(res, "requested", req.RequestedIPAddress().String())
	}

	if relay.remoteID != "" {
		res = append(res, "remoteID", relay.remoteID)
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
			if !s.handleRequest(ctx, req, resp) {
				return nil, true
			}

			return resp, false
		}, nil
	}
}

// handleRequest fills in the reply and reports whether to send it
func (s *Server) handleRequest(ctx context.Context, req, resp *dhcpv4.DHCPv4) bool {
	relay := relayInfoFrom(req)

	// returns false after a panic, so the request is dropped
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panicked, dropping", append(reqSummary(req, relay), "err", r)...)
		}
	}()

	// a reply is only useful while the client still waits for it, so the request is dropped after that
	ctx, cancel := context.WithTimeout(ctx, s.Config.RequestTimeout.Duration)
	defer cancel()

	if violation := s.checkRelayedRequest(req.GatewayIPAddr); violation != nil {
		slog.Error("DHCP relay validation failed, dropping",
			append(reqSummary(req, relay), "giaddr", req.GatewayIPAddr.String(), "reason", violation)...)

		return false
	}

	sn, err := s.findSubnet(relay)
	if err != nil {
		slog.Warn("Duplicate subnet found for Cumulus dhcp-relay, ignoring request", append(reqSummary(req, relay), "err", err)...)
	}

	// only use dhcp for onie and cumulus in the management subnet unless all devices are allowed
	if !s.Config.AnyDeviceOnMgmt && sn != nil && sn.obj.Load().Name == dhcpapi.ManagementSubnet {
		classID := req.ClassIdentifier()
		if !strings.HasPrefix(classID, onieClassIDPrefix) && classID != cumulusClassID {
			// not logged to not spam the logs
			return false
		}
	}

	if sn == nil {
		slog.Info("No subnet found", reqSummary(req, relay)...)

		return false
	}

	slog.Info("Handling", reqSummary(req, relay)...)

	switch req.MessageType() { //nolint:exhaustive
	case dhcpv4.MessageTypeDiscover, dhcpv4.MessageTypeRequest:
		// before the write, so a failure doesn't leave a lease the client never gets offered
		serverID, err := s.serverIdentifier(req.GatewayIPAddr)
		if err != nil {
			return s.dropped(req, relay, fmt.Errorf("getting server identifier: %w", err))
		}

		ip, sp, err := sn.lease(ctx, s.kube, &s.Config, req)
		if errors.Is(err, ErrRequestedIPUnavailable) {
			slog.Info("NAK", append(reqSummary(req, relay), "err", err.Error())...)
			nakResponse(req, resp, serverID)

			return true
		}
		if err != nil {
			return s.dropped(req, relay, fmt.Errorf("updating subnet %s to allocate: %w", sn.obj.Load().Name, err))
		}

		slog.Info("Allocated", append(reqSummary(req, relay), "ip", ip)...)

		sp.respond(req, resp, ip, serverID)
	case dhcpv4.MessageTypeRelease, dhcpv4.MessageTypeDecline:
		mac := req.ClientHWAddr.String()
		if err := sn.persist(ctx, s.kube, func(*dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
			return func(upd *dhcpapi.DHCPSubnet) { delete(upd.Status.Allocated, mac) }, nil
		}); err != nil {
			return s.dropped(req, relay, fmt.Errorf("updating subnet %s to release: %w", sn.obj.Load().Name, err))
		}

		slog.Info("Released", reqSummary(req, relay)...)

		// TODO update response? wasn't done in a previous implementation
	default:
		return s.dropped(req, relay, fmt.Errorf("unsupported DHCP request type")) //nolint:err113
	}

	return true
}

var errDuplicateCumulusSubnet = errors.New("duplicate subnets for Cumulus dhcp-relay")

func (s *Server) findSubnet(relay relayInfo) (*subnet, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Cumulus doesn't support vrf-select so fallback to just checking the circuit ID (vlan interface name)
	if !strings.HasPrefix(relay.remoteID, cumulusRemoteIDPrefix) || relay.circuitID == "" {
		return s.subnets[subnetKey(relay.vrf, relay.circuitID)], nil
	}

	var res *subnet
	for _, some := range s.subnets {
		if strings.EqualFold(some.obj.Load().Spec.CircuitID, relay.circuitID) {
			if res != nil {
				return nil, fmt.Errorf("%w: %s and %s", errDuplicateCumulusSubnet, res.obj.Load().Name, some.obj.Load().Name)
			}

			res = some
		}
	}

	return res, nil
}

// dropped logs why the request is dropped and returns false to drop it
func (s *Server) dropped(req *dhcpv4.DHCPv4, relay relayInfo, err error) bool {
	// overload drops are counted and reported periodically by reportBudget
	switch {
	case errors.Is(err, errUpdateBudget):
		slog.Debug("Over the update budget, dropping", append(reqSummary(req, relay), "err", err.Error())...)
	case errors.Is(err, context.DeadlineExceeded):
		s.budget.expired.Add(1)
		slog.Debug("Request timed out, dropping", append(reqSummary(req, relay), "err", err.Error())...)
	default:
		slog.Error("Error handling, dropping", append(reqSummary(req, relay), "err", err.Error())...)
	}

	return false
}
