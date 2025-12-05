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
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specVRFsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRF]{
	Summary:      "VRFs",
	ValueHandler: specVRFEnforcer,
}

var specVRFEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRF]{
	Summary: "VRF %s",
	CustomHandler: func(basePath string, name string, actual, desired *dozer.SpecVRF, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/network-instances/network-instance[name=%s]", name)

		if err := specVRFBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf base")
		}

		if err := specVRFSAGEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf sag")
		}

		if err := specVRFEVPNMHEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf evpn mh")
		}

		actualInterfaces, desiredInterfaces := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRF) map[string]*dozer.SpecVRFInterface { return value.Interfaces })
		if err := specVRFInterfacesEnforcer.Handle(basePath, actualInterfaces, desiredInterfaces, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf interfaces")
		}

		actualBGP, desiredBGP := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRF) *dozer.SpecVRFBGP { return value.BGP })
		if err := specVRFBGPEnforcer.Handle(basePath, name, actualBGP, desiredBGP, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp")
		}

		actualTableConnections, desiredTableConnections := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRF) map[string]*dozer.SpecVRFTableConnection { return value.TableConnections })
		if err := specVRFTableConnectionsEnforcer.Handle(basePath, actualTableConnections, desiredTableConnections, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf table connections")
		}

		actualStaticRoutes, desiredStaticRoutes := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRF) map[string]*dozer.SpecVRFStaticRoute { return value.StaticRoutes })
		if err := specVRFStaticRoutesEnforcer.Handle(basePath, actualStaticRoutes, desiredStaticRoutes, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf static routes")
		}

		actualEthernetSegments, desiredEthernetSegments := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRF) map[string]*dozer.SpecVRFEthernetSegment { return value.EthernetSegments })
		if err := specVRFEthernetSegmentsEnforcer.Handle(basePath, actualEthernetSegments, desiredEthernetSegments, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf ethernet segments")
		}

		actualAttachedHosts, desiredAttachedHosts := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRF) map[string]*dozer.SpecVRFAttachedHost { return value.AttachedHosts })
		if err := specVRFAttachedHostsEnforcer.Handle(basePath, actualAttachedHosts, desiredAttachedHosts, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf attached hosts")
		}

		return nil
	},
}

var specVRFBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRF]{
	Summary: "VRF %s base",
	Getter: func(_ string, value *dozer.SpecVRF) any {
		return []any{value.Enabled, value.Description}
	},
	UpdateWeight: ActionWeightVRFBaseUpdate,
	DeleteWeight: ActionWeightVRFBaseDelete,
	Marshal: func(name string, value *dozer.SpecVRF) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances{
			NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
				name: {
					Name: pointer.To(name),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config{
						Name:        pointer.To(name),
						Enabled:     value.Enabled,
						Description: value.Description,
					},
				},
			},
		}, nil
	},
}

var specVRFEVPNMHEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRF]{
	Summary: "VRF %s EVPNMH",
	Getter: func(_ string, value *dozer.SpecVRF) any {
		return []any{value.EVPNMH}
	},
	MutateActual: func(_ string, actual *dozer.SpecVRF) *dozer.SpecVRF {
		if actual != nil && actual.EVPNMH.StartupDelay == nil && actual.EVPNMH.MACHoldtime == nil {
			return nil
		}

		return actual
	},
	Path:         "/evpn/evpn-mh/config",
	UpdateWeight: ActionWeightVRFEVPNMHUpdate,
	DeleteWeight: ActionWeightVRFEVPNMHDelete,
	Marshal: func(_ string, value *dozer.SpecVRF) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Evpn_EvpnMh{
			Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Evpn_EvpnMh_Config{
				MacHoldtime:  value.EVPNMH.MACHoldtime,
				StartupDelay: value.EVPNMH.StartupDelay,
			},
		}, nil
	},
}

var specVRFEthernetSegmentsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFEthernetSegment]{
	Summary:      "VRF ethernet segments",
	ValueHandler: specVRFEVPNEthernetSegmentEnforcer,
}

var specVRFEVPNEthernetSegmentEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFEthernetSegment]{
	Summary:      "VRF %s EVPN ethernet segment",
	Path:         "/evpn/ethernet-segments/ethernet-segment[name=%s]",
	UpdateWeight: ActionWeightVRFEthernetSegmentUpdate,
	DeleteWeight: ActionWeightVRFEthernetSegmentDelete,
	Marshal: func(name string, value *dozer.SpecVRFEthernetSegment) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Evpn_EthernetSegments{
			EthernetSegment: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Evpn_EthernetSegments_EthernetSegment{
				name: {
					Name: pointer.To(name),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Evpn_EthernetSegments_EthernetSegment_Config{
						Name:      pointer.To(name),
						EsiType:   oc.OpenconfigEvpn_EsiType_TYPE_0_OPERATOR_CONFIGURED,
						Esi:       oc.UnionString(value.ESI),
						Interface: pointer.To(name),
					},
				},
			},
		}, nil
	},
}

var specVRFSAGEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRF]{
	Summary: "VRF %s SAG",
	Getter: func(_ string, value *dozer.SpecVRF) any {
		return []any{value.AnycastMAC}
	},
	Path:         "/global-sag/config",
	SkipDelete:   true, // TODO check if it's ok
	UpdateWeight: ActionWeightVRFSAGUpdate,
	DeleteWeight: ActionWeightVRFSAGDelete,
	Marshal: func(_ string, value *dozer.SpecVRF) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_GlobalSag{
			Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_GlobalSag_Config{
				AnycastMac: value.AnycastMAC,
			},
		}, nil
	},
}

var specVRFInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFInterface]{
	Summary:      "VRF %s interfaces",
	ValueHandler: specVRFInterfaceEnforcer,
}

// TODO check it works correctly
var specVRFInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFInterface]{
	Summary:      "VRF interface %s",
	Path:         "/interfaces/interface[id=%s]",
	UpdateWeight: ActionWeightVRFInterfaceUpdate,
	DeleteWeight: ActionWeightVRFInterfaceDelete,
	Marshal: func(iface string, _ *dozer.SpecVRFInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces{
			Interface: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface{
				iface: {
					Id: pointer.To(iface),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface_Config{
						Id: pointer.To(iface),
					},
				},
			},
		}, nil
	},
}

var specVRFBGPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF BGP",
	CustomHandler: func(basePath string, name string, actual, desired *dozer.SpecVRFBGP, actions *ActionQueue) error {
		basePath += "/protocols/protocol[identifier=BGP][name=bgp]/bgp"

		if err := specVRFBGPBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp base")
		}

		if err := specVRFBGPL2VPNEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp l2vpn")
		}

		if err := specVRFImportVrfEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp import vrfs")
		}

		if err := specVRFImportPolicyEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp import policy")
		}

		actualNeighbors, desiredNeighbors := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRFBGP) map[string]*dozer.SpecVRFBGPNeighbor { return value.Neighbors })
		if err := specVRFBGPNeighborsEnforcer.Handle(basePath, actualNeighbors, desiredNeighbors, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp neighbors")
		}

		actualNetworks, desiredNetworks := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRFBGP) map[string]*dozer.SpecVRFBGPNetwork { return value.IPv4Unicast.Networks })
		if err := specVRFBGPNetworksEnforcer.Handle(basePath, actualNetworks, desiredNetworks, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp networks")
		}

		return nil
	},
}

var specVRFBGPBaseEnforcerGetter = func(_ string, value *dozer.SpecVRFBGP) any {
	return []any{
		value.AS, value.RouterID, value.NetworkImportCheck,
		// value.IPv4Unicast, // TODO it's probably not enough for some cases, check if current approach is ok
		value.IPv4Unicast.Enabled,
		value.IPv4Unicast.MaxPaths,
		value.IPv4Unicast.MaxPathsIBGP,
		value.IPv4Unicast.TableMap,
	}
}

var specVRFBGPBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary:      "VRF %s BGP base",
	Getter:       specVRFBGPBaseEnforcerGetter,
	UpdateWeight: ActionWeightVRFBGPBaseUpdate,
	DeleteWeight: ActionWeightVRFBGPBaseDelete,
	NoReplace:    true, // it should be okay as we aren't expecting to remove any of the configs
	Marshal: func(_ string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		afiSafi := map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{}
		if value.IPv4Unicast.Enabled {
			ipv4Unicast := &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{
				AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
					AfiSafiName:  oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
					TableMapName: value.IPv4Unicast.TableMap,
				},
			}

			if value.IPv4Unicast.MaxPaths != nil || value.IPv4Unicast.MaxPathsIBGP != nil {
				ipv4Unicast.UseMultiplePaths = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths{}

				if value.IPv4Unicast.MaxPaths != nil {
					ipv4Unicast.UseMultiplePaths.Ebgp = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths_Ebgp{
						Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths_Ebgp_Config{
							MaximumPaths: value.IPv4Unicast.MaxPaths,
						},
					}
				}

				if value.IPv4Unicast.MaxPathsIBGP != nil {
					ipv4Unicast.UseMultiplePaths.Ibgp = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths_Ibgp{
						Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths_Ibgp_Config{
							MaximumPaths: value.IPv4Unicast.MaxPathsIBGP,
						},
					}
				}
			}

			afiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST] = ipv4Unicast
		}

		var as oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config_As_Union
		if value.AS != nil {
			as = oc.UnionUint32(*value.AS)
		}

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
			Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
				Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config{
						As:                 as,
						RouterId:           value.RouterID,
						NetworkImportCheck: value.NetworkImportCheck,
					},
					AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis{
						AfiSafi: afiSafi,
					},
				},
			},
		}, nil
	},
}

