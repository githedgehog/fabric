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
	"strconv"
	"strings"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

const (
	IfacePrefixManagement    = "Management"
	IfacePrefixPhysical      = "Ethernet"
	IfacePrefixVLAN          = "Vlan"
	IfacePrefixPortChannel   = "PortChannel"
	IfaceCPU                 = "CPU"
	IfaceDisabledDescription = "Disabled by Fabric"
)

func isManagement(name string) bool {
	return strings.HasPrefix(name, IfacePrefixManagement)
}

func isPhysical(name string) bool {
	return strings.HasPrefix(name, IfacePrefixPhysical)
}

func isVLAN(name string) bool {
	return strings.HasPrefix(name, IfacePrefixVLAN)
}

func isPortChannel(name string) bool {
	return strings.HasPrefix(name, IfacePrefixPortChannel)
}

func isCPU(name string) bool {
	return name == IfaceCPU
}

var specInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecInterface]{
	Summary:      "Interfaces",
	ValueHandler: specInterfaceEnforcer,
}

var specInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "Interface %s",
	CustomHandler: func(basePath string, name string, actual, desired *dozer.SpecInterface, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/interfaces/interface[name=%s]", name)

		if err := specInterfaceBasePortChannelsEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface base port channels")
		}

		if err := specInterfaceBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface base")
		}

		if err := specInterfaceVLANProxyARPEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface VLAN Proxy ARP")
		}

		actualIPs, desiredIPs := ValueOrNil(actual, desired,
			func(value *dozer.SpecInterface) map[string]*dozer.SpecInterfaceIP { return value.VLANIPs })
		if err := specInterfaceVLANIPsEnforcer.Handle(basePath, actualIPs, desiredIPs, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface IPs")
		}

		actualStaticARPs, desiredStaticARPs := ValueOrNil(actual, desired,
			func(value *dozer.SpecInterface) map[string]*dozer.SpecStaticARP { return value.StaticARPs })
		if err := specInterfaceVLANStaticARPsEnforcer.Handle(basePath, actualStaticARPs, desiredStaticARPs, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface static ARPs")
		}

		if err := specInterfaceEthernetBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface ethernet")
		}

		if err := specInterfaceEthernetSwitchedAccessEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface switched access")
		}

		if err := specInterfaceEthernetSwitchedTrunkEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface switched trunk")
		}

		if err := specInterfacesPortChannelSwitchedAccessEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle port channel switched access")
		}

		if err := specInterfacesPortChannelSwitchedTrunkEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle port channel switched trunk")
		}

		if err := specInterfaceVLANAnycastGatewayEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface VLAN Anycast Gateway")
		}

		actualSubs, desiredSubs := ValueOrNil(actual, desired,
			func(value *dozer.SpecInterface) map[uint32]*dozer.SpecSubinterface { return value.Subinterfaces })
		if err := specInterfaceSubinterfacesEnforcer.Handle(basePath, actualSubs, desiredSubs, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface subinterfaces")
		}

		return nil
	},
}

var specInterfaceBasePortChannelsEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.Description, value.Enabled, value.MTU}
	},
	Skip: func(key string, _, _ *dozer.SpecInterface) bool {
		return !isPortChannel(key)
	},
	Summary:      "Interface %s Base PortChannels",
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfaceBasePortChannelsUpdate,
	DeleteWeight: ActionWeightInterfaceBaseDelete,
	MutateDesired: func(name string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if (isManagement(name) || isPhysical(name)) && desired == nil {
			return &dozer.SpecInterface{
				Enabled:     pointer.To(false),
				Description: pointer.To(IfaceDisabledDescription),
			}
		}

		return desired
	},
	Marshal: marshalSpecInterfaceBaseEnforcer,
}

var specInterfaceBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.Description, value.Enabled, value.MTU}
	},
	Skip: func(key string, _, _ *dozer.SpecInterface) bool {
		return isPortChannel(key)
	},
	Summary:      "Interface %s Base",
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfaceBaseUpdate,
	DeleteWeight: ActionWeightInterfaceBaseDelete,
	MutateDesired: func(name string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if (isManagement(name) || isPhysical(name)) && desired == nil {
			return &dozer.SpecInterface{
				Enabled:     pointer.To(false),
				Description: pointer.To(IfaceDisabledDescription),
			}
		}

		return desired
	},
	Marshal: marshalSpecInterfaceBaseEnforcer,
}

