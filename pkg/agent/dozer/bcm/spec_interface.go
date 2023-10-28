package bcm

import (
	"context"
	"fmt"
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
			func(value *dozer.SpecInterface) map[string]*dozer.SpecInterfaceIP { return value.IPs })
		if err := specInterfaceIPsEnforcer.Handle(basePath, actualIPs, desiredIPs, actions); err != nil {
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

		return nil
	},
}

var specInterfaceBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Getter:       func(key string, value *dozer.SpecInterface) any { return []any{value.Description, value.Enabled} },
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

var specInterfaceIPsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecInterfaceIP]{
	Summary:      "Interface %s IPs",
	ValueHandler: specInterfaceIPEnforcer,
}

var specInterfaceIPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterfaceIP]{
	Summary:      "Interface IP %s", // TODO chain summary as well?
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfaceIPUpdate,
	DeleteWeight: ActionWeightInterfaceIPDelete,
	PathFunc: func(name string, value *dozer.SpecInterfaceIP) string {
		if value.VLAN != nil && *value.VLAN {
			return fmt.Sprintf("/routed-vlan/ipv4/addresses/address[ip=%s]", name)
		}

		return fmt.Sprintf("/subinterfaces/subinterface[index=0]/ipv4[ip=%s]", name)
	},
	Marshal: func(name string, value *dozer.SpecInterfaceIP) (ygot.ValidatedGoStruct, error) {
		if value.VLAN != nil && *value.VLAN {
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
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface{
			Ipv4: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4{
				Addresses: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses{
					Address: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address{
						name: {
							Ip: ygot.String(name),
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address_Config{
								Ip:           ygot.String(name),
								PrefixLength: value.PrefixLen,
								Secondary:    ygot.Bool(false),
							},
						},
					},
				},
			},
		}, nil
	},
}

var specInterfaceEthernetEnforcer = &DefaultValueEnforcer[string, *dozer.SpecInterface]{
	Summary:      "Interface %s Ethernet", // TODO better summary
	Getter:       func(name string, value *dozer.SpecInterface) any { return []any{value.PortChannel, value.Speed} },
	Path:         "/ethernet",
	NoReplace:    true, // TODO can we enable replace? so we can delete the speed config and portchannel member from it
	UpdateWeight: ActionWeightInterfacePortChannelMemberUpdate,
	DeleteWeight: ActionWeightInterfacePortChannelMemberDelete,
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		speed := oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET

		if value.Speed != nil {
			speedR := *value.Speed
			if !strings.HasPrefix(speedR, "SPEED_") {
				speedR = "SPEED_" + speedR
			}

			ok := false
			for speedVal, name := range oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"] {
				if name.Name == speedR {
					speed = oc.E_OpenconfigIfEthernet_ETHERNET_SPEED(speedVal)
					ok = true
					break
				}
			}
			if !ok {
				return nil, errors.Errorf("invalid speed %s", speedR)
			}
		}

		return &oc.OpenconfigInterfaces_Interfaces_Interface{
			Ethernet: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_Config{
					AggregateId: value.PortChannel,
					PortSpeed:   speed,
				},
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
	Summary:      "PortChannel %s",
	Getter:       func(name string, value *dozer.SpecInterface) any { return value.PortChannel },
	Path:         "/aggregation",
	NoReplace:    true,
	UpdateWeight: ActionWeightInterfacePortChannelUpdate,
	DeleteWeight: ActionWeightInterfacePortChannelDelete,
	Marshal: func(name string, value *dozer.SpecInterface) (ygot.ValidatedGoStruct, error) {
		var switched *oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan

		if value.TrunkVLANRange != nil {
			switched = &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan{
				Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config{
					TrunkVlans: []oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config_TrunkVlans_Union{
						oc.UnionString(*value.TrunkVLANRange),
					},
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

		iface := &dozer.SpecInterface{
			Description: ocIface.Config.Description,
			Enabled:     ocIface.Config.Enabled,
			IPs:         map[string]*dozer.SpecInterfaceIP{},
		}

		// just skip interfaces disabled by Fabric
		if iface.Enabled != nil && !*iface.Enabled && iface.Description != nil && *iface.Description == "Disabled by Fabric" {
			continue
		}

		if ocIface.Subinterfaces != nil && len(ocIface.Subinterfaces.Subinterface) > 0 {
			if len(ocIface.Subinterfaces.Subinterface) != 1 {
				return nil, errors.Errorf("only one subinterface expected on interface %s", name)
			}

			sub := ocIface.Subinterfaces.Subinterface[0]
			if sub.Ipv4 != nil && sub.Ipv4.Addresses != nil {
				if len(sub.Ipv4.Addresses.Address) != 1 {
					return nil, errors.Errorf("only one IP address expected on interface %s", name)
				}

				for _, addr := range sub.Ipv4.Addresses.Address {
					if addr.Config == nil || addr.Config.Ip == nil {
						continue
					}

					iface.IPs[*addr.Config.Ip] = &dozer.SpecInterfaceIP{
						PrefixLen: addr.Config.PrefixLength,
					}
				}
			}
		}

		vlan := false
		if ocIface.RoutedVlan != nil {
			vlan = true
			if ocIface.RoutedVlan.Ipv4 != nil && ocIface.RoutedVlan.Ipv4.Addresses != nil {
				if len(ocIface.RoutedVlan.Ipv4.Addresses.Address) != 1 {
					return nil, errors.Errorf("only one IP address expected on interface %s routed vlan", name)
				}

				for _, addr := range ocIface.RoutedVlan.Ipv4.Addresses.Address {
					if addr.Config == nil || addr.Config.Ip == nil {
						continue
					}

					iface.IPs[*addr.Config.Ip] = &dozer.SpecInterfaceIP{
						VLAN:      ygot.Bool(true),
						PrefixLen: addr.Config.PrefixLength,
					}
				}

				if ocIface.RoutedVlan.Ipv4.Config != nil {
					iface.Enabled = ocIface.RoutedVlan.Ipv4.Config.Enabled
				}
			}
		}
		if vlan && !isVLAN(name) {
			return nil, errors.Errorf("interface %s has VLAN config but not a Vlan", name)
		}

		if ocIface.Ethernet != nil && ocIface.Ethernet.Config != nil {
			iface.PortChannel = ocIface.Ethernet.Config.AggregateId

			speed := ocIface.Ethernet.Config.PortSpeed
			if speed > 0 && speed < oc.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_UNKNOWN {
				speedName, _ := strings.CutPrefix(oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"][int64(speed)].Name, "SPEED_")
				iface.Speed = ygot.String(speedName)
			}
		}
		if iface.PortChannel != nil && !isPhysical(name) && !isVLAN(name) {
			return nil, errors.Errorf("interface %s is a port channel member but it's not Ethernet or Vlan", name)
		}

		if ocIface.Aggregation != nil && ocIface.Aggregation.SwitchedVlan != nil && ocIface.Aggregation.SwitchedVlan.Config != nil {
			if len(ocIface.Aggregation.SwitchedVlan.Config.TrunkVlans) != 1 {
				return nil, errors.Errorf("only one trunk VLAN range expected on interface with switched vlan config %s", name)
			}

			val := ocIface.Aggregation.SwitchedVlan.Config.TrunkVlans[0]
			if str, ok := val.(oc.UnionString); ok {
				iface.TrunkVLANRange = stringPtr(string(str))
			} else {
				return nil, errors.Errorf("trunk VLAN range expected to be string on interface with switched vlan config %s", name)
			}
		}
		if iface.TrunkVLANRange != nil && !isPortChannel(name) {
			return nil, errors.Errorf("interface %s has trunk VLAN range config but not a PortChannel", name)
		}

		if ocIface.NatZone != nil && ocIface.NatZone.Config != nil {
			iface.NATZone = ocIface.NatZone.Config.NatZone
		}

		if isPhysical(name) && iface.Enabled != nil && !*iface.Enabled && (iface.Description == nil || *iface.Description == "") {
			// it's disabled we ignore it
			continue
		}

		interfaces[name] = iface
	}

	return interfaces, nil
}
