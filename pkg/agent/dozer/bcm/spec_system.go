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
	"os"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

var specZTPEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "ZTP",
	Path:    "/ztp/config",
	Getter:  func(_ string, value *dozer.Spec) any { return value.ZTP },
	Weight:  ActionWeightSystemZTP,
	Marshal: func(_ string, value *dozer.Spec) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigZtp_Ztp{
			Config: &oc.OpenconfigZtp_Ztp_Config{
				AdminMode: value.ZTP,
			},
		}, nil
	},
}

func loadActualZTP(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocZTP := &oc.OpenconfigZtp_Ztp_Config{}
	err := client.Get(ctx, "/ztp/config", ocZTP, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read ztp config")
	}
	spec.ZTP, err = unmarshalOCZTPConfig(ocZTP)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal ztp config")
	}

	return nil
}

func unmarshalOCZTPConfig(ocVal *oc.OpenconfigZtp_Ztp_Config) (*bool, error) {
	if ocVal == nil {
		return nil, errors.Errorf("no ZTP config found")
	}

	if ocVal.AdminMode == nil {
		return pointer.To(false), nil
	}

	return ocVal.AdminMode, nil
}

var specHostnameEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "Hostname",
	Path:    "/system/config",
	Getter:  func(_ string, value *dozer.Spec) any { return value.Hostname },
	Weight:  ActionWeightSystemHostname,
	Marshal: func(_ string, value *dozer.Spec) (ygot.ValidatedGoStruct, error) {
		return &oc.OpenconfigSystem_System{
			Config: &oc.OpenconfigSystem_System_Config{
				Hostname: value.Hostname,
			},
		}, nil
	},
}

func loadActualHostname(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	gnmiSystemConfig := &oc.OpenconfigSystem_System{}
	err := client.Get(ctx, "/system/config", gnmiSystemConfig, api.DataTypeCONFIG())
	if err != nil {
		return errors.Wrapf(err, "failed to read system config")
	}
	spec.Hostname, err = unmarshalOCSystemConfig(gnmiSystemConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal system config")
	}

	return nil
}

func unmarshalOCSystemConfig(ocVal *oc.OpenconfigSystem_System) (*string, error) {
	if ocVal == nil || ocVal.Config == nil {
		return nil, errors.Errorf("no system config found")
	}

	return ocVal.Config.Hostname, nil
}

var specPortGroupsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecPortGroup]{
	Summary:      "Port groups",
	ValueHandler: specPortGroupEnforcer,
}

var specPortGroupEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPortGroup]{
	Summary: "Port group %s",
	Path:    "/port-groups/port-group[id=%s]",
	Weight:  ActionWeightPortGroup,
	MutateDesired: func(_ string, desired *dozer.SpecPortGroup) *dozer.SpecPortGroup {
		if desired == nil {
			return &dozer.SpecPortGroup{}
		}

		return desired
	},
	Marshal: func(id string, value *dozer.SpecPortGroup) (ygot.ValidatedGoStruct, error) {
		var speed oc.E_OpenconfigIfEthernet_ETHERNET_SPEED
		if value.Speed != nil {
			var ok bool
			speed, ok = MarshalPortSpeed(*value.Speed)
			if !ok {
				return nil, errors.Errorf("invalid speed %s", *value.Speed)
			}
		}

		return &oc.OpenconfigPortGroup_PortGroups{
			PortGroup: map[string]*oc.OpenconfigPortGroup_PortGroups_PortGroup{
				id: {
					Id: pointer.To(id),
					Config: &oc.OpenconfigPortGroup_PortGroups_PortGroup_Config{
						Id:    pointer.To(id),
						Speed: speed,
					},
				},
			},
		}, nil
	},
}

func loadActualPortGroups(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.OpenconfigPortGroup_PortGroups{}
	err := client.Get(ctx, "/port-groups/port-group", ocVal)
	if err != nil {
		if !strings.Contains(err.Error(), "rpc error: code = NotFound") { // TODO rework client to handle it
			return errors.Wrapf(err, "failed to read port groups")
		}
	}
	spec.PortGroups, err = unmarshalOCPortGroups(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal port groups")
	}

	return nil
}

