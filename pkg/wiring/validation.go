package wiring

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ValidateFabric(ctx context.Context, kube client.Client, fabricCfg *meta.FabricConfig) error {
	// for k, v := range kube.Scheme().AllKnownTypes() {
	// 	if !strings.Contains(k.Group, "githedgehog.com") {
	// 		continue
	// 	}
	// 	if !strings.HasSuffix(v.Name(), "List") {
	// 		continue
	// 	}
	// }

	if fabricCfg == nil {
		// TODO remove hardcode
		fabricCfg = &meta.FabricConfig{
			ControlVIP:            "172.30.1.1/24",
			VPCIRBVLANRanges:      []meta.VLANRange{{From: 3000, To: 3999}},
			VPCPeeringVLANRanges:  []meta.VLANRange{{From: 100, To: 999}},
			VPCPeeringDisabled:    false,
			ReservedSubnets:       []string{"172.28.0.0/24", "172.29.0.0/24", "172.30.0.0/24", "172.31.0.0/24"},
			DHCPMode:              meta.DHCPModeHedgehog,
			FabricMode:            meta.FabricModeSpineLeaf,
			FabricMTU:             9100,
			ServerFacingMTUOffset: 64,
		}
	}

	// TODO auto iterate through all types to validate

	rackList := &wiringapi.RackList{}
	if err := kube.List(ctx, rackList); err != nil {
		return errors.Wrapf(err, "error listing racks")
	}

	for _, rack := range rackList.Items {
		rack.Default()
		if _, err := rack.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating rack %s", rack.Name)
		}
	}

	sgGroupList := &wiringapi.SwitchGroupList{}
	if err := kube.List(ctx, sgGroupList); err != nil {
		return errors.Wrapf(err, "error listing switch groups")
	}

	for _, sgGroup := range sgGroupList.Items {
		sgGroup.Default()
		if _, err := sgGroup.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating switch group %s", sgGroup.Name)
		}
	}

	swList := &wiringapi.SwitchList{}
	if err := kube.List(ctx, swList); err != nil {
		return errors.Wrapf(err, "error listing switches")
	}

	for _, sw := range swList.Items {
		sw.Default()
		if _, err := sw.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating switch %s", sw.Name)
		}
	}

	serverList := &wiringapi.ServerList{}
	if err := kube.List(ctx, serverList); err != nil {
		return errors.Wrapf(err, "error listing servers")
	}

	for _, server := range serverList.Items {
		server.Default()
		if _, err := server.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating server %s", server.Name)
		}
	}

	connList := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, connList); err != nil {
		return errors.Wrapf(err, "error listing connections")
	}

	for _, conn := range connList.Items {
		conn.Default()
		if _, err := conn.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating connection %s", conn.Name)
		}
	}

	vlanNsList := &wiringapi.VLANNamespaceList{}
	if err := kube.List(ctx, vlanNsList); err != nil {
		return errors.Wrapf(err, "error listing vlan namespaces")
	}

	for _, vlanNs := range vlanNsList.Items {
		vlanNs.Default()
		if _, err := vlanNs.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating vlan namespace %s", vlanNs.Name)
		}
	}

	ipNsList := &vpcapi.IPv4NamespaceList{}
	if err := kube.List(ctx, ipNsList); err != nil {
		return errors.Wrapf(err, "error listing ipv4 namespaces")
	}

	for _, ipNs := range ipNsList.Items {
		ipNs.Default()
		if _, err := ipNs.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating ipv4 namespace %s", ipNs.Name)
		}
	}

	switchProfileList := &wiringapi.SwitchProfileList{}
	if err := kube.List(ctx, switchProfileList); err != nil {
		return errors.Wrapf(err, "error listing switch profiles")
	}

	for _, switchProfile := range switchProfileList.Items {
		switchProfile.Default()
		if _, err := switchProfile.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating switch profile %s", switchProfile.Name)
		}
	}

	serverProfileList := &wiringapi.ServerProfileList{}
	if err := kube.List(ctx, serverProfileList); err != nil {
		return errors.Wrapf(err, "error listing server profiles")
	}

	for _, serverProfile := range serverProfileList.Items {
		serverProfile.Default()
		if _, err := serverProfile.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating server profile %s", serverProfile.Name)
		}
	}

	externalList := &vpcapi.ExternalList{}
	if err := kube.List(ctx, externalList); err != nil {
		return errors.Wrapf(err, "error listing externals")
	}

	for _, external := range externalList.Items {
		external.Default()
		if _, err := external.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating external %s", external.Name)
		}
	}

	externalAttachmentList := &vpcapi.ExternalAttachmentList{}
	if err := kube.List(ctx, externalAttachmentList); err != nil {
		return errors.Wrapf(err, "error listing external attachments")
	}

	for _, externalAttachment := range externalAttachmentList.Items {
		externalAttachment.Default()
		if _, err := externalAttachment.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating external attachment %s", externalAttachment.Name)
		}
	}

	vpcList := &vpcapi.VPCList{}
	if err := kube.List(ctx, vpcList); err != nil {
		return errors.Wrapf(err, "error listing vpcs")
	}

	for _, vpc := range vpcList.Items {
		vpc.Default()
		if _, err := vpc.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating vpc %s", vpc.Name)
		}
	}

	vpcAttachmentList := &vpcapi.VPCAttachmentList{}
	if err := kube.List(ctx, vpcAttachmentList); err != nil {
		return errors.Wrapf(err, "error listing vpc attachments")
	}

	for _, vpcAttachment := range vpcAttachmentList.Items {
		vpcAttachment.Default()
		if _, err := vpcAttachment.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating vpc attachment %s", vpcAttachment.Name)
		}
	}

	vpcPeeringList := &vpcapi.VPCPeeringList{}
	if err := kube.List(ctx, vpcPeeringList); err != nil {
		return errors.Wrapf(err, "error listing vpc peerings")
	}

	for _, vpcPeering := range vpcPeeringList.Items {
		vpcPeering.Default()
		if _, err := vpcPeering.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating vpc peering %s", vpcPeering.Name)
		}
	}

	extPeeringList := &vpcapi.ExternalPeeringList{}
	if err := kube.List(ctx, extPeeringList); err != nil {
		return errors.Wrapf(err, "error listing external peerings")
	}

	for _, extPeering := range extPeeringList.Items {
		extPeering.Default()
		if _, err := extPeering.Validate(ctx, kube, fabricCfg); err != nil {
			return errors.Wrapf(err, "error validating external peering %s", extPeering.Name)
		}
	}

	return nil
}
