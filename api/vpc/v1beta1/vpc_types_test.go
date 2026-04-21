// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func vpcGen(name string, f ...func(*v1beta1.VPC)) *v1beta1.VPC {
	base := &v1beta1.VPC{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.VPCSpec{
			Subnets: map[string]*v1beta1.VPCSubnet{
				"default": {
					Subnet:  "10.0.1.0/24",
					Gateway: "10.0.1.1",
					VLAN:    100,
				},
			},
		},
	}
	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func TestVPCValidation(t *testing.T) {
	reservedCfg := &meta.FabricConfig{ReservedSubnets: []string{"10.0.0.0/8"}}
	require.NoError(t, reservedCfg.WithReservedSubnets())

	baseKubeObjs := []kclient.Object{
		&v1beta1.IPv4Namespace{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "default",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: v1beta1.IPv4NamespaceSpec{
				Subnets: []string{"10.0.0.0/8"},
			},
		},
		&wiringapi.VLANNamespace{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "default",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: wiringapi.VLANNamespaceSpec{
				Ranges: []meta.VLANRange{{From: 100, To: 4094}},
			},
		},
	}

	tests := []struct {
		name      string
		vpc       *v1beta1.VPC
		objects   []kclient.Object
		fabricCfg *meta.FabricConfig
		err       bool
	}{
		{
			name: "valid vpc",
			vpc:  vpcGen("vpc-01"),
			err:  false,
		},
		{
			name:    "valid vpc with kube",
			vpc:     vpcGen("vpc-01"),
			objects: baseKubeObjs,
			err:     false,
		},
		{
			name: "name too long",
			vpc:  vpcGen("vpc-toolong-n"),
			err:  true,
		},
		{
			name: "name starts with ext.",
			vpc:  vpcGen("ext.foo"),
			err:  true,
		},
		{
			name: "no subnets",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets = map[string]*v1beta1.VPCSubnet{}
			}),
			err: true,
		},
		{
			name: "too many subnets",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				for i := uint16(0); i < 21; i++ {
					vpc.Spec.Subnets[fmt.Sprintf("sub%d", i)] = &v1beta1.VPCSubnet{
						Subnet:  fmt.Sprintf("10.0.%d.0/24", i+2),
						Gateway: fmt.Sprintf("10.0.%d.1", i+2),
						VLAN:    200 + i,
					}
				}
			}),
			err: true,
		},
		{
			name: "missing subnet cidr",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Subnet = ""
			}),
			err: true,
		},
		{
			name: "invalid subnet cidr",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Subnet = "not-a-cidr"
			}),
			err: true,
		},
		{
			name: "subnet prefix too large",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Subnet = "10.0.1.0/31"
				vpc.Spec.Subnets["default"].Gateway = "10.0.1.0"
			}),
			err: true,
		},
		{
			name: "invalid gateway",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Gateway = "not-an-ip"
			}),
			err: true,
		},
		{
			name: "gateway not in subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Gateway = "10.1.0.1"
			}),
			err: true,
		},
		{
			name: "missing vlan",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].VLAN = 0
			}),
			err: true,
		},
		{
			name: "duplicate vlans",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["second"] = &v1beta1.VPCSubnet{
					Subnet:  "10.0.2.0/24",
					Gateway: "10.0.2.1",
					VLAN:    100,
				}
			}),
			err: true,
		},
		{
			name: "overlapping subnets",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["second"] = &v1beta1.VPCSubnet{
					Subnet:  "10.0.1.128/25",
					Gateway: "10.0.1.129",
					VLAN:    101,
				}
			}),
			err: true,
		},
		{
			name:      "reserved subnet",
			vpc:       vpcGen("vpc-01"),
			fabricCfg: reservedCfg,
			err:       true,
		},
		{
			name: "permit with single subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Permit = [][]string{{"default"}}
			}),
			err: true,
		},
		{
			name: "permit references unknown subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Permit = [][]string{{"default", "nonexistent"}}
			}),
			err: true,
		},
		{
			name: "dhcp relay and server both enabled",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].DHCP.Enable = true
				vpc.Spec.Subnets["default"].DHCP.Relay = "192.168.0.1/24"
			}),
			err: true,
		},
		{
			name: "dhcp enabled without range",
			vpc: func() *v1beta1.VPC {
				vpc := vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
					vpc.Spec.Subnets["default"].DHCP.Enable = true
				})
				vpc.Spec.Subnets["default"].DHCP.Range = nil

				return vpc
			}(),
			err: true,
		},
		{
			name: "dhcp range start not before end",
			vpc: func() *v1beta1.VPC {
				vpc := vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
					vpc.Spec.Subnets["default"].DHCP.Enable = true
				})
				vpc.Spec.Subnets["default"].DHCP.Range.Start = "10.0.1.200"
				vpc.Spec.Subnets["default"].DHCP.Range.End = "10.0.1.100"

				return vpc
			}(),
			err: true,
		},
		{
			name: "valid vpc with dhcp",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].DHCP.Enable = true
			}),
			err: false,
		},
		{
			name: "host bgp subnet with dhcp enabled",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"] = &v1beta1.VPCSubnet{
					Subnet:  "10.0.2.0/24",
					HostBGP: true,
					DHCP:    v1beta1.VPCDHCP{Enable: true},
				}
			}),
			err: true,
		},
		{
			name: "static ip is broadcast address",
			vpc: func() *v1beta1.VPC {
				vpc := vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
					vpc.Spec.Subnets["default"].DHCP.Enable = true
					vpc.Spec.Subnets["default"].DHCP.Static = map[string]v1beta1.VPCDHCPStatic{
						"aa:bb:cc:dd:ee:ff": {IP: "10.0.1.255"},
					}
				})

				return vpc
			}(),
			err: true,
		},
		{
			name: "static route missing prefix",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.StaticRoutes = []v1beta1.VPCStaticRoute{
					{NextHops: []string{"10.0.0.1"}},
				}
			}),
			err: true,
		},
		{
			name: "static route missing nexthops",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.StaticRoutes = []v1beta1.VPCStaticRoute{
					{Prefix: "172.16.0.0/12"},
				}
			}),
			err: true,
		},
		{
			name: "subnet not in ipv4namespace",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Subnet = "172.16.0.0/24"
				vpc.Spec.Subnets["default"].Gateway = "172.16.0.1"
			}),
			objects: baseKubeObjs,
			err:     true,
		},
		{
			name: "vlan not in vlannamespace",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].VLAN = 50
			}),
			objects: baseKubeObjs,
			err:     true,
		},
		{
			name: "subnet overlaps with other vpc identical prefix",
			vpc:  vpcGen("vpc-01"),
			objects: append(baseKubeObjs, &v1beta1.VPC{
				ObjectMeta: kmetav1.ObjectMeta{
					Name:      "other-vpc",
					Namespace: kmetav1.NamespaceDefault,
					Labels: map[string]string{
						v1beta1.LabelIPv4NS: "default",
						v1beta1.LabelVLANNS: "default",
					},
				},
				Spec: v1beta1.VPCSpec{
					IPv4Namespace: "default",
					VLANNamespace: "default",
					Subnets: map[string]*v1beta1.VPCSubnet{
						"default": {
							Subnet:  "10.0.1.0/24",
							Gateway: "10.0.1.1",
							VLAN:    200,
						},
					},
				},
			}),
			err: true,
		},
		{
			name: "new vpc subnet contains existing vpc subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Subnet = "10.0.0.0/16"
				vpc.Spec.Subnets["default"].Gateway = "10.0.0.1"
			}),
			objects: append(baseKubeObjs, &v1beta1.VPC{
				ObjectMeta: kmetav1.ObjectMeta{
					Name:      "other-vpc",
					Namespace: kmetav1.NamespaceDefault,
					Labels: map[string]string{
						v1beta1.LabelIPv4NS: "default",
						v1beta1.LabelVLANNS: "default",
					},
				},
				Spec: v1beta1.VPCSpec{
					IPv4Namespace: "default",
					VLANNamespace: "default",
					Subnets: map[string]*v1beta1.VPCSubnet{
						"default": {
							Subnet:  "10.0.1.0/24",
							Gateway: "10.0.1.1",
							VLAN:    200,
						},
					},
				},
			}),
			err: true,
		},
		{
			name: "new vpc subnet contained in existing vpc subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].Subnet = "10.0.1.64/26"
				vpc.Spec.Subnets["default"].Gateway = "10.0.1.65"
			}),
			objects: append(baseKubeObjs, &v1beta1.VPC{
				ObjectMeta: kmetav1.ObjectMeta{
					Name:      "other-vpc",
					Namespace: kmetav1.NamespaceDefault,
					Labels: map[string]string{
						v1beta1.LabelIPv4NS: "default",
						v1beta1.LabelVLANNS: "default",
					},
				},
				Spec: v1beta1.VPCSpec{
					IPv4Namespace: "default",
					VLANNamespace: "default",
					Subnets: map[string]*v1beta1.VPCSubnet{
						"default": {
							Subnet:  "10.0.1.0/24",
							Gateway: "10.0.1.1",
							VLAN:    200,
						},
					},
				},
			}),
			err: true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var kube kclient.Reader
			if test.objects != nil {
				kube = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(test.objects...).
					Build()
			}

			fabricCfg := test.fabricCfg
			if fabricCfg == nil {
				fabricCfg = &meta.FabricConfig{}
			}

			_, err := test.vpc.Validate(t.Context(), kube, fabricCfg)
			if test.err {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "unexpected error during validation")
			}
		})
	}
}
