// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bcm

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

func planVirtualEdge(agent *agentapi.Agent, spec *dozer.Spec) error {
	annotations := agent.GetAnnotations()
	if annotations == nil {
		return errors.Errorf("no annotation")
	}

	cfgMap := map[string]agentapi.VirtualEdgeConfig{}
	edgeAnnotation := annotations[agentapi.VIRTUAL_EDGE_ANNOTATION]
	if edgeAnnotation == "" {
		return nil
	}
	err := json.Unmarshal([]byte(edgeAnnotation), &cfgMap)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal annotation %s", agentapi.VIRTUAL_EDGE_ANNOTATION)
	}

	for _, externalConfig := range cfgMap {
		// Create VRF
		ipnsVrf := ipnsVrfName(externalConfig.VRF)
		if spec.VRFs[ipnsVrf] == nil {
			protocolIP, _, err := net.ParseCIDR(agent.Spec.Switch.ProtocolIP)
			if err != nil {
				return errors.Wrapf(err, "failed to parse protocol ip %s", agent.Spec.Switch.ProtocolIP)
			}

			spec.VRFs[ipnsVrf] = &dozer.SpecVRF{
				Enabled: boolPtr(true),
				// Description:      stringPtr(fmt.Sprintf("IPv4NS %s", external.IPv4Namespace)),
				AnycastMAC:       stringPtr(ANYCAST_MAC),
				Interfaces:       map[string]*dozer.SpecVRFInterface{},
				StaticRoutes:     map[string]*dozer.SpecVRFStaticRoute{},
				TableConnections: map[string]*dozer.SpecVRFTableConnection{},
				BGP: &dozer.SpecVRFBGP{
					AS:                 uint32Ptr(agent.Spec.Switch.ASN),
					RouterID:           stringPtr(protocolIP.String()),
					NetworkImportCheck: boolPtr(true),
					IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
						Enabled:    true,
						MaxPaths:   uint32Ptr(getMaxPaths(agent)),
						Networks:   map[string]*dozer.SpecVRFBGPNetwork{},
						ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
					},
					Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{},
				},
			}
		}

		spec.CommunityLists[extInboundCommListName(ipnsVrf)] = &dozer.SpecCommunityList{
			Members: []string{externalConfig.CommunityIn},
		}

		spec.RouteMaps[extInboundRouteMapName(ipnsVrf)] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					Conditions: dozer.SpecRouteMapConditions{
						MatchCommunityList: stringPtr(extInboundCommListName(ipnsVrf)),
					},
					Result: dozer.SpecRouteMapResultAccept,
				},
				"100": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}

		spec.RouteMaps[extOutboundRouteMapName(ipnsVrf)] = &dozer.SpecRouteMap{
			Statements: map[string]*dozer.SpecRouteMapStatement{
				"10": {
					SetCommunities: []string{externalConfig.CommunityOut},
					Result:         dozer.SpecRouteMapResultAccept,
				},
				"100": {
					Result: dozer.SpecRouteMapResultReject,
				},
			},
		}

		vlanVal, err := strconv.ParseUint(externalConfig.IfVlan, 10, 16)
		if err != nil {
			return errors.Wrapf(err, "failed to parse external attach switch vlan %s", externalConfig.IfVlan)
		}

		vlan := uint16Ptr(uint16(vlanVal))
		ip, ipNet, err := net.ParseCIDR(externalConfig.IfIP)
		if err != nil {
			return errors.Wrapf(err, "failed to parse external attach switch ip %s", externalConfig.IfIP)
		}
		prefixLength, _ := ipNet.Mask.Size()

		spec.Interfaces[externalConfig.IfName] = &dozer.SpecInterface{
			Enabled:     boolPtr(true),
			Description: stringPtr(fmt.Sprintf("Virtual External %s", externalConfig.VRF)),
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{
				uint32(vlanVal): {
					VLAN: vlan,
					IPs: map[string]*dozer.SpecInterfaceIP{
						ip.String(): {
							PrefixLen: uint8Ptr(uint8(prefixLength)),
						},
					},
				},
			},
		}

		subIfaceName := fmt.Sprintf("%s.%s", externalConfig.IfName, externalConfig.IfVlan)
		asnVal, _ := strconv.ParseUint(externalConfig.ASN, 10, 32)

		spec.VRFs[ipnsVrf].Interfaces[subIfaceName] = &dozer.SpecVRFInterface{}
		spec.VRFs[ipnsVrf].BGP.Neighbors[externalConfig.NeighborIP] = &dozer.SpecVRFBGPNeighbor{
			Enabled:                   boolPtr(true),
			Description:               stringPtr(fmt.Sprintf("External attach %s", externalConfig.VRF)),
			RemoteAS:                  uint32Ptr(uint32(asnVal)),
			IPv4Unicast:               boolPtr(true),
			IPv4UnicastImportPolicies: []string{extInboundRouteMapName(ipnsVrf)},
			IPv4UnicastExportPolicies: []string{extOutboundRouteMapName(ipnsVrf)},
		}
		spec.VRFs[ipnsVrf].TableConnections = map[string]*dozer.SpecVRFTableConnection{
			dozer.SpecVRFBGPTableConnectionConnected: {},
			dozer.SpecVRFBGPTableConnectionStatic:    {},
		}

	}

	return nil
}
