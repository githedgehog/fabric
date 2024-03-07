package bcm

import (
	"context"
	"fmt"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specACLsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecACL]{
	Summary:      "ACLs",
	ValueHandler: specACLEnforcer,
}

var specACLEnforcer = &DefaultValueEnforcer[string, *dozer.SpecACL]{
	Summary: "ACL %s",
	CustomHandler: func(basePath string, name string, actual, desired *dozer.SpecACL, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/acl/acl-sets/acl-set[name=%s][type=ACL_IPV4]", name)

		// we aren't passing basepath here as we need to custom handle it
		if err := specACLBaseEnforcer.Handle("", name, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle acl base")
		}

		actualEntries, desiredEntries := ValueOrNil(actual, desired,
			func(value *dozer.SpecACL) map[uint32]*dozer.SpecACLEntry { return value.Entries })
		if err := specACLEntriesEnforcer.Handle(basePath, actualEntries, desiredEntries, actions); err != nil {
			return errors.Wrap(err, "failed to handle acl entries")
		}

		return nil
	},
}

var specACLBaseEnforcer = &DefaultValueEnforcer[string, *dozer.SpecACL]{
	Summary:    "ACL %s base",
	Path:       "/acl/acl-sets/acl-set[name=%s][type=ACL_IPV4]",
	CreatePath: "/acl/acl-sets/acl-set",
	MutateDesired: func(key string, desired *dozer.SpecACL) *dozer.SpecACL {
		if desired != nil && desired.Description == nil {
			desired.Description = ygot.String(key) // workaround to avoid skipping creation of the ACLs with empty description
		}

		return desired
	},
	Getter:       func(name string, value *dozer.SpecACL) any { return value.Description },
	UpdateWeight: ActionWeightACLBaseUpdate,
	DeleteWeight: ActionWeightACLBaseDelete,
	Marshal: func(name string, value *dozer.SpecACL) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigAcl_Acl_AclSets{
			AclSet: map[oc.OpenconfigAcl_Acl_AclSets_AclSet_Key]*oc.OpenconfigAcl_Acl_AclSets_AclSet{
				{
					Type: oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					Name: name,
				}: {
					Name: ygot.String(name),
					Type: oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_Config{
						Name:        ygot.String(name),
						Type:        oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						Description: value.Description,
					},
				},
			},
		}, nil
	},
}

var specACLEntriesEnforcer = &DefaultMapEnforcer[uint32, *dozer.SpecACLEntry]{
	Summary:      "ACL entries",
	ValueHandler: specACLEntryEnforcer,
}

var specACLEntryEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecACLEntry]{
	Summary:          "ACL entry %d",
	Path:             "/acl-entries/acl-entry[sequence-id=%d]",
	CreatePath:       "/acl-entries/acl-entry",
	RecreateOnUpdate: true, // TODO validate
	UpdateWeight:     ActionWeightACLEntryUpdate,
	DeleteWeight:     ActionWeightACLEntryDelete,
	Marshal: func(seq uint32, value *dozer.SpecACLEntry) (ygot.ValidatedGoStruct, error) {
		var action oc.E_OpenconfigAcl_FORWARDING_ACTION
		switch value.Action {
		case "":
			// just unset
		case dozer.SpecACLEntryActionAccept:
			action = oc.OpenconfigAcl_FORWARDING_ACTION_ACCEPT
		case dozer.SpecACLEntryActionDrop:
			action = oc.OpenconfigAcl_FORWARDING_ACTION_DROP
		default:
			return nil, errors.Errorf("unknown ACL Entry action: %s", value.Action)
		}

		var protocol oc.E_OpenconfigPacketMatchTypes_IP_PROTOCOL
		switch value.Protocol {
		case "":
			// just unset
		case dozer.SpecACLEntryProtocolUDP:
			protocol = oc.E_OpenconfigPacketMatchTypes_IP_PROTOCOL(oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_UDP)
		default:
			return nil, errors.Errorf("unknown ACL Entry protocol: %s", value.Protocol)
		}

		transport := &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport{
			Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Transport_Config{},
		}
		if value.SourcePort != nil {
			transport.Config.SourcePort = oc.UnionUint16(*value.SourcePort)
		}
		if value.DestinationPort != nil {
			transport.Config.DestinationPort = oc.UnionUint16(*value.DestinationPort)
		}

		return &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries{
			AclEntry: map[uint32]*oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry{
				seq: {
					SequenceId: ygot.Uint32(seq),
					Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Config{
						SequenceId:  ygot.Uint32(seq),
						Description: value.Description,
					},
					Actions: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Actions{
						Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Actions_Config{
							ForwardingAction: action,
						},
					},
					Ipv4: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4{
						Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Ipv4_Config{
							Protocol:           protocol,
							SourceAddress:      value.SourceAddress,
							DestinationAddress: value.DestinationAddress,
						},
					},
					Transport: transport,
				},
			},
		}, nil
	},
}

var specACLInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecACLInterface]{
	Summary:      "ACL interfaces",
	ValueHandler: specACLInterfaceEnforcer,
}

