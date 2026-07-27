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

// hostBGPSubnetCIDR is the subnet of the hostBGP subnet added by hostBGPSubnet
const hostBGPSubnetCIDR = "10.0.2.0/24"

// hostBGPSubnet adds a hostBGP subnet named "bgp" to the VPC, accepting the given extra
// prefixes with the default prefix lengths
func hostBGPSubnet(prefixes ...string) func(*v1beta1.VPC) {
	var extraPrefixes map[string]v1beta1.VPCSubnetHostBGPPrefix
	for _, prefix := range prefixes {
		if extraPrefixes == nil {
			extraPrefixes = map[string]v1beta1.VPCSubnetHostBGPPrefix{}
		}
		extraPrefixes[prefix] = v1beta1.VPCSubnetHostBGPPrefix{}
	}

	return hostBGPSubnetWith(extraPrefixes)
}

// hostBGPSubnetWith adds a hostBGP subnet named "bgp" to the VPC with the given extra prefix config
func hostBGPSubnetWith(extraPrefixes map[string]v1beta1.VPCSubnetHostBGPPrefix) func(*v1beta1.VPC) {
	return func(vpc *v1beta1.VPC) {
		vpc.Spec.Subnets["bgp"] = &v1beta1.VPCSubnet{
			Subnet:               hostBGPSubnetCIDR,
			HostBGP:              true,
			HostBGPExtraPrefixes: extraPrefixes,
		}
	}
}

// otherVPCGen returns a VPC in the default IPv4/VLAN namespaces, as it would be
// stored in the API, to check the new VPC against
func otherVPCGen(subnets map[string]*v1beta1.VPCSubnet) *v1beta1.VPC {
	return &v1beta1.VPC{
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
			Subnets:       subnets,
		},
	}
}

func hostBGPPrefixes(count int) []string {
	prefixes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		prefixes = append(prefixes, fmt.Sprintf("10.100.%d.0/24", i))
	}

	return prefixes
}

