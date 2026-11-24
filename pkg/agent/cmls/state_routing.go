// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"strings"
	"time"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// the remote AS of a neighbor until it's known, if it's configured by kind
	bgpRemoteASInternal = "internal"
	bgpRemoteASExternal = "external"

	bgpSessionOpenSent    = "opensent"
	bgpSessionOpenConfirm = "openconfirm"
	bgpSessionClearing    = "clearing"
	bgpSessionDeleted     = "deleted"

	bfdSessionAdminDown = "admin-down"
	bfdSessionUnknown   = "unknown"

	// the address family the prefix limit is reported for, also without a limit, so a removed one reads 0
	bgpAFIPv4Unicast = "IPV4_UNICAST"
)

// apiIfaceName translates the NOS interface an unnumbered BGP session or BFD session runs over into the port name used
// by the API: swp1 -> E1/1/1, swp17s1 -> E1/17/2 and swp66.1060 -> E1/66.1060. Names we cannot map are returned as
// they are, so an unexpected one shows up in the state instead of being dropped.
func apiIfaceName(name string, portMap map[string]string) string {
	base, sub, hasSub := strings.Cut(name, ".")
	apiName, exists := portMap[base]
	if !exists {
		if strings.HasPrefix(name, nvue.InterfaceTypeSwp) {
			slog.Warn("Port mapping not found for interface-keyed peer", "interface", name)
		}

		return name
	}
	if hasSub {
		return apiName + "." + sub
	}

	return apiName
}

// routedVRFs are the VRFs to report BGP neighbors and BFD peers for, all but the management one
func (c *CumulusProcessor) routedVRFs(ctx context.Context) ([]string, error) {
	vrfs, err := c.nvue.VRFs(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting vrfs: %w", err)
	}

	routed := make([]string, 0, len(vrfs))
	for _, vrf := range vrfs {
		if vrf != nvue.VRFMgmt {
			routed = append(routed, vrf)
		}
	}

	return routed, nil
}

func (c *CumulusProcessor) updateBGPNeighbors(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, vrfs []string, portMap map[string]string, vxLegacy bool) error {
	for _, vrf := range vrfs {
		neighbors, err := c.nvue.BGPNeighbors(ctx, vrf)
		if err != nil && !errors.Is(err, nvue.ErrNotFound) {
			return fmt.Errorf("getting bgp neighbors of vrf %s: %w", vrf, err)
		}
		// the times are reported as ages, as of the response
		now := c.now()

		// a VRF without BGP or without any neighbors is reported empty
		vrfSt := map[string]agentapi.SwitchStateBGPNeighbor{}
		for nosName, neighbor := range neighbors {
			unnumbered, err := bgpNeighborUnnumbered(nosName, &neighbor, vxLegacy)
			if err != nil {
				return fmt.Errorf("bgp neighbor %s in vrf %s: %w", nosName, vrf, err)
			}

			// an unnumbered neighbor is keyed by the interface it runs over
			name := nosName
			if unnumbered {
				name = apiIfaceName(nosName, portMap)
			}

			st, err := bgpNeighborState(reg, vrf, name, &neighbor, now)
			if err != nil {
				return fmt.Errorf("bgp neighbor %s in vrf %s: %w", nosName, vrf, err)
			}
			vrfSt[name] = st
		}

		swState.BGPNeighbors[vrf] = vrfSt
	}

	return nil
}

func bgpNeighborUnnumbered(name string, neighbor *nvue.BGPNeighbor, vxLegacy bool) (bool, error) {
	switch neighbor.Type {
	case nvue.BGPNeighborNumbered:
		return false, nil
	case nvue.BGPNeighborUnnumbered:
		return true, nil
	case "":
		if !vxLegacy {
			return false, errors.New("no type") //nolint:err113
		}

		// Cumulus Linux 5.16 doesn't tell, the unnumbered neighbors are named after their interfaces
		_, err := netip.ParseAddr(name)

		return err != nil, nil
	default:
		return false, fmt.Errorf("unknown type %q", neighbor.Type) //nolint:err113
	}
}