func unmarshalOCPortGroups(ocVal *oc.OpenconfigPortGroup_PortGroups) (map[string]*dozer.SpecPortGroup, error) { //nolint:unparam
	portGroups := map[string]*dozer.SpecPortGroup{}

	if ocVal == nil {
		return portGroups, nil
	}

	for name, portGroup := range ocVal.PortGroup {
		if portGroup.Config == nil {
			continue
		}

		speed := portGroup.Config.Speed

		// skip default speeds
		if portGroup.State != nil && speed == portGroup.State.DefaultSpeed {
			continue
		}

		portGroups[name] = &dozer.SpecPortGroup{
			Speed: UnmarshalPortSpeed(speed),
		}
	}

	return portGroups, nil
}

var specPortBreakoutsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecPortBreakout]{
	Summary:      "Port Breakout",
	ValueHandler: specPortBreakoutEnforcer,
}

var specPortBreakoutEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPortBreakout]{
	Summary:    "Port Breakout %s",
	Path:       "/components/component[name=%s]/port/breakout-mode/groups/group[index=1]/config",
	Weight:     ActionWeightPortBreakout,
	SkipDelete: true,
	NoReplace:  true,
	Marshal: func(_ string, value *dozer.SpecPortBreakout) (ygot.ValidatedGoStruct, error) {
		parts := strings.Split(value.Mode, "x")
		if len(parts) != 2 {
			return nil, errors.Errorf("invalid breakout mode %s, incorrect number of parts separated by 'x'", value.Mode)
		}

		numR, err := strconv.ParseUint(parts[0], 10, 8)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid breakouts num %s, isn't uint8", value.Mode)
		}
		num := uint8(numR)

		speed, ok := MarshalPortSpeed(parts[1])
		if !ok {
			return nil, errors.Errorf("invalid breakout speed %s", parts[1])
		}

		if num == 0 || speed == oc.OpenconfigIfEthernet_ETHERNET_SPEED_UNSET {
			return nil, errors.Errorf("invalid breakout mode %s", value.Mode)
		}

		return &oc.OpenconfigPlatform_Components_Component_Port_BreakoutMode_Groups_Group{
			Config: &oc.OpenconfigPlatform_Components_Component_Port_BreakoutMode_Groups_Group_Config{
				Index:         pointer.To(uint8(1)),
				NumBreakouts:  pointer.To(num),
				BreakoutSpeed: speed,
				BreakoutOwner: oc.OpenconfigPlatform_Components_Component_Port_BreakoutMode_Groups_Group_Config_BreakoutOwner_MANUAL,
				// NumPhysicalChannels: pointer.To(0), // TODO check if it's really needed
			},
		}, nil
	},
}

func loadActualPortBreakouts(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.SonicPortBreakout_SonicPortBreakout{}
	err := client.Get(ctx, "/sonic-port-breakout/BREAKOUT_CFG", ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to read port breakouts")
	}
	spec.PortBreakouts, err = unmarshalOCPortBreakouts(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal port breakouts")
	}

	return nil
}

func unmarshalOCPortBreakouts(ocVal *oc.SonicPortBreakout_SonicPortBreakout) (map[string]*dozer.SpecPortBreakout, error) { //nolint:unparam
	portBreakouts := map[string]*dozer.SpecPortBreakout{}

	if ocVal == nil || ocVal.BREAKOUT_CFG == nil {
		return portBreakouts, nil
	}

	for _, breakoutCfg := range ocVal.BREAKOUT_CFG.BREAKOUT_CFG_LIST {
		if breakoutCfg.Port == nil || breakoutCfg.BrkoutMode == nil || breakoutCfg.Status == nil {
			continue
		}

		if *breakoutCfg.Status != "Completed" || breakoutCfg.BreakoutOwner != oc.SonicPortBreakout_SonicPortBreakout_BREAKOUT_CFG_BREAKOUT_CFG_LIST_BreakoutOwner_MANUAL {
			continue
		}

		portBreakouts[*breakoutCfg.Port] = &dozer.SpecPortBreakout{
			Mode: UnmarshalPortBreakout(*breakoutCfg.BrkoutMode),
		}
	}

	return portBreakouts, nil
}

var specUsersEnforcer = &DefaultMapEnforcer[string, *dozer.SpecUser]{
	Summary:      "Users",
	ValueHandler: specUserEnforcer,
}

