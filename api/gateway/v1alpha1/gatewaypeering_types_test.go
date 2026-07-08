// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vpcv1beta1 "go.githedgehog.com/fabric/api/vpc/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPeeringDefaultEmpty(t *testing.T) {
	ref := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	ref.Labels = map[string]string{}

	peering := &GatewayPeering{}
	peering.Default()

	assert.Equal(t, ref, peering)
}

func TestPeeringWithVpcsNoNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup

	peering := common.DeepCopy()
	peering.Default()
	assert.NoError(t, peering.Validate(t.Context(), nil, nil), "peering should be valid")

	assert.Equal(t, ref, peering)
}

func TestPeeringWithMultipleItemsInIPs(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24", Not: "10.0.1.1/32"},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}

	peering := common.DeepCopy()
	peering.Default()
	assert.Error(t, peering.Validate(t.Context(), nil, nil), "multiple selection in the same PeeringEntryIP should be invalid")
}

func TestPeeringWithMultipleItemsInAs(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24", Not: "192.168.1.1/32"},
					},
					NAT: &PeeringNAT{
						Static: &PeeringNATStatic{},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup

	peering := common.DeepCopy()
	peering.Default()
	assert.Error(t, peering.Validate(t.Context(), nil, nil), "multiple selection in the same PeeringEntryAs should be invalid")
}

func TestPeeringWithStaticNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24"},
					},
					NAT: &PeeringNAT{
						Static: &PeeringNATStatic{},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.2.0/24"},
					},
					NAT: &PeeringNAT{
						Static: &PeeringNATStatic{},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup

	peering := common.DeepCopy()
	peering.Default()
	assert.NoError(t, peering.Validate(t.Context(), nil, nil), "peering should be valid")

	assert.Equal(t, ref, peering)
}

func TestPeeringWithDoubleMasqueradeNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24"},
					},
					NAT: &PeeringNAT{
						Masquerade: &PeeringNATMasquerade{},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.2.0/24"},
					},
					NAT: &PeeringNAT{
						Masquerade: &PeeringNATMasquerade{
							IdleTimeout: kmetav1.Duration{Duration: 3 * time.Minute},
						},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup
	ref.Spec.Peering["vpc1"].Expose[0].NAT = &PeeringNAT{
		Masquerade: &PeeringNATMasquerade{
			IdleTimeout: kmetav1.Duration{Duration: DefaultMasqueradeIdleTimeout},
		},
	}

	peering := common.DeepCopy()
	peering.Default()
	assert.Error(t, peering.Validate(t.Context(), nil, nil), "masquerade on both sides should not be allowed")
	assert.Equal(t, ref, peering)
}

func TestPeeringWithMasqueradeAndStaticNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24"},
					},
					NAT: &PeeringNAT{
						Static: &PeeringNATStatic{},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.2.0/24"},
					},
					NAT: &PeeringNAT{
						Masquerade: &PeeringNATMasquerade{
							IdleTimeout: kmetav1.Duration{Duration: 3 * time.Minute},
						},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup
	peering := common.DeepCopy()
	peering.Default()
	assert.Error(t, peering.Validate(t.Context(), nil, nil), "masquerade plus static should not be allowed")
	assert.Equal(t, ref, peering)
}

func TestPeeringWithPortForwardNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24"},
					},
					NAT: &PeeringNAT{
						PortForward: &PeeringNATPortForward{
							Ports: []PeeringNATPortForwardEntry{
								{Protocol: "tcp", Port: "80", As: "8080"},
								{Protocol: "udp", Port: "90-100", As: "8090-8100"},
								{Port: "88", As: "8088"},
							},
						},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup
	ref.Spec.Peering["vpc1"].Expose[0].NAT.PortForward.IdleTimeout.Duration = DefaultPortForwardIdleTimeout

	peering := common.DeepCopy()
	peering.Default()
	assert.NoError(t, peering.Validate(t.Context(), nil, nil), "peering should be valid")

	assert.Equal(t, ref, peering)
}