var marshalSpecInterfaceBaseEnforcer = func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
	val := &oc.OpenconfigInterfaces_Interfaces_Interface{
		Name: pointer.To(name),
		Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
			Name:        pointer.To(name),
			Description: value.Description,
			Enabled:     value.Enabled,
			Mtu:         value.MTU, // TODO we'll not be able to unset it as we can't use replace
		},
	}

	if isPortChannel(name) {
		val.Aggregation = &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation{
			Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_Config{},
		}
	}

	if isVLAN(name) {
		val.RoutedVlan = &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan{
			Ipv4: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Config{},
			},
		}
	}

	return &oc.OpenconfigInterfaces_Interfaces{
		Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
			name: val,
		},
	}, nil
}

var specInterfaceVLANIPsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecInterfaceIP]{
	Summary:      "Interface %s VLAN IPs",
	ValueHandler: specInterfaceVLANIPEnforcer,
}

var specInterfaceVLANIPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterfaceIP]{
	Summary: "Interface VLAN IP %s", // TODO chain summary as well?
	Path:    "/routed-vlan/ipv4/addresses/address[ip=%s]",
	// NoReplace:    true, // TODO check if it'll work correctly
	// SkipDelete:   true, // TODO check how good remove/add/replace IP works
	UpdateWeight: ActionWeightInterfaceVLANIPsUpdate,
	DeleteWeight: ActionWeightInterfaceVLANIPsDelete,
	Marshal: func(name string, value *dozer.SpecInterfaceIP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses{
			Address: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses_Address{
				name: {
					Ip: pointer.To(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses_Address_Config{
						Ip:           pointer.To(name),
						PrefixLength: value.PrefixLen,
						Secondary:    pointer.To(false),
					},
				},
			},
		}, nil
	},
}

var specInterfaceVLANStaticARPsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecStaticARP]{
	Summary:      "Interface %s VLAN Static ARPs",
	ValueHandler: specInterfaceVLANStaticARPEnforcer,
}

var specInterfaceVLANStaticARPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecStaticARP]{
	Summary:      "Interface VLAN Static ARP %s",
	Path:         "/routed-vlan/ipv4/neighbors/neighbor[ip=%s]",
	UpdateWeight: ActionWeightInterfaceVLANStaticARPUpdate,
	DeleteWeight: ActionWeightInterfaceVLANStaticARPDelete,
	Marshal: func(name string, value *dozer.SpecStaticARP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Neighbors{
			Neighbor: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Neighbors_Neighbor{
				name: {
					Ip: pointer.To(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Neighbors_Neighbor_Config{
						Ip:               pointer.To(name),
						LinkLayerAddress: pointer.To(value.MAC),
					},
				},
			},
		}, nil
	},
}

var specInterfaceSubinterfaceStaticARPsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecStaticARP]{
	Summary:      "Subinterface %s Static ARPs",
	ValueHandler: specInterfaceSubinterfaceStaticARPEnforcer,
}

var specInterfaceSubinterfaceStaticARPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecStaticARP]{
	Summary:      "SubInterface Static ARP %s",
	Path:         "/ipv4/neighbors/neighbor[ip=%s]",
	UpdateWeight: ActionWeightInterfaceSubinterfaceStaticARPUpdate,
	DeleteWeight: ActionWeightInterfaceSubinterfaceStaticARPDelete,
	Marshal: func(name string, value *dozer.SpecStaticARP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors{
			Neighbor: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors_Neighbor{
				name: {
					Ip: pointer.To(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Neighbors_Neighbor_Config{
						Ip:               pointer.To(name),
						LinkLayerAddress: pointer.To(value.MAC),
					},
				},
			},
		}, nil
	},
}

var specInterfaceVLANProxyARPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary:          "Interface VLAN Proxy-ARP",
	Path:             "/routed-vlan/ipv4/proxy-arp/config",
	UpdateWeight:     ActionWeightProxyARPUpdate,
	DeleteWeight:     ActionWeightProxyARPDelete,
	RecreateOnUpdate: true,
	Getter:           func(key string, value *dozer.SpecInterface) any { return value.ProxyARP },
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		mode := oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_DISABLE
		if value != nil && value.ProxyARP != nil {
			if value.ProxyARP.All {
				mode = oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_ALL
			} else {
				mode = oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_REMOTE_ONLY
			}
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp{
			Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config{
				Mode: mode,
			},
		}, nil
	},
}

var specInterfaceSubinterfaceProxyARPEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecSubinterface]{
	Summary:          "Subinterface %d Proxy-ARP",
	Path:             "/ipv4/proxy-arp/config",
	UpdateWeight:     ActionWeightProxyARPUpdate,
	DeleteWeight:     ActionWeightProxyARPDelete,
	RecreateOnUpdate: true,
	Getter:           func(idx uint32, value *dozer.SpecSubinterface) any { return value.ProxyARP },
	Marshal: func(_ uint32, value *dozer.SpecSubinterface) (ygot.ValidatedGoStruct, error) {
		mode := oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_DISABLE
		if value != nil && value.ProxyARP != nil {
			if value.ProxyARP.All {
				mode = oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_ALL
			} else {
				mode = oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_REMOTE_ONLY
			}
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_ProxyArp{
			Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_ProxyArp_Config{
				Mode: mode,
			},
		}, nil
	},
}

var specInterfaceSubinterfacesEnforcer = &DefaultMapEnforcer[uint32, *dozer.SpecSubinterface]{
	Summary:      "Subinterface %s",
	ValueHandler: specInterfaceSubinterfaceEnforcer,
}

var specInterfaceSubinterfaceEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecSubinterface]{
	Summary: "Subinterface %d", // TODO chain summary as well?
	CustomHandler: func(basePath string, idx uint32, actual, desired *dozer.SpecSubinterface, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/subinterfaces/subinterface[index=%d]", idx)

		if err := specInterfaceSubinterfaceBaseEnforcer.Handle(basePath, idx, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle subinterface base")
		}

		if err := specInterfaceSubinterfaceProxyARPEnforcer.Handle(basePath, idx, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle subinterface Proxy ARP")
		}

		actualIPs, desiredIPs := ValueOrNil(actual, desired,
			func(value *dozer.SpecSubinterface) map[string]*dozer.SpecInterfaceIP { return value.IPs })
		if err := specInterfaceSubinterfaceIPsEnforcer.Handle(basePath, actualIPs, desiredIPs, actions); err != nil {
			return errors.Wrap(err, "failed to handle subinterface IPs")
		}

		actualStaticARPs, desiredStaticARPs := ValueOrNil(actual, desired,
			func(value *dozer.SpecSubinterface) map[string]*dozer.SpecStaticARP { return value.StaticARPs })
		if err := specInterfaceSubinterfaceStaticARPsEnforcer.Handle(basePath, actualStaticARPs, desiredStaticARPs, actions); err != nil {
			return errors.Wrap(err, "failed to handle subinterface static ARPs")
		}

		return nil // TODO
	},
}

var specInterfaceSubinterfaceBaseEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecSubinterface]{
	Summary:      "Subinterface Base %d",
	NoReplace:    true, // TODO check if it'll work correctly
	UpdateWeight: ActionWeightInterfaceSubinterfaceUpdate,
	DeleteWeight: ActionWeightInterfaceSubinterfaceDelete,
	Marshal: func(idx uint32, value *dozer.SpecSubinterface) (ygot.ValidatedGoStruct, error) {
		var vlan *oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Vlan
		if value.VLAN != nil {
			vlan = &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Vlan{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Vlan_Config{
					VlanId: oc.UnionUint16(*value.VLAN),
				},
			}
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces{
			Subinterface: map[uint32]*oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface{
				idx: {
					Index: pointer.To(idx),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Config{
						Index: pointer.To(idx),
					},
					Vlan: vlan,
					Ipv4: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4{
						SagIpv4: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_SagIpv4{
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_SagIpv4_Config{
								StaticAnycastGateway: value.AnycastGateways, // TODO extract into a separate code so we can remove values
							},
						},
					},
				},
			},
		}, nil
	},
}

var specInterfaceSubinterfaceIPsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecInterfaceIP]{
	Summary:      "Subinterface IPs %s",
	ValueHandler: specInterfaceSubinterfaceIPEnforcer,
}

var specInterfaceSubinterfaceIPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterfaceIP]{
	Summary:    "Subinterface IP %s",
	CreatePath: "/ipv4/addresses/address",
	Path:       "/ipv4/addresses/address[ip=%s]",
	NoReplace:  true,
	// SkipDelete:  true, // TODO check if it's needed
	UpdateWeight: ActionWeightInterfaceSubinterfaceIPsUpdate,
	DeleteWeight: ActionWeightInterfaceSubinterfaceIPsDelete,
	Marshal: func(ip string, value *dozer.SpecInterfaceIP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses{
			Address: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address{
				ip: {
					Ip: pointer.To(ip),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address_Config{
						Ip:           pointer.To(ip),
						PrefixLength: value.PrefixLen,
						Secondary:    pointer.To(false),
					},
				},
			},
		}, nil
	},
}

var specInterfaceEthernetBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "Interface %s Ethernet Base", // TODO better summary
	Skip:    func(name string, _, _ *dozer.SpecInterface) bool { return !isPhysical(name) },
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.PortChannel, value.Speed, value.AutoNegotiate} // , value.TrunkVLANs, value.AccessVLAN}
	},
	Path:      "/ethernet",
	NoReplace: true,
	// TODO do we need recreate on update so we can remove from the port channel? and than SwitchedEnforcer will need to be triggered too
	UpdateWeight: ActionWeightInterfaceEthernetBaseUpdate,
	DeleteWeight: ActionWeightInterfaceEthernetBaseDelete,
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		autoNeg := value.AutoNegotiate
		speed := oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET

		if (autoNeg == nil || !*autoNeg) && value.Speed != nil {
			var ok bool
			speed, ok = MarshalPortSpeed(*value.Speed)
			if !ok {
				return nil, errors.Errorf("invalid speed %s", *value.Speed)
			}

			autoNeg = nil
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface{
			Ethernet: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_Config{
					AggregateId:   value.PortChannel,
					PortSpeed:     speed,
					AutoNegotiate: autoNeg,
				},
			},
		}, nil
	},
}

var specInterfaceEthernetSwitchedAccessEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "Interface %s Switched Access VLAN",
	Skip:    func(name string, _, _ *dozer.SpecInterface) bool { return !isPhysical(name) },
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.AccessVLAN}
	},
	MutateActual: func(_ string, actual *dozer.SpecInterface) *dozer.SpecInterface {
		if actual != nil && actual.AccessVLAN == nil {
			return nil
		}

		return actual
	},
	MutateDesired: func(_ string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if desired != nil && desired.AccessVLAN == nil {
			return nil
		}

		return desired
	},
	Path:         "/ethernet/switched-vlan/config/access-vlan",
	UpdateWeight: ActionWeightInterfaceEthernetSwitchedAccessUpdate,
	DeleteWeight: ActionWeightInterfaceEthernetSwitchedAccessDelete,
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan_Config{
			AccessVlan: value.AccessVLAN,
		}, nil
	},
}

var specInterfaceEthernetSwitchedTrunkEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "Interface %s Switched Trunk VLANs",
	Skip:    func(name string, _, _ *dozer.SpecInterface) bool { return !isPhysical(name) },
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.TrunkVLANs}
	},
	MutateActual: func(_ string, actual *dozer.SpecInterface) *dozer.SpecInterface {
		if actual != nil && len(actual.TrunkVLANs) == 0 {
			return nil
		}

		return actual
	},
	MutateDesired: func(_ string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if desired != nil && len(desired.TrunkVLANs) == 0 {
			return nil
		}

		return desired
	},
	Path:         "/ethernet/switched-vlan/config/trunk-vlans",
	UpdateWeight: ActionWeightInterfaceEthernetSwitchedTrunkUpdate,
	DeleteWeight: ActionWeightInterfaceEthernetSwitchedTrunkDelete,
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		trunkVLANs, err := marshalEthernetTrunkVLANs(value)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal trunk VLANs")
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan_Config{
			TrunkVlans: trunkVLANs,
		}, nil
	},
}

var specInterfacesPortChannelSwitchedAccessEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "PortChannel %s Switched Access VLAN",
	Skip:    func(name string, _, _ *dozer.SpecInterface) bool { return !isPortChannel(name) },
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.AccessVLAN}
	},
	MutateActual: func(_ string, actual *dozer.SpecInterface) *dozer.SpecInterface {
		if actual != nil && actual.AccessVLAN == nil {
			return nil
		}

		return actual
	},
	MutateDesired: func(_ string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if desired != nil && desired.AccessVLAN == nil {
			return nil
		}

		return desired
	},
	Path:         "/aggregation/switched-vlan/config/access-vlan",
	UpdateWeight: ActionWeightInterfacePortChannelSwitchedAccessUpdate,
	DeleteWeight: ActionWeightInterfacePortChannelSwitchedAccessDelete,
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config{
			AccessVlan: value.AccessVLAN,
		}, nil
	},
}

var specInterfacesPortChannelSwitchedTrunkEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "PortChannel %s Switched Trunk VLANs",
	Skip:    func(name string, _, _ *dozer.SpecInterface) bool { return !isPortChannel(name) },
	Getter: func(_ string, value *dozer.SpecInterface) any {
		return []any{value.TrunkVLANs}
	},
	MutateActual: func(_ string, actual *dozer.SpecInterface) *dozer.SpecInterface {
		if actual != nil && len(actual.TrunkVLANs) == 0 {
			return nil
		}

		return actual
	},
	MutateDesired: func(_ string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if desired != nil && len(desired.TrunkVLANs) == 0 {
			return nil
		}

		return desired
	},
	Path:         "/aggregation/switched-vlan/config/trunk-vlans",
	UpdateWeight: ActionWeightInterfacePortChannelSwitchedTrunkUpdate,
	DeleteWeight: ActionWeightInterfacePortChannelSwitchedTrunkDelete,
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		trunkVLANs, err := marshalPortChannelTrunkVLANs(value)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal trunk VLANs")
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config{
			TrunkVlans: trunkVLANs,
		}, nil
	},
}

var specInterfaceVLANAnycastGatewayEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary:      "Interface %s VLAN Anycast Gateway",
	Skip:         func(name string, _, _ *dozer.SpecInterface) bool { return !isVLAN(name) },
	Getter:       func(_ string, value *dozer.SpecInterface) any { return value.VLANAnycastGateway },
	Path:         "/routed-vlan/ipv4/sag-ipv4/config/static-anycast-gateway",
	SkipDelete:   true, // TODO check if it's ok
	UpdateWeight: ActionWeightInterfaceVLANAnycastGatewayUpdate,
	DeleteWeight: ActionWeightInterfaceVLANAnycastGatewayDelete,
	Marshal: func(_ string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_SagIpv4_Config{
			StaticAnycastGateway: value.VLANAnycastGateway,
		}, nil
	},
}

func loadActualInterfaces(ctx context.Context, agent *agentapi.Agent, client *gnmi.Client, spec *dozer.Spec) error {
	ocInterfaces := &oc.OpenconfigInterfaces_Interfaces{}
	err := client.Get(ctx, "/interfaces/interface", ocInterfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to read interfaces")
	}
	spec.Interfaces, err = unmarshalOCInterfaces(agent, ocInterfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal interfaces")
	}

	return nil
}

func unmarshalProxyARP(ocVal oc.E_OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode) (*dozer.SpecProxyARP, error) {
	var pa *dozer.SpecProxyARP
	switch ocVal {
	case oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_UNSET:
	case oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_DISABLE:
	case oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_REMOTE_ONLY:
		pa = &dozer.SpecProxyARP{All: false}
	case oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_ProxyArp_Config_Mode_ALL:
		pa = &dozer.SpecProxyARP{All: true}
	default:
		return nil, errors.Errorf("unknown Proxy ARP mode %v", ocVal)
	}

	return pa, nil
}

