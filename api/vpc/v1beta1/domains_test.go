// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCheckCommonDomain(t *testing.T) {
	const planeB = "plane-b"

	sw := func(name string, domains ...string) *wiringapi.Switch {
		return &wiringapi.Switch{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Domains: domains}},
		}
	}
	unbundled := func(name, leaf string) *wiringapi.Connection {
		return &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{Unbundled: &wiringapi.ConnUnbundled{
				Link: wiringapi.ServerToSwitchLink{
					Server: wiringapi.NewBasePortName("server-01/enp2s1"),
					Switch: wiringapi.NewBasePortName(leaf + "/E1/1"),
				},
			}},
		}
	}

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		unbundled("on-default", "leaf-default"),
		unbundled("on-b", "leaf-b"),
		unbundled("on-shared", "leaf-shared"),
		unbundled("on-old", "leaf-old"),
		unbundled("on-missing", "leaf-missing"),
		sw("leaf-default", wiringapi.DefaultFabricDomain),
		sw("leaf-b", planeB),
		sw("leaf-shared", wiringapi.DefaultFabricDomain, planeB),
		sw("leaf-old"),
	).Build()

	for _, tt := range []struct {
		name  string
		pin   string
		conns []string
		err   string
	}{
		{name: "single domain", conns: []string{"on-default", "on-old"}},
		{name: "shared leaf only", conns: []string{"on-shared"}},
		{name: "shared leaf narrowed to one domain", conns: []string{"on-shared", "on-b"}},
		{
			name: "two domains", conns: []string{"on-default", "on-shared", "on-b"},
			err: "switch leaf-default is in domains [default], sharing none with the other attachments in [plane-b]",
		},
		{
			name: "written before domains existed, then another domain", conns: []string{"on-old", "on-b"},
			err: "sharing none",
		},
		{name: "pinned", pin: planeB, conns: []string{"on-shared", "on-b"}},
		{
			name: "pinned elsewhere", pin: planeB, conns: []string{"on-shared", "on-default"},
			err: "switch leaf-default is in domains [default], not in pinned domain plane-b",
		},
		{name: "same connection twice", conns: []string{"on-b", "on-b"}},
		{name: "connection not found", conns: []string{"on-default", "nope"}},
		{name: "switch not found", conns: []string{"on-b", "on-missing"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			switches, err := v1beta1.ConnectionSwitches(t.Context(), kube, kmetav1.NamespaceDefault, tt.conns)
			require.NoError(t, err)

			err = v1beta1.CheckCommonDomain(tt.pin, switches)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestVPCDomainPin(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&v1beta1.IPv4Namespace{
				ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
				Spec:       v1beta1.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0/16"}},
			},
			&wiringapi.VLANNamespace{
				ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
				Spec:       wiringapi.VLANNamespaceSpec{Ranges: []meta.VLANRange{{From: 100, To: 200}}},
			},
			&wiringapi.Fabric{
				ObjectMeta: kmetav1.ObjectMeta{Name: "default", Namespace: kmetav1.NamespaceDefault},
				Spec: wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{
					"default": {SpineASN: 65100, GatewayASN: 65534},
					"plane-b": {SpineASN: 65099, GatewayASN: 65535},
				}},
			},
		).
		Build()

	vpc := vpcGen("vpc-pin", func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Domain = "plane-b" })
	_, err := vpc.Validate(t.Context(), kube, &meta.FabricConfig{})
	require.NoError(t, err)
	require.Contains(t, vpc.Labels, wiringapi.ListLabelDomain("plane-b"))

	vpc = vpcGen("vpc-pin", func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Domain = "plane-c" })
	_, err = vpc.Validate(t.Context(), kube, &meta.FabricConfig{})
	require.ErrorContains(t, err, "domain plane-c not found in fabric default")
}

func TestVPCConnections(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	attach := func(name, subnet, conn string) *v1beta1.VPCAttachment {
		attach := &v1beta1.VPCAttachment{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       v1beta1.VPCAttachmentSpec{Subnet: subnet, Connection: conn},
		}
		attach.Default()

		return attach
	}
	staticExternal := func(name, vpc string) *wiringapi.Connection {
		conn := &wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{StaticExternal: &wiringapi.ConnStaticExternal{
				WithinVPC: vpc,
				Link:      wiringapi.ConnStaticExternalLink{Switch: wiringapi.ConnStaticExternalLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-03/E1/1"}}},
			}},
		}
		conn.Default()

		return conn
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		attach("vpc-01--a", "vpc-01/default", "conn-a"),
		attach("vpc-01--b", "vpc-01/other", "conn-b"),
		attach("vpc-02--c", "vpc-02/default", "conn-c"),
		staticExternal("static-1", "vpc-01"),
		staticExternal("static-2", "vpc-02"),
	).Build()

	conns, err := v1beta1.VPCConnections(t.Context(), kube, kmetav1.NamespaceDefault, "vpc-01", "", "")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"conn-a", "conn-b", "static-1"}, conns)

	conns, err = v1beta1.VPCConnections(t.Context(), kube, kmetav1.NamespaceDefault, "vpc-01", "vpc-01--a", "static-1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"conn-b"}, conns)
}
