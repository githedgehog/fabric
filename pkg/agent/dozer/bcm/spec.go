package bcm

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
)

var specEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "Spec",
	CustomHandler: func(basePath string, key string, actual, desired *dozer.Spec, actions *ActionQueue) error {
		if actual == nil {
			actual = &dozer.Spec{}
		}
		if desired == nil {
			desired = &dozer.Spec{}
		}

		if err := specZTPEnforcer.Handle(basePath, "", actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle ztp")
		}

		if err := specHostnameEnforcer.Handle(basePath, "", actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle hostname")
		}

		if err := specPortGroupsEnforcer.Handle(basePath, actual.PortGroups, desired.PortGroups, actions); err != nil {
			return errors.Wrap(err, "failed to handle port groups")
		}

		if err := specPortBreakoutsEnforcer.Handle(basePath, actual.PortBreakouts, desired.PortBreakouts, actions); err != nil {
			return errors.Wrap(err, "failed to handle port breakouts")
		}

		if err := specUsersEnforcer.Handle(basePath, actual.Users, desired.Users, actions); err != nil {
			return errors.Wrap(err, "failed to handle users")
		}

		if err := specUsersAuthorizedKeysEnforcer.Handle(basePath, actual.Users, desired.Users, actions); err != nil {
			return errors.Wrap(err, "failed to handle users authorized keys")
		}

		if err := specInterfacesEnforcer.Handle(basePath, actual.Interfaces, desired.Interfaces, actions); err != nil {
			return errors.Wrap(err, "failed to handle interfaces")
		}

		if err := specMCLAGDomainsEnforcer.Handle(basePath, actual.MCLAGs, desired.MCLAGs, actions); err != nil {
			return errors.Wrap(err, "failed to handle mclag domains")
		}

		if err := specMCLAGInterfacesEnforcer.Handle(basePath, actual.MCLAGInterfaces, desired.MCLAGInterfaces, actions); err != nil {
			return errors.Wrap(err, "failed to handle mclag interfaces")
		}

		if err := specDHCPRelaysEnforcer.Handle(basePath, actual.DHCPRelays, desired.DHCPRelays, actions); err != nil {
			return errors.Wrap(err, "failed to handle dhcp relays")
		}

		if err := specACLsEnforcer.Handle(basePath, actual.ACLs, desired.ACLs, actions); err != nil {
			return errors.Wrap(err, "failed to handle acls")
		}

		if err := specACLInterfacesEnforcer.Handle(basePath, actual.ACLInterfaces, desired.ACLInterfaces, actions); err != nil {
			return errors.Wrap(err, "failed to handle acl interfaces")
		}

		if err := specNATsEnforcer.Handle(basePath, actual.NATs, desired.NATs, actions); err != nil {
			return errors.Wrap(err, "failed to handle nats")
		}

		if err := specVRFsEnforcer.Handle(basePath, actual.VRFs, desired.VRFs, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrfs")
		}

		if err := specRouteMapsEnforcer.Handle(basePath, actual.RouteMaps, desired.RouteMaps, actions); err != nil {
			return errors.Wrap(err, "failed to handle route maps")
		}

		return nil
	},
}

func loadActualSpec(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	if err := loadActualZTP(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load ztp")
	}

	if err := loadActualHostname(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load hostname")
	}

	if err := loadActualPortGroups(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load port groups")
	}

	if err := loadActualPortBreakouts(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load port breakouts")
	}

	if err := loadActualUsers(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load users")
	}

	if err := loadActualInterfaces(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load interfaces")
	}

	if err := loadActualMCLAGs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load mclag")
	}

	if err := loadActualDHCPRelays(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load dhcp relay interfaces")
	}

	if err := loadActualNATs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load nat instances")
	}

	if err := loadActualACLs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load acls")
	}

	if err := loadActualACLInterfaces(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load acl interfaces")
	}

	if err := loadActualVRFs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load vrfs")
	}

	if err := loadActualRouteMaps(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load route maps")
	}

	return nil
}
