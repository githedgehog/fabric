// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package apiutil

import (
	"context"
	"fmt"
	"strings"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type BGPNeighborStatus struct {
	RemoteName                      string          `json:"remoteName,omitempty"`
	Type                            BGPNeighborType `json:"type,omitempty"`
	Unnumbered                      bool            `json:"unnumbered,omitempty"`
	Expected                        bool            `json:"expected,omitempty"`
	ConnectionName                  string          `json:"connectionName,omitempty"`
	ConnectionType                  string          `json:"connectionType,omitempty"`
	Port                            string          `json:"port,omitempty"`
	agentapi.SwitchStateBGPNeighbor `json:",inline"`
}

type BFDPeerStatus struct {
	RemoteName                  string          `json:"remoteName,omitempty"`
	Type                        BGPNeighborType `json:"type,omitempty"`
	Unnumbered                  bool            `json:"unnumbered,omitempty"`
	Expected                    bool            `json:"expected,omitempty"`
	ConnectionName              string          `json:"connectionName,omitempty"`
	ConnectionType              string          `json:"connectionType,omitempty"`
	Port                        string          `json:"port,omitempty"`
	agentapi.SwitchStateBFDPeer `json:",inline"`
}

type BGPNeighborType string

const (
	BGPNeighborTypeFabric   BGPNeighborType = "fabric"
	BGPNeighborTypeExternal BGPNeighborType = "external"
	BGPNeighborTypeGateway  BGPNeighborType = "gateway"
)

// BGPSwitchStatus is the BGP neighbors of a switch together with the BFD peers, as BFD runs on the BGP sessions
type BGPSwitchStatus struct {
	// vrf -> neighbor key
	Neighbors map[string]map[string]BGPNeighborStatus `json:"neighbors,omitempty"`
	// vrf -> peer address
	BFDPeers map[string]map[string]BFDPeerStatus `json:"bfdPeers,omitempty"`
}

// bgpConnTypes are the only connections with BGP sessions on the switch side
var bgpConnTypes = []string{
	wiringapi.ConnectionTypeFabric,
	wiringapi.ConnectionTypeMesh,
	wiringapi.ConnectionTypeExternal,
	wiringapi.ConnectionTypeGateway,
}

// externalsData is the Externals and their BGP attachments any switch may peer with, loaded once for all of them
type externalsData struct {
	externals map[string]*vpcapi.External
	// only the BGP ones (not static), by connection
	attachments map[string][]*vpcapi.ExternalAttachment
}

func loadExternals(ctx context.Context, kube kclient.Reader, filter SwitchFilter) (*externalsData, error) {
	data := &externalsData{
		externals:   map[string]*vpcapi.External{},
		attachments: map[string][]*vpcapi.ExternalAttachment{},
	}

	// externals and their attachments are per fabric, but the domain is only on the externals and attachments are
	// looked up by connection anyway
	sel := kclient.MatchingLabels(filter.fabricLabels(false))

	extList := &vpcapi.ExternalList{}
	if err := kube.List(ctx, extList, kclient.InNamespace(kmetav1.NamespaceDefault), sel); err != nil {
		return nil, fmt.Errorf("listing externals: %w", err)
	}
	// keyed by every external, including ones with static prefixes: those may still have BGP
	// attachments. Whether a session is expected is decided per attachment.
	for idx := range extList.Items {
		data.externals[extList.Items[idx].Name] = &extList.Items[idx]
	}

	attachList := &vpcapi.ExternalAttachmentList{}
	if err := kube.List(ctx, attachList, kclient.InNamespace(kmetav1.NamespaceDefault), sel); err != nil {
		return nil, fmt.Errorf("listing externalattachments: %w", err)
	}
	for idx := range attachList.Items {
		attach := &attachList.Items[idx]
		if attach.Spec.Static != nil {
			continue
		}

		data.attachments[attach.Spec.Connection] = append(data.attachments[attach.Spec.Connection], attach)
	}

	return data, nil
}

// GetBGPStatus returns the BGP neighbors and BFD peers of the switches selected by the filter, keyed by switch name
func GetBGPStatus(ctx context.Context, kube kclient.Reader, fabCfg *meta.FabricConfig, filter SwitchFilter) (map[string]*BGPSwitchStatus, error) {
	if fabCfg == nil {
		return nil, fmt.Errorf("fabric config is nil") //nolint:err113
	}

	if err := filter.check(ctx, kube); err != nil {
		return nil, err
	}

	connTypes, err := labels.NewRequirement(wiringapi.LabelConnectionType, selection.In, bgpConnTypes)
	if err != nil {
		return nil, fmt.Errorf("building connection type selector: %w", err)
	}

	exts, err := loadExternals(ctx, kube, filter)
	if err != nil {
		return nil, err
	}

	out := map[string]*BGPSwitchStatus{}
	if err := forEachSwitch(ctx, kube, filter, []labels.Requirement{*connTypes}, func(in *switchInput) error {
		neighs, err := bgpNeighbors(ctx, exts, in)
		if err != nil {
			return fmt.Errorf("getting BGP neighbors for switch %s: %w", in.sw.Name, err)
		}

		out[in.sw.Name] = &BGPSwitchStatus{
			Neighbors: neighs,
			BFDPeers:  bfdPeers(in.ag, neighs),
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return out, nil
}

// bgpNeighborKey returns the local port as the agent names it and the key the agent reports the
// session over the given link under: the peer IP for a numbered link, the local port for an
// unnumbered one, since that is what the session is keyed by. On TH5 the peering runs over the
// workaround SVI, reported as port.vlan. Only the workaround VLANs come from the agent, they are
// allocated in its catalog.
func bgpNeighborKey(sp *wiringapi.SwitchProfile, ag *agentapi.Agent, local wiringapi.ConnFabricLinkSwitch, remoteIP string) (string, string, error) {
	// a breakout-capable port is E1/53 in the wiring but E1/53/1 to the agent, which resolves
	// interfaces through the NOS port mapping
	wiringPort := local.LocalPortName()
	port, err := sp.Spec.NormalizePortName(wiringPort)
	if err != nil {
		return "", "", fmt.Errorf("normalizing port name %s: %w", wiringPort, err)
	}

	if remoteIP != "" {
		return port, strings.Split(remoteIP, "/")[0], nil
	}

	if sp.Spec.SwitchSilicon == switchprofile.SiliconBroadcomTH5 {
		// the catalog keys the workaround VLANs by the port name as the wiring spells it, and the
		// agent reports the SVI under that same name
		if vlan, ok := ag.Spec.Catalog.TH5WorkaroundVLANs[wiringPort]; ok {
			return port, fmt.Sprintf("%s.%d", wiringPort, vlan), nil
		}
	}

	return port, port, nil
}

func bgpNeighbors(ctx context.Context, exts *externalsData, in *switchInput) (map[string]map[string]BGPNeighborStatus, error) {
	sw, ag := in.sw, in.ag

	_, sp, err := in.cache.switchProfile(ctx, sw.Name)
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]BGPNeighborStatus{}

	for vrf, vrfNeighbors := range ag.Status.State.BGPNeighbors {
		out[vrf] = map[string]BGPNeighborStatus{}
		for name, neighbor := range vrfNeighbors {
			out[vrf][name] = BGPNeighborStatus{
				SwitchStateBGPNeighbor: neighbor,
			}
		}
	}

	extConns := []*wiringapi.Connection{}

	if out["default"] == nil {
		out["default"] = map[string]BGPNeighborStatus{}
	}

	fabricPeers := make(map[string]bool)

	for _, conn := range in.conns {
		if conn.Spec.Fabric != nil { //nolint:gocritic
			for _, link := range conn.Spec.Fabric.Links {
				curr, other := link.Spine, link.Leaf
				if sw.Name == other.DeviceName() {
					curr, other = other, curr
				} else if sw.Name != curr.DeviceName() {
					continue
				}
				fabricPeers[other.DeviceName()] = true

				port, key, err := bgpNeighborKey(sp, ag, curr, other.IP)
				if err != nil {
					return nil, fmt.Errorf("fabric connection %s: %w", conn.Name, err)
				}

				neigh, ok := out["default"][key]
				if !ok {
					neigh = BGPNeighborStatus{}
				}

				neigh.RemoteName = other.Port
				neigh.Type = BGPNeighborTypeFabric
				neigh.Unnumbered = other.IP == ""
				neigh.Expected = true
				neigh.ConnectionName = conn.Name
				neigh.ConnectionType = conn.Spec.Type()
				neigh.Port = port

				out["default"][key] = neigh
			}
		} else if conn.Spec.Mesh != nil {
			for _, link := range conn.Spec.Mesh.Links {
				curr, other := link.Leaf1, link.Leaf2
				if sw.Name == other.DeviceName() {
					curr, other = other, curr
				} else if sw.Name != curr.DeviceName() {
					continue
				}
				fabricPeers[other.DeviceName()] = true

				port, key, err := bgpNeighborKey(sp, ag, curr, other.IP)
				if err != nil {
					return nil, fmt.Errorf("mesh connection %s: %w", conn.Name, err)
				}

				neigh, ok := out["default"][key]
				if !ok {
					neigh = BGPNeighborStatus{}
				}

				neigh.RemoteName = other.Port
				neigh.Type = BGPNeighborTypeFabric
				neigh.Unnumbered = other.IP == ""
				neigh.Expected = true
				neigh.ConnectionName = conn.Name
				neigh.ConnectionType = conn.Spec.Type()
				neigh.Port = port

				out["default"][key] = neigh
			}
		} else if conn.Spec.External != nil {
			extConns = append(extConns, conn)
		} else if conn.Spec.Gateway != nil {
			for _, link := range conn.Spec.Gateway.Links {
				port, key, err := bgpNeighborKey(sp, ag, link.Switch, link.Gateway.IP)
				if err != nil {
					return nil, fmt.Errorf("gateway connection %s: %w", conn.Name, err)
				}

				neigh, ok := out["default"][key]
				if !ok {
					neigh = BGPNeighborStatus{}
				}

				neigh.RemoteName = link.Gateway.Port
				neigh.Type = BGPNeighborTypeGateway
				neigh.Unnumbered = link.Gateway.IP == ""
				neigh.Expected = true
				neigh.ConnectionName = conn.Name
				neigh.ConnectionType = conn.Spec.Type()
				neigh.Port = port

				out["default"][key] = neigh
			}
		}
	}

	for peer := range fabricPeers {
		peerSw, err := in.cache.switchByName(ctx, peer)
		if err != nil {
			return nil, fmt.Errorf("peer: %w", err)
		}
		if peerSw.Spec.ProtocolIP == "" {
			return nil, fmt.Errorf("no protocol IP found for peer %s", peer) //nolint:goerr113
		}
		ip := strings.Split(peerSw.Spec.ProtocolIP, "/")[0]
		neigh, ok := out["default"][ip]
		if !ok {
			neigh = BGPNeighborStatus{}
		}

		neigh.RemoteName = peer
		neigh.Type = BGPNeighborTypeFabric
		neigh.Expected = true
		neigh.Port = "Lo"
		out["default"][ip] = neigh
	}

	for _, conn := range extConns {
		if conn.Spec.External.Link.Switch.DeviceName() != sw.Name {
			continue
		}

		for _, extAtt := range exts.attachments[conn.Name] {
			ext, ok := exts.externals[extAtt.Spec.External]
			if !ok {
				return nil, fmt.Errorf("external %s not found", extAtt.Spec.External) //nolint:goerr113
			}

			// TODO dedup with agent code
			vrf := "VrfE" + ext.Name
			if _, ok := out[vrf]; !ok {
				out[vrf] = map[string]BGPNeighborStatus{}
			}
			neigh, ok := out[vrf][extAtt.Spec.Neighbor.IP]
			if !ok {
				out[vrf][extAtt.Spec.Neighbor.IP] = BGPNeighborStatus{}
			}

			port, err := sp.Spec.NormalizePortName(conn.Spec.External.Link.Switch.LocalPortName())
			if err != nil {
				return nil, fmt.Errorf("external connection %s: %w", conn.Name, err)
			}

			neigh.RemoteName = ext.Name
			neigh.Expected = true
			neigh.Type = BGPNeighborTypeExternal
			neigh.Port = port
			neigh.ConnectionName = conn.Name
			neigh.ConnectionType = conn.Spec.Type()

			out[vrf][extAtt.Spec.Neighbor.IP] = neigh
		}
	}

	return out, nil
}

// bfdPeers enriches the BFD peers the agent reports with the connection metadata of the BGP neighbors and makes sure
// the expected ones are present even when there is no BFD session in the switch state
func bfdPeers(ag *agentapi.Agent, bgpNeighbors map[string]map[string]BGPNeighborStatus) map[string]map[string]BFDPeerStatus {
	out := map[string]map[string]BFDPeerStatus{}
	for vrf, vrfPeers := range ag.Status.State.BFDPeers {
		out[vrf] = map[string]BFDPeerStatus{}
		for addr, peer := range vrfPeers {
			out[vrf][addr] = BFDPeerStatus{
				SwitchStateBFDPeer: peer,
			}
		}
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
			peer.Unnumbered = bgpNeighbor.Unnumbered
			peer.Expected = bgpNeighbor.Expected
			peer.ConnectionName = bgpNeighbor.ConnectionName
			peer.ConnectionType = bgpNeighbor.ConnectionType
			peer.Port = bgpNeighbor.Port
			out[vrf][addr] = peer
		}
	}

	return out
}
