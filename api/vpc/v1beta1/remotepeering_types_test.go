// Copyright 2026 Hedgehog
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

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmetav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func rpGen(name string, f ...func(rp *v1beta1.RemotePeering)) *v1beta1.RemotePeering {
	base := &v1beta1.RemotePeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.RemotePeeringSpec{
			Links: []v1beta1.RemotePeeringLink{
				{Connection: "leaf-01--external", VLAN: 101, RemoteASN: 64801},
			},
			Local: map[string]v1beta1.RemotePeeringVPC{
				"vpc-01": {Subnets: []string{"default"}},
			},
			Remote: v1beta1.RemotePeeringRemote{
				Prefixes: []string{"10.1.1.0/24"},
			},
		},
	}

	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func TestRemotePeeringValidation(t *testing.T) {
	baseObjs := []kclient.Object{
		ipv4NamespaceObj(),
		vpcGen("vpc-01"),
		defaulted(&wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01--external", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				External: &wiringapi.ConnExternal{
					Link: wiringapi.ConnExternalLink{Switch: wiringapi.BasePortName{Port: "leaf-01/E1/1"}},
				},
			},
		}),
		defaulted(&wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01--fabric", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.ConnectionSpec{
				Fabric: &wiringapi.ConnFabric{
					Links: []wiringapi.FabricLink{{
						Spine: wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "spine-01/E1/1"}},
						Leaf:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "leaf-01/E1/2"}},
					}},
				},
			},
		}),
	}
	asnCfg := &meta.FabricConfig{SpineASN: 65100, LeafASNStart: 65101, LeafASNEnd: 65200, GatewayASN: 65534}
	leafOnB := defaulted(&wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01", Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Domains: []string{"plane-b"}}},
	})
	leafWithVLANNs := func(vlanNs string) *wiringapi.Switch {
		return defaulted(&wiringapi.Switch{
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01", Namespace: kmetav1.NamespaceDefault},
			Spec:       wiringapi.SwitchSpec{VLANNamespaces: []string{vlanNs}},
		})
	}
	withLink := func(link v1beta1.RemotePeeringLink) func(*v1beta1.RemotePeering) {
		return func(rp *v1beta1.RemotePeering) { rp.Spec.Links = append(rp.Spec.Links, link) }
	}
	withRemote := func(prefixes ...string) func(*v1beta1.RemotePeering) {
		return func(rp *v1beta1.RemotePeering) { rp.Spec.Remote.Prefixes = prefixes }
	}

	tests := []struct {
		name    string
		rp      *v1beta1.RemotePeering
		objects []kclient.Object
		cfg     *meta.FabricConfig
		err     bool
		warns   bool
	}{
		{
			name: "valid",
			rp:   rpGen("rp-01"),
		},
		{
			name: "valid with a second link and BFD",
			rp: rpGen("rp-01", withLink(v1beta1.RemotePeeringLink{
				Connection: "leaf-01--external", VLAN: 102, BFD: &v1beta1.ExternalAttachmentBFD{},
			})),
		},
		{
			name: "valid with a default route and no remote ASN",
			rp: rpGen("rp-01", withRemote("0.0.0.0/0", "10.1.1.0/24"), func(rp *v1beta1.RemotePeering) {
				rp.Spec.Links[0].RemoteASN = 0
			}),
		},
		{
			name: "not in the default namespace",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Namespace = "fabric-ns" }),
			err:  true,
		},
		{
			name: "name too long",
			rp:   rpGen("rp-0123456789"),
			err:  true,
		},
		{
			name: "loopback workaround",
			rp:   rpGen("rp-01"),
			cfg:  &meta.FabricConfig{LoopbackWorkaround: true},
			err:  true,
		},
		{
			name: "no links",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links = nil }),
			err:  true,
		},
		{
			name: "link without connection",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].Connection = "" }),
			err:  true,
		},
		{
			name: "same connection and VLAN twice",
			rp:   rpGen("rp-01", withLink(v1beta1.RemotePeeringLink{Connection: "leaf-01--external", VLAN: 101})),
			err:  true,
		},
		{
			name: "invalid BFD",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Links[0].BFD = &v1beta1.ExternalAttachmentBFD{MinRX: 1}
			}),
			err: true,
		},
		{
			name: "inbound ACL",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Links[0].InboundACL = &v1beta1.ACLSpec{Statements: []v1beta1.ACLStatement{
					{Seq: 10, Action: v1beta1.ACLActionDeny, Protocol: v1beta1.ACLProtocolIP, SrcPrefix: v1beta1.ACLAny, DstPrefix: "10.0.0.0/16"},
				}}
			}),
		},
		{
			name: "inbound ACL in the reserved sequence range",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Links[0].InboundACL = &v1beta1.ACLSpec{Statements: []v1beta1.ACLStatement{
					{Seq: 5, Action: v1beta1.ACLActionDeny, Protocol: v1beta1.ACLProtocolIP, SrcPrefix: v1beta1.ACLAny, DstPrefix: v1beta1.ACLAny},
				}}
			}),
			err: true,
		},
		{
			name:  "BFD with BFD disabled in the fabric",
			rp:    rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].BFD = &v1beta1.ExternalAttachmentBFD{} }),
			cfg:   &meta.FabricConfig{DisableBFD: true},
			warns: true,
		},
		{
			name: "no local VPCs",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Local = nil }),
			err:  true,
		},
		{
			name: "local VPC without a name",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Local[""] = v1beta1.RemotePeeringVPC{Subnets: []string{"default"}}
			}),
			err: true,
		},
		{
			name: "local VPC without subnets",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Local["vpc-01"] = v1beta1.RemotePeeringVPC{} }),
			err:  true,
		},
		{
			name: "local subnet listed twice",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Local["vpc-01"] = v1beta1.RemotePeeringVPC{Subnets: []string{"default", "default"}}
			}),
			err: true,
		},
		{
			name: "local VPC does not exist",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Local["vpc-02"] = v1beta1.RemotePeeringVPC{Subnets: []string{"default"}}
			}),
			err: true,
		},
		{
			name: "local subnet does not exist",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Local["vpc-01"] = v1beta1.RemotePeeringVPC{Subnets: []string{"other"}}
			}),
			err: true,
		},
		{
			name: "local VPC in another IPv4Namespace",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Local["vpc-02"] = v1beta1.RemotePeeringVPC{Subnets: []string{"default"}}
			}),
			objects: []kclient.Object{
				defaulted(&v1beta1.IPv4Namespace{
					ObjectMeta: kmetav1.ObjectMeta{Name: "ns-02", Namespace: kmetav1.NamespaceDefault},
					Spec:       v1beta1.IPv4NamespaceSpec{Subnets: []string{"10.2.0.0/16"}},
				}),
				vpcGen("vpc-02", func(vpc *v1beta1.VPC) {
					vpc.Spec.IPv4Namespace = "ns-02"
					vpc.Spec.Subnets["default"].Subnet = "10.2.1.0/24"
					vpc.Spec.Subnets["default"].Gateway = "10.2.1.1"
				}),
			},
			err: true,
		},
		{
			name: "local VPC outside the domain",
			rp: rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
				rp.Spec.Local["vpc-02"] = v1beta1.RemotePeeringVPC{Subnets: []string{"default"}}
			}),
			objects: []kclient.Object{
				vpcGen("vpc-02", func(vpc *v1beta1.VPC) { vpc.Spec.Topology.Domains = []string{"plane-b"} }),
			},
			err: true,
		},
		{
			name: "no remote prefixes",
			rp:   rpGen("rp-01", withRemote()),
			err:  true,
		},
		{
			name: "invalid remote prefix",
			rp:   rpGen("rp-01", withRemote("10.1.1.0")),
			err:  true,
		},
		{
			name: "remote prefix with host bits",
			rp:   rpGen("rp-01", withRemote("10.1.1.1/24")),
			err:  true,
		},
		{
			name: "IPv6 remote prefix",
			rp:   rpGen("rp-01", withRemote("2001:db8::/64")),
			err:  true,
		},
		{
			name: "remote prefix listed twice",
			rp:   rpGen("rp-01", withRemote("10.1.1.0/24", "10.1.1.0/24")),
			err:  true,
		},
		{
			name: "remote prefix inside the IPv4Namespace",
			rp:   rpGen("rp-01", withRemote("10.0.200.0/24")),
			err:  true,
		},
		{
			name: "remote prefix containing the IPv4Namespace",
			rp:   rpGen("rp-01", withRemote("10.0.0.0/8")),
		},
		{
			name: "connection does not exist",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].Connection = "leaf-01--other" }),
			err:  true,
		},
		{
			name: "connection is not external",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].Connection = "leaf-01--fabric" }),
			err:  true,
		},
		{
			name: "remote ASN is the own spine ASN",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].RemoteASN = 65100 }),
			cfg:  asnCfg,
			err:  true,
		},
		{
			name: "VLAN used by an external attachment",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].VLAN = 100 }),
			objects: []kclient.Object{
				l3ExtAttGen("ext-att-01"),
			},
			err: true,
		},
		{
			name: "VLAN next to an external attachment",
			rp:   rpGen("rp-01"),
			objects: []kclient.Object{
				l3ExtAttGen("ext-att-01"),
			},
		},
		{
			name: "VLAN used by another remote peering",
			rp:   rpGen("rp-01"),
			objects: []kclient.Object{
				rpGen("rp-02"),
			},
			err: true,
		},
		{
			name: "VLAN next to another remote peering",
			rp:   rpGen("rp-01"),
			objects: []kclient.Object{
				rpGen("rp-02", func(rp *v1beta1.RemotePeering) { rp.Spec.Links[0].VLAN = 102 }),
			},
		},
		{
			name: "updating itself",
			rp:   rpGen("rp-01"),
			objects: []kclient.Object{
				rpGen("rp-01"),
			},
		},
		{
			name:    "switch outside the domain",
			rp:      rpGen("rp-01"),
			objects: []kclient.Object{leafOnB},
			err:     true,
		},
		{
			name:    "switch with the VLAN namespace of the local VPCs",
			rp:      rpGen("rp-01"),
			objects: []kclient.Object{leafWithVLANNs(wiringapi.DefaultVLANNamespace)},
		},
		{
			name:    "switch without the VLAN namespace of a local VPC",
			rp:      rpGen("rp-01"),
			objects: []kclient.Object{leafWithVLANNs("other")},
			err:     true,
		},
		{
			name: "domain not in the fabric",
			rp:   rpGen("rp-01", func(rp *v1beta1.RemotePeering) { rp.Spec.Topology.Domain = "plane-b" }),
			err:  true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.cfg
			if cfg == nil {
				cfg = &meta.FabricConfig{}
			}
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(withObjs(withObjs(baseObjs, test.objects...), fabricObj(wiringapi.DefaultFabric, cfg))...).
				Build()
			warns, err := test.rp.Validate(t.Context(), kube, cfg)
			if test.err {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "unexpected error during validation")
			}
			if test.warns {
				require.NotEmpty(t, warns, "expected a warning but got none")
			} else {
				require.Empty(t, warns, "unexpected warning during validation")
			}
		})
	}
}

