package bcm

import (
	"context"
	"fmt"
	"strings"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
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
				},
			},
		}, nil
	},
}

var specVRFInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFInterface]{
	Summary:      "VRF %s interfaces",
	ValueHandler: specVRFInterfaceEnforcer,
}

var specVRFInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFInterface]{
	Summary:      "VRF interface %s",
	Path:         "/interfaces/interface[id=%s]",
	UpdateWeight: ActionWeightVRFInterfaceUpdate,
	DeleteWeight: ActionWeightVRFInterfaceDelete,
	Marshal: func(id string, value *dozer.SpecVRFInterface) (ygot.ValidatedGoStruct, error) {
		// it's currently not needed as we're only using the default VRF
		return nil, errors.Errorf("not implemented")
	},
}

var specVRFBGPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary: "VRF BGP",
	CustomHandler: func(basePath string, name string, actual, desired *dozer.SpecVRFBGP, actions *ActionQueue) error {
		basePath += "/protocols/protocol[identifier=BGP][name=bgp]/bgp"

		if err := specVRFBGPBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp base")
		}

		actualNeighbors, desiredNeighbors := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRFBGP) map[string]*dozer.SpecVRFBGPNeighbor { return value.Neighbors })
		if err := specVRFBGPNeighborsEnforcer.Handle(basePath, actualNeighbors, desiredNeighbors, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp neighbors")
		}

		actualNetworks, desiredNetworks := ValueOrNil(actual, desired,
			func(value *dozer.SpecVRFBGP) map[string]*dozer.SpecVRFBGPNetwork { return value.Networks })
		if err := SpecVRFBGPNetworksEnforcer.Handle(basePath, actualNetworks, desiredNetworks, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf bgp networks")
		}

		return nil
	},
}

var specVRFBGPBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGP]{
	Summary:      "VRF %s BGP base",
	Getter:       func(name string, value *dozer.SpecVRFBGP) any { return []any{value.AS, value.NetworkImportCheck} },
	UpdateWeight: ActionWeightVRFBGPBaseUpdate,
	DeleteWeight: ActionWeightVRFBGPBaseDelete,
	Marshal: func(name string, value *dozer.SpecVRFBGP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
			Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
				Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config{
						As:                 value.AS,
						NetworkImportCheck: value.NetworkImportCheck,
					},
					AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis{
						AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{
							oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
								AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
								Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
									AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
								},
							},
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
		return &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
			Neighbor: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
				name: {
					NeighborAddress: ygot.String(name),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config{
						NeighborAddress: ygot.String(name),
						Enabled:         value.Enabled,
						PeerAs:          value.RemoteAS,
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
						},
					},
				},
			},
		}, nil
	},
}

var SpecVRFBGPNetworksEnforcer = &DefaultMapEnforcer[string, *dozer.SpecVRFBGPNetwork]{
	Summary:      "VRF BGP networks",
	ValueHandler: SpecVRFBGPNetworkEnforcer,
}

var SpecVRFBGPNetworkEnforcer = &DefaultValueEnforcer[string, *dozer.SpecVRFBGPNetwork]{
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
			Networks:  map[string]*dozer.SpecVRFBGPNetwork{},
		}
		if ocVRF.Protocols != nil && ocVRF.Protocols.Protocol != nil {
			bgpProto := ocVRF.Protocols.Protocol[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
				Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				Name:       "bgp",
			}]

			if bgpProto != nil && bgpProto.Bgp != nil {
				bgpConfig := bgpProto.Bgp

				if bgpConfig.Global != nil && bgpConfig.Global.Config != nil {
					bgp.AS = bgpConfig.Global.Config.As
					bgp.NetworkImportCheck = bgpConfig.Global.Config.NetworkImportCheck

					if bgpConfig.Global.AfiSafis != nil || bgpConfig.Global.AfiSafis.AfiSafi != nil {
						unicast := bgpConfig.Global.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST]
						if unicast != nil && unicast.NetworkConfig != nil {
							for name := range unicast.NetworkConfig.Network {
								bgp.Networks[name] = &dozer.SpecVRFBGPNetwork{}
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
						if neighbor.AfiSafis != nil && neighbor.AfiSafis.AfiSafi != nil {
							unicast := neighbor.AfiSafis.AfiSafi[oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST]
							if unicast != nil && unicast.Config != nil {
								ipv4Unicast = unicast.Config.Enabled
							}
						}

						bgp.Neighbors[neighborName] = &dozer.SpecVRFBGPNeighbor{
							Enabled:     neighbor.Config.Enabled,
							RemoteAS:    neighbor.Config.PeerAs,
							IPv4Unicast: ipv4Unicast,
						}
					}
				}
			}
		}

		enabled := ocVRF.Config.Enabled
		if name == "default" {
			enabled = ygot.Bool(true)
		}

		vrfs[name] = &dozer.SpecVRF{
			Enabled:     enabled,
			Description: ocVRF.Config.Description,
			Interfaces:  interfaces,
			BGP:         bgp,
		}
	}

	return vrfs, nil
}
