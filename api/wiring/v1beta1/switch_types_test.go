// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
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
		GatewayASN:          65201,
		ManagementSubnet:    "172.30.0.0/21",
		ManagementDHCPStart: "172.30.4.0",
		ManagementDHCPEnd:   "172.30.7.254",
		EnableGateway:       true,
	}

	backendFabric := &wiringapi.Fabric{
		ObjectMeta: kmetav1.ObjectMeta{Name: "backend", Namespace: "default"},
		Spec: wiringapi.FabricSpec{
			LeafASNStart: 64101,
			LeafASNEnd:   64199,
			Domains:      map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: 64100, GatewayASN: 64200}},
		},
	}
	inBackend := func(sw *wiringapi.Switch) *wiringapi.Switch {
		sw.Spec.Topology.Fabric = "backend"

		return sw
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
			name:        "leafInOtherFabric",
			objects:     []kclient.Object{backendFabric},
			dut:         inBackend(getLeaf("leaf-backend", 64150, "172.30.0.8/21")),
			expectError: false,
		},
		{
			// within the default fabric's range, which does not apply to the backend fabric
			name:        "leafInOtherFabricOutOfRange",
			objects:     []kclient.Object{backendFabric},
			dut:         inBackend(getLeaf("leaf-backend", 65101, "172.30.0.8/21")),
			expectError: true,
		},
		{
			name:        "leafFabricNotFound",
			objects:     []kclient.Object{},
			dut:         inBackend(getLeaf("leaf-backend", 64150, "172.30.0.8/21")),
			expectError: true,
		},
		{
			name:        "spineInOtherFabric",
			objects:     []kclient.Object{backendFabric},
			dut:         inBackend(getSpine("spine-backend", 64100)),
			expectError: false,
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

func TestSwitchDefaultPortLocators(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour).Format(time.DateTime)

	for _, test := range []struct {
		name string
		in   map[string]string
		want []string // expected port names, values are checked to be within the allowed range
	}{
		{
			name: "perPort",
			in:   map[string]string{"E1/1": "10m", "E1/2": ""},
			want: []string{"E1/1", "E1/2"},
		},
		{
			name: "allPortsSupersedesPerPort",
			in:   map[string]string{"*": "10m", "E1/1": "10m", "E1/2": ""},
			want: []string{"*"},
		},
		{
			name: "allPortsExpired",
			in:   map[string]string{"*": past, "E1/1": "10m"},
			want: []string{},
		},
		{
			name: "expiredPerPortDropped",
			in:   map[string]string{"E1/1": past, "E1/2": "10m"},
			want: []string{"E1/2"},
		},
		{
			name: "unparseableKept",
			in:   map[string]string{"E1/1": "whenever"},
			want: []string{"E1/1"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sw := &wiringapi.Switch{
				ObjectMeta: kmetav1.ObjectMeta{Name: "leaf1", Namespace: "default"},
				Spec:       wiringapi.SwitchSpec{PortLocators: test.in},
			}
			sw.Default()

			require.ElementsMatch(t, test.want, lo.Keys(sw.Spec.PortLocators))

			for name, value := range sw.Spec.PortLocators {
				if test.in[name] == "whenever" {
					require.Equal(t, "whenever", value, "unparseable value should be left as-is")

					continue
				}

				expire, err := time.ParseInLocation(time.DateTime, value, time.UTC)
				require.NoError(t, err, "defaulted value should be an exact time")
				require.WithinRange(t, expire, now, now.Add(wiringapi.PortLocatorMaxExpire+5*time.Second))
			}
		})
	}
}