var specUserEnforcer = &DefaultValueEnforcer[string, *dozer.SpecUser]{
	Summary: "User %s",
	Path:    "/system/aaa/authentication/users/user[username=%s]",
	Weight:  ActionWeightUser,
	Marshal: func(name string, value *dozer.SpecUser) (ygot.ValidatedGoStruct, error) {
		var passwd, passwdHash *string
		if len(value.Password) == 63 && strings.HasPrefix(value.Password, "$5$") {
			passwdHash = pointer.To(value.Password)
		} else {
			passwd = pointer.To(value.Password)
		}

		return &oc.OpenconfigSystem_System_Aaa_Authentication_Users{
			User: map[string]*oc.OpenconfigSystem_System_Aaa_Authentication_Users_User{
				name: {
					Username: pointer.To(name),
					Config: &oc.OpenconfigSystem_System_Aaa_Authentication_Users_User_Config{
						Username:       pointer.To(name),
						Password:       passwd,
						PasswordHashed: passwdHash,
						Role:           oc.UnionString(value.Role),
					},
				},
			},
		}, nil
	},
}

var specUsersAuthorizedKeysEnforcer = &DefaultMapEnforcer[string, *dozer.SpecUser]{
	Summary:      "Users authorized keys",
	ValueHandler: specUserAuthorizedKeysEnforcer,
}

var specUserAuthorizedKeysEnforcer = &DefaultValueEnforcer[string, *dozer.SpecUser]{
	Summary: "User %s authorized keys",
	CustomHandler: func(_, name string, _, user *dozer.SpecUser, actions *ActionQueue) error {
		if user != nil {
			if err := actions.Add(&Action{
				ASummary: fmt.Sprintf("User %s authorized keys", name),
				Weight:   ActionWeightUserAuthorizedKeys,
				CustomFunc: func(_ context.Context, _ *gnmi.Client) error {
					osUser, err := osuser.Lookup(name)
					if err != nil {
						return errors.Wrapf(err, "failed to lookup user %s", name)
					}

					uid, err := strconv.Atoi(osUser.Uid)
					if err != nil {
						return errors.Wrapf(err, "failed to parse uid %s", osUser.Uid)
					}
					gid, err := strconv.Atoi(osUser.Gid)
					if err != nil {
						return errors.Wrapf(err, "failed to parse gid %s", osUser.Gid)
					}

					sshDir := filepath.Join("/home", name, ".ssh")
					err = os.MkdirAll(sshDir, 0o700)
					if err != nil {
						return errors.Wrapf(err, "failed to create ssh dir %s", sshDir)
					}

					err = os.Chown(sshDir, uid, gid)
					if err != nil {
						return errors.Wrapf(err, "failed to chown ssh dir %s", sshDir)
					}

					err = os.WriteFile(filepath.Join(sshDir, "authorized_keys"), []byte( //nolint:gosec
						strings.Join(append([]string{
							"# Hedgehog Agent managed keys, do not edit manually",
						}, user.AuthorizedKeys...), "\n")+"\n",
					), 0o644)
					if err != nil {
						return errors.Wrapf(err, "failed to write authorized_keys for user %s", name)
					}

					err = os.Chown(filepath.Join(sshDir, "authorized_keys"), uid, gid)
					if err != nil {
						return errors.Wrapf(err, "failed to chown authorized_keys for user %s", name)
					}

					return nil
				},
			}); err != nil {
				return errors.Wrapf(err, "failed to add custom action to update authorized keys")
			}
		}

		return nil
	},
}

func loadActualUsers(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocVal := &oc.OpenconfigSystem_System_Aaa_Authentication_Users{}
	err := client.Get(ctx, "/system/aaa/authentication/users/user", ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to read users")
	}
	spec.Users, err = unmarshalOCUsers(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal users")
	}

	return nil
}

func unmarshalOCUsers(ocVal *oc.OpenconfigSystem_System_Aaa_Authentication_Users) (map[string]*dozer.SpecUser, error) { //nolint:unparam
	users := map[string]*dozer.SpecUser{}

	if ocVal == nil {
		return users, nil
	}

	for name, user := range ocVal.User {
		if name == gnmi.AgentUser {
			continue
		}
		if user.Config == nil || user.Config.Role == nil {
			continue
		}

		if union, ok := user.Config.Role.(oc.UnionString); ok {
			users[name] = &dozer.SpecUser{
				Role: string(union),
			}
		}
	}

	return users, nil
}