func unmarshalOCInterfaces(agent *agentapi.Agent, ocVal *oc.OpenconfigInterfaces_Interfaces) (map[string]*dozer.SpecInterface, error) {
	interfaces := map[string]*dozer.SpecInterface{}

	if ocVal == nil {
		return interfaces, nil
	}

	sp := agent.Spec.SwitchProfile
	if sp == nil {
		return nil, errors.New("switch profile is not set")
	}

	skipSpeedPorts := map[string]bool{}
	for _, port := range sp.Ports {
		if port.Group != "" {
			skipSpeedPorts[port.NOSName] = true
		}
	}

	breakoutNames, err := sp.GetAllBreakoutNOSNames()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get all breakout NOS names")
	}
	for name, val := range breakoutNames {
		if !val {
			continue
		}

		skipSpeedPorts[name] = true
	}

	for name, ocIface := range ocVal.Interface {
		if ocIface.Config == nil {
			continue
		}

		if strings.HasPrefix(name, "vtep") || strings.Contains(name, ".") || strings.Contains(name, "|") {
			continue
		}

		mtu := ocIface.Config.Mtu
		if mtu != nil { // TODO it's a hack for now, assuming 9100 is a default MTU for everything other than Mgmt interface (1500), to be replaced with SwitchProfile
			if isManagement(name) && *mtu == 1500 || !isManagement(name) && *mtu == 9100 {
				mtu = nil
			}
		}

		iface := &dozer.SpecInterface{
			Description:   ocIface.Config.Description,
			Enabled:       ocIface.Config.Enabled,
			MTU:           mtu,
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
			VLANIPs:       map[string]*dozer.SpecInterfaceIP{},
			StaticARPs:    map[string]*dozer.SpecStaticARP{},
		}

		// just skip interfaces disabled by Fabric
		if iface.Enabled != nil && !*iface.Enabled && iface.Description != nil && *iface.Description == "Disabled by Fabric" {
			continue
		}

		if ocIface.Subinterfaces != nil && len(ocIface.Subinterfaces.Subinterface) > 0 {
			for id, sub := range ocIface.Subinterfaces.Subinterface {
				if sub.Config == nil {
					continue
				}

				subIface := &dozer.SpecSubinterface{
					IPs:        map[string]*dozer.SpecInterfaceIP{},
					StaticARPs: map[string]*dozer.SpecStaticARP{},
				}

				if sub.Ipv4 != nil && sub.Ipv4.Addresses != nil {
					for _, addr := range sub.Ipv4.Addresses.Address {
						if addr.Config == nil || addr.Config.Ip == nil {
							continue
						}

						subIface.IPs[*addr.Config.Ip] = &dozer.SpecInterfaceIP{
							PrefixLen: addr.Config.PrefixLength,
						}
					}
				}

				if sub.Ipv4 != nil && sub.Ipv4.SagIpv4 != nil && sub.Ipv4.SagIpv4.Config != nil {
					subIface.AnycastGateways = sub.Ipv4.SagIpv4.Config.StaticAnycastGateway
				}

				if sub.Ipv4 != nil && sub.Ipv4.ProxyArp != nil && sub.Ipv4.ProxyArp.Config != nil {
					pa, err := unmarshalProxyARP(sub.Ipv4.ProxyArp.Config.Mode)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to unmarshal proxy-arp for %s.%d", name, id)
					}
					subIface.ProxyARP = pa
				}

				if sub.Ipv4 != nil && sub.Ipv4.Neighbors != nil && len(sub.Ipv4.Neighbors.Neighbor) > 0 {
					for _, n := range sub.Ipv4.Neighbors.Neighbor {
						if n.Config == nil || n.Config.Ip == nil || n.Config.LinkLayerAddress == nil {
							continue
						}
						subIface.StaticARPs[*n.Config.Ip] = &dozer.SpecStaticARP{
							IP:  *n.Config.Ip,
							MAC: *n.Config.LinkLayerAddress,
						}
					}
				}

				if sub.Vlan != nil {
					if sub.Vlan.Config != nil {
						subIface.VLAN, err = unmarshalVLAN(sub.Vlan.Config.VlanId)
						if err != nil {
							return nil, errors.Wrapf(err, "failed to unmarshal VLAN for %s.%d", name, id)
						}
					}

					if sub.Vlan.State != nil {
						subIface.VLAN, err = unmarshalVLAN(sub.Vlan.State.VlanId)
						if err != nil {
							return nil, errors.Wrapf(err, "failed to unmarshal VLAN for %s.%d", name, id)
						}
					}
				}

				iface.Subinterfaces[id] = subIface
			}
		}

		vlan := false
		if ocIface.RoutedVlan != nil {
			vlan = true
			if ocIface.RoutedVlan.Ipv4 != nil {
				if ocIface.RoutedVlan.Ipv4.Addresses != nil {
					for _, addr := range ocIface.RoutedVlan.Ipv4.Addresses.Address {
						if addr.Config == nil || addr.Config.Ip == nil {
							continue
						}

						iface.VLANIPs[*addr.Config.Ip] = &dozer.SpecInterfaceIP{
							PrefixLen: addr.Config.PrefixLength,
						}
					}

					if ocIface.RoutedVlan.Ipv4.Config != nil {
						iface.Enabled = pointer.To(true) // just to keep track of the fact that there is a config for it
					}
				}
				if ocIface.RoutedVlan.Ipv4.SagIpv4 != nil && ocIface.RoutedVlan.Ipv4.SagIpv4.Config != nil {
					iface.VLANAnycastGateway = ocIface.RoutedVlan.Ipv4.SagIpv4.Config.StaticAnycastGateway
				}
				if ocIface.RoutedVlan.Ipv4.ProxyArp != nil && ocIface.RoutedVlan.Ipv4.ProxyArp.Config != nil {
					pa, err := unmarshalProxyARP(ocIface.RoutedVlan.Ipv4.ProxyArp.Config.Mode)
					if err != nil {
						return nil, errors.Wrapf(err, "failed to unmarshal proxy-arp for VLAN interface %s", name)
					}
					iface.ProxyARP = pa
				}
				if ocIface.RoutedVlan.Ipv4.Neighbors != nil && len(ocIface.RoutedVlan.Ipv4.Neighbors.Neighbor) > 0 {
					for _, n := range ocIface.RoutedVlan.Ipv4.Neighbors.Neighbor {
						if n.Config == nil || n.Config.Ip == nil || n.Config.LinkLayerAddress == nil {
							continue
						}
						iface.StaticARPs[*n.Config.Ip] = &dozer.SpecStaticARP{
							IP:  *n.Config.Ip,
							MAC: *n.Config.LinkLayerAddress,
						}
					}
				}
			}
		}
		if vlan && !isVLAN(name) {
			return nil, errors.Errorf("interface %s has VLAN config but not a Vlan", name)
		}

		if ocIface.Ethernet != nil {
			if ocIface.Ethernet.Config != nil {
				iface.PortChannel = ocIface.Ethernet.Config.AggregateId

				if !isManagement(name) && !skipSpeedPorts[name] { // TODO support configuring speed on Mgmt interface
					iface.Speed = UnmarshalPortSpeed(ocIface.Ethernet.Config.PortSpeed)
				}

				iface.AutoNegotiate = ocIface.Ethernet.Config.AutoNegotiate
			}

			if ocIface.Ethernet.SwitchedVlan != nil && ocIface.Ethernet.SwitchedVlan.Config != nil {
				var err error
				iface.TrunkVLANs, err = unmarshalEthernetTrunkVLANs(ocIface.Ethernet.SwitchedVlan.Config.TrunkVlans)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to unmarshal trunk VLANs")
				}
				iface.AccessVLAN = ocIface.Ethernet.SwitchedVlan.Config.AccessVlan
			}
		}
		if iface.PortChannel != nil && !isPhysical(name) && !isVLAN(name) {
			return nil, errors.Errorf("interface %s is a port channel member but it's not Ethernet or Vlan", name)
		}

		if isPortChannel(name) && ocIface.Aggregation != nil && ocIface.Aggregation.SwitchedVlan != nil && ocIface.Aggregation.SwitchedVlan.Config != nil {
			var err error
			iface.TrunkVLANs, err = unmarshalPortChannelTrunkVLANs(ocIface.Aggregation.SwitchedVlan.Config.TrunkVlans)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to unmarshal trunk VLANs")
			}
			iface.AccessVLAN = ocIface.Aggregation.SwitchedVlan.Config.AccessVlan
		}

		if isPhysical(name) && iface.Enabled != nil && !*iface.Enabled && (iface.Description == nil || *iface.Description == "") {
			// it's disabled we ignore it
			continue
		}

		interfaces[name] = iface
	}

	return interfaces, nil
}

