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

	"github.com/openconfig/gnmic/pkg/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
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
			desired.Description = pointer.To(key) // workaround to avoid skipping creation of the ACLs with empty description
		}

		return desired
	},
	Getter:       func(_ string, value *dozer.SpecACL) any { return value.Description },
	UpdateWeight: ActionWeightACLBaseUpdate,
	DeleteWeight: ActionWeightACLBaseDelete,
	Marshal: func(name string, value *dozer.SpecACL) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigAcl_Acl_AclSets{
			AclSet: map[oc.OpenconfigAcl_Acl_AclSets_AclSet_Key]*oc.OpenconfigAcl_Acl_AclSets_AclSet{
				{
					Type: oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					Name: name,
				}: {
					Name: pointer.To(name),
					Type: oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_Config{
						Name:        pointer.To(name),
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
		case dozer.SpecACLEntryActionDiscard:
			action = oc.OpenconfigAcl_FORWARDING_ACTION_DISCARD
		case dozer.SpecACLEntryActionTransit:
			action = oc.OpenconfigAcl_FORWARDING_ACTION_TRANSIT
		default:
			return nil, errors.Errorf("unknown ACL Entry action: %s", value.Action)
		}

		var protocol oc.E_OpenconfigPacketMatchTypes_IP_PROTOCOL
		switch value.Protocol { //nolint:exhaustive
		case "", dozer.SpecACLEntryProtocolIP:
			// unset — matches any IP protocol
		case dozer.SpecACLEntryProtocolTCP:
			protocol = oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_TCP
		case dozer.SpecACLEntryProtocolUDP:
			protocol = oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_UDP
		case dozer.SpecACLEntryProtocolICMP:
			protocol = oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_ICMP
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
		if value.DestinationPortRange != nil { // mutually exclusive with DestinationPort; range takes precedence
			transport.Config.DestinationPort = oc.UnionString(*value.DestinationPortRange)
		}
		if len(value.TCPFlags) > 0 {
			flags, err := marshalTCPFlags(value.TCPFlags)
			if err != nil {
				return nil, err
			}
			transport.Config.TcpFlags = flags
		}
		if value.TCPSessionEstablished != nil {
			transport.Config.TcpSessionEstablished = value.TCPSessionEstablished
		}
		if value.ICMPType != nil {
			transport.Config.IcmpType = value.ICMPType
		}
		if value.ICMPCode != nil {
			transport.Config.IcmpCode = value.ICMPCode
		}

		return &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries{
			AclEntry: map[uint32]*oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry{
				seq: {
					SequenceId: pointer.To(seq),
					Config: &oc.OpenconfigAcl_Acl_AclSets_AclSet_AclEntries_AclEntry_Config{
						SequenceId:  pointer.To(seq),
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

var specACLInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecACLInterface]{
	Summary:      "ACL interface %s",
	Path:         "/acl/interfaces/interface[id=%s]",
	UpdateWeight: ActionWeightACLInterfaceUpdate,
	DeleteWeight: ActionWeightACLInterfaceDelete,
	// CustomHandler is used to work around a SONiC gNMI server bug: when an ingress ACL set
	// is included in a combined replace request, the SONiC handler panics with "assignment
	// to entry in nil map". The fix is to send the interface entry (with egress only) first,
	// then send ingress in a separate gNMI update.
	CustomHandler: func(_ string, name string, actual, desired *dozer.SpecACLInterface, actions *ActionQueue) error {
		ifacePath := fmt.Sprintf("/acl/interfaces/interface[id=%s]", name)

		if desired == nil {
			return actions.Add(&Action{
				Weight:   ActionWeightACLInterfaceDelete,
				ASummary: fmt.Sprintf("Delete ACL interface %s", name),
				Type:     ActionTypeDelete,
				Path:     ifacePath,
			})
		}

		actionType := ActionTypeUpdate
		actionSummary := fmt.Sprintf("Create ACL interface %s", name)
		if actual != nil {
			actionType = ActionTypeReplace
			actionSummary = fmt.Sprintf("Update ACL interface %s", name)
		}

		// omit ingress from the main request and send it separately below.
		val, err := marshalACLInterface(name, desired, false)
		if err != nil {
			return err
		}

		if err := actions.Add(&Action{
			Weight:   ActionWeightACLInterfaceUpdate,
			ASummary: actionSummary,
			Type:     actionType,
			Path:     ifacePath,
			Value:    val,
		}); err != nil {
			return errors.Wrap(err, "failed to add ACL interface action")
		}

		if desired.Ingress != nil {
			ingressVal := &oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets{
				IngressAclSet: map[oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Key]*oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet{
					{
						SetName: *desired.Ingress,
						Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
					}: {
						SetName: desired.Ingress,
						Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						Config: &oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets_IngressAclSet_Config{
							SetName: desired.Ingress,
							Type:    oc.OpenconfigAcl_ACL_TYPE_ACL_IPV4,
						},
					},
				},
			}

			if err := actions.Add(&Action{
				Weight:   ActionWeightACLInterfaceUpdate,
				ASummary: fmt.Sprintf("Update ACL interface %s ingress", name),
				Type:     ActionTypeUpdate,
				Path:     ifacePath + "/ingress-acl-sets/ingress-acl-set",
				Value:    ingressVal,
			}); err != nil {
				return errors.Wrap(err, "failed to add ACL interface ingress action")
			}
		}

		return nil
	},
}

// marshalACLInterface builds the ygot value for an ACL interface binding.
// When includeIngress is false, IngressAclSets is omitted from the result.
func marshalACLInterface(name string, value *dozer.SpecACLInterface, includeIngress bool) (ygot.ValidatedGoStruct, error) {
	var ingressACLSets *oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets
	if includeIngress && value.Ingress != nil {
		ingressACLSets = &oc.OpenconfigAcl_Acl_Interfaces_Interface_IngressAclSets{
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

	var egressACLSets *oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets
	if value.Egress != nil {
		egressACLSets = &oc.OpenconfigAcl_Acl_Interfaces_Interface_EgressAclSets{
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

	ifaceRef := &oc.OpenconfigAcl_Acl_Interfaces_Interface_InterfaceRef_Config{
		Interface: pointer.To(name),
	}
	if before, after, ok := strings.Cut(name, "."); ok {
		idx, err := strconv.ParseUint(after, 10, 32)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid subinterface index in ACL interface name %q", name)
		}
		ifaceRef.Interface = pointer.To(before)
		ifaceRef.Subinterface = pointer.To(uint32(idx))
	}

	return &oc.OpenconfigAcl_Acl_Interfaces{
		Interface: map[string]*oc.OpenconfigAcl_Acl_Interfaces_Interface{
			name: {
				Id: pointer.To(name),
				Config: &oc.OpenconfigAcl_Acl_Interfaces_Interface_Config{
					Id: pointer.To(name),
				},
				InterfaceRef: &oc.OpenconfigAcl_Acl_Interfaces_Interface_InterfaceRef{
					Config: ifaceRef,
				},
				IngressAclSets: ingressACLSets,
				EgressAclSets:  egressACLSets,
			},
		},
	}, nil
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

func unmarshalOCACLs(ocVal *oc.OpenconfigAcl_Acl) (map[string]*dozer.SpecACL, error) { //nolint:unparam
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
					// Protocol is a union type (E_IP_PROTOCOL or UnionUint8). When absent
					// in the gNMI response, ygot leaves the interface as nil — not as the
					// UNSET enum value (0). Match nil explicitly so that other valid but
					// unsupported protocols (IP_GRE, IP_AUTH, …) are not silently mapped to IP.
					switch entry.Ipv4.Config.Protocol {
					case nil, oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_UNSET:
						// absent or explicitly unset → match any IP
						protocol = dozer.SpecACLEntryProtocolIP
					case oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_TCP:
						protocol = dozer.SpecACLEntryProtocolTCP
					case oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_UDP:
						protocol = dozer.SpecACLEntryProtocolUDP
					case oc.OpenconfigPacketMatchTypes_IP_PROTOCOL_IP_ICMP:
						protocol = dozer.SpecACLEntryProtocolICMP
					}
				} else {
					// No IPv4 container returned by switch (e.g. entry with no addresses
					// and no specific protocol) → match any IP
					protocol = dozer.SpecACLEntryProtocolIP
				}

				var sourcePort, destinationPort *uint16
				var destinationPortRange *string
				var tcpFlags []dozer.SpecACLEntryTCPFlag
				var tcpSessionEstablished *bool
				var icmpType, icmpCode *uint8
				if entry.Transport != nil && entry.Transport.Config != nil {
					if entry.Transport.Config.SourcePort != nil {
						if union, ok := entry.Transport.Config.SourcePort.(oc.UnionUint16); ok {
							sourcePort = pointer.To(uint16(union))
						}
					}
					if entry.Transport.Config.DestinationPort != nil {
						switch v := entry.Transport.Config.DestinationPort.(type) {
						case oc.UnionUint16:
							destinationPort = pointer.To(uint16(v))
						case oc.UnionString:
							destinationPortRange = pointer.To(string(v))
						}
					}
					tcpFlags = unmarshalTCPFlags(entry.Transport.Config.TcpFlags)
					tcpSessionEstablished = entry.Transport.Config.TcpSessionEstablished
					icmpType = entry.Transport.Config.IcmpType
					icmpCode = entry.Transport.Config.IcmpCode
				}

				var action dozer.SpecACLEntryAction
				switch entry.Actions.Config.ForwardingAction { //nolint:exhaustive
				case oc.OpenconfigAcl_FORWARDING_ACTION_UNSET:
					// just unset
				case oc.OpenconfigAcl_FORWARDING_ACTION_ACCEPT:
					action = dozer.SpecACLEntryActionAccept
				case oc.OpenconfigAcl_FORWARDING_ACTION_DROP:
					action = dozer.SpecACLEntryActionDrop
				case oc.OpenconfigAcl_FORWARDING_ACTION_DISCARD:
					action = dozer.SpecACLEntryActionDiscard
				case oc.OpenconfigAcl_FORWARDING_ACTION_TRANSIT:
					action = dozer.SpecACLEntryActionTransit
				}

				entries[seq] = &dozer.SpecACLEntry{
					Description:           entry.Config.Description,
					SourceAddress:         sourceAddress,
					DestinationAddress:    destinationAddress,
					Protocol:              protocol,
					SourcePort:            sourcePort,
					DestinationPort:       destinationPort,
					DestinationPortRange:  destinationPortRange,
					TCPFlags:              tcpFlags,
					TCPSessionEstablished: tcpSessionEstablished,
					ICMPType:              icmpType,
					ICMPCode:              icmpCode,
					Action:                action,
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

func unmarshalOCACLInterfaces(ocVal *oc.OpenconfigAcl_Acl) (map[string]*dozer.SpecACLInterface, error) { //nolint:unparam
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

var tcpFlagToOC = map[dozer.SpecACLEntryTCPFlag]oc.E_OpenconfigPacketMatchTypes_TCP_FLAGS{
	dozer.SpecACLEntryTCPFlagFin:    oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN,
	dozer.SpecACLEntryTCPFlagNotFin: oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_FIN,
	dozer.SpecACLEntryTCPFlagSyn:    oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN,
	dozer.SpecACLEntryTCPFlagNotSyn: oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_SYN,
	dozer.SpecACLEntryTCPFlagRst:    oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST,
	dozer.SpecACLEntryTCPFlagNotRst: oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_RST,
	dozer.SpecACLEntryTCPFlagPsh:    oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH,
	dozer.SpecACLEntryTCPFlagNotPsh: oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_PSH,
	dozer.SpecACLEntryTCPFlagAck:    oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK,
	dozer.SpecACLEntryTCPFlagNotAck: oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_ACK,
	dozer.SpecACLEntryTCPFlagUrg:    oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG,
	dozer.SpecACLEntryTCPFlagNotUrg: oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_URG,
}

var ocToTCPFlag = map[oc.E_OpenconfigPacketMatchTypes_TCP_FLAGS]dozer.SpecACLEntryTCPFlag{ //nolint:exhaustive
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_FIN:     dozer.SpecACLEntryTCPFlagFin,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_FIN: dozer.SpecACLEntryTCPFlagNotFin,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_SYN:     dozer.SpecACLEntryTCPFlagSyn,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_SYN: dozer.SpecACLEntryTCPFlagNotSyn,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_RST:     dozer.SpecACLEntryTCPFlagRst,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_RST: dozer.SpecACLEntryTCPFlagNotRst,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_PSH:     dozer.SpecACLEntryTCPFlagPsh,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_PSH: dozer.SpecACLEntryTCPFlagNotPsh,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_ACK:     dozer.SpecACLEntryTCPFlagAck,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_ACK: dozer.SpecACLEntryTCPFlagNotAck,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_URG:     dozer.SpecACLEntryTCPFlagUrg,
	oc.OpenconfigPacketMatchTypes_TCP_FLAGS_TCP_NOT_URG: dozer.SpecACLEntryTCPFlagNotUrg,
}

func marshalTCPFlags(flags []dozer.SpecACLEntryTCPFlag) ([]oc.E_OpenconfigPacketMatchTypes_TCP_FLAGS, error) {
	result := make([]oc.E_OpenconfigPacketMatchTypes_TCP_FLAGS, 0, len(flags))
	for _, f := range flags {
		ocFlag, ok := tcpFlagToOC[f]
		if !ok {
			return nil, errors.Errorf("unknown TCP flag: %s", f)
		}
		result = append(result, ocFlag)
	}

	return result, nil
}

func unmarshalTCPFlags(flags []oc.E_OpenconfigPacketMatchTypes_TCP_FLAGS) []dozer.SpecACLEntryTCPFlag {
	if len(flags) == 0 {
		return nil
	}
	result := make([]dozer.SpecACLEntryTCPFlag, 0, len(flags))
	for _, f := range flags {
		if specFlag, ok := ocToTCPFlag[f]; ok {
			result = append(result, specFlag)
		}
	}

	return result
}