func TestPeeringWithPortForwardAndMasqueradeSameSideNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24"},
					},
					NAT: &PeeringNAT{
						PortForward: &PeeringNATPortForward{
							Ports: []PeeringNATPortForwardEntry{
								{Protocol: "tcp", Port: "80", As: "8080"},
								{Protocol: "udp", Port: "90-100", As: "8090-8100"},
								{Port: "88", As: "8088"},
							},
							IdleTimeout: kmetav1.Duration{Duration: DefaultPortForwardIdleTimeout},
						},
					},
				},
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.2.0/24"},
					},
					NAT: &PeeringNAT{
						Masquerade: &PeeringNATMasquerade{
							IdleTimeout: kmetav1.Duration{Duration: 3 * time.Minute},
						},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.3.0/24"},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup
	peering := common.DeepCopy()
	peering.Default()
	assert.NoError(t, peering.Validate(t.Context(), nil, nil), "peering should be valid")
	assert.Equal(t, ref, peering)
}

func TestPeeringWithPortForwardAndMasqueradeNAT(t *testing.T) {
	common := &GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
		},
	}
	common.Spec.Peering = map[string]*PeeringEntry{
		"vpc1": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.1.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.1.0/24"},
					},
					NAT: &PeeringNAT{
						PortForward: &PeeringNATPortForward{
							Ports: []PeeringNATPortForwardEntry{
								{Protocol: "tcp", Port: "80", As: "8080"},
								{Protocol: "udp", Port: "90-100", As: "8090-8100"},
								{Port: "88", As: "8088"},
							},
							IdleTimeout: kmetav1.Duration{Duration: DefaultPortForwardIdleTimeout},
						},
					},
				},
			},
		},
		"vpc2": {
			Expose: []PeeringEntryExpose{
				{
					IPs: []PeeringEntryIP{
						{CIDR: "10.0.2.0/24"},
					},
					As: []PeeringEntryAs{
						{CIDR: "192.168.2.0/24"},
					},
					NAT: &PeeringNAT{
						Masquerade: &PeeringNATMasquerade{
							IdleTimeout: kmetav1.Duration{Duration: 3 * time.Minute},
						},
					},
				},
			},
		},
	}

	ref := common.DeepCopy()
	ref.Labels = map[string]string{
		ListLabelVPC("vpc1"): "true",
		ListLabelVPC("vpc2"): "true",
	}
	ref.Spec.GatewayGroup = DefaultGatewayGroup
	peering := common.DeepCopy()
	peering.Default()
	assert.Error(t, peering.Validate(t.Context(), nil, nil), "masquerade + portForward should not be allowed")
	assert.Equal(t, ref, peering)
}

