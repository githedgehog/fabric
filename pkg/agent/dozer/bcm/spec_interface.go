package bcm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

const (
	INTERFACE_PREFIX_MANAGEMENT    = "Management"
	INTERFACE_PREFIX_PHYSICAL      = "Ethernet"
	INTERFACE_PREFIX_VLAN          = "Vlan"
	INTERFACE_PREFIX_PORT_CHANNEL  = "PortChannel"
	INTERFACE_DISABLED_DESCRIPTION = "Disabled by Fabric"
)

func isManagement(name string) bool {
	return strings.HasPrefix(name, INTERFACE_PREFIX_MANAGEMENT)
}

func isPhysical(name string) bool {
	return strings.HasPrefix(name, INTERFACE_PREFIX_PHYSICAL)
}

func isVLAN(name string) bool {
	return strings.HasPrefix(name, INTERFACE_PREFIX_VLAN)
}

func isPortChannel(name string) bool {
	return strings.HasPrefix(name, INTERFACE_PREFIX_PORT_CHANNEL)
}

var specInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecInterface]{
	Summary:      "Interfaces",
	ValueHandler: specInterfaceEnforcer,
}

var specInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "Interface %s",
	CustomHandler: func(basePath string, name string, actual, desired *dozer.SpecInterface, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/interfaces/interface[name=%s]", name)

		if err := specInterfaceBaseEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface base")
		}

		actualIPs, desiredIPs := ValueOrNil(actual, desired,
			func(value *dozer.SpecInterface) map[string]*dozer.SpecInterfaceIP { return value.VLANIPs })
		if err := specInterfaceVLANIPsEnforcer.Handle(basePath, actualIPs, desiredIPs, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface IPs")
		}

		if err := specInterfaceEthernetEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface port channel member")
		}

		if err := specInterfaceNATZoneEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle interface NAT zone")
		}

		if err := specInterfacesPortChannelEnforcer.Handle(basePath, name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle port channel")
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

var specInterfaceBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Getter: func(key string, value *dozer.SpecInterface) any {
		return []any{value.Description, value.Enabled, value.MTU}
	},
	Summary:      "Interface %s base",
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfaceBaseUpdate,
	DeleteWeight: ActionWeightInterfaceBaseDelete,
	MutateDesired: func(name string, desired *dozer.SpecInterface) *dozer.SpecInterface {
		if (isManagement(name) || isPhysical(name)) && desired == nil {
			return &dozer.SpecInterface{
				Enabled:     ygot.Bool(false),
				Description: ygot.String(INTERFACE_DISABLED_DESCRIPTION),
			}
		}
		return desired
	},
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		val := &oc.OpenconfigInterfaces_Interfaces_Interface{
			Name: ygot.String(name),
			Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
				Name:        ygot.String(name),
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
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Config{
						Enabled: ygot.Bool(true),
					},
				},
			}
		}

		return &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				name: val,
			},
		}, nil
	},
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
					Ip: ygot.String(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses_Address_Config{
						Ip:           ygot.String(name),
						PrefixLength: value.PrefixLen,
						Secondary:    ygot.Bool(false),
					},
				},
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

		actualIPs, desiredIPs := ValueOrNil(actual, desired,
			func(value *dozer.SpecSubinterface) map[string]*dozer.SpecInterfaceIP { return value.IPs })
		if err := specInterfaceSubinterfaceIPsEnforcer.Handle(basePath, actualIPs, desiredIPs, actions); err != nil {
			return errors.Wrap(err, "failed to handle subinterface IPs")
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
					Index: ygot.Uint32(idx),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Config{
						Index: ygot.Uint32(idx),
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
					Ip: ygot.String(ip),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address_Config{
						Ip:           ygot.String(ip),
						PrefixLength: value.PrefixLen,
						Secondary:    ygot.Bool(false),
					},
				},
			},
		}, nil
	},
}

var specInterfaceEthernetEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "Interface %s Ethernet", // TODO better summary
	Skip:    func(name string, actual, desired *dozer.SpecInterface) bool { return !isPhysical(name) },
	Getter: func(name string, value *dozer.SpecInterface) any {
		return []any{value.PortChannel, value.Speed, value.TrunkVLANs, value.AccessVLAN}
	},
	Path:         "/ethernet",
	NoReplace:    true, // TODO can we enable replace? so we can delete the speed config and portchannel member from it
	UpdateWeight: ActionWeightInterfacePortChannelMemberUpdate,
	DeleteWeight: ActionWeightInterfacePortChannelMemberDelete,
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		speed := oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET

		if value.Speed != nil {
			var ok bool
			speed, ok = MarshalPortSpeed(*value.Speed)
			if !ok {
				return nil, errors.Errorf("invalid speed %s", *value.Speed)
			}
		}

		// TODO move it to a separate enforcer as we'll not be able to replace it
		var switched *oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan
		if len(value.TrunkVLANs) > 0 || value.AccessVLAN != nil {
			trunkVLANs, err := marshalEthernetTrunkVLANs(value)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal trunk VLANs")
			}

			switched = &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_SwitchedVlan_Config{
					// InterfaceMode: oc.OpenconfigVlan_VlanModeType_UNSET, // TODO should we use TRUNK or ACCESS?
					TrunkVlans: trunkVLANs,
					AccessVlan: value.AccessVLAN,
				},
			}
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface{
			Ethernet: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_Config{
					AggregateId: value.PortChannel,
					PortSpeed:   speed,
				},
				SwitchedVlan: switched,
			},
		}, nil
	},
}

var specInterfaceNATZoneEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary:      "Interface %s NAT zone",
	Getter:       func(name string, value *dozer.SpecInterface) any { return value.NATZone },
	Path:         "/nat-zone",
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfaceNATZoneUpdate,
	DeleteWeight: ActionWeightInterfaceNATZoneDelete,
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface{
			NatZone: &oc.OpenconfigInterfaces_Interfaces_Interface_NatZone{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_NatZone_Config{
					NatZone: value.NATZone,
				},
			},
		}, nil
	},
}

var specInterfacesPortChannelEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary: "PortChannel %s",
	Skip:    func(name string, actual, desired *dozer.SpecInterface) bool { return !isPortChannel(name) },
	Getter: func(name string, value *dozer.SpecInterface) any {
		return []any{value.TrunkVLANs, value.AccessVLAN}
	},
	Path:         "/aggregation",
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfacePortChannelUpdate,
	DeleteWeight: ActionWeightInterfacePortChannelDelete,
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		var switched *oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan

		if value.TrunkVLANs != nil {
			trunkVLANs, err := marshalPortChannelTrunkVLANs(value)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to marshal trunk VLANs")
			}

			// TODO extract to a separate enforcer as we'll not be able to replace TrunkVLANs
			switched = &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config{
					InterfaceMode: oc.OpenconfigVlan_VlanModeType_TRUNK,
					TrunkVlans:    trunkVLANs,
					AccessVlan:    value.AccessVLAN, // TODO should we use UNSET mode or would it work with TRUNK or we should use NativeVlan?
				},
			}
		}

		val := &oc.OpenconfigInterfaces_Interfaces_Interface{
			Aggregation: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation{
				Config:       &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_Config{},
				SwitchedVlan: switched,
			},
		}

		return val, nil
	},
}

var specInterfaceVLANAnycastGatewayEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary:      "Interface %s VLAN Anycast Gateway",
	Skip:         func(name string, actual, desired *dozer.SpecInterface) bool { return !isVLAN(name) },
	Getter:       func(name string, value *dozer.SpecInterface) any { return value.VLANAnycastGateway },
	Path:         "/routed-vlan/ipv4/sag-ipv4/config/static-anycast-gateway",
	SkipDelete:   true, // TODO check if it's ok
	UpdateWeight: ActionWeightInterfaceVLANAnycastGatewayUpdate,
	DeleteWeight: ActionWeightInterfaceVLANAnycastGatewayDelete,
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_SagIpv4_Config{
			StaticAnycastGateway: value.VLANAnycastGateway,
		}, nil
	},
}

func loadActualInterfaces(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocInterfaces := &oc.OpenconfigInterfaces_Interfaces{}
	err := client.Get(ctx, "/interfaces/interface", ocInterfaces, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read interfaces")
	}
	spec.Interfaces, err = unmarshalOCInterfaces(ocInterfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal interfaces")
	}

	return nil
}