// TODO there is a good chance that it'll not be able to replace the ACLs for interface but we don't need it now
var specACLInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecACLInterface]{
	Summary:      "ACL interface %s",
	Path:         "/acl/interfaces/interface[id=%s]",
	UpdateWeight: ActionWeightACLInterfaceUpdate,
	DeleteWeight: ActionWeightACLInterfaceDelete,
	Marshal: func(name string, value *dozer.SpecACLInterface) (ygot.ValidatedGoStruct, error) {
		var ingressAclSets *oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets
		if value.Ingress != nil {
			ingressAclSets = &oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets{
				IngressAclSet: map[oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Key]*oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet{
					{
						SetName: *value.Ingress,
						Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					}: {
						SetName: value.Ingress,
						Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						Config: &oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Config{
							SetName: value.Ingress,
							Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						},
					},
				},
			}
		}

		var egressAclSets *oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets
		if value.Egress != nil {
			egressAclSets = &oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets{
				EgressAclSet: map[oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Key]*oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet{
					{
						SetName: *value.Egress,
						Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					}: {
						SetName: value.Egress,
						Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						Config: &oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets_EgressAclSet_Config{
							SetName: value.Egress,
							Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						},
					},
				},
			}
		}

		return &oc.OpenconfigAcl_Acl_Interfaces{
			Interface: map[string]*oc.OpenconfigAcl_Acl_Interfaces_Interface{
				name: {
					Id: ygot.String(name),
					Config: &oc.OpenconfigAcl_Acl_Interfaces_Interface_Config{
						Id: ygot.String(name),
					},
					InterfaceRef: &oc.OpenconfigAcl_Acl_Interfaces_Interface_InterfaceRef{
						Config: &oc.OpenconfigAcl_Acl_Interfaces_Interface_InterfaceRef_Config{
							Interface: ygot.String(name),
						},
					},
					IngressAclSets: ingressAclSets,
					EgressAclSets:  egressAclSets,
				},
			},
		}, nil
	},
}

func loadActualACLs(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.OpenconfigAcl_Acl{}
	err := client.Get(ctx, "/acl/acl-sets", ocVal, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read acls")
	}

	spec.ACLs, err = unmarshalOCACLs(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal acls")
	}

	return nil
}

func unmarshalOCACLs(ocVal *oc.OpenconfigAcl_Acl) (map[string]*dozer.SpecACL, error) {
	acls := map[string]*dozer.SpecACL{}

	if ocVal == nil || ocVal.AclSets == nil {
		return acls, nil
	}

	for key, acl := range ocVal.AclSets.AclSet {
		if key.Type != oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
			continue
		}

		entries := map[uint32]*dozer.SpecACLEntry{}
		if acl.AclEntries != nil {
			for seq, entry := range acl.AclEntries.AclEntry {
				if entry.Config == nil {
					continue
				}

				var protocol dozer.SpecACLEntryProtocol
				var sourceAddress, destinationAddress *string
				if entry.Ipv4 != nil && entry.Ipv4.Config != nil {
					if entry.Ipv4.Config.SourceAddress != nil {
						sourceAddress = entry.Ipv4.Config.SourceAddress
					}
					if entry.Ipv4.Config.DestinationAddress != nil {
						destinationAddress = entry.Ipv4.Config.DestinationAddress
					}
					if entry.Ipv4.Config.Protocol == oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_UDP {
						protocol = dozer.SpecACLEntryProtocolUDP
					}
				}

				var sourcePort, destinationPort *uint16
				if entry.Transport != nil && entry.Transport.Config != nil {
					if entry.Transport.Config.SourcePort != nil {
						if union, ok := entry.Transport.Config.SourcePort.(oc.UnionUint16); ok {
							sourcePort = ygot.Uint16(uint16(union))
						}
					}
					if entry.Transport.Config.DestinationPort != nil {
						if union, ok := entry.Transport.Config.DestinationPort.(oc.UnionUint16); ok {
							destinationPort = ygot.Uint16(uint16(union))
						}
					}
				}

				var action dozer.SpecACLEntryAction
				if entry.Actions.Config.ForwardingAction == oc.OpenconfigAcl_FORWARDING_ACTION_ACCEPT {
					action = dozer.SpecACLEntryActionAccept
				} else if entry.Actions.Config.ForwardingAction == oc.OpenconfigAcl_FORWARDING_ACTION_DROP {
					action = dozer.SpecACLEntryActionDrop
				}

				entries[seq] = &dozer.SpecACLEntry{
					Description:        entry.Config.Description,
					SourceAddress:      sourceAddress,
					DestinationAddress: destinationAddress,
					Protocol:           protocol,
					SourcePort:         sourcePort,
					DestinationPort:    destinationPort,
					Action:             action,
				}
			}
		}

		acls[key.Name] = &dozer.SpecACL{
			Description: acl.Config.Description,
			Entries:     entries,
		}
	}

	return acls, nil
}

func loadActualACLInterfaces(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.OpenconfigAcl_Acl{}
	err := client.Get(ctx, "/acl/interfaces", ocVal, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read acl interfaces")
	}

	spec.ACLInterfaces, err = unmarshalOCACLInterfaces(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal acl interfaces")
	}

	return nil
}

func unmarshalOCACLInterfaces(ocVal *oc.OpenconfigAcl_Acl) (map[string]*dozer.SpecACLInterface, error) {
	interfaces := map[string]*dozer.SpecACLInterface{}

	if ocVal == nil || ocVal.Interfaces == nil {
		return interfaces, nil
	}

	for name, iface := range ocVal.Interfaces.Interface {
		if iface.IngressAclSets == nil && iface.EgressAclSets == nil {
			continue
		}

		var ingress *string
		var egress *string

		if iface.IngressAclSets != nil {
			for key, value := range iface.IngressAclSets.IngressAclSet {
				if key.Type != oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
					continue
				}

				ingress = value.SetName
			}
		}

		if iface.EgressAclSets != nil {
			for key, value := range iface.EgressAclSets.EgressAclSet {
				if key.Type != oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4 {
					continue
				}

				egress = value.SetName
			}
		}

		interfaces[name] = &dozer.SpecACLInterface{
			Ingress: ingress,
			Egress:  egress,
		}
	}

	return interfaces, nil
}
