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
	"time"

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
	vrf           string
	circuitID     string
	remoteID      string
	unexpectedVSS string // hex of a VSS leaves never send, the request is dropped
}

func relayInfoFrom(req *dhcpv4.DHCPv4) relayInfo {
	relay := relayInfo{}

	relayAgentInfo := req.RelayAgentInfo()
	if relayAgentInfo == nil {
		return relay
	}

	// VSS (RFC 6607): leaves send type 0 with the VRF name, type 0xff alone is the global (default) VRF
	switch vss := relayAgentInfo.Get(dhcpv4.VirtualSubnetSelectionSubOption); {
	case len(vss) == 0, len(vss) == 1 && vss[0] == 0xff:
	case vss[0] == 0 && strings.TrimSpace(string(vss[1:])) != "":
		relay.vrf = strings.ToLower(strings.TrimSpace(string(vss[1:])))
	default:
		relay.unexpectedVSS = fmt.Sprintf("%x", vss)
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
		return fmt.Errorf("unknown GIADDR %s", str) //nolint:err113
	}

	return nil
}

// setupDHCP4Plugin sets up the coredhcp handler. Any non-nil response is sent to the client, so every path that can't
// produce a proper reply returns nil to drop the request instead of sending an empty OFFER/ACK (yiaddr 0.0.0.0)
func (s *Server) setupDHCP4Plugin(ctx context.Context) plugins.SetupFunc4 {
	return func(args ...string) (handler.Handler4, error) {
		return func(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
			start := time.Now()
			relay := relayInfoFrom(req)

			out := s.handleRequest(ctx, req, resp, relay)
			if out.msg != "" {
				attrs := append(reqSummary(req, relay), out.attrs...)
				slog.Log(ctx, out.level, out.msg, append(attrs, "took", time.Since(start))...)
			}
			if !out.reply {
				return nil, true
			}

			return resp, false
		}, nil
	}
}

// outcome is what happened to a request, logged as a single line once it's handled
type outcome struct {
	level slog.Level
	msg   string // nothing is logged if empty
	attrs []any
	reply bool
}

func drop(level slog.Level, reason string, attrs ...any) outcome {
	return outcome{level: level, msg: "Dropped", attrs: append([]any{"reason", reason}, attrs...)}
}

// handleRequest fills in the reply and returns the outcome
func (s *Server) handleRequest(ctx context.Context, req, resp *dhcpv4.DHCPv4, relay relayInfo) outcome {
	// returns the zero outcome after a panic, so the request is dropped without another log line
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Dropped", append(reqSummary(req, relay), "reason", "panicked", "err", r)...)
		}
	}()

	// a reply is only useful while the client still waits for it, so the request is dropped after that
	ctx, cancel := context.WithTimeout(ctx, s.Config.RequestTimeout.Duration)
	defer cancel()

	if violation := s.checkRelayedRequest(req.GatewayIPAddr); violation != nil {
		return drop(slog.LevelError, "relay validation failed", "giaddr", req.GatewayIPAddr.String(), "err", violation)
	}
	if relay.unexpectedVSS != "" {
		return drop(slog.LevelWarn, "unexpected VSS", "vss", relay.unexpectedVSS)
	}

	sn, err := s.findSubnet(relay)
	if err != nil {
		return drop(slog.LevelWarn, err.Error())
	}

	// the management network is never relayed and VPCs are only reachable through the leaf relays, so anything else
	// is a misconfigured relay or a forged relay agent info
	if sn != nil {
		relayed := !req.GatewayIPAddr.IsUnspecified()
		mgmt := sn.obj.Load().Name == dhcpapi.ManagementSubnet
		switch {
		case relayed && mgmt:
			return drop(slog.LevelWarn, "management subnet over a relay", "giaddr", req.GatewayIPAddr.String())
		case !relayed && !mgmt:
			return drop(slog.LevelWarn, "VPC subnet without a relay", "subnet", sn.obj.Load().Name)
		}
	}

	// only use dhcp for onie and cumulus in the management subnet unless all devices are allowed
	if !s.Config.AnyDeviceOnMgmt && sn != nil && sn.obj.Load().Name == dhcpapi.ManagementSubnet {
		classID := req.ClassIdentifier()
		if !strings.HasPrefix(classID, onieClassIDPrefix) && classID != cumulusClassID {
			// not logged to not spam the logs
			return outcome{}
		}
	}

	if sn == nil {
		return drop(slog.LevelInfo, "no subnet")
	}

	name := sn.obj.Load().Name
	withSubnet := func(out outcome) outcome {
		out.attrs = append([]any{"subnet", name}, out.attrs...)

		return out
	}

	switch req.MessageType() { //nolint:exhaustive
	case dhcpv4.MessageTypeDiscover, dhcpv4.MessageTypeRequest:
		serverID := s.controlVIP

		ip, info, err := sn.lease(ctx, s.kube, &s.Config, req)
		if errors.Is(err, ErrRequestedIPUnavailable) {
			nakResponse(req, resp, serverID)

			// without the wrapping context, the subnet is logged anyway
			reason := err.Error()
			if idx := strings.Index(reason, ErrRequestedIPUnavailable.Error()); idx > 0 {
				reason = reason[idx:]
			}

			return withSubnet(outcome{level: slog.LevelInfo, msg: "NAKed", attrs: []any{"reason", reason}, reply: true})
		}
		if err != nil {
			return withSubnet(s.dropOnError(err))
		}

		info.spec.respond(req, resp, ip, serverID)

		attrs := []any{"ip", ip.String()}
		if info.notWanted != "" {
			attrs = append(attrs, "reason", info.notWanted)
		}
		if req.MessageType() == dhcpv4.MessageTypeDiscover {
			return withSubnet(outcome{level: slog.LevelInfo, msg: "Offered", attrs: attrs, reply: true})
		}

		return withSubnet(outcome{level: slog.LevelInfo, msg: "Acked", attrs: append(attrs, "written", info.written), reply: true})
	case dhcpv4.MessageTypeRelease, dhcpv4.MessageTypeDecline:
		mac := req.ClientHWAddr.String()
		if err := sn.persist(ctx, s.kube, func(*dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
			return func(upd *dhcpapi.DHCPSubnet) { delete(upd.Status.Allocated, mac) }, nil
		}); err != nil {
			return withSubnet(s.dropOnError(err))
		}

		// TODO update response? wasn't done in a previous implementation
		return withSubnet(outcome{level: slog.LevelInfo, msg: "Released", reply: true})
	default:
		return withSubnet(drop(slog.LevelError, "unsupported message type"))
	}
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

func (s *Server) dropOnError(err error) outcome {
	switch {
	// overload drops are counted and reported periodically by reportBudget
	case errors.Is(err, errUpdateBudget):
		return drop(slog.LevelDebug, "over the update budget", "err", err)
	case errors.Is(err, context.DeadlineExceeded):
		s.budget.expired.Add(1)

		return drop(slog.LevelDebug, "timed out", "err", err)
	case errors.Is(err, errSubnetDeleted):
		return drop(slog.LevelInfo, "subnet deleted")
	case errors.Is(err, context.Canceled):
		return drop(slog.LevelDebug, "shutting down")
	default:
		return drop(slog.LevelError, "failed", "err", err)
	}
}