func TestValidateDefaultDestination(t *testing.T) {
	for _, tt := range []struct {
		name   string
		expose PeeringEntryExpose
		error  bool
	}{
		{
			name: "default with nothing else",
			expose: PeeringEntryExpose{
				DefaultDestination: true,
			},
			error: false,
		},
		{
			name: "default with IP",
			expose: PeeringEntryExpose{
				IPs: []PeeringEntryIP{
					{
						CIDR: "10.0.1.0/24",
					},
				},
				DefaultDestination: true,
			},
			error: true,
		},
		{
			name: "default with As",
			expose: PeeringEntryExpose{
				As: []PeeringEntryAs{
					{
						CIDR: "10.0.1.0/24",
					},
				},
				DefaultDestination: true,
			},
			error: true,
		},
		{
			name: "default with NAT",
			expose: PeeringEntryExpose{
				NAT: &PeeringNAT{
					Static: &PeeringNATStatic{},
				},
				DefaultDestination: true,
			},
			error: true,
		},
		{
			name: "IP with no default",
			expose: PeeringEntryExpose{
				IPs: []PeeringEntryIP{
					{
						CIDR: "10.0.1.0/24",
					},
				},
				DefaultDestination: false,
			},
			error: false,
		},
		{
			name: "no default and no IP",
			expose: PeeringEntryExpose{
				As: []PeeringEntryAs{
					{
						CIDR: "10.0.1.0/24",
					},
				},
			},
			error: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			peering := &GatewayPeering{
				Spec: PeeringSpec{
					GatewayGroup: DefaultGatewayGroup,
					Peering: map[string]*PeeringEntry{
						"vpc1": {
							Expose: []PeeringEntryExpose{
								tt.expose,
							},
						},
						"vpc2": {
							Expose: []PeeringEntryExpose{
								{
									IPs: []PeeringEntryIP{
										{
											CIDR: "10.10.1.0/24",
										},
									},
								},
							},
						},
					},
				},
			}
			ctx := t.Context()
			peering.Default()
			err := peering.Validate(ctx, nil, nil)
			if tt.error {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func withName[T kclient.Object](name string, obj T) T {
	obj.SetName(name)
	obj.SetNamespace(kmetav1.NamespaceDefault)

	return obj
}

func generatePeering(name string, f ...func(p *GatewayPeering)) *GatewayPeering {
	peering := withName(name, &GatewayPeering{
		Spec: PeeringSpec{
			GatewayGroup: DefaultGatewayGroup,
			Peering: map[string]*PeeringEntry{
				"vpc-1": {
					Expose: []PeeringEntryExpose{
						{
							IPs: []PeeringEntryIP{
								{
									CIDR: "10.0.1.0/24",
								},
							},
						},
					},
				},
				"vpc-2": {
					Expose: []PeeringEntryExpose{
						{
							IPs: []PeeringEntryIP{
								{
									CIDR: "10.0.2.0/24",
								},
							},
						},
					},
				},
			},
		},
	})
	peering.Default()

	for _, fn := range f {
		fn(peering)
	}

	return peering
}

func generateExternalPeering(name string, f ...func(p *GatewayPeering)) *GatewayPeering {
	peering := withName(name, &GatewayPeering{
		Spec: PeeringSpec{
			GatewayGroup: DefaultGatewayGroup,
			Peering: map[string]*PeeringEntry{
				"vpc-1": {
					Expose: []PeeringEntryExpose{
						{
							IPs: []PeeringEntryIP{
								{
									CIDR: "10.0.1.0/24",
								},
							},
						},
					},
				},
				"ext.out-1": {
					Expose: []PeeringEntryExpose{
						{
							DefaultDestination: true,
						},
					},
				},
			},
		},
	})
	peering.Default()

	for _, fn := range f {
		fn(peering)
	}

	return peering
}

func TestValidateCIDROverlap(t *testing.T) {
	basePeering := withName("base", &GatewayPeering{
		Spec: PeeringSpec{
			GatewayGroup: DefaultGatewayGroup,
			Peering: map[string]*PeeringEntry{
				"vpc-1": {
					Expose: []PeeringEntryExpose{
						{
							IPs: []PeeringEntryIP{
								{
									CIDR: "10.0.1.0/24",
								},
							},
						},
					},
				},
				"vpc-45": {
					Expose: []PeeringEntryExpose{
						{
							IPs: []PeeringEntryIP{
								{
									CIDR: "10.0.45.0/24",
								},
							},
						},
					},
				},
			},
		},
	})
	basePeering.Default()
	gwGroup := withName(DefaultGatewayGroup, &GatewayGroup{
		Spec: GatewayGroupSpec{},
	})
	// broad subnets so any CIDR in the overlap tests is considered part of the VPC
	vpc1 := withName("vpc-1", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"default": {Subnet: "10.0.0.0/8"},
			},
		},
	})
	vpc2 := withName("vpc-2", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"default": {Subnet: "10.0.0.0/8"},
			},
		},
	})
	vpc45 := withName("vpc-45", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"default": {Subnet: "10.0.0.0/8"},
			},
		},
	})

	baseObjs := []kclient.Object{basePeering, gwGroup, vpc1, vpc2, vpc45}

	tests := []struct {
		name    string
		peering *GatewayPeering
		objs    []kclient.Object
		err     bool
	}{
		{
			name:    "no overlap",
			peering: generatePeering("no-overlap"),
			objs:    baseObjs,
		},
		{
			name: "IP clash",
			peering: generatePeering("ip-clash", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-2"].Expose = []PeeringEntryExpose{
					{
						IPs: []PeeringEntryIP{
							{
								CIDR: "10.0.45.0/24",
							},
						},
					},
				}
			}),
			objs: baseObjs,
			err:  true,
		},
		{
			name: "NAT clash",
			peering: generatePeering("nat-clash", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-2"].Expose = []PeeringEntryExpose{
					{
						IPs: []PeeringEntryIP{
							{
								CIDR: "10.0.2.0/25",
							},
						},
						As: []PeeringEntryAs{
							{
								CIDR: "10.0.45.0/25",
							},
						},
						NAT: &PeeringNAT{
							Static: &PeeringNATStatic{},
						},
					},
				}
			}),
			objs: baseObjs,
			err:  true,
		},
		{
			name: "NAT does not clash",
			peering: generatePeering("nat-does-not-clash", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-2"].Expose = []PeeringEntryExpose{
					{
						IPs: []PeeringEntryIP{
							{
								CIDR: "10.0.2.0/25",
							},
						},
						As: []PeeringEntryAs{
							{
								CIDR: "10.0.3.0/25",
							},
						},
						NAT: &PeeringNAT{
							Static: &PeeringNATStatic{},
						},
					},
				}
			}),
			objs: baseObjs,
		},
		{
			name: "missing NAT spec with non-empty AS",
			peering: generatePeering("missing-nat-spec", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-2"].Expose = []PeeringEntryExpose{
					{
						IPs: []PeeringEntryIP{
							{
								CIDR: "10.0.2.0/25",
							},
						},
						As: []PeeringEntryAs{
							{
								CIDR: "10.0.3.0/25",
							},
						},
					},
				}
			}),
			objs: baseObjs,
			err:  true,
		},
		{
			name: "default does not clash",
			peering: generatePeering("use-default", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose = []PeeringEntryExpose{
					{
						DefaultDestination: true,
					},
				}
			}),
			objs: baseObjs,
		},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme), "should add gateway API to scheme")
	require.NoError(t, vpcv1beta1.AddToScheme(scheme), "should add vpc API to scheme")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				Build()
			tt.peering.Default()
			actual := tt.peering.Validate(ctx, kube, nil)
			if tt.err {
				require.Error(t, actual)
			} else {
				require.NoError(t, actual)
			}
		})
	}
}