var specVRFBGPL2VPNEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF %s BGP L2VPN",
	Getter: func(name string, value *dozer.SpecVRFBGP) any {
		return []any{specVRFBGPBaseEnforcerGetter(name, value), value.L2VPNEVPN}
	},
	Path:         "/global/afi-safis/afi-safi[afi-safi-name=L2VPN_EVPN]",
	UpdateWeight: ActionWeightVRFBGPL2VPNUpdate,
	DeleteWeight: ActionWeightVRFBGPL2VPNDelete,
	Marshal: func(_ string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		routeAdvertise := map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise_RouteAdvertiseList{}
		if value.L2VPNEVPN.AdvertiseIPv4Unicast != nil && *value.L2VPNEVPN.AdvertiseIPv4Unicast {
			routeAdvertise[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST] = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise_RouteAdvertiseList{
				AdvertiseAfiSafi: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise_RouteAdvertiseList_Config{
					AdvertiseAfiSafi: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
					RouteMap:         value.L2VPNEVPN.AdvertiseIPv4UnicastRouteMaps,
				},
			}
		}

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis{
			AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{ //nolint:exhaustive
				oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN: {
					AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
						AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
					},
					L2VpnEvpn: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn{
						Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_Config{
							AdvertiseAllVni:    value.L2VPNEVPN.AdvertiseAllVNI,
							AdvertiseDefaultGw: value.L2VPNEVPN.AdvertiseDefaultGw,
						},
						RouteAdvertise: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise{
							RouteAdvertiseList: routeAdvertise,
						},
					},
				},
			},
		}, nil
	},
}

var specVRFBGPNeighborsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFBGPNeighbor]{
	Summary:      "VRF BGP neighbors",
	ValueHandler: specVRFBGPNeighborEnforcer,
}

var specVRFBGPNeighborEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGPNeighbor]{
	Summary:      "VRF BGP neighbor %s",
	Path:         "/neighbors/neighbor[neighbor-address=%s]",
	UpdateWeight: ActionWeightVRFBGPNeighborUpdate,
	DeleteWeight: ActionWeightVRFBGPNeighborDelete,
	Marshal: func(name string, value *dozer.SpecVRFBGPNeighbor) (ygot.ValidatedGoStruct, error) {
		var peerType oc.E_OpenconfigBgp_PeerType
		if value.PeerType != nil {
			if *value.PeerType == dozer.SpecVRFBGPNeighborPeerTypeInternal {
				peerType = oc.OpenconfigBgp_PeerType_INTERNAL
			} else if *value.PeerType == dozer.SpecVRFBGPNeighborPeerTypeExternal {
				peerType = oc.OpenconfigBgp_PeerType_EXTERNAL
			} else {
				return nil, errors.Errorf("unknown peer type %s", *value.PeerType)
			}
		}

		var ipApplyPolicy *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy
		if value.IPv4UnicastImportPolicies != nil || value.IPv4UnicastExportPolicies != nil {
			ipApplyPolicy = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy{
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy_Config{
					ImportPolicy: value.IPv4UnicastImportPolicies,
					ExportPolicy: value.IPv4UnicastExportPolicies,
				},
			}
		}

		var l2VPNEVPNAllowOwnAS *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AllowOwnAs
		if value.L2VPNEVPNAllowOwnAS != nil {
			l2VPNEVPNAllowOwnAS = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AllowOwnAs{
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AllowOwnAs_Config{
					Enabled: value.L2VPNEVPNAllowOwnAS,
				},
			}
		}

		var l2ApplyPolicy *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy
		if value.L2VPNEVPNImportPolicies != nil {
			l2ApplyPolicy = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy{
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy_Config{
					ImportPolicy: value.L2VPNEVPNImportPolicies,
				},
			}
		}

		var remoteAS oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config_PeerAs_Union
		if value.RemoteAS != nil {
			remoteAS = oc.UnionUint32(*value.RemoteAS)
		}

		var bfd *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_EnableBfd
		if value.BFDProfile != nil {
			bfd = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_EnableBfd{
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_EnableBfd_Config{
					Enabled:    pointer.To(true),
					BfdProfile: value.BFDProfile,
				},
			}
		}

		bgpNeigh := &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
			Neighbor: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
				name: {
					NeighborAddress: pointer.To(name),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config{
						NeighborAddress:                pointer.To(name),
						Enabled:                        value.Enabled,
						Description:                    value.Description,
						PeerAs:                         remoteAS,
						PeerType:                       peerType,
						DisableEbgpConnectedRouteCheck: value.DisableConnectedCheck,
						CapabilityExtendedNexthop:      value.ExtendedNexthop,
					},
					AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis{
						AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi{ //nolint:exhaustive,nolintlint
							oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
								AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
								Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Config{
									AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
									Enabled:     value.IPv4Unicast,
								},
								ApplyPolicy: ipApplyPolicy,
							},
							oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN: {
								AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
								Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Config{
									AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
									Enabled:     value.L2VPNEVPN,
								},
								ApplyPolicy: l2ApplyPolicy,
								AllowOwnAs:  l2VPNEVPNAllowOwnAS,
							},
						},
					},
					EnableBfd: bfd,
				},
			},
		}

		if value.UpdateSource != nil && *value.UpdateSource != "" {
			bgpNeigh.Neighbor[name].Transport = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Transport{
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Transport_Config{
					LocalAddress: value.UpdateSource,
				},
			}
		}

		return bgpNeigh, nil
	},
}

var specVRFBGPNetworksEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFBGPNetwork]{
	Summary:      "VRF BGP networks",
	ValueHandler: specVRFBGPNetworkEnforcer,
}

var specVRFBGPNetworkEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGPNetwork]{
	Summary:      "VRF BGP network %s",
	Path:         "/global/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/network-config/network[prefix=%s]",
	UpdateWeight: ActionWeightVRFBGPNetworkUpdate,
	DeleteWeight: ActionWeightVRFBGPNetworkDelete,
	Marshal: func(prefix string, _ *dozer.SpecVRFBGPNetwork) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig{
			Network: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network{
				prefix: {
					Prefix: pointer.To(prefix),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network_Config{
						Prefix: pointer.To(prefix),
					},
				},
			},
		}, nil
	},
}

var specVRFImportVrfEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF BGP import VRF %s",
	Getter: func(name string, value *dozer.SpecVRFBGP) any {
		return []any{specVRFBGPBaseEnforcerGetter(name, value), value.IPv4Unicast.ImportVRFs} // TODO check if it helps
	},
	MutateDesired: func(_ string, desired *dozer.SpecVRFBGP) *dozer.SpecVRFBGP {
		if desired != nil && len(desired.IPv4Unicast.ImportVRFs) == 0 {
			return nil
		}

		return desired
	},
	MutateActual: func(_ string, actual *dozer.SpecVRFBGP) *dozer.SpecVRFBGP {
		if actual != nil && len(actual.IPv4Unicast.ImportVRFs) == 0 {
			return nil
		}

		return actual
	},
	Path:         "/global/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/import-network-instance/config/name",
	UpdateWeight: ActionWeightVRFBGPImportVRFUpdate,
	DeleteWeight: ActionWeightVRFBGPImportVRFDelete,
	Marshal: func(_ string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		imports := lo.Keys(value.IPv4Unicast.ImportVRFs)
		sort.Strings(imports)

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_ImportNetworkInstance_Config{
			Name: imports,
		}, nil
	},
}

var specVRFImportPolicyEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF BGP import policy %s",
	Getter: func(name string, value *dozer.SpecVRFBGP) any {
		return []any{specVRFBGPBaseEnforcerGetter(name, value), value.IPv4Unicast.ImportPolicy} // TODO check if it helps
	},
	MutateDesired: func(_ string, desired *dozer.SpecVRFBGP) *dozer.SpecVRFBGP {
		if desired != nil && desired.IPv4Unicast.ImportPolicy == nil {
			return nil
		}

		return desired
	},
	MutateActual: func(_ string, actual *dozer.SpecVRFBGP) *dozer.SpecVRFBGP {
		if actual != nil && actual.IPv4Unicast.ImportPolicy == nil {
			return nil
		}

		return actual
	},
	Path:         "/global/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/import-network-instance/config/policy-name",
	UpdateWeight: ActionWeightVRFBGPImportVRFPolicyUpdate,
	DeleteWeight: ActionWeightVRFBGPImportVRFPolicyDelete,
	Marshal: func(_ string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_ImportNetworkInstance_Config{
			PolicyName: value.IPv4Unicast.ImportPolicy,
		}, nil
	},
}

var specVRFTableConnectionsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFTableConnection]{
	Summary:      "VRF table connections",
	ValueHandler: specVRFTableConnectionEnforcer,
}

