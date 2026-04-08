// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil

import (
	"context"
	"fmt"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type BFDPeerStatus struct {
	RemoteName                  string          `json:"remoteName,omitempty"`
	Type                        BGPNeighborType `json:"type,omitempty"`
	Expected                    bool            `json:"expected,omitempty"`
	ConnectionName              string          `json:"connectionName,omitempty"`
	ConnectionType              string          `json:"connectionType,omitempty"`
	Port                        string          `json:"port,omitempty"`
	agentapi.SwitchStateBFDPeer `json:",inline"`
}

func GetBFDPeers(ctx context.Context, kube kclient.Reader, fabCfg *meta.FabricConfig, sw *wiringapi.Switch) (map[string]map[string]BFDPeerStatus, error) {
	if sw == nil {
		return nil, fmt.Errorf("switch is nil") //nolint:goerr113
	}
	if fabCfg == nil {
		return nil, fmt.Errorf("fabric config is nil") //nolint:goerr113
	}

	ag := &agentapi.Agent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: sw.Name, Namespace: sw.Namespace}, ag); err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", sw.Name, err)
	}

	out := map[string]map[string]BFDPeerStatus{}
	for vrf, vrfPeers := range ag.Status.State.BFDPeers {
		out[vrf] = map[string]BFDPeerStatus{}
		for addr, peer := range vrfPeers {
			out[vrf][addr] = BFDPeerStatus{
				SwitchStateBFDPeer: peer,
			}
		}
	}

	// Enrich BFD peers with connection metadata from BGP neighbors and ensure
	// expected neighbors are present even when no BFD session exists in switch state.
	bgpNeighbors, err := GetBGPNeighbors(ctx, kube, fabCfg, sw)
	if err != nil {
		return nil, fmt.Errorf("getting BGP neighbors for BFD enrichment: %w", err)
	}

	for vrf, bgpVRF := range bgpNeighbors {
		if _, ok := out[vrf]; !ok {
			out[vrf] = map[string]BFDPeerStatus{}
		}

		for addr, bgpNeighbor := range bgpVRF {
			// Skip BGP neighbors that don't run BFD: loopbacks and externals
			if bgpNeighbor.Port == "" || bgpNeighbor.Port == "Lo" || bgpNeighbor.Type == BGPNeighborTypeExternal {
				continue
			}

			peer := out[vrf][addr]
			peer.RemoteName = bgpNeighbor.RemoteName
			peer.Type = bgpNeighbor.Type
			peer.Expected = bgpNeighbor.Expected
			peer.ConnectionName = bgpNeighbor.ConnectionName
			peer.ConnectionType = bgpNeighbor.ConnectionType
			peer.Port = bgpNeighbor.Port
			out[vrf][addr] = peer
		}
	}

	return out, nil
}
