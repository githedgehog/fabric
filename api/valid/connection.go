// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package valid

import (
	"context"
	"fmt"
	"slices"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ meta.ConnectionValidator = Connection

// Connection checks the VPCs and gateways referenced by a connection, the wiring API can't import their packages
func Connection(ctx context.Context, kube kclient.Reader, checkConn kclient.Object) error {
	if kube == nil {
		return fmt.Errorf("kube client is nil") //nolint:err113
	}

	conn, ok := checkConn.(*wiringapi.Connection)
	if !ok {
		return fmt.Errorf("unexpected connection type %T for %q", checkConn, checkConn.GetName()) //nolint:err113
	}

	if err := validateStaticExternal(ctx, kube, conn); err != nil {
		return err
	}

	return validateGateway(ctx, kube, conn)
}

// validateStaticExternal checks that the static external connection is valid
func validateStaticExternal(ctx context.Context, kube kclient.Reader, conn *wiringapi.Connection) error {
	if conn.Spec.StaticExternal != nil && conn.Spec.StaticExternal.WithinVPC != "" {
		path := field.NewPath("spec", "staticExternal", "withinVPC")
		vpc := &vpcapi.VPC{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: conn.Spec.StaticExternal.WithinVPC, Namespace: conn.Namespace}, vpc) // TODO namespace could be different?
		if kapierrors.IsNotFound(err) {
			return field.NotFound(path, conn.Spec.StaticExternal.WithinVPC)
		}
		if err != nil {
			return fmt.Errorf("failed to get vpc %s: %w", conn.Spec.StaticExternal.WithinVPC, err) // TODO replace with some internal error to not expose to the user
		}

		connFabric := conn.Spec.Topology.Fabric
		if vpcFabric := vpc.Spec.Topology.Fabric; vpcFabric != connFabric {
			return field.Invalid(path, vpc.Name, fmt.Sprintf("connection is in fabric %s but vpc %s is in fabric %s", connFabric, vpc.Name, vpcFabric))
		}

		// the VPC's VRF is built on the switch
		swName := conn.Spec.StaticExternal.Link.Switch.DeviceName()
		sw := &wiringapi.Switch{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: swName, Namespace: conn.Namespace}, sw); err != nil {
			return fmt.Errorf("failed to get switch %s: %w", swName, err) // TODO replace with some internal error to not expose to the user
		}
		vpcDomains, swDomains := vpc.Spec.Topology.Domains, sw.Spec.Topology.Domains
		// a stored VPC the refresh on fabric-ctrl initialization had to leave alone may have none, and the check
		// below would pass when there's nothing to compare
		if len(vpcDomains) == 0 {
			return field.Invalid(path, vpc.Name, fmt.Sprintf("vpc %s has no domains", vpc.Name))
		}
		if slices.ContainsFunc(vpcDomains, func(domain string) bool { return !slices.Contains(swDomains, domain) }) {
			return field.Invalid(path, vpc.Name, fmt.Sprintf("vpc %s is in domains %v but switch %s is in domains %v", vpc.Name, vpcDomains, swName, swDomains))
		}
	}

	return nil
}

// validateGateway checks that the gateways of a gateway connection exist and are in its fabric
func validateGateway(ctx context.Context, kube kclient.Reader, conn *wiringapi.Connection) error {
	if conn.Spec.Gateway == nil {
		return nil
	}

	connFabric := conn.Spec.Topology.Fabric
	for idx, link := range conn.Spec.Gateway.Links {
		path := field.NewPath("spec", "gateway", "links").Index(idx).Child("gateway", "port")
		gw := &gwapi.Gateway{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: link.Gateway.DeviceName(), Namespace: conn.Namespace}, gw)
		// a connection admitted before its gateway would never have its fabric checked
		if kapierrors.IsNotFound(err) {
			return field.Invalid(path, link.Gateway.Port, fmt.Sprintf("gateway %s not found", link.Gateway.DeviceName()))
		}
		if err != nil {
			return fmt.Errorf("failed to get gateway %s: %w", link.Gateway.DeviceName(), err) // TODO replace with some internal error to not expose to the user
		}

		if gwFabric := gw.Spec.Topology.Fabric; gwFabric != connFabric {
			return field.Invalid(path, link.Gateway.Port, fmt.Sprintf("connection is in fabric %s but gateway %s is in fabric %s", connFabric, gw.Name, gwFabric))
		}

		sw := &wiringapi.Switch{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: link.Switch.DeviceName(), Namespace: conn.Namespace}, sw); err != nil {
			return fmt.Errorf("failed to get switch %s: %w", link.Switch.DeviceName(), err) // TODO replace with some internal error to not expose to the user
		}
		gwDomain := gw.Spec.Topology.Domain
		if swDomains := sw.Spec.Topology.Domains; !slices.Equal(swDomains, []string{gwDomain}) {
			return field.Invalid(path, link.Gateway.Port, fmt.Sprintf("gateway %s is in domain %s but switch %s is in domains %v", gw.Name, gwDomain, sw.Name, swDomains))
		}
	}

	return nil
}