// TODO replace with proper handling, delete will not work correctly now
var specVRFTableConnectionEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFTableConnection]{
	Summary:      "VRF table connection %s",
	Path:         "/table-connections/table-connection",
	NoReplace:    true,
	UpdateWeight: ActionWrightVRFTableConnectionUpdate,
	DeleteWeight: ActionWrightVRFTableConnectionDelete,
	Marshal: func(key string, value *dozer.SpecVRFTableConnection) (ygot.ValidatedGoStruct, error) {
		var proto oc.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE

		if key == dozer.SpecVRFBGPTableConnectionConnected {
			proto = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED
		} else if key == dozer.SpecVRFBGPTableConnectionStatic {
			proto = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC
		} else if key == dozer.SpecVRFBGPTableConnectionAttachedHost {
			proto = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ATTACHED_HOST
		} else {
			return nil, errors.Errorf("unknown table connection key %s", key)
		}

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections{
			TableConnection: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection{
				{
					SrcProtocol:   proto,
					DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
					AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
				}: {
					AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
					SrcProtocol:   proto,
					DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config{
						AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
						DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
						SrcProtocol:   proto,
						ImportPolicy:  value.ImportPolicies,
					},
				},
			},
		}, nil
	},
}

var specVRFStaticRoutesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFStaticRoute]{
	Summary:      "VRF static routes",
	ValueHandler: specVRFStaticRouteEnforcer,
}

var specVRFStaticRouteEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFStaticRoute]{
	Summary:          "VRF static route %s",
	Path:             "/protocols/protocol[identifier=STATIC][name=static]/static-routes/static[prefix=%s]",
	RecreateOnUpdate: true,
	UpdateWeight:     ActionWeightVRFStaticRouteUpdate,
	DeleteWeight:     ActionWeightVRFStaticRouteDelete,
	Marshal: func(prefix string, value *dozer.SpecVRFStaticRoute) (ygot.ValidatedGoStruct, error) {
		nextHops := map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop{}

		for _, nextHop := range value.NextHops {
			var index string
			switch {
			case nextHop.Interface != nil && nextHop.IP != "":
				index = fmt.Sprintf("%s_%s", *nextHop.Interface, nextHop.IP)
			case nextHop.Interface != nil:
				index = *nextHop.Interface
			case nextHop.IP != "":
				index = nextHop.IP
			default:
				return nil, errors.New("next hop must have at least one of interface or IP defined")
			}
			var ifaceRef *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_InterfaceRef
			if nextHop.Interface != nil {
				ifaceRef = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_InterfaceRef{
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_InterfaceRef_Config{
						Interface: nextHop.Interface,
					},
				}
			}
			var nh oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config_NextHop_Union
			if nextHop.IP != "" {
				nh = oc.UnionString(nextHop.IP)
			}
			nextHops[index] = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop{
				Index: pointer.To(index),
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config{
					Index:   pointer.To(index),
					NextHop: nh,
				},
				InterfaceRef: ifaceRef,
			}
		}

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes{
			Static: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static{
				prefix: {
					Prefix: pointer.To(prefix),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config{
						Prefix: pointer.To(prefix),
					},
					NextHops: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops{
						NextHop: nextHops,
					},
				},
			},
		}, nil
	},
}

var specVRFAttachedHostsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFAttachedHost]{
	Summary:      "VRF attached hosts",
	ValueHandler: specVRFAttachedHostEnforcer,
}

var specVRFAttachedHostEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFAttachedHost]{
	Summary: "VRF attached host",
	Path:    "/protocols/protocol[identifier=ATTACHED_HOST][name=attached-host]/attached-host/interfaces/interface[address-family=IPV4][interface-id=%s]",
	// CreatePath:   "/protocols/protocol[identifier=ATTACHED_HOST][name=attached-host]/attached-host/interfaces/interface",
	UpdateWeight: ActionWeightVRFAttachedHostUpdate,
	DeleteWeight: ActionWeightVRFAttachedHostDelete,
	Marshal: func(iface string, value *dozer.SpecVRFAttachedHost) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_AttachedHost_Interfaces{
			Interface: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_AttachedHost_Interfaces_Interface_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_AttachedHost_Interfaces_Interface{
				{
					InterfaceId:   iface,
					AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
				}: {
					InterfaceId:   pointer.To(iface),
					AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_AttachedHost_Interfaces_Interface_Config{
						InterfaceId:   pointer.To(iface),
						AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
					},
				},
			},
		}, nil
	},
}

func loadActualVRFs(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.OpenconfigNetworkInstance_NetworkInstances{}
	err := client.Get(ctx, "/network-instances/network-instance", ocVal, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read vrfs")
	}
	spec.VRFs, err = unmarshalOCVRFs(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal vrfs")
	}

	return nil
}

