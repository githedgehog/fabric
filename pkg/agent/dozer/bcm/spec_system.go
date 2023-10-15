package bcm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openconfig/gnmic/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specZTPEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "ZTP",
	Path:    "/ztp/config",
	Getter:  func(key string, value *dozer.Spec) any { return value.ZTP },
	Weight:  ActionWeightSystemZTP,
	Marshal: func(key string, value *dozer.Spec) (ygot.ValidatedGoStruct, error) {
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
		return ygot.Bool(false), nil
	}

	return ocVal.AdminMode, nil
}

var specHostnameEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "Hostname",
	Path:    "/system/config",
	Getter:  func(key string, value *dozer.Spec) any { return value.Hostname },
	Weight:  ActionWeightSystemHostname,
	Marshal: func(key string, value *dozer.Spec) (ygot.ValidatedGoStruct, error) {
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
	MutateDesired: func(key string, desired *dozer.SpecPortGroup) *dozer.SpecPortGroup {
		if desired == nil {
			return &dozer.SpecPortGroup{}
		}

		return desired
	},
	Marshal: func(id string, value *dozer.SpecPortGroup) (ygot.ValidatedGoStruct, error) {
		var speed oc.E_OpenconfigIfEthernet_ETHERNET_SPEED
		if value.Speed != nil {
			ok := false
			for speedVal, name := range oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"] {
				if name.Name == *value.Speed {
					speed = oc.E_OpenconfigIfEthernet_ETHERNET_SPEED(speedVal)
					ok = true
					break
				}
			}
			if !ok {
				return nil, errors.Errorf("invalid speed %s", *value.Speed)
			}
		}

		return &oc.OpenconfigPortGroup_PortGroups{
			PortGroup: map[string]*oc.OpenconfigPortGroup_PortGroups_PortGroup{
				id: {
					Id: ygot.String(id),
					Config: &oc.OpenconfigPortGroup_PortGroups_PortGroup_Config{
						Id:    ygot.String(id),
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
		return errors.Wrapf(err, "failed to read port groups")
	}
	spec.PortGroups, err = unmarshalOCPortGroups(ocVal)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal port groups")
	}

	return nil
}

func unmarshalOCPortGroups(ocVal *oc.OpenconfigPortGroup_PortGroups) (map[string]*dozer.SpecPortGroup, error) {
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
		val := &dozer.SpecPortGroup{}
		if speed > 0 && speed < oc.OpenconfigIfEthernet_ETHERNET_SPEED_SPEED_UNKNOWN {
			val.Speed = ygot.String(oc.ΛEnum["E_OpenconfigIfEthernet_ETHERNET_SPEED"][int64(speed)].Name)
		}
		portGroups[name] = val
	}

	return portGroups, nil
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
			passwdHash = ygot.String(value.Password)
		} else {
			passwd = ygot.String(value.Password)
		}

		return &oc.OpenconfigSystem_System_Aaa_Authentication_Users{
			User: map[string]*oc.OpenconfigSystem_System_Aaa_Authentication_Users_User{
				name: {
					Username: ygot.String(name),
					Config: &oc.OpenconfigSystem_System_Aaa_Authentication_Users_User_Config{
						Username:       ygot.String(name),
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
	CustomHandler: func(basePath, name string, _, user *dozer.SpecUser, actions *ActionQueue) error {
		if user != nil {
			actions.Add(&Action{
				ASummary: fmt.Sprintf("User %s authorized keys", name),
				CustomFunc: func() error {
					slog.Debug("Setting authorized_keys", "user", name)
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

					err = os.WriteFile(filepath.Join(sshDir, "authorized_keys"), []byte(
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
			})
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

func unmarshalOCUsers(ocVal *oc.OpenconfigSystem_System_Aaa_Authentication_Users) (map[string]*dozer.SpecUser, error) {
	users := map[string]*dozer.SpecUser{}

	if ocVal == nil {
		return users, nil
	}

	for name, user := range ocVal.User {
		if name == gnmi.AGENT_USER {
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