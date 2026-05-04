// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"slices"
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

func ptr[T any](v T) *T { return &v }

func l3ExtAttGen(name string, f ...func(att *v1beta1.ExternalAttachment)) *v1beta1.ExternalAttachment {
	base := &v1beta1.ExternalAttachment{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.ExternalAttachmentSpec{
			External:   "external-01",
			Connection: "leaf-01--external",
			Neighbor: v1beta1.ExternalAttachmentNeighbor{
				ASN: 64000,
				IP:  "10.90.0.4",
			},
			Switch: v1beta1.ExternalAttachmentSwitch{
				VLAN: 100,
				IP:   "10.90.0.5/31",
			},
		},
	}

	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func staticExtAttGen(name string, f ...func(att *v1beta1.ExternalAttachment)) *v1beta1.ExternalAttachment {
	base := &v1beta1.ExternalAttachment{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      name,
			Namespace: kmetav1.NamespaceDefault,
		},
		Spec: v1beta1.ExternalAttachmentSpec{
			External:   "external-02",
			Connection: "leaf-01--external",
			Static: &v1beta1.ExternalAttachmentStatic{
				RemoteIP: "10.45.0.2",
				VLAN:     200,
				Proxy:    true,
			},
		},
	}
	for _, fn := range f {
		fn(base)
	}
	base.Default()

	return base
}

func withObjs(base []kclient.Object, objs ...kclient.Object) []kclient.Object {
	return append(slices.Clone(base), objs...)
}