// TODO dedup
func marshalEthernetTrunkVLANs(value *dozer.SpecInterface) ([]oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan_Config_TrunkVlans_Union, error) {
	trunkVLANs := []oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan_Config_TrunkVlans_Union{}
	for _, vlan := range value.TrunkVLANs {
		if strings.Contains(vlan, "..") {
			trunkVLANs = append(trunkVLANs, oc.UnionString(vlan))
		} else {
			value, err := strconv.ParseUint(vlan, 10, 16)
			if err != nil {
				return nil, errors.Wrapf(err, "can't parse %s", vlan)
			}
			vlanVal := uint16(value)
			trunkVLANs = append(trunkVLANs, oc.UnionUint16(vlanVal))
		}
	}

	return trunkVLANs, nil
}

// TODO dedup
func marshalPortChannelTrunkVLANs(value *dozer.SpecInterface) ([]oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config_TrunkVlans_Union, error) {
	trunkVLANs := []oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config_TrunkVlans_Union{}
	for _, vlan := range value.TrunkVLANs {
		if strings.Contains(vlan, "..") {
			trunkVLANs = append(trunkVLANs, oc.UnionString(vlan))
		} else {
			value, err := strconv.ParseUint(vlan, 10, 16)
			if err != nil {
				return nil, errors.Wrapf(err, "can't parse %s", vlan)
			}
			vlanVal := uint16(value)
			trunkVLANs = append(trunkVLANs, oc.UnionUint16(vlanVal))
		}
	}

	return trunkVLANs, nil
}

