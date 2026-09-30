// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTopologyDomainImmutable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).Build()

	vpc := func(domains ...string) *vpcapi.VPC {
		return &vpcapi.VPC{Spec: vpcapi.VPCSpec{Topology: vpcapi.VPCTopology{Domains: domains}}}
	}
	ext := func(domain string) *vpcapi.External {
		return &vpcapi.External{Spec: vpcapi.ExternalSpec{Topology: vpcapi.ExternalTopology{Domain: domain}}}
	}
	gw := func(domain string) *gwapi.Gateway {
		return &gwapi.Gateway{Spec: gwapi.GatewaySpec{Topology: gwapi.GatewayTopology{Domain: domain}}}
	}
	gwGr := func(domain string) *gwapi.GatewayGroup {
		return &gwapi.GatewayGroup{Spec: gwapi.GatewayGroupSpec{Topology: gwapi.GatewayGroupTopology{Domain: domain}}}
	}
	vpcWh := &VPCWebhook{KubeClient: kube}
	extWh := &ExternalWebhook{KubeClient: kube}
	gwWh := &GatewayWebhook{Reader: kube}
	gwGrWh := &GatewayGroupWebhook{Reader: kube}

	for _, tt := range []struct {
		name      string
		update    func() error
		immutable bool
	}{
		{name: "vpc domain changed", immutable: true, update: func() error {
			_, err := vpcWh.ValidateUpdate(t.Context(), vpc(wiringapi.DefaultFabricDomain), vpc("plane-b"))

			return err
		}},
		{name: "vpc domain added", immutable: true, update: func() error {
			_, err := vpcWh.ValidateUpdate(t.Context(), vpc(wiringapi.DefaultFabricDomain), vpc(wiringapi.DefaultFabricDomain, "plane-b"))

			return err
		}},
		// stored before domains existed, then defaulted by admission on its first update
		{name: "vpc domain defaulted", update: func() error {
			_, err := vpcWh.ValidateUpdate(t.Context(), vpc(), vpc(wiringapi.DefaultFabricDomain))

			return err
		}},
		{name: "vpc domains reordered", update: func() error {
			_, err := vpcWh.ValidateUpdate(t.Context(), vpc("plane-b", wiringapi.DefaultFabricDomain), vpc(wiringapi.DefaultFabricDomain, "plane-b"))

			return err
		}},
		{name: "external domain changed", immutable: true, update: func() error {
			_, err := extWh.ValidateUpdate(t.Context(), ext(wiringapi.DefaultFabricDomain), ext("plane-b"))

			return err
		}},
		{name: "external domain defaulted", update: func() error {
			_, err := extWh.ValidateUpdate(t.Context(), ext(""), ext(wiringapi.DefaultFabricDomain))

			return err
		}},
		{name: "gateway domain changed", immutable: true, update: func() error {
			_, err := gwWh.ValidateUpdate(t.Context(), gw(wiringapi.DefaultFabricDomain), gw("plane-b"))

			return err
		}},
		{name: "gateway domain defaulted", update: func() error {
			_, err := gwWh.ValidateUpdate(t.Context(), gw(""), gw(wiringapi.DefaultFabricDomain))

			return err
		}},
		{name: "gateway group domain changed", immutable: true, update: func() error {
			_, err := gwGrWh.ValidateUpdate(t.Context(), gwGr(wiringapi.DefaultFabricDomain), gwGr("plane-b"))

			return err
		}},
		{name: "gateway group domain defaulted", update: func() error {
			_, err := gwGrWh.ValidateUpdate(t.Context(), gwGr(""), gwGr(wiringapi.DefaultFabricDomain))

			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.update()
			if tt.immutable {
				require.ErrorContains(t, err, "is immutable")

				return
			}
			// the rest of validation fails on the bare objects, past the immutability check
			if err != nil {
				require.NotContains(t, err.Error(), "is immutable")
			}
		})
	}
}
