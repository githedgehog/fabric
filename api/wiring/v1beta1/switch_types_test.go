// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHydrationValidation(t *testing.T) {
	ctx := t.Context()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	leafSwitch := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      "leaf1",
			Namespace: "default",
		},
		Spec: wiringapi.SwitchSpec{
			Role:       wiringapi.SwitchRoleServerLeaf,
			Redundancy: wiringapi.SwitchRedundancy{},
			ASN:        65101,
			IP:         "172.30.0.8/21",
			VTEPIP:     "172.30.12.0/32",
			ProtocolIP: "172.30.8.2/32",
		},
	}
	spineSwitch := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      "spine1",
			Namespace: "default",
		},
		Spec: wiringapi.SwitchSpec{
			Role:       wiringapi.SwitchRoleSpine,
			Redundancy: wiringapi.SwitchRedundancy{},
			ASN:        65100,
			IP:         "172.30.0.8/21",
			VTEPIP:     "172.30.12.0/32",
			ProtocolIP: "172.30.8.2/32",
		},
	}
	mclagSwitch := &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{
			Name:      "leaf1",
			Namespace: "default",
		},
		Spec: wiringapi.SwitchSpec{
			Role: wiringapi.SwitchRoleServerLeaf,
			Redundancy: wiringapi.SwitchRedundancy{
				Type:  meta.RedundancyTypeMCLAG,
				Group: "mclag-1",
			},
			ASN:        65101,
			IP:         "172.30.0.8/21",
			VTEPIP:     "172.30.12.0/32",
			ProtocolIP: "172.30.8.2/32",
		},
	}

	fabricCfg := &meta.FabricConfig{
		ControlVIP:     "172.30.0.1/32",
		ProtocolSubnet: "172.30.8.0/22",
		VTEPSubnet:     "172.30.12.0/22",
	}

	for _, test := range []struct {
		name        string
		objects     []kclient.Object
		dut         *wiringapi.Switch
		expectError bool
	}{
		{
			name:        "emptyList",
			objects:     []kclient.Object{},
			dut:         leafSwitch,
			expectError: false,
		},
		{
			name: "VTEPCollision",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65102,
						IP:         "172.30.0.5/21",
						VTEPIP:     "172.30.12.0/32",
						ProtocolIP: "172.30.8.5/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: true,
		},
		{
			name: "IPCollision",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65102,
						IP:         "172.30.0.8/21",
						VTEPIP:     "172.30.12.2/32",
						ProtocolIP: "172.30.8.5/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: true,
		},
		{
			name: "ProtocolIPCollision",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65102,
						IP:         "172.30.0.5/21",
						VTEPIP:     "172.30.12.2/32",
						ProtocolIP: "172.30.8.2/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: true,
		},
		{
			name: "ASNCollision",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65101,
						IP:         "172.30.0.5/21",
						VTEPIP:     "172.30.12.2/32",
						ProtocolIP: "172.30.8.5/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: true,
		},
		{
			name: "noCollision",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65102,
						IP:         "172.30.0.5/21",
						VTEPIP:     "172.30.12.2/32",
						ProtocolIP: "172.30.8.5/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: false,
		},
		{
			name: "spineSameASN",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "spine2",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleSpine,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65100,
						IP:         "172.30.0.9/21",
						VTEPIP:     "172.30.12.1/32",
						ProtocolIP: "172.30.8.3/32",
					},
				},
			},
			dut:         spineSwitch,
			expectError: false,
		},
		{
			name: "spineDifferentASN",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "spine2",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role:       wiringapi.SwitchRoleSpine,
						Redundancy: wiringapi.SwitchRedundancy{},
						ASN:        65105,
						IP:         "172.30.0.9/21",
						VTEPIP:     "172.30.12.1/32",
						ProtocolIP: "172.30.8.3/32",
					},
				},
			},
			dut:         spineSwitch,
			expectError: true,
		},
		{
			name:        "mclagPeerAbsent",
			objects:     []kclient.Object{},
			dut:         mclagSwitch,
			expectError: false,
		},
		{
			name: "mclagPeerDifferentASN",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role: wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{
							Type:  meta.RedundancyTypeMCLAG,
							Group: "mclag-1",
						},
						ASN:        65102,
						IP:         "172.30.0.9/21",
						VTEPIP:     "172.30.12.0/32",
						ProtocolIP: "172.30.8.3/32",
					}},
			},
			dut:         mclagSwitch,
			expectError: true,
		},
		{
			name: "mclagPeerDifferentVTEP",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role: wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{
							Type:  meta.RedundancyTypeMCLAG,
							Group: "mclag-1",
						},
						ASN:        65101,
						IP:         "172.30.0.9/21",
						VTEPIP:     "172.30.12.3/32",
						ProtocolIP: "172.30.8.3/32",
					}},
			},
			dut:         mclagSwitch,
			expectError: true,
		},
		{
			name: "mclagPeerAllGood",
			objects: []kclient.Object{
				&wiringapi.Switch{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "leaf5",
						Namespace: "default",
					},
					Spec: wiringapi.SwitchSpec{
						Role: wiringapi.SwitchRoleServerLeaf,
						Redundancy: wiringapi.SwitchRedundancy{
							Type:  meta.RedundancyTypeMCLAG,
							Group: "mclag-1",
						},
						ASN:        65101,
						IP:         "172.30.0.9/21",
						VTEPIP:     "172.30.12.0/32",
						ProtocolIP: "172.30.8.3/32",
					}},
			},
			dut:         mclagSwitch,
			expectError: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(test.objects...).
				Build()

			err := test.dut.HydrationValidation(ctx, kube, fabricCfg)
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