// TODO dedup
func unmarshalEthernetTrunkVLANs(vlans []oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan_Config_TrunkVlans_Union) ([]string, error) {
	trunkVLANs := []string{}
	for _, vlan := range vlans {
		if str, ok := vlan.(oc.UnionString); ok {
			trunkVLANs = append(trunkVLANs, string(str))
		} else if num, ok := vlan.(oc.UnionUint16); ok {
			trunkVLANs = append(trunkVLANs, strconv.FormatUint(uint64(num), 10))
		} else {
			return nil, errors.Errorf("unknown type %v", vlan)
		}
	}

	return trunkVLANs, nil
}

// TODO dedup
func unmarshalPortChannelTrunkVLANs(vlans []oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config_TrunkVlans_Union) ([]string, error) {
	trunkVLANs := []string{}
	for _, vlan := range vlans {
		if str, ok := vlan.(oc.UnionString); ok {
			trunkVLANs = append(trunkVLANs, string(str))
		} else if num, ok := vlan.(oc.UnionUint16); ok {
			trunkVLANs = append(trunkVLANs, strconv.FormatUint(uint64(num), 10))
		} else {
			return nil, errors.Errorf("unknown type %v", vlan)
		}
	}

	return trunkVLANs, nil
}

func unmarshalVLAN(in any) (*uint16, error) {
	if strVal, ok := in.(oc.UnionString); ok {
		vlanVal, err := strconv.ParseUint(string(strVal), 10, 16)
		if err != nil {
			return nil, errors.Wrapf(err, "can't parse %s", in)
		}

		return pointer.To(uint16(vlanVal)), nil
	} else if numVal, ok := in.(oc.UnionUint16); ok {
		return pointer.To(uint16(numVal)), nil
	}

	return nil, errors.Errorf("unknown vlan id type %v", in)
}
