// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

type BFDPeer struct {
	// Peer and Local are the addresses, link-local ones for the sessions of unnumbered BGP neighbors
	Peer      string `json:"peer"`
	Local     string `json:"local"`
	Interface string `json:"interface"`
	Profile   string `json:"profile"`
	State     string `json:"state"`
	Multihop  Toggle `json:"multihop"`
	// UpDownTime is how long the session is in its state for, in seconds
	UpDownTime       Number  `json:"up-down-time"`
	SessionUp        Counter `json:"session-up"`
	SessionDown      Counter `json:"session-down"`
	DetectMultiplier Number  `json:"detect-multiplier"`
	// MinRxInterval and MinTxInterval are in ms
	MinRxInterval Number `json:"min-rx-interval"`
	MinTxInterval Number `json:"min-tx-interval"`
}

// BFDPeers returns the BFD sessions of a VRF in full in one request, keyed by their local discriminator
func (c *Client) BFDPeers(ctx context.Context, vrf string) (map[string]BFDPeer, error) {
	peers := map[string]BFDPeer{}
	if err := c.get(ctx, pathOf("vrf", vrf, "router", "bfd", "peers"), nil, &peers); err != nil {
		return nil, err
	}

	return peers, nil
}
