// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVPCAttachmentValidation(t *testing.T) {
	const planeB = "plane-b"

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	objMeta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	// defaulted as stored, so that no domains given means the default one
	sw := func(name string, domains ...string) *wiringapi.Switch {
		return defaulted(&wiringapi.Switch{ObjectMeta: objMeta(name), Spec: wiringapi.SwitchSpec{
			Profile:        "profile",
			VLANNamespaces: []string{wiringapi.DefaultVLANNamespace},
			Topology:       wiringapi.SwitchTopology{Domains: domains},
		}})
	}
	link := func(serverPort, leaf string) wiringapi.ServerToSwitchLink {
		return wiringapi.ServerToSwitchLink{
			Server: wiringapi.NewBasePortName("server-01/" + serverPort),
			Switch: wiringapi.NewBasePortName(leaf + "/E1/1"),
		}
	}
	unbundled := defaulted(&wiringapi.Connection{ObjectMeta: objMeta("server-01--unbundled--leaf-01"), Spec: wiringapi.ConnectionSpec{
		Unbundled: &wiringapi.ConnUnbundled{Link: link("enp2s1", "leaf-01")},
	}})
	eslag := defaulted(&wiringapi.Connection{ObjectMeta: objMeta("server-01--eslag--leaf-01--leaf-02"), Spec: wiringapi.ConnectionSpec{
		ESLAG: &wiringapi.ConnESLAG{Links: []wiringapi.ServerToSwitchLink{link("enp2s1", "leaf-01"), link("enp2s2", "leaf-02")}},
	}})
	fabric := &wiringapi.Fabric{ObjectMeta: objMeta(wiringapi.DefaultFabric), Spec: wiringapi.FabricSpec{
		Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}, planeB: {}},
	}}
	profile := &wiringapi.SwitchProfile{ObjectMeta: objMeta("profile"), Spec: wiringapi.SwitchProfileSpec{
		Features: wiringapi.SwitchProfileFeatures{L2VNI: true, L3VNI: true},
	}}
	vpcIn := func(domains ...string) *v1beta1.VPC {
		vpc := vpcGen("vpc-01")
		vpc.Spec.Topology.Domains = domains

		return vpc
	}
	attach := func(name, subnet, conn string) *v1beta1.VPCAttachment {
		attach := &v1beta1.VPCAttachment{ObjectMeta: objMeta(name), Spec: v1beta1.VPCAttachmentSpec{Subnet: subnet, Connection: conn}}
		attach.Default()

		return attach
	}
	onUnbundled := attach("vpc-01--unbundled", "vpc-01/default", unbundled.Name)
	onESLAG := attach("vpc-01--eslag", "vpc-01/default", eslag.Name)

	for _, tt := range []struct {
		name    string
		attach  *v1beta1.VPCAttachment
		objects []kclient.Object
		err     string
	}{
		{
			name:    "valid",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain), unbundled, sw("leaf-01", wiringapi.DefaultFabricDomain)},
		},
		{
			name:    "vpc not found",
			attach:  onUnbundled,
			objects: []kclient.Object{unbundled, sw("leaf-01")},
			err:     "vpc vpc-01 not found",
		},
		{
			name:    "subnet not found",
			attach:  attach("vpc-01--other", "vpc-01/other", unbundled.Name),
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain), unbundled, sw("leaf-01")},
			err:     "subnet other not found in vpc vpc-01",
		},
		{
			name:    "connection not found",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain)},
			err:     "connection server-01--unbundled--leaf-01 not found",
		},
		{
			name:   "subnet already attached to the connection",
			attach: onUnbundled,
			objects: []kclient.Object{
				vpcIn(wiringapi.DefaultFabricDomain), unbundled, sw("leaf-01"),
				attach("vpc-01--unbundled-again", "vpc-01/default", unbundled.Name),
			},
			err: "connection server-01--unbundled--leaf-01 already attached to vpc subnet vpc-01/default",
		},
		{
			name:    "switch with domains defaulted",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain), unbundled, sw("leaf-01")},
		},
		{
			name:    "switch outside the vpc domain",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(planeB), unbundled, sw("leaf-01", wiringapi.DefaultFabricDomain)},
			err:     "vpc vpc-01 is in domains [plane-b] but switch leaf-01 is in domains [default]",
		},
		{
			name:    "switch in the vpc domain and another",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(planeB), unbundled, sw("leaf-01", wiringapi.DefaultFabricDomain, planeB)},
		},
		{
			name:    "vpc with domains defaulted, switch in another domain",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcGen("vpc-01"), unbundled, sw("leaf-01", planeB)},
			err:     "vpc vpc-01 is in domains [default] but switch leaf-01 is in domains [plane-b]",
		},
		{
			name:    "eslag with one switch outside the vpc domain",
			attach:  onESLAG,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain), eslag, sw("leaf-01"), sw("leaf-02", planeB)},
			err:     "vpc vpc-01 is in domains [default] but switch leaf-02 is in domains [plane-b]",
		},
		{
			name:    "vpc in two domains, switch in both",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain, planeB), unbundled, sw("leaf-01", wiringapi.DefaultFabricDomain, planeB)},
		},
		{
			name:    "vpc in two domains, switch in one of them",
			attach:  onUnbundled,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain, planeB), unbundled, sw("leaf-01", planeB)},
			err:     "vpc vpc-01 is in domains [default plane-b] but switch leaf-01 is in domains [plane-b]",
		},
		{
			name:    "vpc in two domains, eslag with one switch in only one of them",
			attach:  onESLAG,
			objects: []kclient.Object{vpcIn(wiringapi.DefaultFabricDomain, planeB), eslag, sw("leaf-01", wiringapi.DefaultFabricDomain, planeB), sw("leaf-02", wiringapi.DefaultFabricDomain)},
			err:     "vpc vpc-01 is in domains [default plane-b] but switch leaf-02 is in domains [default]",
		},
		{
			name:    "eslag with both switches in the vpc domain",
			attach:  onESLAG,
			objects: []kclient.Object{vpcIn(planeB), eslag, sw("leaf-01", planeB), sw("leaf-02", wiringapi.DefaultFabricDomain, planeB)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(tt.objects, profile, fabric)...).Build()

			_, err := tt.attach.Validate(t.Context(), kube, nil)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
