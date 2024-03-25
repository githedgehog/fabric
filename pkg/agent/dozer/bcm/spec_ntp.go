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

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specNTPEnforcer = &DefaultValueEnforcer[string, *dozer.SpecNTP]{
	Summary: "NTP",
	Path:    "/system/ntp/config",
	Weight:  ActionWeightNTP,
	Marshal: func(_ string, value *dozer.SpecNTP) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigSystem_System_Ntp{
			Config: &oc.OpenconfigSystem_System_Ntp_Config{
				SourceInterface: value.SourceInterface,
			},
		}, nil
	},
}

var specNTPServersEnforcer = &DefaultMapEnforcer[string, *dozer.SpecNTPServer]{
	Summary:      "NTP servers",
	ValueHandler: specNTPServerEnforcer,
}

var specNTPServerEnforcer = &DefaultValueEnforcer[string, *dozer.SpecNTPServer]{
	Summary:      "NTP server %s",
	Path:         "/system/ntp/servers/server[address=%s]",
	UpdateWeight: ActionWeightNTPServerUpdate,
	DeleteWeight: ActionWeightNTPServerDelete,
	Marshal: func(name string, value *dozer.SpecNTPServer) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigSystem_System_Ntp_Servers{
			Server: map[string]*oc.OpenconfigSystem_System_Ntp_Servers_Server{
				name: {
					Address: pointer.To(name),
					Config: &oc.OpenconfigSystem_System_Ntp_Servers_Server_Config{
						Address: pointer.To(name),
						Prefer:  value.Prefer,
					},
				},
			},
		}, nil
	},
}

func loadActualNTP(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocNTP := &oc.OpenconfigSystem_System_Ntp{}
	err := client.Get(ctx, "/system/ntp/config", ocNTP)
	if err != nil {
		return errors.Wrapf(err, "failed to read ntp config")
	}
	spec.NTP, err = unmarshalOCNTP(ocNTP)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal ntp config")
	}

	return nil
}

func unmarshalOCNTP(ocVal *oc.OpenconfigSystem_System_Ntp) (*dozer.SpecNTP, error) { //nolint:unparam
	if ocVal == nil || ocVal.Config == nil {
		return &dozer.SpecNTP{}, nil
	}

	return &dozer.SpecNTP{
		SourceInterface: ocVal.Config.SourceInterface,
	}, nil
}

func loadActualNTPServers(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocNTP := &oc.OpenconfigSystem_System_Ntp{}
	err := client.Get(ctx, "/system/ntp/servers", ocNTP)
	if err != nil {
		return errors.Wrapf(err, "failed to read ntp servers")
	}
	spec.NTPServers, err = unmarshalOCNTPServers(ocNTP)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal ntp servers")
	}

	return nil
}

func unmarshalOCNTPServers(ocVal *oc.OpenconfigSystem_System_Ntp) (map[string]*dozer.SpecNTPServer, error) { //nolint:unparam
	if ocVal == nil || ocVal.Servers == nil {
		return map[string]*dozer.SpecNTPServer{}, nil
	}

	servers := map[string]*dozer.SpecNTPServer{}
	for name, ocServer := range ocVal.Servers.Server {
		servers[name] = &dozer.SpecNTPServer{}

		if ocServer.Config != nil {
			servers[name].Prefer = ocServer.Config.Prefer
		}
	}

	return servers, nil
}
