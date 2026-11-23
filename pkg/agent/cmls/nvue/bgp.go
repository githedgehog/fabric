// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"net/url"
)

const (
	BGPNeighborNumbered   = "numbered"
	BGPNeighborUnnumbered = "unnumbered"

	BGPSessionEstablished = "established"

	// BGPPeerGroupNone is the peer group of a neighbor without one
	BGPPeerGroupNone = "none"
)

type BGPNeighbor struct {
	// Type is BGPNeighborNumbered or BGPNeighborUnnumbered, an unnumbered neighbor is named after its interface
	Type        string `json:"type"`
	Description string `json:"description"`
	PeerGroup   string `json:"peer-group"`
	// RemoteAS is the AS of the peer, or its configured kind such as "external" until it's known
	RemoteAS       Number             `json:"remote-as"`
	LocalAS        BGPNeighborLocalAS `json:"local-as"`
	RemoteRouterID string             `json:"remote-router-id"`
	RemoteHostname string             `json:"remote-hostname"`
	RemotePort     Number             `json:"remote-port"`

	SessionState string `json:"session-state"`
	// Uptime is how long the session is established for and LastResetTime how long ago it was reset, in ms
	Uptime                 Number  `json:"uptime"`
	LastResetTime          Number  `json:"last-reset-time"`
	LastResetReason        string  `json:"last-reset-reason"`
	ConnectionsEstablished Counter `json:"connections-established"`
	ConnectionsDropped     Counter `json:"connections-dropped"`

	MessageStats  BGPMessageStats                     `json:"message-stats"`
	AddressFamily map[string]BGPNeighborAddressFamily `json:"address-family"`
	BFD           *BGPNeighborBFD                     `json:"bfd"`
}

type BGPNeighborLocalAS struct {
	ASN Number `json:"asn"`
}

type BGPMessageStats struct {
	RxOpens          Counter `json:"rx-opens"`
	RxKeepalives     Counter `json:"rx-keepalives"`
	RxRouteRefreshes Counter `json:"rx-route-refreshes"`
	RxTotal          Counter `json:"rx-total"`
	TxOpens          Counter `json:"tx-opens"`
	TxKeepalives     Counter `json:"tx-keepalives"`
	TxRouteRefreshes Counter `json:"tx-route-refreshes"`
	TxTotal          Counter `json:"tx-total"`
}

// BGPNeighborAddressFamily has the prefixes exchanged with the neighbor in an address family such as "ipv4-unicast" or
// "l2vpn-evpn"
type BGPNeighborAddressFamily struct {
	RxPrefix Counter `json:"rx-prefix"`
	TxPrefix Counter `json:"tx-prefix"`
}

type BGPNeighborBFD struct {
	Profile string `json:"profile"`
	Status  string `json:"status"`
}

// BGPNeighbors returns the BGP neighbors of a VRF in full in one request, by their address or, for the unnumbered ones,
// their interface. It's ErrNotFound for a VRF without BGP or without any neighbors.
func (c *Client) BGPNeighbors(ctx context.Context, vrf string) (map[string]BGPNeighbor, error) {
	neighbors := map[string]BGPNeighbor{}
	path := pathOf("vrf", vrf, "router", "bgp", "neighbor")
	if err := c.get(ctx, path, url.Values{"view": {"detail"}}, &neighbors); err != nil {
		return nil, err
	}

	return neighbors, nil
}