func TestValidateCIDRBelongsToVPC(t *testing.T) {
	gwGroup := withName(DefaultGatewayGroup, &GatewayGroup{})
	vpc1 := withName("vpc-1", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"sub1": {Subnet: "10.0.1.0/24"},
			},
		},
	})
	vpc2 := withName("vpc-2", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"sub1": {Subnet: "10.0.2.0/24"},
			},
		},
	})
	external := withName("out-1", &vpcv1beta1.External{
		Spec: vpcv1beta1.ExternalSpec{},
	})

	tests := []struct {
		name    string
		peering *GatewayPeering
		objs    []kclient.Object
		err     bool
	}{
		{
			name:    "exact subnet match",
			peering: generatePeering("exact-match"),
			objs:    []kclient.Object{gwGroup, vpc1, vpc2},
		},
		{
			name: "CIDR is sub-range of VPC subnet",
			peering: generatePeering("sub-range", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose[0].IPs[0].CIDR = "10.0.1.128/25"
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
		},
		{
			name: "CIDR not in any VPC subnet",
			peering: generatePeering("cidr-not-in-vpc", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose[0].IPs[0].CIDR = "192.168.1.0/24"
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
			err:  true,
		},
		{
			name: "CIDR is exact subnet match of another VPC",
			peering: generatePeering("match-wrong-vpc", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose[0].IPs[0].CIDR = "10.0.2.0/24"
				p.Spec.Peering["vpc-2"].Expose[0].IPs[0].CIDR = "10.0.1.0/24"
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
			err:  true,
		},
		{
			name:    "VPC not found",
			peering: generatePeering("vpc-not-found"),
			objs:    []kclient.Object{gwGroup, vpc1}, // vpc-2 absent
			err:     true,
		},
		{
			name: "default destination skipped",
			peering: generatePeering("default-destination", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose = []PeeringEntryExpose{
					{DefaultDestination: true},
				}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
		},
		{
			name: "VPCSubnet reference exists",
			peering: generatePeering("vpcsubnet-valid", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose[0].IPs[0] = PeeringEntryIP{VPCSubnet: "sub1"}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
		},
		{
			name: "VPCSubnet reference not found in VPC",
			peering: generatePeering("vpcsubnet-invalid", func(p *GatewayPeering) {
				p.Spec.Peering["vpc-1"].Expose[0].IPs[0] = PeeringEntryIP{VPCSubnet: "nonexistent"}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
			err:  true,
		},
		{
			name:    "external peering with default flag",
			peering: generateExternalPeering("ext-default"),
			objs:    []kclient.Object{gwGroup, vpc1, external},
		},
		{
			name: "external peering with IP CIDR",
			peering: generateExternalPeering("ext-ip", func(p *GatewayPeering) {
				expose := PeeringEntryExpose{
					DefaultDestination: false,
					IPs: []PeeringEntryIP{
						{
							CIDR: "1.0.0.0/8",
						},
					},
				}
				p.Spec.Peering["ext.out-1"].Expose[0] = expose
			}),
			objs: []kclient.Object{gwGroup, vpc1, external},
		},
		{
			name:    "external not found",
			peering: generateExternalPeering("ext-missing"),
			objs:    []kclient.Object{gwGroup, vpc1},
			err:     true,
		},
		{
			name: "external with VPCSubnet",
			peering: generateExternalPeering("ext-vpcsubnet", func(p *GatewayPeering) {
				expose := PeeringEntryExpose{
					DefaultDestination: false,
					IPs: []PeeringEntryIP{
						{
							VPCSubnet: "subnet-01",
						},
					},
				}
				p.Spec.Peering["ext.out-1"].Expose[0] = expose
			}),
			objs: []kclient.Object{gwGroup, vpc1, external},
			err:  true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))
	require.NoError(t, vpcv1beta1.AddToScheme(scheme))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				Build()
			tt.peering.Default()
			actual := tt.peering.Validate(ctx, kube, nil)
			if tt.err {
				require.Error(t, actual)
			} else {
				require.NoError(t, actual)
			}
		})
	}
}

func TestValidateACLNoKube(t *testing.T) {
	for _, tt := range []struct {
		name string
		acl  *PeeringACL
		err  bool
	}{
		{
			name: "nil ACL is valid",
			acl:  nil,
		},
		{
			name: "empty ACL defaults to deny-unless-exposed",
			acl:  &PeeringACL{},
		},
		{
			name: "explicit deny default",
			acl:  &PeeringACL{Default: ACLDefaultDeny},
		},
		{
			name: "invalid default action",
			acl:  &PeeringACL{Default: "bogus"},
			err:  true,
		},
		{
			name: "valid rule with from only",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{From: "vpc-1", Action: ACLActionAllow},
				},
			},
		},
		{
			name: "valid rule with to only",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{To: "vpc-2", Action: ACLActionDeny},
				},
			},
		},
		{
			name: "invalid action",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{From: "vpc-1", Action: "bogus"},
				},
			},
			err: true,
		},
		{
			name: "neither from nor to set",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{Action: ACLActionAllow},
				},
			},
			err: true,
		},
		{
			name: "from does not match either VPC",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{From: "vpc-3", Action: ACLActionAllow},
				},
			},
			err: true,
		},
		{
			name: "to does not match either VPC",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{To: "vpc-3", Action: ACLActionAllow},
				},
			},
			err: true,
		},
		{
			name: "valid rule name",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{Name: "web-traffic", From: "vpc-1", Action: ACLActionAllow},
				},
			},
		},
		{
			name: "invalid rule name characters",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{Name: "Web_Traffic!", From: "vpc-1", Action: ACLActionAllow},
				},
			},
			err: true,
		},
		{
			name: "rule name too long",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{Name: strings.Repeat("a", 65), From: "vpc-1", Action: ACLActionAllow},
				},
			},
			err: true,
		},
		{
			name: "source CIDR and VPCSubnet both set",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source: []PeeringACLMatchEndpoint{
								{CIDR: "10.0.1.0/24", VPCSubnet: "sub1"},
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "destination CIDR and VPCSubnet both set",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Destination: []PeeringACLMatchEndpoint{
								{CIDR: "10.0.2.0/24", VPCSubnet: "sub1"},
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "invalid source CIDR",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source: []PeeringACLMatchEndpoint{
								{CIDR: "not-a-cidr"},
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "invalid destination CIDR",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Destination: []PeeringACLMatchEndpoint{
								{CIDR: "not-a-cidr"},
							},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "valid source and destination CIDR with ports",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						To:     "vpc-2",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source:      []PeeringACLMatchEndpoint{{CIDR: "10.0.1.0/24", Ports: []string{"80", "443"}}},
							Destination: []PeeringACLMatchEndpoint{{CIDR: "10.0.2.0/24", Ports: []string{"8080-8090"}}},
						},
					},
				},
			},
		},
		{
			name: "invalid source port",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source: []PeeringACLMatchEndpoint{{CIDR: "10.0.1.0/24", Ports: []string{"not-a-port"}}},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "invalid destination port",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Destination: []PeeringACLMatchEndpoint{{CIDR: "10.0.2.0/24", Ports: []string{"not-a-port"}}},
						},
					},
				},
			},
			err: true,
		},
		{
			name: "valid string match protocol",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source:   []PeeringACLMatchEndpoint{{CIDR: "10.0.1.0/24"}},
							Protocol: ACLMatchProtocolTCP,
						},
					},
				},
			},
		},
		{
			name: "valid numeric match protocol",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source:   []PeeringACLMatchEndpoint{{CIDR: "10.0.1.0/24"}},
							Protocol: "42",
						},
					},
				},
			},
		},
		{
			name: "invalid string match protocol",
			acl: &PeeringACL{
				Rules: []PeeringACLRule{
					{
						From:   "vpc-1",
						Action: ACLActionAllow,
						Match: PeeringACLMatch{
							Source:   []PeeringACLMatchEndpoint{{CIDR: "10.0.1.0/24"}},
							Protocol: "not-a-protocol",
						},
					},
				},
			},
			err: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			peering := generatePeering("test-peering", func(p *GatewayPeering) {
				p.Spec.ACL = tt.acl
			})
			peering.Default()
			err := peering.Validate(t.Context(), nil, nil)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateACLVPCSubnet(t *testing.T) {
	gwGroup := withName(DefaultGatewayGroup, &GatewayGroup{})
	vpc1 := withName("vpc-1", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"sub1": {Subnet: "10.0.1.0/24"},
			},
		},
	})
	vpc2 := withName("vpc-2", &vpcv1beta1.VPC{
		Spec: vpcv1beta1.VPCSpec{
			Subnets: map[string]*vpcv1beta1.VPCSubnet{
				"sub1": {Subnet: "10.0.2.0/24"},
			},
		},
	})
	external := withName("out-1", &vpcv1beta1.External{
		Spec: vpcv1beta1.ExternalSpec{},
	})

	tests := []struct {
		name    string
		peering *GatewayPeering
		objs    []kclient.Object
		err     bool
	}{
		{
			name: "source VPCSubnet reference exists, from implicit",
			peering: generatePeering("acl-src-implicit-from", func(p *GatewayPeering) {
				p.Spec.ACL = &PeeringACL{
					Rules: []PeeringACLRule{
						{
							To:     "vpc-2",
							Action: ACLActionAllow,
							Match: PeeringACLMatch{
								Source: []PeeringACLMatchEndpoint{{VPCSubnet: "sub1"}},
							},
						},
					},
				}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
		},
		{
			name: "destination VPCSubnet reference exists, to implicit",
			peering: generatePeering("acl-dst-implicit-to", func(p *GatewayPeering) {
				p.Spec.ACL = &PeeringACL{
					Rules: []PeeringACLRule{
						{
							From:   "vpc-1",
							Action: ACLActionAllow,
							Match: PeeringACLMatch{
								Destination: []PeeringACLMatchEndpoint{{VPCSubnet: "sub1"}},
							},
						},
					},
				}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
		},
		{
			name: "source VPCSubnet reference not found in VPC",
			peering: generatePeering("acl-src-invalid-subnet", func(p *GatewayPeering) {
				p.Spec.ACL = &PeeringACL{
					Rules: []PeeringACLRule{
						{
							From:   "vpc-1",
							To:     "vpc-2",
							Action: ACLActionAllow,
							Match: PeeringACLMatch{
								Source: []PeeringACLMatchEndpoint{{VPCSubnet: "nonexistent"}},
							},
						},
					},
				}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
			err:  true,
		},
		{
			name: "destination VPCSubnet reference not found in VPC",
			peering: generatePeering("acl-dst-invalid-subnet", func(p *GatewayPeering) {
				p.Spec.ACL = &PeeringACL{
					Rules: []PeeringACLRule{
						{
							From:   "vpc-1",
							To:     "vpc-2",
							Action: ACLActionAllow,
							Match: PeeringACLMatch{
								Destination: []PeeringACLMatchEndpoint{{VPCSubnet: "nonexistent"}},
							},
						},
					},
				}
			}),
			objs: []kclient.Object{gwGroup, vpc1, vpc2},
			err:  true,
		},
		{
			name: "source VPCSubnet referencing an external peering entry",
			peering: generateExternalPeering("acl-src-external", func(p *GatewayPeering) {
				p.Spec.ACL = &PeeringACL{
					Rules: []PeeringACLRule{
						{
							From:   "ext.out-1",
							To:     "vpc-1",
							Action: ACLActionAllow,
							Match: PeeringACLMatch{
								Source: []PeeringACLMatchEndpoint{{VPCSubnet: "sub1"}},
							},
						},
					},
				}
			}),
			objs: []kclient.Object{gwGroup, vpc1, external},
			err:  true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, AddToScheme(scheme))
	require.NoError(t, vpcv1beta1.AddToScheme(scheme))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				Build()
			tt.peering.Default()
			actual := tt.peering.Validate(ctx, kube, nil)
			if tt.err {
				require.Error(t, actual)
			} else {
				require.NoError(t, actual)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	for _, tt := range []struct {
		in    string
		error bool
	}{
		{in: "", error: true},
		{in: "80", error: false},
		{in: "80-80", error: false},
		{in: "80,443", error: true},
		{in: "80,443,3000-3100", error: true},
		{in: "80,443,3000-3100,", error: true},
		{in: "80,443,3000-3100,8080", error: true},
		{in: "  80  ", error: true},
		{in: "  80  ,  443  ", error: true},
		{in: "  80  ,  443  ,  3000-3100  ", error: true},
		{in: "  80  ,443,3000-3100,8080", error: true},
		{in: "80-79", error: true},
		{in: "0", error: true},
		{in: "65536", error: true},
		{in: "1-65536", error: true},
		{in: "0-80", error: true},
		{in: "-80", error: true},
		{in: "80-", error: true},
		{in: "  -  80  ", error: true},
		{in: "  80  -  ", error: true},
		{in: "1-80,65536", error: true},
	} {
		t.Run("_"+strings.ReplaceAll(tt.in, " ", "_"), func(t *testing.T) {
			err := validatePort(tt.in)
			require.Equal(t, tt.error, err != nil)
		})
	}
}