func unmarshalOCInterfaces(ocVal *oc.OpenconfigInterfaces_Interfaces) (map[string]*dozer.SpecInterface, error) {
	interfaces := map[string]*dozer.SpecInterface{}

	if ocVal == nil {
		return interfaces, nil
	}

	for name, ocIface := range ocVal.Interface {
		if ocIface.Config == nil {
			continue
		}

		if strings.HasPrefix(name, "vtep") {
			continue
		}

		mtu := ocIface.Config.Mtu
		if mtu != nil { // TODO it's a hack for now, assuming 9100 is a default MTU for everything other than Mgmt interface (1500)
			if isManagement(name) && *mtu == 1500 || !isManagement(name) && *mtu == 9100 {
				mtu = nil
			}

			mtu = nil
		}

		iface := &dozer.SpecInterface{
			Description:   ocIface.Config.Description,
			Enabled:       ocIface.Config.Enabled,
			MTU:           mtu,
			Subinterfaces: map[uint32]*dozer.SpecSubinterface{},
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
					IPs: map[string]*dozer.SpecInterfaceIP{},
				}

				if sub.Ipv4 != nil && sub.Ipv4.Addresses != nil {
					if len(sub.Ipv4.Addresses.Address) != 1 {
						return nil, errors.Errorf("only one IP address expected on subinterface %s.%d", name, id)
					}

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

				if sub.Vlan != nil && sub.Vlan.Config != nil {
					var vlan *uint16

					vlanID := sub.Vlan.Config.VlanId
					if strVal, ok := vlanID.(oc.UnionString); ok {
						vlanVal, err := strconv.ParseUint(string(strVal), 10, 16)
						if err != nil {
							return nil, errors.Wrapf(err, "can't parse %s", vlanID)
						}
						vlan = ygot.Uint16(uint16(vlanVal))
					} else if numVal, ok := vlanID.(oc.UnionUint16); ok {
						vlan = ygot.Uint16(uint16(numVal))
					} else {
						return nil, errors.Errorf("unknown vlan id type %v for %s.%d", vlanID, name, id)
					}

					subIface.VLAN = vlan
				}

				iface.Subinterfaces[id] = subIface
			}
		}

		vlan := false
		if ocIface.RoutedVlan != nil {
			vlan = true
			if ocIface.RoutedVlan.Ipv4 != nil {
				if ocIface.RoutedVlan.Ipv4.Addresses != nil {
					if len(ocIface.RoutedVlan.Ipv4.Addresses.Address) != 1 {
						return nil, errors.Errorf("only one IP address expected on interface %s routed vlan", name)
					}

					for _, addr := range ocIface.RoutedVlan.Ipv4.Addresses.Address {
						if addr.Config == nil || addr.Config.Ip == nil {
							continue
						}

						iface.VLANIPs[*addr.Config.Ip] = &dozer.SpecInterfaceIP{
							PrefixLen: addr.Config.PrefixLength,
						}
					}

					if ocIface.RoutedVlan.Ipv4.Config != nil {
						iface.Enabled = ocIface.RoutedVlan.Ipv4.Config.Enabled
					}
				}
				if ocIface.RoutedVlan.Ipv4.SagIpv4 != nil && ocIface.RoutedVlan.Ipv4.SagIpv4.Config != nil {
					iface.VLANAnycastGateway = ocIface.RoutedVlan.Ipv4.SagIpv4.Config.StaticAnycastGateway
				}
			}
		}
		if vlan && !isVLAN(name) {
			return nil, errors.Errorf("interface %s has VLAN config but not a Vlan", name)
		}

		if ocIface.Ethernet != nil && ocIface.Ethernet.Config != nil {
			iface.PortChannel = ocIface.Ethernet.Config.AggregateId

			if !isManagement(name) { // TODO support configuring speed on Mgmt interface
				iface.Speed = UnmarshalPortSpeed(ocIface.Ethernet.Config.PortSpeed)
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

		if ocIface.Aggregation != nil && ocIface.Aggregation.SwitchedVlan != nil && ocIface.Aggregation.SwitchedVlan.Config != nil {
			var err error
			iface.TrunkVLANs, err = unmarshalPortChannelTrunkVLANs(ocIface.Aggregation.SwitchedVlan.Config.TrunkVlans)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to unmarshal trunk VLANs")
			}
			iface.AccessVLAN = ocIface.Aggregation.SwitchedVlan.Config.AccessVlan
		}

		if ocIface.NatZone != nil && ocIface.NatZone.Config != nil {
			if ocIface.NatZone.Config.NatZone != nil && *ocIface.NatZone.Config.NatZone != 0 {
				iface.NATZone = ocIface.NatZone.Config.NatZone
			}
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
