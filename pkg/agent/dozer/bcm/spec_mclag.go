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
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specMCLAGDomainsEnforcer = &DefaultMapEnforcer[uint32, *dozer.SpecMCLAGDomain]{
	Summary:      "MCLAG domains",
	ValueHandler: specMCLAGDomainEnforcer,
}

var specMCLAGDomainEnforcer = &DefaultValueEnforcer[uint32, *dozer.SpecMCLAGDomain]{
	Summary:      "MCLAG domain %d",
	Path:         "/mclag/mclag-domains/mclag-domain[domain-id=%d]",
	UpdateWeight: ActionWeightMCLAGDomainUpdate,
	DeleteWeight: ActionWeightMCLAGDomainDelete,
	Marshal: func(id uint32, value *dozer.SpecMCLAGDomain) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigMclag_Mclag_MclagDomains{
			MclagDomain: map[uint32]*oc.OpenconfigMclag_Mclag_MclagDomains_MclagDomain{
				id: {
					Config: &oc.OpenconfigMclag_Mclag_MclagDomains_MclagDomain_Config{
						DomainId:      pointer.To(id),
						SourceAddress: pointer.To(strings.SplitN(value.SourceIP, "/", 2)[0]), // TODO is it good enough?
						PeerAddress:   pointer.To(strings.SplitN(value.PeerIP, "/", 2)[0]),
						PeerLink:      pointer.To(value.PeerLink),
					},
					DomainId: pointer.To(id),
				},
			},
		}, nil
	},
}

var specMCLAGInterfacesEnforcer = &DefaultMapEnforcer[string, *dozer.SpecMCLAGInterface]{
	Summary:      "MCLAG interfaces",
	ValueHandler: specMCLAGInterfaceEnforcer,
}

var specMCLAGInterfaceEnforcer = &DefaultValueEnforcer[string, *dozer.SpecMCLAGInterface]{
	Summary:      "MCLAG interface %s",
	Path:         "/mclag/interfaces/interface[name=%s]",
	UpdateWeight: ActionWeightMCLAGInterfaceUpdate,
	DeleteWeight: ActionWeightMCLAGInterfaceDelete,
	Marshal: func(name string, value *dozer.SpecMCLAGInterface) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigMclag_Mclag_Interfaces{
			Interface: map[string]*oc.OpenconfigMclag_Mclag_Interfaces_Interface{
				name: {
					Name: pointer.To(name),
					Config: &oc.OpenconfigMclag_Mclag_Interfaces_Interface_Config{
						MclagDomainId: pointer.To(value.DomainID),
					},
				},
			},
		}, nil
	},
}

func loadActualMCLAGs(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	gnmiMCLAG := &oc.OpenconfigMclag_Mclag{}
	err := client.Get(ctx, "/mclag/mclag-domains", gnmiMCLAG, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read mclag domains")
	}
	err = client.Get(ctx, "/mclag/interfaces", gnmiMCLAG)
	if err != nil {
		return errors.Wrapf(err, "failed to read mclag interfaces")
	}

	spec.MCLAGs, err = unmarshalOCMCLAGDomains(gnmiMCLAG.MclagDomains)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal mclag")
	}

	spec.MCLAGInterfaces, err = unmarshalOCMCLAGInterfaces(gnmiMCLAG.Interfaces)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal mclag interfaces")
	}

	return nil
}

func unmarshalOCMCLAGDomains(ocVal *oc.OpenconfigMclag_Mclag_MclagDomains) (map[uint32]*dozer.SpecMCLAGDomain, error) {
	mclag := map[uint32]*dozer.SpecMCLAGDomain{}

	if ocVal == nil {
		return mclag, nil
	}

	for domainID, ocDomain := range ocVal.MclagDomain {
		mclag[domainID] = &dozer.SpecMCLAGDomain{
			SourceIP: *ocDomain.Config.SourceAddress,
			PeerIP:   *ocDomain.Config.PeerAddress,
			PeerLink: *ocDomain.Config.PeerLink,
			// Members:  members,
		}
	}

	return mclag, nil
}

func unmarshalOCMCLAGInterfaces(ocVal *oc.OpenconfigMclag_Mclag_Interfaces) (map[string]*dozer.SpecMCLAGInterface, error) {
	members := map[string]*dozer.SpecMCLAGInterface{}

	if ocVal == nil {
		return members, nil
	}

	for name, mclagMember := range ocVal.Interface {
		if mclagMember.Config != nil && mclagMember.Config.MclagDomainId != nil {
			members[name] = &dozer.SpecMCLAGInterface{
				DomainID: *mclagMember.Config.MclagDomainId,
			}
		}
	}

	return members, nil
}
