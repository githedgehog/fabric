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

type BGPNeighborType string

const (
	BGPNeighborTypeFabric   BGPNeighborType = "fabric"
	BGPNeighborTypeExternal BGPNeighborType = "external"
	BGPNeighborTypeGateway  BGPNeighborType = "gateway"
)

// bgpNeighborKey returns the key the agent reports the session over the given link under: the peer
// IP for a numbered link, the local port for an unnumbered one, since that is what the session is
// keyed by. On TH5 the peering runs over the workaround SVI, reported as port.vlan.
func bgpNeighborKey(ag *agentapi.Agent, local wiringapi.ConnFabricLinkSwitch, remoteIP string) (string, error) {
	if remoteIP != "" {
		return strings.Split(remoteIP, "/")[0], nil
	}

	if ag.Spec.SwitchProfile == nil {
		return "", fmt.Errorf("switch profile is not set for %s", ag.Name) //nolint:goerr113
	}

	port := local.LocalPortName()

	if ag.Spec.SwitchProfile.SwitchSilicon == switchprofile.SiliconBroadcomTH5 {
		if vlan, ok := ag.Spec.Catalog.TH5WorkaroundVLANs[port]; ok {
			return fmt.Sprintf("%s.%d", port, vlan), nil
		}
	}

	return port, nil
}

func GetBGPNeighbors(ctx context.Context, kube kclient.Reader, fabCfg *meta.FabricConfig, sw *wiringapi.Switch) (map[string]map[string]BGPNeighborStatus, error) {
	if sw == nil {
		return nil, fmt.Errorf("switch is nil") //nolint:goerr113
	}
	if fabCfg == nil {
		return nil, fmt.Errorf("fabric config is nil") //nolint:goerr113
	}

	out := map[string]map[string]BGPNeighborStatus{}

	ag := &agentapi.Agent{}
	if err := kube.Get(ctx, kclient.ObjectKey{Name: sw.Name, Namespace: sw.Namespace}, ag); err != nil {
		return nil, fmt.Errorf("getting agent %s: %w", sw.Name, err)
	}

	for vrf, vrfNeighbors := range ag.Status.State.BGPNeighbors {
		out[vrf] = map[string]BGPNeighborStatus{}
		for name, neighbor := range vrfNeighbors {
			out[vrf][name] = BGPNeighborStatus{
				SwitchStateBGPNeighbor: neighbor,
			}
		}
	}

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return nil, fmt.Errorf("listing switches: %w", err)
	}

	switches := map[string]*wiringapi.Switch{}
	for _, sw := range swList.Items {
		switches[sw.Name] = &sw
	}

	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := kube.List(ctx, extAttachments); err != nil {
		return nil, fmt.Errorf("listing externalattachments: %w", err)
	}

	extList := &vpcapi.ExternalList{}
	if err := kube.List(ctx, extList); err != nil {
		return nil, fmt.Errorf("listing externals: %w", err)
	}

	exts := map[string]*vpcapi.External{}
	for _, ext := range extList.Items {
		if ext.Spec.Static != nil {
			continue
		}
		exts[ext.Name] = &ext
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, wiringapi.MatchingLabelsForListLabelSwitch(sw.Name)); err != nil {
		return nil, fmt.Errorf("listing connections: %w", err)
	}

	extConns := map[string]*wiringapi.Connection{}

	if out["default"] == nil {
		out["default"] = map[string]BGPNeighborStatus{}
	}

	fabricPeers := make(map[string]bool)

	for _, conn := range conns.Items {
		if conn.Spec.Fabric != nil { //nolint:gocritic
			for _, link := range conn.Spec.Fabric.Links {
				curr, other := link.Spine, link.Leaf
				if sw.Name == other.DeviceName() {
					curr, other = other, curr
				} else if sw.Name != curr.DeviceName() {
					continue
				}
				fabricPeers[other.DeviceName()] = true

				key, err := bgpNeighborKey(ag, curr, other.IP)
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
				neigh.Port = curr.LocalPortName()

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

				key, err := bgpNeighborKey(ag, curr, other.IP)
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
				neigh.Port = curr.LocalPortName()

				out["default"][key] = neigh
			}
		} else if conn.Spec.External != nil {
			extConns[conn.Name] = &conn
		} else if conn.Spec.Gateway != nil {
			for _, link := range conn.Spec.Gateway.Links {
				key, err := bgpNeighborKey(ag, link.Switch, link.Gateway.IP)
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
				neigh.Port = link.Switch.LocalPortName()

				out["default"][key] = neigh
			}
		}
	}

	for peer := range fabricPeers {
		peerSpec, ok := ag.Spec.Switches[peer]
		if !ok {
			return nil, fmt.Errorf("no switch found for peer %s", peer) //nolint:goerr113
		}
		if peerSpec.ProtocolIP == "" {
			return nil, fmt.Errorf("no protocol IP found for peer %s", peer) //nolint:goerr113
		}
		ip := strings.Split(peerSpec.ProtocolIP, "/")[0]
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

	for _, extAtt := range extAttachments.Items {
		if extAtt.Spec.Static != nil {
			continue
		}
		conn, ok := extConns[extAtt.Spec.Connection]
		if !ok || conn.Spec.External == nil {
			continue
		}

		if conn.Spec.External.Link.Switch.DeviceName() != sw.Name {
			continue
		}

		ext, ok := exts[extAtt.Spec.External]
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

		neigh.RemoteName = ext.Name
		neigh.Expected = true
		neigh.Type = BGPNeighborTypeExternal
		neigh.Port = conn.Spec.External.Link.Switch.LocalPortName()
		neigh.ConnectionName = conn.Name
		neigh.ConnectionType = conn.Spec.Type()

		out[vrf][extAtt.Spec.Neighbor.IP] = neigh
	}

	return out, nil
}