func unmarshalOCVRFs(ocVal *oc.OpenconfigNetworkInstance_NetworkInstances) (map[string]*dozer.SpecVRF, error) {
	vrfs := map[string]*dozer.SpecVRF{}

	if ocVal == nil {
		return vrfs, nil
	}

	for name, ocVRF := range ocVal.NetworkInstance {
		if strings.HasPrefix(name, "Vlan") || ocVRF.Config == nil {
			continue
		}

		interfaces := map[string]*dozer.SpecVRFInterface{}
		if ocVRF.Interfaces != nil && name != VRFDefault { // all interfaces are in the default VRF implicitly
			for ifaceName := range ocVRF.Interfaces.Interface {
				interfaces[ifaceName] = &dozer.SpecVRFInterface{}
			}
		}

		bgp := &dozer.SpecVRFBGP{
			Neighbors: map[string]*dozer.SpecVRFBGPNeighbor{},
			IPv4Unicast: dozer.SpecVRFBGPIPv4Unicast{
				Networks:   map[string]*dozer.SpecVRFBGPNetwork{},
				ImportVRFs: map[string]*dozer.SpecVRFBGPImportVRF{},
			},
		}
		var attachedHosts map[string]*dozer.SpecVRFAttachedHost

		bgpOk := false
		if ocVRF.Protocols != nil && ocVRF.Protocols.Protocol != nil {
			bgpProto := ocVRF.Protocols.Protocol[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
				Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				Name:       "bgp",
			}]
			if bgpProto != nil && bgpProto.Bgp != nil {
				if bgpProto.AttachedHost != nil {
					attachedHosts = map[string]*dozer.SpecVRFAttachedHost{}
					if bgpProto.AttachedHost.Interfaces != nil {
						for ifaceKey := range bgpProto.AttachedHost.Interfaces.Interface {
							attachedHosts[ifaceKey.InterfaceId] = &dozer.SpecVRFAttachedHost{}
						}
					}
				}

				bgpConfig := bgpProto.Bgp
				if bgpConfig.Global != nil && bgpConfig.Global.Config != nil {
					bgpOk = true

					// TODO parse https://datatracker.ietf.org/doc/html/rfc5396
					if bgpConfig.Global.Config.As != nil {
						if val, ok := bgpConfig.Global.Config.As.(oc.UnionUint32); ok {
							bgp.AS = pointer.To(uint32(val))
						} else {
							return nil, errors.Errorf("failed to unmarshal AS %v (only uint32 is supported)", bgpConfig.Global.Config.As)
						}
					}
					bgp.RouterID = bgpConfig.Global.Config.RouterId
					bgp.NetworkImportCheck = bgpConfig.Global.Config.NetworkImportCheck

					if bgpConfig.Global.AfiSafis != nil && bgpConfig.Global.AfiSafis.AfiSafi != nil {
						ipv4Unicast := bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST]
						if ipv4Unicast != nil {
							bgp.IPv4Unicast.Enabled = true
							bgp.IPv4Unicast.TableMap = ipv4Unicast.Config.TableMapName
							if ipv4Unicast.NetworkConfig != nil {
								for name := range ipv4Unicast.NetworkConfig.Network {
									bgp.IPv4Unicast.Networks[name] = &dozer.SpecVRFBGPNetwork{}
								}
							}
							if ipv4Unicast.ImportNetworkInstance != nil && ipv4Unicast.ImportNetworkInstance.Config != nil {
								bgp.IPv4Unicast.ImportPolicy = ipv4Unicast.ImportNetworkInstance.Config.PolicyName
								for _, name := range ipv4Unicast.ImportNetworkInstance.Config.Name {
									bgp.IPv4Unicast.ImportVRFs[name] = &dozer.SpecVRFBGPImportVRF{}
								}
							}
							if ipv4Unicast.UseMultiplePaths != nil {
								if ipv4Unicast.UseMultiplePaths.Ebgp != nil && ipv4Unicast.UseMultiplePaths.Ebgp.Config != nil {
									if ipv4Unicast.UseMultiplePaths.Ebgp.Config.MaximumPaths != nil && *ipv4Unicast.UseMultiplePaths.Ebgp.Config.MaximumPaths != 1 {
										bgp.IPv4Unicast.MaxPaths = ipv4Unicast.UseMultiplePaths.Ebgp.Config.MaximumPaths
									}
								}
								if ipv4Unicast.UseMultiplePaths.Ibgp != nil && ipv4Unicast.UseMultiplePaths.Ibgp.Config != nil {
									if ipv4Unicast.UseMultiplePaths.Ibgp.Config.MaximumPaths != nil && *ipv4Unicast.UseMultiplePaths.Ibgp.Config.MaximumPaths != 1 {
										bgp.IPv4Unicast.MaxPathsIBGP = ipv4Unicast.UseMultiplePaths.Ibgp.Config.MaximumPaths
									}
								}
							}
						}

						if bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN] != nil {
							l2vpnEVPN := bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN].L2VpnEvpn
							if l2vpnEVPN != nil {
								bgp.L2VPNEVPN.Enabled = true
								if l2vpnEVPN.Config != nil {
									bgp.L2VPNEVPN.AdvertiseAllVNI = l2vpnEVPN.Config.AdvertiseAllVni
									bgp.L2VPNEVPN.AdvertiseDefaultGw = l2vpnEVPN.Config.AdvertiseDefaultGw
								}
								if l2vpnEVPN.RouteAdvertise != nil {
									for _, route := range l2vpnEVPN.RouteAdvertise.RouteAdvertiseList {
										if route.Config != nil && route.Config.AdvertiseAfiSafi == oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST {
											bgp.L2VPNEVPN.AdvertiseIPv4Unicast = pointer.To(true)
											bgp.L2VPNEVPN.AdvertiseIPv4UnicastRouteMaps = route.Config.RouteMap

											break
										}
									}
								}
							}
						}
					}
				}

				if bgpConfig.Neighbors != nil {
					for neighborName, neighbor := range bgpConfig.Neighbors.Neighbor {
						if neighbor.Config == nil {
							continue
						}
						var ipv4Unicast *bool
						var ipv4ImportPolicies []string
						var ipv4ExportPolicies []string
						var l2vpnEVPN *bool
						var l2ImportPolicies []string
						var l2VPNEVPNAllowOwnAS *bool
						if neighbor.AfiSafis != nil && neighbor.AfiSafis.AfiSafi != nil {
							ocIPv4Unicast := neighbor.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST]
							if ocIPv4Unicast != nil && ocIPv4Unicast.Config != nil {
								ipv4Unicast = ocIPv4Unicast.Config.Enabled
								if ocIPv4Unicast.ApplyPolicy != nil && ocIPv4Unicast.ApplyPolicy.Config != nil {
									ipv4ImportPolicies = ocIPv4Unicast.ApplyPolicy.Config.ImportPolicy
									ipv4ExportPolicies = ocIPv4Unicast.ApplyPolicy.Config.ExportPolicy
								}
							}

							ocL2VPNEVPN := neighbor.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN]
							if ocL2VPNEVPN != nil {
								if ocL2VPNEVPN.Config != nil {
									l2vpnEVPN = ocL2VPNEVPN.Config.Enabled
								}
								if ocL2VPNEVPN.ApplyPolicy != nil && ocL2VPNEVPN.ApplyPolicy.Config != nil {
									l2ImportPolicies = ocL2VPNEVPN.ApplyPolicy.Config.ImportPolicy
								}
								if ocL2VPNEVPN.AllowOwnAs != nil && ocL2VPNEVPN.AllowOwnAs.Config != nil {
									l2VPNEVPNAllowOwnAS = ocL2VPNEVPN.AllowOwnAs.Config.Enabled
								}
							}
						}

						var peerType *string
						if neighbor.Config.PeerType == oc.OpenconfigBgp_PeerType_INTERNAL {
							peerType = pointer.To(dozer.SpecVRFBGPNeighborPeerTypeInternal)
						} else if neighbor.Config.PeerType == oc.OpenconfigBgp_PeerType_EXTERNAL {
							peerType = pointer.To(dozer.SpecVRFBGPNeighborPeerTypeExternal)
						}

						// TODO parse https://datatracker.ietf.org/doc/html/rfc5396
						var remoteAS *uint32
						if neighbor.Config.PeerAs != nil {
							if val, ok := neighbor.Config.PeerAs.(oc.UnionUint32); ok {
								remoteAS = pointer.To(uint32(val))
							} else {
								return nil, errors.Errorf("failed to unmarshal Peer AS %v (only uint32 is supported)", neighbor.Config.PeerAs)
							}
						}

						var bfdProfile *string
						if neighbor.EnableBfd != nil && neighbor.EnableBfd.Config != nil {
							if neighbor.EnableBfd.Config.Enabled != nil && *neighbor.EnableBfd.Config.Enabled {
								bfdProfile = neighbor.EnableBfd.Config.BfdProfile
							}
						}

						bgp.Neighbors[neighborName] = &dozer.SpecVRFBGPNeighbor{
							Enabled:                   neighbor.Config.Enabled,
							Description:               neighbor.Config.Description,
							RemoteAS:                  remoteAS,
							PeerType:                  peerType,
							IPv4Unicast:               ipv4Unicast,
							IPv4UnicastImportPolicies: ipv4ImportPolicies,
							IPv4UnicastExportPolicies: ipv4ExportPolicies,
							L2VPNEVPN:                 l2vpnEVPN,
							L2VPNEVPNImportPolicies:   l2ImportPolicies,
							L2VPNEVPNAllowOwnAS:       l2VPNEVPNAllowOwnAS,
							BFDProfile:                bfdProfile,
							DisableConnectedCheck:     neighbor.Config.DisableEbgpConnectedRouteCheck,
							ExtendedNexthop:           neighbor.Config.CapabilityExtendedNexthop,
						}
						if neighbor.Transport != nil && neighbor.Transport.Config != nil {
							bgp.Neighbors[neighborName].UpdateSource = neighbor.Transport.Config.LocalAddress
						}
					}
				}
			}
		}

		tableConns := map[string]*dozer.SpecVRFTableConnection{}

		if ocVRF.TableConnections != nil {
			for key, tableConnection := range ocVRF.TableConnections.TableConnection {
				if key.DstProtocol != oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP {
					continue
				}
				if key.AddressFamily != oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4 {
					continue
				}
				if tableConnection.Config == nil {
					continue
				}

				name := ""
				switch key.SrcProtocol { //nolint:exhaustive
				case oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED:
					name = dozer.SpecVRFBGPTableConnectionConnected
				case oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC:
					name = dozer.SpecVRFBGPTableConnectionStatic
				case oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ATTACHED_HOST:
					name = dozer.SpecVRFBGPTableConnectionAttachedHost
				default:
					continue
				}

				tableConns[name] = &dozer.SpecVRFTableConnection{
					ImportPolicies: tableConnection.Config.ImportPolicy,
				}
			}
		}

		staticRoutes := map[string]*dozer.SpecVRFStaticRoute{}

		if ocVRF.Protocols != nil && ocVRF.Protocols.Protocol != nil {
			staticProto := ocVRF.Protocols.Protocol[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
				Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
				Name:       "static",
			}]
			if staticProto != nil && staticProto.StaticRoutes != nil {
				for prefix, staticRoute := range staticProto.StaticRoutes.Static {
					nextHops := []dozer.SpecVRFStaticRouteNextHop{}
					if staticRoute.NextHops != nil {
						for _, nextHop := range staticRoute.NextHops.NextHop {
							var iface *string
							ip := ""

							if nextHop.InterfaceRef != nil && nextHop.InterfaceRef.Config != nil {
								iface = nextHop.InterfaceRef.Config.Interface
							}
							if nextHop.Config != nil && nextHop.Config.NextHop != nil {
								if union, ok := nextHop.Config.NextHop.(oc.UnionString); ok {
									ip = string(union)
								}
							}
							if iface == nil && ip == "" {
								// this should never happen, should we error out?
								continue
							}

							nextHops = append(nextHops, dozer.SpecVRFStaticRouteNextHop{
								Interface: iface,
								IP:        ip,
							})
						}
					}
					slices.SortStableFunc(nextHops, NextHopCompare)

					staticRoutes[prefix] = &dozer.SpecVRFStaticRoute{
						NextHops: nextHops,
					}
				}
			}
		}

		evpnMH := dozer.SpecVRFEVPNMH{}
		es := map[string]*dozer.SpecVRFEthernetSegment{}

		if ocVRF.Evpn != nil {
			if ocVRF.Evpn.EvpnMh != nil && ocVRF.Evpn.EvpnMh.Config != nil {
				evpnMH.MACHoldtime = ocVRF.Evpn.EvpnMh.Config.MacHoldtime
				evpnMH.StartupDelay = ocVRF.Evpn.EvpnMh.Config.StartupDelay
			}

			// only get ethernet segments from the default VRF
			if name == VRFDefault && ocVRF.Evpn.EthernetSegments != nil {
				for name, ocES := range ocVRF.Evpn.EthernetSegments.EthernetSegment {
					if ocES.Config == nil {
						continue
					}
					if ocES.Config.EsiType != oc.OpenconfigEvpn_EsiType_TYPE_0_OPERATOR_CONFIGURED {
						continue
					}

					esi, ok := ocES.Config.Esi.(oc.UnionString)
					if !ok {
						return nil, errors.Errorf("invalid ESI %v for %s", ocES.Config.Esi, name)
					}

					es[name] = &dozer.SpecVRFEthernetSegment{
						ESI: string(esi),
					}
				}
			}
		}

		enabled := ocVRF.Config.Enabled
		if enabled == nil {
			enabled = pointer.To(true)
		}

		if !bgpOk {
			bgp = nil
		}

		var anycastMAC *string
		if ocVRF.GlobalSag != nil && ocVRF.GlobalSag.Config != nil {
			anycastMAC = ocVRF.GlobalSag.Config.AnycastMac
		}

		vrfs[name] = &dozer.SpecVRF{
			Enabled:          enabled,
			Description:      ocVRF.Config.Description,
			AnycastMAC:       anycastMAC,
			Interfaces:       interfaces,
			BGP:              bgp,
			TableConnections: tableConns,
			StaticRoutes:     staticRoutes,
			EVPNMH:           evpnMH,
			EthernetSegments: es,
			AttachedHosts:    attachedHosts,
		}
	}

	return vrfs, nil
}