func bgpNeighborState(reg *switchstate.Registry, vrf, name string, neighbor *nvue.BGPNeighbor, now time.Time) (agentapi.SwitchStateBGPNeighbor, error) {
	st := agentapi.SwitchStateBGPNeighbor{
		// the neighbors shut down are listed too, there's no telling them apart but by their state
		Enabled:                true,
		ConnectionsDropped:     neighbor.ConnectionsDropped.Value,
		EstablishedTransitions: neighbor.ConnectionsEstablished.Value,
		LastResetReason:        neighbor.LastResetReason,
		RemoteRouterID:         neighbor.RemoteRouterID,
		Prefixes:               map[string]agentapi.SwitchStateBGPNeighborPrefixes{},
	}

	if neighbor.PeerGroup != nvue.BGPPeerGroupNone {
		st.PeerGroup = neighbor.PeerGroup
	}
	if neighbor.RemotePort.OK {
		st.PeerPort = uint16(min(max(neighbor.RemotePort.Value, 0), math.MaxUint16))
	}
	if neighbor.LocalAS.ASN.OK {
		st.LocalAS = uint32(min(max(neighbor.LocalAS.ASN.Value, 0), math.MaxUint32))
	}

	// the remote AS is a number or, until the session is up, the kind it's configured by, which settles the peer type
	switch {
	case neighbor.RemoteAS.OK:
		st.PeerAS = uint32(min(max(neighbor.RemoteAS.Value, 0), math.MaxUint32))
		st.PeerType = agentapi.BGPPeerTypeExternal
		if st.PeerAS == st.LocalAS {
			st.PeerType = agentapi.BGPPeerTypeInternal
		}
	case neighbor.RemoteAS.Raw == bgpRemoteASInternal:
		st.PeerType = agentapi.BGPPeerTypeInternal
	case neighbor.RemoteAS.Raw == bgpRemoteASExternal:
		st.PeerType = agentapi.BGPPeerTypeExternal
	default:
		slog.Warn("Unknown BGP neighbor remote AS", "neighbor", name, "vrf", vrf, "remoteAS", neighbor.RemoteAS.Raw)
	}

	switch neighbor.SessionState {
	case string(agentapi.BGPNeighborSessionStateIdle), bgpSessionClearing, bgpSessionDeleted:
		st.SessionState = agentapi.BGPNeighborSessionStateIdle
	case string(agentapi.BGPNeighborSessionStateConnect):
		st.SessionState = agentapi.BGPNeighborSessionStateConnect
	case string(agentapi.BGPNeighborSessionStateActive):
		st.SessionState = agentapi.BGPNeighborSessionStateActive
	case bgpSessionOpenSent:
		st.SessionState = agentapi.BGPNeighborSessionStateOpenSent
	case bgpSessionOpenConfirm:
		st.SessionState = agentapi.BGPNeighborSessionStateOpenConfirm
	case nvue.BGPSessionEstablished:
		st.SessionState = agentapi.BGPNeighborSessionStateEstablished
	default:
		return st, fmt.Errorf("unknown session state %q", neighbor.SessionState) //nolint:err113
	}

	// the timers are how long ago, in ms
	if st.SessionState == agentapi.BGPNeighborSessionStateEstablished && neighbor.Uptime.OK && neighbor.Uptime.Value > 0 {
		st.LastEstablished = kmetav1.Time{Time: now.Add(-millis(neighbor.Uptime.Value))}
	}
	if neighbor.LastResetTime.OK && neighbor.LastResetTime.Value > 0 {
		st.LastResetTime = kmetav1.Time{Time: now.Add(-millis(neighbor.LastResetTime.Value))}
	}

	msgs := neighbor.MessageStats
	st.Messages = agentapi.BGPMessages{
		Received: agentapi.BGPMessagesCounters{
			Open:         msgs.RxOpens.Value,
			Keepalive:    msgs.RxKeepalives.Value,
			RouteRefresh: msgs.RxRouteRefreshes.Value,
		},
		Sent: agentapi.BGPMessagesCounters{
			Open:         msgs.TxOpens.Value,
			Keepalive:    msgs.TxKeepalives.Value,
			RouteRefresh: msgs.TxRouteRefreshes.Value,
		},
	}

	for af, afSt := range neighbor.AddressFamily {
		// named like the Broadcom agent reports them, such as ipv4-unicast -> IPV4_UNICAST
		afName := strings.ToUpper(strings.ReplaceAll(af, "-", "_"))
		if !afSt.RxPrefix.OK && !afSt.TxPrefix.OK {
			continue
		}

		prefixes := agentapi.SwitchStateBGPNeighborPrefixes{
			Received: uint32(min(afSt.RxPrefix.Value, math.MaxUint32)),
			Sent:     uint32(min(afSt.TxPrefix.Value, math.MaxUint32)),
		}
		st.Prefixes[afName] = prefixes

		reg.BGPNeighborMetrics.Prefixes.Received.WithLabelValues(vrf, name, afName).Set(float64(prefixes.Received))
		reg.BGPNeighborMetrics.Prefixes.Sent.WithLabelValues(vrf, name, afName).Set(float64(prefixes.Sent))
	}
	reg.BGPNeighborMetrics.Prefixes.Limit.WithLabelValues(vrf, name, bgpAFIPv4Unicast).Set(0)

	peerTypeID, err := st.PeerType.ID()
	if err != nil {
		return st, fmt.Errorf("getting peer type ID: %w", err)
	}
	sessionStateID, err := st.SessionState.ID()
	if err != nil {
		return st, fmt.Errorf("getting session state ID: %w", err)
	}

	reg.BGPNeighborMetrics.Enabled.WithLabelValues(vrf, name).Set(boolToFloat64(st.Enabled))
	reg.BGPNeighborMetrics.ConnectionsDropped.WithLabelValues(vrf, name).Set(float64(st.ConnectionsDropped))
	reg.BGPNeighborMetrics.EstablishedTransitions.WithLabelValues(vrf, name).Set(float64(st.EstablishedTransitions))
	reg.BGPNeighborMetrics.PeerType.WithLabelValues(vrf, name).Set(float64(peerTypeID))
	reg.BGPNeighborMetrics.SessionState.WithLabelValues(vrf, name).Set(float64(sessionStateID))

	reg.BGPNeighborMetrics.Messages.Received.Open.WithLabelValues(vrf, name).Set(float64(st.Messages.Received.Open))
	reg.BGPNeighborMetrics.Messages.Received.Keepalive.WithLabelValues(vrf, name).Set(float64(st.Messages.Received.Keepalive))
	reg.BGPNeighborMetrics.Messages.Received.RouteRefresh.WithLabelValues(vrf, name).Set(float64(st.Messages.Received.RouteRefresh))
	reg.BGPNeighborMetrics.Messages.Sent.Open.WithLabelValues(vrf, name).Set(float64(st.Messages.Sent.Open))
	reg.BGPNeighborMetrics.Messages.Sent.Keepalive.WithLabelValues(vrf, name).Set(float64(st.Messages.Sent.Keepalive))
	reg.BGPNeighborMetrics.Messages.Sent.RouteRefresh.WithLabelValues(vrf, name).Set(float64(st.Messages.Sent.RouteRefresh))

	return st, nil
}