func TestExternalAttachmentValidation(t *testing.T) {
	baseObjs := []kclient.Object{
		&v1beta1.External{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "external-01",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: v1beta1.ExternalSpec{
				IPv4Namespace:     "default",
				InboundCommunity:  "50000:1001",
				OutboundCommunity: "50000:1002",
			},
		},
		&v1beta1.External{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "external-02",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: v1beta1.ExternalSpec{
				IPv4Namespace: "default",
				Static: &v1beta1.ExternalStaticSpec{
					Prefixes: []string{"0.0.0.0/0"},
				},
			},
		},
		&wiringapi.Connection{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      "leaf-01--external",
				Namespace: kmetav1.NamespaceDefault,
			},
			Spec: wiringapi.ConnectionSpec{
				External: &wiringapi.ConnExternal{
					Link: wiringapi.ConnExternalLink{
						Switch: wiringapi.BasePortName{
							Port: "leaf-01/E1/1",
						},
					},
				},
			},
		},
	}
	tests := []struct {
		name    string
		extAtt  *v1beta1.ExternalAttachment
		objects []kclient.Object
		err     bool
	}{
		{
			name:    "valid BGP external attachment",
			extAtt:  l3ExtAttGen("ext-att-01"),
			objects: baseObjs,
			err:     false,
		},
		{
			name:    "valid static external attachment",
			extAtt:  staticExtAttGen("ext-att-02"),
			objects: baseObjs,
			err:     false,
		},
		{
			name:    "external does not exist",
			extAtt:  l3ExtAttGen("ext-att-03", func(att *v1beta1.ExternalAttachment) { att.Spec.External = "external-456" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "connection does not exist",
			extAtt:  l3ExtAttGen("ext-att-04", func(att *v1beta1.ExternalAttachment) { att.Spec.Connection = "conn-456" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "l3 attach with static external",
			extAtt:  l3ExtAttGen("ext-att-05", func(att *v1beta1.ExternalAttachment) { att.Spec.External = "external-02" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "static attach with l3 external",
			extAtt:  staticExtAttGen("ext-att-06", func(att *v1beta1.ExternalAttachment) { att.Spec.External = "external-01" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:   "multiple attaches same vlan",
			extAtt: l3ExtAttGen("ext-att-07"),
			objects: withObjs(baseObjs,
				staticExtAttGen("vlan-clash", func(att *v1beta1.ExternalAttachment) { att.Spec.Static.VLAN = 100 })),
			err: true,
		},
		{
			name:   "multiple attaches different vlans",
			extAtt: l3ExtAttGen("ext-att-08"),
			objects: withObjs(baseObjs,
				staticExtAttGen("no-clash")),
			err: false,
		},
		{
			name:    "static attach with both proxy and IP specified",
			extAtt:  staticExtAttGen("ext-att-09", func(att *v1beta1.ExternalAttachment) { att.Spec.Static.IP = "10.45.0.1/24" }),
			objects: baseObjs,
			err:     true,
		},
		{
			name:    "static attach with neither proxy nor IP specified",
			extAtt:  staticExtAttGen("ext-att-09", func(att *v1beta1.ExternalAttachment) { att.Spec.Static.Proxy = false }),
			objects: baseObjs,
			err:     true,
		},
		{
			name: "valid static attach without proxy",
			extAtt: staticExtAttGen("ext-att-09", func(att *v1beta1.ExternalAttachment) {
				att.Spec.Static.IP = "10.45.0.1/24"
				att.Spec.Static.Proxy = false
			}),
			objects: baseObjs,
		},
		{
			name: "valid attachment with inline inbound ACL",
			extAtt: l3ExtAttGen("ext-att-10", func(att *v1beta1.ExternalAttachment) {
				att.Spec.InboundACL = &v1beta1.ACLSpec{
					Statements: []v1beta1.ACLStatement{
						{
							Seq:       10,
							Action:    v1beta1.ACLActionPermit,
							Protocol:  v1beta1.ACLProtocolIP,
							SrcPrefix: v1beta1.ACLAny,
							DstPrefix: v1beta1.ACLAny,
						},
					},
				}
			}),
			objects: baseObjs,
		},
		{
			name: "invalid inline inbound ACL",
			extAtt: l3ExtAttGen("ext-att-11", func(att *v1beta1.ExternalAttachment) {
				att.Spec.InboundACL = &v1beta1.ACLSpec{
					Statements: []v1beta1.ACLStatement{
						{
							Seq:       10,
							Action:    v1beta1.ACLActionPermit,
							Protocol:  "gre",
							SrcPrefix: v1beta1.ACLAny,
							DstPrefix: v1beta1.ACLAny,
						},
					},
				}
			}),
			objects: baseObjs,
			err:     true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(test.objects...).
				Build()
			_, err := test.extAtt.Validate(t.Context(), kube, &meta.FabricConfig{})
			if test.err {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "unexpected error during validation")
			}
		})
	}
}

func extAttWithACL(stmts ...v1beta1.ACLStatement) *v1beta1.ExternalAttachment {
	return l3ExtAttGen("acl-test", func(att *v1beta1.ExternalAttachment) {
		att.Spec.InboundACL = &v1beta1.ACLSpec{Statements: stmts}
	})
}

func TestExternalAttachmentInboundACLValidation(t *testing.T) {
	tests := []struct {
		name   string
		extAtt *v1beta1.ExternalAttachment
		err    bool
	}{
		{
			name: "valid TCP statement",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:       10,
				Action:    v1beta1.ACLActionPermit,
				Protocol:  v1beta1.ACLProtocolTCP,
				SrcPrefix: "10.0.0.0/8",
				DstPrefix: v1beta1.ACLAny,
			}),
		},
		{
			name: "valid UDP statement with port range",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:            10,
				Action:         v1beta1.ACLActionDiscard,
				Protocol:       v1beta1.ACLProtocolUDP,
				SrcPrefix:      v1beta1.ACLAny,
				DstPrefix:      "192.168.0.0/16",
				PortRangeBegin: 1024,
				PortRangeEnd:   65535,
			}),
		},
		{
			name: "valid ICMP statement with type and code",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:       10,
				Action:    v1beta1.ACLActionDeny,
				Protocol:  v1beta1.ACLProtocolICMP,
				SrcPrefix: v1beta1.ACLAny,
				DstPrefix: v1beta1.ACLAny,
				ICMPType:  ptr[uint8](8),
				ICMPCode:  ptr[uint8](0),
			}),
		},
		{
			name: "valid IP statement with any prefixes",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:       10,
				Action:    v1beta1.ACLActionPermit,
				Protocol:  v1beta1.ACLProtocolIP,
				SrcPrefix: v1beta1.ACLAny,
				DstPrefix: v1beta1.ACLAny,
			}),
		},
		{
			name: "valid TCP statement with TCP filters",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:        10,
				Action:     v1beta1.ACLActionPermit,
				Protocol:   v1beta1.ACLProtocolTCP,
				SrcPrefix:  v1beta1.ACLAny,
				DstPrefix:  v1beta1.ACLAny,
				TCPFilters: &v1beta1.ACLTCPFilters{Established: true},
			}),
		},
		{
			name: "invalid TCP statement with illegal TCP filter combo",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:        10,
				Action:     v1beta1.ACLActionPermit,
				Protocol:   v1beta1.ACLProtocolTCP,
				SrcPrefix:  v1beta1.ACLAny,
				DstPrefix:  v1beta1.ACLAny,
				TCPFilters: &v1beta1.ACLTCPFilters{Established: true, NotSyn: true},
			}),
			err: true,
		},
		{
			name: "valid empty ACL",
			extAtt: l3ExtAttGen("acl-empty", func(att *v1beta1.ExternalAttachment) {
				att.Spec.InboundACL = &v1beta1.ACLSpec{}
			}),
		},
		{
			name: "valid multiple statements",
			extAtt: extAttWithACL(
				v1beta1.ACLStatement{Seq: 10, Action: v1beta1.ACLActionTransit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: v1beta1.ACLAny, DstPrefix: v1beta1.ACLAny},
				v1beta1.ACLStatement{Seq: 20, Action: v1beta1.ACLActionDeny, Protocol: v1beta1.ACLProtocolUDP, SrcPrefix: "10.0.0.0/8", DstPrefix: v1beta1.ACLAny},
			),
		},
		{
			name: "duplicate sequence numbers",
			extAtt: extAttWithACL(
				v1beta1.ACLStatement{Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: v1beta1.ACLAny, DstPrefix: v1beta1.ACLAny},
				v1beta1.ACLStatement{Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolUDP, SrcPrefix: v1beta1.ACLAny, DstPrefix: v1beta1.ACLAny},
			),
			err: true,
		},
		{
			name: "sequence number below minimum",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:       9,
				Action:    v1beta1.ACLActionPermit,
				Protocol:  v1beta1.ACLProtocolTCP,
				SrcPrefix: v1beta1.ACLAny,
				DstPrefix: v1beta1.ACLAny,
			}),
			err: true,
		},
		{
			name: "invalid action",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: "drop", Protocol: v1beta1.ACLProtocolIP, SrcPrefix: "10.0.0.0/8", DstPrefix: v1beta1.ACLAny,
			}),
			err: true,
		},
		{
			name: "invalid protocol",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: "gre", SrcPrefix: "10.0.0.0/8", DstPrefix: v1beta1.ACLAny,
			}),
			err: true,
		},
		{
			name: "invalid srcPrefix",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: "not-a-cidr", DstPrefix: v1beta1.ACLAny,
			}),
			err: true,
		},
		{
			name: "invalid dstPrefix",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: "10.0.0.0/8", DstPrefix: "300.0.0.0/8",
			}),
			err: true,
		},
		{
			name: "TCP filters on UDP protocol",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:        10,
				Action:     v1beta1.ACLActionPermit,
				Protocol:   v1beta1.ACLProtocolUDP,
				SrcPrefix:  "10.0.0.0/8",
				DstPrefix:  v1beta1.ACLAny,
				TCPFilters: &v1beta1.ACLTCPFilters{Syn: true},
			}),
			err: true,
		},
		{
			name: "ICMP type on TCP protocol",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: "10.0.0.0/8", DstPrefix: v1beta1.ACLAny,
				ICMPType: ptr[uint8](8),
			}),
			err: true,
		},
		{
			name: "ICMP code on UDP protocol",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolUDP, SrcPrefix: "10.0.0.0/8", DstPrefix: v1beta1.ACLAny,
				ICMPCode: ptr[uint8](0),
			}),
			err: true,
		},
		{
			name: "port range on IP protocol",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:            10,
				Action:         v1beta1.ACLActionPermit,
				Protocol:       v1beta1.ACLProtocolIP,
				SrcPrefix:      "10.0.0.0/8",
				DstPrefix:      v1beta1.ACLAny,
				PortRangeBegin: 80,
				PortRangeEnd:   443,
			}),
			err: true,
		},
		{
			name: "port range begin greater than end",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:            10,
				Action:         v1beta1.ACLActionPermit,
				Protocol:       v1beta1.ACLProtocolTCP,
				SrcPrefix:      "10.0.0.0/8",
				DstPrefix:      v1beta1.ACLAny,
				PortRangeBegin: 443,
				PortRangeEnd:   80,
			}),
			err: true,
		},
		{
			name: "port range on ICMP protocol",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:            10,
				Action:         v1beta1.ACLActionPermit,
				Protocol:       v1beta1.ACLProtocolICMP,
				SrcPrefix:      "10.0.0.0/8",
				DstPrefix:      v1beta1.ACLAny,
				PortRangeBegin: 1,
				PortRangeEnd:   100,
			}),
			err: true,
		},
		{
			name: "empty srcPrefix",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: "", DstPrefix: v1beta1.ACLAny,
			}),
			err: true,
		},
		{
			name: "empty dstPrefix",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: "10.0.0.0/8", DstPrefix: "",
			}),
			err: true,
		},
		{
			name: "port range begin is zero",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq: 10, Action: v1beta1.ACLActionPermit, Protocol: v1beta1.ACLProtocolTCP, SrcPrefix: "10.0.0.0/8", DstPrefix: v1beta1.ACLAny,
				PortRangeEnd: 80,
			}),
			err: true,
		},
		{
			name: "contradictory TCP flags fin and notFin",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:        10,
				Action:     v1beta1.ACLActionPermit,
				Protocol:   v1beta1.ACLProtocolTCP,
				SrcPrefix:  "10.0.0.0/8",
				DstPrefix:  v1beta1.ACLAny,
				TCPFilters: &v1beta1.ACLTCPFilters{Fin: true, NotFin: true},
			}),
			err: true,
		},
		{
			name: "contradictory TCP flags syn and notSyn",
			extAtt: extAttWithACL(v1beta1.ACLStatement{
				Seq:        10,
				Action:     v1beta1.ACLActionPermit,
				Protocol:   v1beta1.ACLProtocolTCP,
				SrcPrefix:  "10.0.0.0/8",
				DstPrefix:  v1beta1.ACLAny,
				TCPFilters: &v1beta1.ACLTCPFilters{Syn: true, NotSyn: true},
			}),
			err: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.extAtt.Spec.InboundACL.Validate()
			if test.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