// without a client nothing that needs other objects is checked, the fabric neither
func TestRemotePeeringValidationWithoutClient(t *testing.T) {
	_, err := rpGen("rp-01").Validate(t.Context(), nil, &meta.FabricConfig{})
	require.NoError(t, err)
}

// the list labels of VPCs and connections it no longer uses go away, so they don't keep it in the way of their
// updates and deletes
func TestRemotePeeringDefaultDropsUnusedLabels(t *testing.T) {
	rp := rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
		rp.Spec.Local["vpc-02"] = v1beta1.RemotePeeringVPC{Subnets: []string{"default"}}
		rp.Spec.Links = append(rp.Spec.Links, v1beta1.RemotePeeringLink{Connection: "leaf-02--external", VLAN: 101})
	})
	require.Contains(t, rp.Labels, v1beta1.ListLabelVPC("vpc-02"))
	require.Contains(t, rp.Labels, wiringapi.ListLabelConnection("leaf-02--external"))

	delete(rp.Spec.Local, "vpc-02")
	rp.Spec.Links = rp.Spec.Links[:1]
	rp.Default()
	require.NotContains(t, rp.Labels, v1beta1.ListLabelVPC("vpc-02"))
	require.NotContains(t, rp.Labels, wiringapi.ListLabelConnection("leaf-02--external"))
	require.Contains(t, rp.Labels, v1beta1.ListLabelVPC("vpc-01"))
	require.Contains(t, rp.Labels, wiringapi.ListLabelConnection("leaf-01--external"))
}

// empty names would make invalid label keys, the validation reports them instead
func TestRemotePeeringDefaultEmptyNames(t *testing.T) {
	rp := rpGen("rp-01", func(rp *v1beta1.RemotePeering) {
		rp.Spec.Links[0].Connection = ""
		rp.Spec.Local[""] = v1beta1.RemotePeeringVPC{Subnets: []string{"default"}}
	})
	require.Empty(t, kmetav1validation.ValidateLabels(rp.Labels, field.NewPath("metadata.labels")))
}
