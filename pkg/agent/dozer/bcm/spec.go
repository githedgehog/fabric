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

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
)

var specEnforcer = &DefaultValueEnforcer[string, *dozer.Spec]{
	Summary: "Spec",
	CustomHandler: func(basePath string, _ string, actual, desired *dozer.Spec, actions *ActionQueue) error {
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

		if err := specLLDPEnforcer.Handle(basePath, "", actual.LLDP, desired.LLDP, actions); err != nil {
			return errors.Wrap(err, "failed to handle lldp")
		}

		if err := specLLDPInterfacesEnforcer.Handle(basePath, actual.LLDPInterfaces, desired.LLDPInterfaces, actions); err != nil {
			return errors.Wrap(err, "failed to handle lldp interfaces")
		}

		if err := specNTPEnforcer.Handle(basePath, "", actual.NTP, desired.NTP, actions); err != nil {
			return errors.Wrap(err, "failed to handle ntp")
		}

		if err := specNTPServersEnforcer.Handle(basePath, actual.NTPServers, desired.NTPServers, actions); err != nil {
			return errors.Wrap(err, "failed to handle ntp servers")
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

		if err := specPrefixListsEnforcer.Handle(basePath, actual.PrefixLists, desired.PrefixLists, actions); err != nil {
			return errors.Wrap(err, "failed to handle prefix lists")
		}

		if err := specRouteMapsEnforcer.Handle(basePath, actual.RouteMaps, desired.RouteMaps, actions); err != nil {
			return errors.Wrap(err, "failed to handle route maps")
		}

		if err := specVXLANTunnelsEnforcer.Handle(basePath, actual.VXLANTunnels, desired.VXLANTunnels, actions); err != nil {
			return errors.Wrap(err, "failed to handle vxlan tunnels")
		}

		if err := specVXLANEVPNNVOsEnforcer.Handle(basePath, actual.VXLANEVPNNVOs, desired.VXLANEVPNNVOs, actions); err != nil {
			return errors.Wrap(err, "failed to handle vxlan evpn nvos")
		}

		if err := specVXLANTunnelMapsEnforcer.Handle(basePath, actual.VXLANTunnelMap, desired.VXLANTunnelMap, actions); err != nil {
			return errors.Wrap(err, "failed to handle vxlan tunnel map")
		}

		if err := specVRFVNIMapEnforcer.Handle(basePath, actual.VRFVNIMap, desired.VRFVNIMap, actions); err != nil {
			return errors.Wrap(err, "failed to handle vrf vni map")
		}

		if err := specSuppressVLANNeighsEnforcer.Handle(basePath, actual.SuppressVLANNeighs, desired.SuppressVLANNeighs, actions); err != nil {
			return errors.Wrap(err, "failed to handle suppress vlan neighs")
		}

		if err := specCommunityListsEnforcer.Handle(basePath, actual.CommunityLists, desired.CommunityLists, actions); err != nil {
			return errors.Wrap(err, "failed to handle community lists")
		}

		if err := specLSTGroupsEnforcer.Handle(basePath, actual.LSTGroups, desired.LSTGroups, actions); err != nil {
			return errors.Wrap(err, "failed to handle lst groups")
		}

		if err := specLSTInterfacesEnforcer.Handle(basePath, actual.LSTInterfaces, desired.LSTInterfaces, actions); err != nil {
			return errors.Wrap(err, "failed to handle lst interfaces")
		}

		if err := specPortChannelConfigsEnforcer.Handle(basePath, actual.PortChannelConfigs, desired.PortChannelConfigs, actions); err != nil {
			return errors.Wrap(err, "failed to handle port channel configs")
		}

		return nil
	},
}

func loadActualSpec(ctx context.Context, agent *agentapi.Agent, client *gnmi.Client, spec *dozer.Spec) error {
	if err := loadActualZTP(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load ztp")
	}

	if err := loadActualHostname(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load hostname")
	}

	if err := loadActualLLDP(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load lldp")
	}

	if err := loadActualLLDPInterfaces(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load lldp interfaces")
	}

	if err := loadActualNTP(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load ntp")
	}

	if err := loadActualNTPServers(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load ntp servers")
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

	if err := loadActualInterfaces(ctx, agent, client, spec); err != nil {
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

	if err := loadActualPrefixLists(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load prefix lists")
	}

	if err := loadActualCommunityLists(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load community lists")
	}

	if err := loadActualRouteMaps(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load route maps")
	}

	if err := loadActualVXLANs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load vxlan")
	}

	if err := loadActualVRFVNIMap(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load vrf vni map")
	}

	if err := loadActualSuppressVLANNeighs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load suppress vlan neighs")
	}

	if err := loadActualLSTGroups(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load lst groups")
	}

	if err := loadActualLSTInterfaces(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load lst interfaces")
	}

	if err := loadActualPortChannelConfigs(ctx, client, spec); err != nil {
		return errors.Wrapf(err, "failed to load port channel configs")
	}

	return nil
}
