package bcm

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
	"golang.org/x/exp/maps"
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

		return nil
	},
}

var specVRFBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRF]{
	Summary:      "VRF %s base",
	Getter:       func(name string, value *dozer.SpecVRF) any { return []any{value.Enabled, value.Description} },
	UpdateWeight: ActionWeightVRFBaseUpdate,
	DeleteWeight: ActionWeightVRFBaseDelete,
	Marshal: func(name string, value *dozer.SpecVRF) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances{
			NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config{
						Name:        ygot.String(name),
						Enabled:     value.Enabled,
						Description: value.Description,
					},
					GlobalSag: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_GlobalSag{
						Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_GlobalSag_Config{
							AnycastMac: value.AnycastMAC,
						},
					},
				},
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
	Marshal: func(iface string, value *dozer.SpecVRFInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces{
			Interface: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface{
				iface: {
					Id: ygot.String(iface),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface_Config{
						Id: ygot.String(iface),
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

		if err := specVRFImportVrfEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp import vrfs")
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

var specVRFBGPBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF %s BGP base",
	Getter: func(name string, value *dozer.SpecVRFBGP) any {
		return []any{value.AS, value.NetworkImportCheck, value.IPv4Unicast.Enabled, value.L2VPNEVPN.Enabled}
	},
	UpdateWeight: ActionWeightVRFBGPBaseUpdate,
	DeleteWeight: ActionWeightVRFBGPBaseDelete,
	Marshal: func(name string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		afiSafi := map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{}
		if value.IPv4Unicast.Enabled {
			ipv4Unicast := &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{
				AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
					AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
				},
			}

			if value.IPv4Unicast.MaxPaths != nil {
				ipv4Unicast.UseMultiplePaths = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths{
					Ebgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths_Ebgp{
						Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_UseMultiplePaths_Ebgp_Config{
							MaximumPaths: value.IPv4Unicast.MaxPaths,
						},
					},
				}
			}

			afiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST] = ipv4Unicast
		}
		if value.L2VPNEVPN.Enabled {
			routeAdvertise := map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise_RouteAdvertiseList{}
			if value.L2VPNEVPN.AdvertiseIPv4Unicast != nil && *value.L2VPNEVPN.AdvertiseIPv4Unicast {
				routeAdvertise[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST] = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise_RouteAdvertiseList{
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise_RouteAdvertiseList_Config{
						AdvertiseAfiSafi: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
					},
				}
			}

			afiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN] = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{
				AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
					AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
				},
				L2VpnEvpn: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn{
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_Config{
						AdvertiseAllVni: value.L2VPNEVPN.AdvertiseAllVNI,
					},
					// TODO extract as we'll not be able to replace it
					RouteAdvertise: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_L2VpnEvpn_RouteAdvertise{
						RouteAdvertiseList: routeAdvertise,
					},
				},
			}
		}

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
			Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
				Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config{
						As:                 value.AS,
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

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
			Neighbor: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
				name: {
					NeighborAddress: ygot.String(name),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config{
						NeighborAddress: ygot.String(name),
						Enabled:         value.Enabled,
						Description:     value.Description,
						PeerAs:          value.RemoteAS,
						PeerType:        peerType,
					},
					AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis{
						AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi{
							oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
								AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
								Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Config{
									AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
									Enabled:     value.IPv4Unicast,
								},
							},
							oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN: {
								AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
								Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Config{
									AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN,
									Enabled:     value.L2VPNEVPN,
								},
							},
						},
					},
				},
			},
		}, nil
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
	Marshal: func(prefix string, value *dozer.SpecVRFBGPNetwork) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig{
			Network: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network{
				prefix: {
					Prefix: ygot.String(prefix),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network_Config{
						Prefix: ygot.String(prefix),
					},
				},
			},
		}, nil
	},
}

var specVRFImportVrfEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF BGP import VRF %s",
	Getter: func(name string, value *dozer.SpecVRFBGP) any {
		return value.IPv4Unicast.ImportVRFs
	},
	MutateDesired: func(key string, desired *dozer.SpecVRFBGP) *dozer.SpecVRFBGP {
		if desired != nil && len(desired.IPv4Unicast.ImportVRFs) == 0 {
			return nil
		}

		return desired
	},
	MutateActual: func(key string, actual *dozer.SpecVRFBGP) *dozer.SpecVRFBGP {
		if actual != nil && len(actual.IPv4Unicast.ImportVRFs) == 0 {
			return nil
		}

		return actual
	},
	Path:         "/global/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/import-network-instance/config/name",
	UpdateWeight: ActionWeightVRFBGPImportVRFUpdate,
	DeleteWeight: ActionWeightVRFBGPImportVRFDelete,
	Marshal: func(name string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		imports := maps.Keys(value.IPv4Unicast.ImportVRFs)
		sort.Strings(imports)

		slog.Warn("Scheduling VRF imports update", "name", name, "imports", imports, "rawImports", value.IPv4Unicast.ImportVRFs)

		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_ImportNetworkInstance_Config{
			Name: imports,
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

func loadActualVRFs(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.OpenconfigNetworkInstance_NetworkInstances{}
	err := client.Get(ctx, "/network-instances/network-instance", ocVal)
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
		if ocVRF.Interfaces != nil && name != "default" { // all interfaces are in the default VRF implicitly
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
		bgpOk := false
		if ocVRF.Protocols != nil && ocVRF.Protocols.Protocol != nil {
			bgpProto := ocVRF.Protocols.Protocol[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
				Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				Name:       "bgp",
			}]

			if bgpProto != nil && bgpProto.Bgp != nil {
				bgpConfig := bgpProto.Bgp

				if bgpConfig.Global != nil && bgpConfig.Global.Config != nil {
					bgpOk = true
					bgp.AS = bgpConfig.Global.Config.As
					bgp.RouterID = bgpConfig.Global.Config.RouterId
					bgp.NetworkImportCheck = bgpConfig.Global.Config.NetworkImportCheck

					if bgpConfig.Global.AfiSafis != nil && bgpConfig.Global.AfiSafis.AfiSafi != nil {
						ipv4Unicast := bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST]
						if ipv4Unicast != nil {
							bgp.IPv4Unicast.Enabled = true
							if ipv4Unicast.NetworkConfig != nil {
								for name := range ipv4Unicast.NetworkConfig.Network {
									bgp.IPv4Unicast.Networks[name] = &dozer.SpecVRFBGPNetwork{}
								}
							}
							if ipv4Unicast.ImportNetworkInstance != nil && ipv4Unicast.ImportNetworkInstance.Config != nil {
								for _, name := range ipv4Unicast.ImportNetworkInstance.Config.Name {
									bgp.IPv4Unicast.ImportVRFs[name] = &dozer.SpecVRFBGPImportVRF{}
								}
							}
							if ipv4Unicast.UseMultiplePaths != nil && ipv4Unicast.UseMultiplePaths.Ebgp != nil && ipv4Unicast.UseMultiplePaths.Ebgp.Config != nil {
								if ipv4Unicast.UseMultiplePaths.Ebgp.Config.MaximumPaths != nil && *ipv4Unicast.UseMultiplePaths.Ebgp.Config.MaximumPaths != 1 {
									bgp.IPv4Unicast.MaxPaths = ipv4Unicast.UseMultiplePaths.Ebgp.Config.MaximumPaths
								}
							}
						}

						if bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN] != nil {
							l2vpnEVPN := bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN].L2VpnEvpn
							if l2vpnEVPN != nil {
								bgp.L2VPNEVPN.Enabled = true
								if l2vpnEVPN.Config != nil {
									bgp.L2VPNEVPN.AdvertiseAllVNI = l2vpnEVPN.Config.AdvertiseAllVni
								}
								if l2vpnEVPN.RouteAdvertise != nil {
									for _, route := range l2vpnEVPN.RouteAdvertise.RouteAdvertiseList {
										if route.Config != nil && route.Config.AdvertiseAfiSafi == oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST {
											bgp.L2VPNEVPN.AdvertiseIPv4Unicast = ygot.Bool(true)
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
						var l2vpnEVPN *bool
						if neighbor.AfiSafis != nil && neighbor.AfiSafis.AfiSafi != nil {
							ocIPv4Unicast := neighbor.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST]
							if ocIPv4Unicast != nil && ocIPv4Unicast.Config != nil {
								ipv4Unicast = ocIPv4Unicast.Config.Enabled
							}

							ocL2VPNEVPN := neighbor.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_L2VPN_EVPN]
							if ocL2VPNEVPN != nil && ocL2VPNEVPN.Config != nil {
								l2vpnEVPN = ocL2VPNEVPN.Config.Enabled
							}
						}

						var peerType *string
						if neighbor.Config.PeerType == oc.OpenconfigBgp_PeerType_INTERNAL {
							peerType = ygot.String(dozer.SpecVRFBGPNeighborPeerTypeInternal)
						} else if neighbor.Config.PeerType == oc.OpenconfigBgp_PeerType_EXTERNAL {
							peerType = ygot.String(dozer.SpecVRFBGPNeighborPeerTypeExternal)
						}

						bgp.Neighbors[neighborName] = &dozer.SpecVRFBGPNeighbor{
							Enabled:     neighbor.Config.Enabled,
							Description: neighbor.Config.Description,
							RemoteAS:    neighbor.Config.PeerAs,
							PeerType:    peerType,
							IPv4Unicast: ipv4Unicast,
							L2VPNEVPN:   l2vpnEVPN,
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

				name := dozer.SpecVRFBGPTableConnectionStatic
				if key.SrcProtocol == oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED {
					name = dozer.SpecVRFBGPTableConnectionConnected
				} else if key.SrcProtocol != oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC {
					continue
				}

				tableConns[name] = &dozer.SpecVRFTableConnection{
					ImportPolicies: tableConnection.Config.ImportPolicy,
				}
			}
		}

		enabled := ocVRF.Config.Enabled
		if enabled == nil {
			enabled = ygot.Bool(true)
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
		}
	}

	return vrfs, nil
}
