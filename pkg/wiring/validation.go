package wiring

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ValidateFabric(ctx context.Context, kube client.Client, fabricCfg *meta.FabricConfig) error {
	for k, v := range kube.Scheme().AllKnownTypes() {
		if !strings.Contains(k.Group, "githedgehog.com") {
			continue
		}
		if !strings.HasSuffix(v.Name(), "List") {
			continue
		}

		fmt.Println(k, v)
	}

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

	return nil
}
