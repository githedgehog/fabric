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
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specNATsEnforcer = &DefaultMapEnforcer[uint32, *dozer.SpecNAT]{
	Summary:      "NATs",
	ValueHandler: specNATEnforcer,
}

var specNATEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecNAT]{
	Summary: "NAT %s",
	CustomHandler: func(basePath string, key uint32, actual, desired *dozer.SpecNAT, actions *ActionQueue) error {
		basePath += fmt.Sprintf("/nat/instances/instance[id=%d]", key)

		if err := specNATBaseEnforcer.Handle(basePath, key, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle nat base")
		}

		actualPools, desiredPools := ValueOrNil(actual, desired,
			func(value *dozer.SpecNAT) map[string]*dozer.SpecNATPool { return value.Pools })
		if err := specNATPoolsEnforcer.Handle(basePath, actualPools, desiredPools, actions); err != nil {
			return errors.Wrap(err, "failed to handle nat pools")
		}

		actualBindings, desiredBindings := ValueOrNil(actual, desired,
			func(value *dozer.SpecNAT) map[string]*dozer.SpecNATBinding { return value.Bindings })
		if err := specNATBindingsEnforcer.Handle(basePath, actualBindings, desiredBindings, actions); err != nil {
			return errors.Wrap(err, "failed to handle nat bindings")
		}

		actualStatic, desiredStatic := ValueOrNil(actual, desired,
			func(value *dozer.SpecNAT) map[string]*dozer.SpecNATEntry { return value.Static })
		if err := specNATEntriesEnforcer.Handle(basePath, actualStatic, desiredStatic, actions); err != nil {
			return errors.Wrap(err, "failed to handle nat entries")
		}

		return nil
	},
}

var specNATBaseEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecNAT]{
	Summary: "NAT %d base",
	Getter:  func(id uint32, value *dozer.SpecNAT) any { return value.Enable },
	MutateDesired: func(id uint32, desired *dozer.SpecNAT) *dozer.SpecNAT {
		if id == 0 && desired == nil {
			desired = &dozer.SpecNAT{
				Enable: ygot.Bool(false),
			}
		}

		return desired
	},
	UpdateWeight: ActionWeightNATBaseUpdate,
	DeleteWeight: ActionWeightNATBaseDelete,
	Marshal: func(id uint32, value *dozer.SpecNAT) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNat_Nat_Instances{
			Instance: map[uint32]*oc.OpenconfigNat_Nat_Instances_Instance{
				id: {
					Id: ygot.Uint32(id),
					Config: &oc.OpenconfigNat_Nat_Instances_Instance_Config{
						Id:     ygot.Uint32(id),
						Enable: value.Enable,
					},
				},
			},
		}, nil
	},
}

var specNATPoolsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecNATPool]{
	Summary:      "NAT pools",
	ValueHandler: specNATPoolEnforcer,
}

var specNATPoolEnforcer = &DefaultValueEnforcer[string, *dozer.SpecNATPool]{
	Summary:      "NAT pool %s",
	Path:         "/nat-pool/nat-pool-entry[pool-name=%s]",
	UpdateWeight: ActionWeightNATPoolUpdate,
	DeleteWeight: ActionWeightNATPoolDelete,
	Marshal: func(name string, value *dozer.SpecNATPool) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigNat_Nat_Instances_Instance_NatPool{
			NatPoolEntry: map[string]*oc.OpenconfigNat_Nat_Instances_Instance_NatPool_NatPoolEntry{
				name: {
					PoolName: ygot.String(name),
					Config: &oc.OpenconfigNat_Nat_Instances_Instance_NatPool_NatPoolEntry_Config{
						PoolName: ygot.String(name),
						NatIp:    value.Range,
					},
				},
			},
		}, nil
	},
}

var specNATBindingsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecNATBinding]{
	Summary:      "NAT bindings",
	ValueHandler: specNATBindingEnforcer,
}

var specNATBindingEnforcer = &DefaultValueEnforcer[string, *dozer.SpecNATBinding]{
	Summary:      "NAT binding %s",
	Path:         "/nat-acl-pool-binding/nat-acl-pool-binding-entry[name=%s]",
	UpdateWeight: ActionWeightNATBindingUpdate,
	DeleteWeight: ActionWeightNATBindingDelete,
	Marshal: func(name string, value *dozer.SpecNATBinding) (ygot.ValidatedGoStruct, error) {
		natType := oc.OpenconfigNat_NAT_TYPE_UNSET
		if value.Type == dozer.SpecNATTypeSNAT {
			natType = oc.OpenconfigNat_NAT_TYPE_SNAT
		} else if value.Type == dozer.SpecNATTypeDNAT {
			natType = oc.OpenconfigNat_NAT_TYPE_DNAT
		}

		return &oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding{
			NatAclPoolBindingEntry: map[string]*oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding_NatAclPoolBindingEntry{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding_NatAclPoolBindingEntry_Config{
						Name:    ygot.String(name),
						NatPool: value.Pool,
						Type:    natType,
					},
				},
			},
		}, nil
	},
}

var specNATEntriesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecNATEntry]{
	Summary:      "NAT entries (static)",
	ValueHandler: specNATEntryEnforcer,
}

var specNATEntryEnforcer = &DefaultValueEnforcer[string, *dozer.SpecNATEntry]{
	Summary:      "NAT entry (static) %s",
	Path:         "/nat-mapping-table/nat-mapping-entry[external-address=%s]",
	UpdateWeight: ActionWeightNATEntryUpdate,
	DeleteWeight: ActionWeightNATEntryDelete,
	Marshal: func(externalIP string, value *dozer.SpecNATEntry) (ygot.ValidatedGoStruct, error) {
		natType := oc.OpenconfigNat_NAT_TYPE_UNSET
		if value.Type == dozer.SpecNATTypeSNAT {
			natType = oc.OpenconfigNat_NAT_TYPE_SNAT
		} else if value.Type == dozer.SpecNATTypeDNAT {
			natType = oc.OpenconfigNat_NAT_TYPE_DNAT
		}

		return &oc.OpenconfigNat_Nat_Instances_Instance_NatMappingTable{
			NatMappingEntry: map[string]*oc.OpenconfigNat_Nat_Instances_Instance_NatMappingTable_NatMappingEntry{
				externalIP: {
					ExternalAddress: ygot.String(externalIP),
					Config: &oc.OpenconfigNat_Nat_Instances_Instance_NatMappingTable_NatMappingEntry_Config{
						ExternalAddress: ygot.String(externalIP),
						InternalAddress: value.InternalAddress,
						Type:            natType,
					},
				},
			},
		}, nil
	},
}

func loadActualNATs(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocNATInstances := &oc.OpenconfigNat_Nat_Instances{}
	err := client.Get(ctx, "/nat/instances/instance", ocNATInstances, api.DataTypeCONFIG())
	if err != nil {
		if !strings.Contains(err.Error(), "rpc error: code = InvalidArgument desc = Node nat not found") { // TODO rework client to handle it
			return errors.Wrapf(err, "failed to read nat instances")
		}
	}
	spec.NATs, err = unmarshalOCNATInstances(ocNATInstances)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal nat instances")
	}

	return nil
}

func unmarshalOCNATInstances(ocVal *oc.OpenconfigNat_Nat_Instances) (map[uint32]*dozer.SpecNAT, error) {
	instances := map[uint32]*dozer.SpecNAT{}

	if ocVal == nil {
		return instances, nil
	}

	for id, ocNAT := range ocVal.Instance {
		if ocNAT.Config == nil || ocNAT.Config.Enable == nil || !*ocNAT.Config.Enable {
			continue
		}

		pools := map[string]*dozer.SpecNATPool{}
		if ocNAT.NatPool != nil {
			for name, pool := range ocNAT.NatPool.NatPoolEntry {
				if pool.Config == nil || pool.Config.NatIp == nil {
					continue
				}

				pools[name] = &dozer.SpecNATPool{
					Range: pool.Config.NatIp,
				}
			}
		}

		bindings := map[string]*dozer.SpecNATBinding{}
		if ocNAT.NatAclPoolBinding != nil {
			for name, bind := range ocNAT.NatAclPoolBinding.NatAclPoolBindingEntry {
				if bind.Config == nil || bind.Config.NatPool == nil {
					continue
				}

				natType := dozer.SpecNATTypeUnset
				if bind.Config.Type == oc.OpenconfigNat_NAT_TYPE_SNAT {
					natType = dozer.SpecNATTypeSNAT
				} else if bind.Config.Type == oc.OpenconfigNat_NAT_TYPE_DNAT {
					natType = dozer.SpecNATTypeDNAT
				}

				bindings[name] = &dozer.SpecNATBinding{
					Pool: bind.Config.NatPool,
					Type: natType,
				}
			}
		}

		static := map[string]*dozer.SpecNATEntry{}
		if ocNAT.NatMappingTable != nil {
			for externalIP, entry := range ocNAT.NatMappingTable.NatMappingEntry {
				if entry.Config == nil || entry.Config.InternalAddress == nil {
					continue
				}

				natType := dozer.SpecNATTypeUnset
				if entry.Config.Type == oc.OpenconfigNat_NAT_TYPE_SNAT {
					natType = dozer.SpecNATTypeSNAT
				} else if entry.Config.Type == oc.OpenconfigNat_NAT_TYPE_DNAT {
					natType = dozer.SpecNATTypeDNAT
				}

				static[externalIP] = &dozer.SpecNATEntry{
					InternalAddress: entry.Config.InternalAddress,
					Type:            natType,
				}
			}
		}

		instances[id] = &dozer.SpecNAT{
			Enable:   ocNAT.Config.Enable,
			Pools:    pools,
			Bindings: bindings,
			Static:   static,
		}
	}

	return instances, nil
}