func millis(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}

func (c *CumulusProcessor) updateBFDPeers(ctx context.Context, reg *switchstate.Registry, swState *agentapi.SwitchState, vrfs []string, portMap map[string]string, vxLegacy bool) error {
	for _, vrf := range vrfs {
		peers, err := c.nvue.BFDPeers(ctx, vrf)
		if err != nil && !errors.Is(err, nvue.ErrNotFound) {
			return fmt.Errorf("getting bfd peers of vrf %s: %w", vrf, err)
		}
		// the times are reported as ages, as of the response
		now := c.now()

		for id, peer := range peers {
			// Cumulus Linux 5.16 reports it as a JSON boolean
			if peer.Multihop.Bool && !vxLegacy {
				return fmt.Errorf("bfd peer %s in vrf %s: multihop as a boolean %s", id, vrf, peer.Multihop.Raw) //nolint:err113
			}

			// the multihop sessions of the overlay are reported too, keyed by the VTEP loopbacks like the BGP neighbors
			// TODO decide on the overlay BFD, https://github.com/githedgehog/fabric/issues/1844

			// an unnumbered session runs over an IPv6 link-local address, which is of no use to anyone: key it by the
			// interface instead, the way the BGP neighbor is reported
			name := peer.Peer
			if addr, err := netip.ParseAddr(name); err == nil && addr.Is6() && addr.IsLinkLocalUnicast() && peer.Interface != "" {
				name = apiIfaceName(peer.Interface, portMap)
			}

			st := agentapi.SwitchStateBFDPeer{
				Profile:            peer.Profile,
				FailureTransitions: peer.SessionDown.Value,
			}

			switch peer.State {
			case string(agentapi.BFDSessionStateUp):
				st.SessionState = agentapi.BFDSessionStateUp
			case string(agentapi.BFDSessionStateDown):
				st.SessionState = agentapi.BFDSessionStateDown
			case string(agentapi.BFDSessionStateInit):
				st.SessionState = agentapi.BFDSessionStateInit
			case bfdSessionAdminDown:
				st.SessionState = agentapi.BFDSessionStateAdminDown
			case bfdSessionUnknown:
				st.SessionState = agentapi.BFDSessionStateUnset
			default:
				return fmt.Errorf("bfd peer %s in vrf %s: unknown state %q", name, vrf, peer.State) //nolint:err113
			}

			// up-down-time is how long the session is in its state for, in seconds
			if st.SessionState == agentapi.BFDSessionStateUp && peer.UpDownTime.OK && peer.UpDownTime.Value > 0 {
				st.LastUpTime = kmetav1.Time{Time: now.Add(-time.Duration(peer.UpDownTime.Value * float64(time.Second)))}
			}

			sessionStateID, err := st.SessionState.ID()
			if err != nil {
				return fmt.Errorf("getting bfd session state ID: %w", err)
			}
			reg.BFDPeerMetrics.SessionState.WithLabelValues(vrf, name).Set(float64(sessionStateID))
			reg.BFDPeerMetrics.FailureTransitions.WithLabelValues(vrf, name).Set(float64(st.FailureTransitions))

			if swState.BFDPeers[vrf] == nil {
				swState.BFDPeers[vrf] = map[string]agentapi.SwitchStateBFDPeer{}
			}
			swState.BFDPeers[vrf][name] = st
		}
	}

	return nil
}