func TestVPCValidation(t *testing.T) {
	reservedCfg := &meta.FabricConfig{ReservedSubnets: []string{"10.0.0.0/8"}}
	require.NoError(t, reservedCfg.WithReservedSubnets())

	// reserves a range that doesn't collide with the subnets used by vpcGen
	reservedRailCfg := &meta.FabricConfig{ReservedSubnets: []string{"192.168.0.0/16"}}
	require.NoError(t, reservedRailCfg.WithReservedSubnets())

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
			name: "host bgp subnet with extra prefixes",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.100.1.0/24", "10.100.11.0/28")),
			err:  false,
		},
		{
			name:    "host bgp subnet with extra prefixes and kube",
			vpc:     vpcGen("vpc-01", hostBGPSubnet("10.100.1.0/24", "10.100.11.0/28")),
			objects: baseKubeObjs,
			err:     false,
		},
		{
			name: "host bgp extra prefixes on non host bgp subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].HostBGPExtraPrefixes = map[string]v1beta1.VPCSubnetHostBGPPrefix{
					"10.100.1.0/24": {},
				}
			}),
			err: true,
		},
		{
			name: "host bgp prefix lengths on non host bgp subnet",
			vpc: vpcGen("vpc-01", func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["default"].HostBGPMinPrefixLen = 24
				vpc.Spec.Subnets["default"].HostBGPMaxPrefixLen = 32
			}),
			err: true,
		},
		{
			name: "host bgp extra prefix is not a cidr",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("not-a-cidr")),
			err:  true,
		},
		{
			name: "host bgp extra prefix is a bare ip",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.100.1.0")),
			err:  true,
		},
		{
			name: "host bgp extra prefix is ipv6",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("2001:db8::/64")),
			err:  true,
		},
		{
			name: "host bgp extra prefix is ipv4 mapped ipv6",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("::ffff:10.100.1.0/120")),
			err:  true,
		},
		{
			name: "host bgp extra prefix is not in canonical form",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.100.1.5/24")),
			err:  true,
		},
		{
			name: "host bgp extra prefixes at the limit",
			vpc:  vpcGen("vpc-01", hostBGPSubnet(hostBGPPrefixes(100)...)),
			err:  false,
		},
		{
			name: "too many host bgp extra prefixes",
			vpc:  vpcGen("vpc-01", hostBGPSubnet(hostBGPPrefixes(101)...)),
			err:  true,
		},
		{
			name: "host bgp extra prefix overlaps its own subnet",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.0.2.128/25")),
			err:  true,
		},
		{
			name: "host bgp extra prefix overlaps another subnet of the same vpc",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.0.1.0/24")),
			err:  true,
		},
		{
			name: "host bgp extra prefixes overlap across subnets of the same vpc",
			vpc: vpcGen("vpc-01", hostBGPSubnet("10.100.1.0/24"), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp-2"] = &v1beta1.VPCSubnet{
					Subnet:               "10.0.4.0/24",
					HostBGP:              true,
					HostBGPExtraPrefixes: map[string]v1beta1.VPCSubnetHostBGPPrefix{"10.100.1.128/25": {}},
				}
			}),
			err: true,
		},
		{
			name:      "host bgp extra prefix is reserved",
			vpc:       vpcGen("vpc-01", hostBGPSubnet("192.168.5.0/24")),
			fabricCfg: reservedRailCfg,
			err:       true,
		},
		{
			name:      "host bgp extra prefix outside of the reserved subnets",
			vpc:       vpcGen("vpc-01", hostBGPSubnet("10.100.1.0/24")),
			fabricCfg: reservedRailCfg,
			err:       false,
		},
		{
			name:    "host bgp extra prefix not in ipv4namespace",
			vpc:     vpcGen("vpc-01", hostBGPSubnet("172.16.5.0/24")),
			objects: baseKubeObjs,
			err:     true,
		},
		{
			name: "host bgp extra prefix overlaps other vpc subnet",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.0.9.0/24")),
			objects: append(baseKubeObjs, otherVPCGen(map[string]*v1beta1.VPCSubnet{
				"default": {
					Subnet:  "10.0.9.0/24",
					Gateway: "10.0.9.1",
					VLAN:    200,
				},
			})),
			err: true,
		},
		{
			name: "subnet overlaps other vpc host bgp extra prefix",
			vpc:  vpcGen("vpc-01"),
			objects: append(baseKubeObjs, otherVPCGen(map[string]*v1beta1.VPCSubnet{
				"bgp": {
					Subnet:               "10.0.9.0/24",
					HostBGP:              true,
					HostBGPExtraPrefixes: map[string]v1beta1.VPCSubnetHostBGPPrefix{"10.0.1.0/24": {}},
				},
			})),
			err: true,
		},
		{
			name: "host bgp extra prefix overlaps other vpc host bgp extra prefix",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.100.1.128/25")),
			objects: append(baseKubeObjs, otherVPCGen(map[string]*v1beta1.VPCSubnet{
				"bgp": {
					Subnet:               "10.0.9.0/24",
					HostBGP:              true,
					HostBGPExtraPrefixes: map[string]v1beta1.VPCSubnetHostBGPPrefix{"10.100.1.0/24": {}},
				},
			})),
			err: true,
		},
		{
			name: "host bgp extra prefixes not overlapping other vpc host bgp extra prefixes",
			vpc:  vpcGen("vpc-01", hostBGPSubnet("10.100.1.0/24")),
			objects: append(baseKubeObjs, otherVPCGen(map[string]*v1beta1.VPCSubnet{
				"bgp": {
					Subnet:               "10.0.9.0/24",
					HostBGP:              true,
					HostBGPExtraPrefixes: map[string]v1beta1.VPCSubnetHostBGPPrefix{"10.100.2.0/24": {}},
				},
			})),
			err: false,
		},
		{
			name: "host bgp subnet prefix lengths widening the subnet range",
			vpc: vpcGen("vpc-01", hostBGPSubnet(), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"].HostBGPMinPrefixLen = 24
				vpc.Spec.Subnets["bgp"].HostBGPMaxPrefixLen = 32
			}),
			err: false,
		},
		{
			name: "host bgp subnet min prefix length greater than max",
			vpc: vpcGen("vpc-01", hostBGPSubnet(), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"].HostBGPMinPrefixLen = 32
				vpc.Spec.Subnets["bgp"].HostBGPMaxPrefixLen = 28
			}),
			err: true,
		},
		{
			name: "host bgp subnet max prefix length above 32",
			vpc: vpcGen("vpc-01", hostBGPSubnet(), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"].HostBGPMaxPrefixLen = 33
			}),
			err: true,
		},
		{
			name: "host bgp subnet min prefix length shorter than the subnet",
			vpc: vpcGen("vpc-01", hostBGPSubnet(), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"].HostBGPMinPrefixLen = 16
			}),
			err: true,
		},
		{
			name: "host bgp extra prefix with its own prefix lengths",
			vpc: vpcGen("vpc-01", hostBGPSubnetWith(map[string]v1beta1.VPCSubnetHostBGPPrefix{
				"10.100.1.0/24":  {},
				"10.100.11.0/28": {MinPrefixLen: 28, MaxPrefixLen: 32},
			})),
			err: false,
		},
		{
			name: "host bgp extra prefix min prefix length greater than max",
			vpc: vpcGen("vpc-01", hostBGPSubnetWith(map[string]v1beta1.VPCSubnetHostBGPPrefix{
				"10.100.1.0/24": {MinPrefixLen: 32, MaxPrefixLen: 28},
			})),
			err: true,
		},
		{
			name: "host bgp extra prefix max prefix length above 32",
			vpc: vpcGen("vpc-01", hostBGPSubnetWith(map[string]v1beta1.VPCSubnetHostBGPPrefix{
				"10.100.1.0/24": {MaxPrefixLen: 33},
			})),
			err: true,
		},
		{
			name: "host bgp extra prefix min prefix length shorter than the prefix",
			vpc: vpcGen("vpc-01", hostBGPSubnetWith(map[string]v1beta1.VPCSubnetHostBGPPrefix{
				"10.100.1.0/24": {MinPrefixLen: 16},
			})),
			err: true,
		},
		{
			name: "host bgp extra prefix inherits an unusable subnet min prefix length",
			vpc: vpcGen("vpc-01", hostBGPSubnetWith(map[string]v1beta1.VPCSubnetHostBGPPrefix{
				"10.100.11.0/28": {},
			}), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"].HostBGPMinPrefixLen = 24
			}),
			err: true,
		},
		{
			name: "host bgp extra prefix overrides an unusable subnet min prefix length",
			vpc: vpcGen("vpc-01", hostBGPSubnetWith(map[string]v1beta1.VPCSubnetHostBGPPrefix{
				"10.100.11.0/28": {MinPrefixLen: 28},
			}), func(vpc *v1beta1.VPC) {
				vpc.Spec.Subnets["bgp"].HostBGPMinPrefixLen = 24
			}),
			err: false,
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

func TestVPCSubnetHostBGPPrefixLens(t *testing.T) {
	tests := []struct {
		name      string
		subnet    v1beta1.VPCSubnet
		prefixCfg v1beta1.VPCSubnetHostBGPPrefix
		// expected for the subnet's own IP range
		subnetMin, subnetMax uint8
		// expected for an extra prefix configured with prefixCfg
		extraMin, extraMax uint8
	}{
		{
			name:      "nothing set defaults to 32",
			subnetMin: 32, subnetMax: 32,
			extraMin: 32, extraMax: 32,
		},
		{
			name:      "extra prefix inherits the subnet lengths",
			subnet:    v1beta1.VPCSubnet{HostBGPMinPrefixLen: 24, HostBGPMaxPrefixLen: 30},
			subnetMin: 24, subnetMax: 30,
			extraMin: 24, extraMax: 30,
		},
		{
			name:      "extra prefix overrides both subnet lengths",
			subnet:    v1beta1.VPCSubnet{HostBGPMinPrefixLen: 24, HostBGPMaxPrefixLen: 30},
			prefixCfg: v1beta1.VPCSubnetHostBGPPrefix{MinPrefixLen: 28, MaxPrefixLen: 32},
			subnetMin: 24, subnetMax: 30,
			extraMin: 28, extraMax: 32,
		},
		{
			name:      "extra prefix overrides only the min length",
			subnet:    v1beta1.VPCSubnet{HostBGPMinPrefixLen: 24, HostBGPMaxPrefixLen: 30},
			prefixCfg: v1beta1.VPCSubnetHostBGPPrefix{MinPrefixLen: 28},
			subnetMin: 24, subnetMax: 30,
			extraMin: 28, extraMax: 30,
		},
		{
			name:      "extra prefix overrides only the max length",
			subnet:    v1beta1.VPCSubnet{HostBGPMinPrefixLen: 24, HostBGPMaxPrefixLen: 30},
			prefixCfg: v1beta1.VPCSubnetHostBGPPrefix{MaxPrefixLen: 32},
			subnetMin: 24, subnetMax: 30,
			extraMin: 24, extraMax: 32,
		},
		{
			name:      "extra prefix overrides the unset subnet lengths",
			prefixCfg: v1beta1.VPCSubnetHostBGPPrefix{MinPrefixLen: 28, MaxPrefixLen: 30},
			subnetMin: 32, subnetMax: 32,
			extraMin: 28, extraMax: 30,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minLen, maxLen := test.subnet.HostBGPPrefixLens()
			require.Equal(t, test.subnetMin, minLen, "subnet min prefix length")
			require.Equal(t, test.subnetMax, maxLen, "subnet max prefix length")

			minLen, maxLen = test.subnet.HostBGPExtraPrefixLens(test.prefixCfg)
			require.Equal(t, test.extraMin, minLen, "extra prefix min prefix length")
			require.Equal(t, test.extraMax, maxLen, "extra prefix max prefix length")
		})
	}
}