func TestSwitchDomainsValidation(t *testing.T) {
	const otherFabric = "plane-a-fabric"
	swGen := func(f ...func(sw *wiringapi.Switch)) *wiringapi.Switch {
		sw := withName("leaf-01", &wiringapi.Switch{
			Spec: wiringapi.SwitchSpec{
				Role:       wiringapi.SwitchRoleServerLeaf,
				Profile:    switchprofile.DellS5232FON.Name,
				ASN:        65101,
				IP:         "172.30.1.1/21",
				ProtocolIP: "172.30.11.1/32",
				VTEPIP:     "172.30.12.1/32",
			},
		})
		for _, fn := range f {
			fn(sw)
		}
		sw.Default()

		return sw
	}
	spine := func(asn uint32) func(sw *wiringapi.Switch) {
		return func(sw *wiringapi.Switch) {
			sw.Spec.Role = wiringapi.SwitchRoleSpine
			sw.Spec.ASN = asn
			sw.Spec.VTEPIP = ""
		}
	}
	// a second leaf, with addresses of its own
	leaf2 := func(sw *wiringapi.Switch) {
		sw.Name = "leaf-02"
		sw.Spec.ASN = 65102
		sw.Spec.IP = "172.30.1.2/21"
		sw.Spec.ProtocolIP = "172.30.11.2/32"
		sw.Spec.VTEPIP = "172.30.12.2/32"
	}
	domains := func(domains ...string) func(sw *wiringapi.Switch) {
		return func(sw *wiringapi.Switch) { sw.Spec.Topology.Domains = domains }
	}
	eslag := func(sw *wiringapi.Switch) {
		sw.Spec.Redundancy = wiringapi.SwitchRedundancy{Group: "eslag-1", Type: meta.RedundancyTypeESLAG}
	}

	base := []kclient.Object{
		vlanNSGen("default", []meta.VLANRange{{From: 1000, To: 2999}}),
		withName("eslag-1", &wiringapi.SwitchGroup{}),
		withName("default", &wiringapi.Fabric{Spec: wiringapi.FabricSpec{LeafASNStart: 65101, LeafASNEnd: 65200, Domains: map[string]wiringapi.FabricDomainSpec{
			"default": {SpineASN: 65100, GatewayASN: 65534},
			"plane-b": {SpineASN: 65099, GatewayASN: 65535},
		}}}),
		withName(otherFabric, &wiringapi.Fabric{Spec: wiringapi.FabricSpec{LeafASNStart: 65101, LeafASNEnd: 65200, Domains: map[string]wiringapi.FabricDomainSpec{
			"plane-a": {SpineASN: 64100, GatewayASN: 64201},
		}}}),
	}

	for _, tt := range []struct {
		name    string
		sw      *wiringapi.Switch
		objects []kclient.Object
		err     string
	}{
		{name: "defaulted", sw: swGen()},
		{name: "shared leaf", sw: swGen(domains("plane-b", "default"))},
		{name: "spine", sw: swGen(spine(65099), domains("plane-b"))},
		{
			name: "spine with the ASN of another domain", err: "spine leaf-01 ASN 65100 is not the spine ASN 65099 of domain plane-b",
			sw: swGen(spine(65100), domains("plane-b")),
		},
		{
			name: "spine in two domains", err: "spine must be in exactly one domain",
			sw: swGen(spine(65099), domains("plane-b", "default")),
		},
		{
			name: "domain not in fabric", err: "domain plane-c not found in fabric default",
			sw: swGen(domains("plane-c")),
		},
		{
			name: "duplicate domain", err: "domains must be unique",
			sw: swGen(domains("default", "default")),
		},
		{
			name: "defaulted in a fabric without a default domain", err: "domain default not found in fabric plane-a-fabric",
			sw: swGen(func(sw *wiringapi.Switch) { sw.Spec.Topology.Fabric = otherFabric }),
		},
		{
			name: "named domain in another fabric",
			sw: swGen(domains("plane-a"), func(sw *wiringapi.Switch) {
				sw.Spec.Topology.Fabric = otherFabric
			}),
		},
		{
			name:    "redundancy group with the same domains",
			sw:      swGen(eslag, domains("default", "plane-b")),
			objects: []kclient.Object{swGen(leaf2, eslag, domains("plane-b", "default"))},
		},
		{
			name:    "redundancy group sharing a domain, joining another one at a time",
			sw:      swGen(eslag, domains("default", "plane-b")),
			objects: []kclient.Object{swGen(leaf2, eslag)},
		},
		{
			name:    "redundancy group sharing no domain",
			sw:      swGen(eslag, domains("plane-b")),
			objects: []kclient.Object{swGen(leaf2, eslag)},
			err:     "switch shares no domain with the other switches of redundancy group eslag-1, switch leaf-02 is in domains [default]",
		},
		{
			name: "redundancy group member written before domains existed",
			sw:   swGen(eslag),
			objects: []kclient.Object{func() *wiringapi.Switch {
				sw := swGen(leaf2, eslag)
				sw.Spec.Topology.Domains = nil

				return sw
			}()},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &meta.FabricConfig{
				ControlVIP:          "172.30.0.1/32",
				ProtocolSubnet:      "172.30.8.0/21",
				VTEPSubnet:          "172.30.12.0/22",
				ManagementSubnet:    "172.30.0.0/21",
				ManagementDHCPStart: "172.30.4.0",
			}
			scheme := runtime.NewScheme()
			require.NoError(t, wiringapi.AddToScheme(scheme))
			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(withObjs(base, tt.objects...)...).Build()
			profiles := switchprofile.NewDefaultSwitchProfiles()
			require.NoError(t, profiles.RegisterAll(t.Context(), kube, cfg))
			require.NoError(t, profiles.Enforce(t.Context(), kube, cfg, false))

			_, err := tt.sw.Validate(t.Context(), kube, cfg)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestSwitchDefaultDomains(t *testing.T) {
	sw := &wiringapi.Switch{}
	sw.Default()
	require.Equal(t, []string{wiringapi.DefaultFabricDomain}, sw.Spec.Topology.Domains)
	require.Contains(t, sw.Labels, wiringapi.ListLabelDomain(wiringapi.DefaultFabricDomain))

	sw = &wiringapi.Switch{Spec: wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Domains: []string{"plane-b", "plane-a"}}}}
	sw.Default()
	require.Equal(t, []string{"plane-a", "plane-b"}, sw.Spec.Topology.Domains)
}
