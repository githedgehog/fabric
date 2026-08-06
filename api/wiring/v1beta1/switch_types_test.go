// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
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
	require.NoError(t, gwapi.AddToScheme(scheme))
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
	getLeaf := func(name string, asn uint32, ip string) *wiringapi.Switch {
		leaf := leafSwitch.DeepCopy()
		leaf.Name = name
		leaf.Spec.ASN = asn
		leaf.Spec.IP = ip

		return leaf
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
	getSpine := func(name string, asn uint32) *wiringapi.Switch {
		spine := spineSwitch.DeepCopy()
		spine.Name = name
		spine.Spec.ASN = asn

		return spine
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
		ControlVIP:          "172.30.0.1/32",
		ProtocolSubnet:      "172.30.8.0/22",
		VTEPSubnet:          "172.30.12.0/22",
		SpineASN:            65100,
		LeafASNStart:        65101,
		LeafASNEnd:          65200,
		ManagementSubnet:    "172.30.0.0/21",
		ManagementDHCPStart: "172.30.4.0",
		ManagementDHCPEnd:   "172.30.7.254",
		EnableGateway:       true,
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
			name:        "leafASNOutOfRange",
			objects:     []kclient.Object{},
			dut:         getLeaf("leaf-out-of-range", 65000, "172.30.0.8/21"),
			expectError: true,
		},
		{
			name:        "mgmtIPOutOfRange",
			objects:     []kclient.Object{},
			dut:         getLeaf("leaf-mgmt-out-of-range", 65101, "172.29.240.33/21"),
			expectError: true,
		},
		{
			name:        "mgmtIPInDHCPRange",
			objects:     []kclient.Object{},
			dut:         getLeaf("leaf-mgmt-in-dhcp-range", 65101, "172.30.5.123/21"),
			expectError: true,
		},
		{
			name: "spineCorrectASN",
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
			name:        "spineWrongASN",
			objects:     []kclient.Object{},
			dut:         getSpine("spine-wrong-asn", 65101),
			expectError: true,
		},
		{
			name: "VTEPCollisionWithGateway",
			objects: []kclient.Object{
				&gwapi.Gateway{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "default",
					},
					Spec: gwapi.GatewaySpec{
						ProtocolIP: "172.30.8.45/32",
						VTEPIP:     "172.30.12.0/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: true,
		},
		{
			name: "ProtocolIPCollisionWithGateway",
			objects: []kclient.Object{
				&gwapi.Gateway{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "default",
					},
					Spec: gwapi.GatewaySpec{
						ProtocolIP: "172.30.8.2/32",
						VTEPIP:     "172.30.12.45/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: true,
		},
		{
			name: "noCollisionWithGateway",
			objects: []kclient.Object{
				&gwapi.Gateway{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "default",
					},
					Spec: gwapi.GatewaySpec{
						ProtocolIP: "172.30.8.45/32",
						VTEPIP:     "172.30.12.45/32",
					},
				},
			},
			dut:         leafSwitch,
			expectError: false,
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

func TestNormalizePortLocator(t *testing.T) {
	now := time.Now().UTC()

	for _, test := range []struct {
		name        string
		in          string
		expectError bool
		// expectExpire is the expected expire time as an offset from now, ignored if expectExpired is true
		expectExpire  time.Duration
		expectExpired bool
	}{
		{name: "empty", in: "", expectExpire: wiringapi.PortLocatorDefaultExpire},
		{name: "duration", in: "10m", expectExpire: 10 * time.Minute},
		{name: "durationSeconds", in: "90s", expectExpire: 90 * time.Second},
		{name: "durationZero", in: "0s", expectExpired: true},
		{name: "durationTooLong", in: "1h", expectExpire: wiringapi.PortLocatorMaxExpire},
		{name: "durationExactlyMax", in: "20m", expectExpire: wiringapi.PortLocatorMaxExpire},
		{name: "durationNegative", in: "-1m", expectExpired: true},
		{name: "time", in: now.Add(7 * time.Minute).Format(time.DateTime), expectExpire: 7 * time.Minute},
		{name: "timeTooLate", in: now.Add(3 * time.Hour).Format(time.DateTime), expectExpire: wiringapi.PortLocatorMaxExpire},
		{name: "timeInPast", in: now.Add(-7 * time.Minute).Format(time.DateTime), expectExpired: true},
		{name: "garbage", in: "whenever", expectError: true},
		{name: "timeWrongFormat", in: now.Format(time.RFC3339), expectError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ok, expire, err := wiringapi.NormalizePortLocator(test.in)
			if test.expectError {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)

			if test.expectExpired {
				require.False(t, ok)
				require.Empty(t, expire)

				return
			}
			require.True(t, ok)

			parsed, err := time.ParseInLocation(time.DateTime, expire, time.UTC)
			require.NoError(t, err)
			require.WithinDuration(t, now.Add(test.expectExpire), parsed, 5*time.Second)
		})
	}
}
