// Copyright 2023 Hedgehog
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
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	AltIP40 string = "172.30.128.40/31"
	AltIP41 string = "172.30.128.41/31"
)

func withObjs(base []kclient.Object, objs ...kclient.Object) []kclient.Object {
	return append(slices.Clone(base), objs...)
}

func fabricConnGen(name string, f ...func(conn *wiringapi.Connection)) *wiringapi.Connection {
	conn := withName(name, &wiringapi.Connection{
		Spec: wiringapi.ConnectionSpec{
			Fabric: &wiringapi.ConnFabric{
				Links: []wiringapi.FabricLink{
					{
						Spine: wiringapi.ConnFabricLinkSwitch{
							BasePortName: wiringapi.BasePortName{
								Port: "spine-01/E1/1",
							},
							IP: "172.30.128.0/31",
						},
						Leaf: wiringapi.ConnFabricLinkSwitch{
							BasePortName: wiringapi.BasePortName{
								Port: "leaf-01/E1/1",
							},
							IP: "172.30.128.1/31",
						},
					},
				},
			},
		},
	})

	for _, fn := range f {
		fn(conn)
	}

	return conn
}

func meshConnGen(name string, f ...func(conn *wiringapi.Connection)) *wiringapi.Connection {
	conn := withName(name, &wiringapi.Connection{
		Spec: wiringapi.ConnectionSpec{
			Mesh: &wiringapi.ConnMesh{
				Links: []wiringapi.MeshLink{
					{
						Leaf1: wiringapi.ConnFabricLinkSwitch{
							BasePortName: wiringapi.BasePortName{
								Port: "leaf-01/E1/1",
							},
							IP: "172.30.128.0/31",
						},
						Leaf2: wiringapi.ConnFabricLinkSwitch{
							BasePortName: wiringapi.BasePortName{
								Port: "leaf-02/E1/1",
							},
							IP: "172.30.128.1/31",
						},
					},
				},
			},
		},
	})

	for _, fn := range f {
		fn(conn)
	}

	return conn
}

func gwConnGen(name string, f ...func(conn *wiringapi.Connection)) *wiringapi.Connection {
	conn := withName(name, &wiringapi.Connection{
		Spec: wiringapi.ConnectionSpec{
			Gateway: &wiringapi.ConnGateway{
				Links: []wiringapi.GatewayLink{
					{
						Switch: wiringapi.ConnFabricLinkSwitch{
							BasePortName: wiringapi.BasePortName{
								Port: "spine-01/E1/1",
							},
							IP: "172.30.128.0/31",
						},
						Gateway: wiringapi.ConnGatewayLinkGateway{
							BasePortName: wiringapi.BasePortName{
								Port: "gateway-1/enp2s1",
							},
							IP: "172.30.128.1/31",
						},
					},
				},
			},
		},
	})

	for _, fn := range f {
		fn(conn)
	}

	return conn
}

func TestConnectionValidation(t *testing.T) {
	base := []kclient.Object{
		withName("spine-01",
			&wiringapi.Switch{
				Spec: wiringapi.SwitchSpec{
					Role:    wiringapi.SwitchRoleSpine,
					ASN:     65100,
					Profile: switchprofile.DellS5232FON.Name,
				},
			}),
		withName("leaf-01",
			&wiringapi.Switch{
				Spec: wiringapi.SwitchSpec{
					Role:    wiringapi.SwitchRoleServerLeaf,
					ASN:     65101,
					Profile: switchprofile.DellS5232FON.Name,
				},
			}),
		withName("leaf-02",
			&wiringapi.Switch{
				Spec: wiringapi.SwitchSpec{
					Role:    wiringapi.SwitchRoleServerLeaf,
					ASN:     65102,
					Profile: switchprofile.DellS5232FON.Name,
				},
			}),
	}

	for _, tt := range []struct {
		name       string
		conn       *wiringapi.Connection
		withClient bool
		objects    []kclient.Object
		err        bool
	}{
		{
			name: "static-ext-default-route",
			conn: withName("static-ext-default-route", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"0.0.0.0/0",
								},
							},
						},
					},
				},
			}),
		},
		{
			name: "static-ext-default-route-and-not",
			conn: withName("static-ext-default-route-and-not", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"0.0.0.0/0",
									"0.0.0.0/0",
								},
							},
						},
					},
				},
			}),
			err: true,
		},
		{
			name: "static-ext-default-route-and-not-2",
			conn: withName("static-ext-default-route-and-not-2", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"0.0.0.0/0",
									"1.2.3.0/24",
								},
							},
						},
					},
				},
			}),
			err: true,
		},
		{
			name: "static-ext--not-default-route",
			conn: withName("static-ext--not-default-route", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"1.2.3.0/24",
								},
							},
						},
					},
				},
			}),
		},
		{
			name: "static-ext-overlap-route",
			conn: withName("static-ext-overlap-route", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"172.30.1.0/24",
								},
							},
						},
					},
				},
			}),
			err: true,
		},
		{
			name: "static-ext-overlap-route-2",
			conn: withName("static-ext-overlap-route-2", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"172.30.0.0/16",
								},
							},
						},
					},
				},
			}),
			err: true,
		},
		{
			name: "static-ext-overlap-route-3",
			conn: withName("static-ext-overlap-route-3", &wiringapi.Connection{
				Spec: wiringapi.ConnectionSpec{
					StaticExternal: &wiringapi.ConnStaticExternal{
						Link: wiringapi.ConnStaticExternalLink{
							Switch: wiringapi.ConnStaticExternalLinkSwitch{
								BasePortName: wiringapi.BasePortName{
									Port: "some/some",
								},
								IP:      "192.168.1.2/24",
								NextHop: "192.168.1.1",
								Subnets: []string{
									"172.30.1.42/32",
								},
							},
						},
					},
				},
			}),
			err: true,
		},
		{
			name:       "collision-fabric-with-fabric-IP",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Fabric.Links[0].Spine.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-fabric-with-fabric-port",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.IP = AltIP41
				conn.Spec.Fabric.Links[0].Spine.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-fabric-with-fabric",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Fabric.Links[0].Spine.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
				conn.Spec.Fabric.Links[0].Leaf.IP = AltIP41
				conn.Spec.Fabric.Links[0].Spine.IP = AltIP40
			})),
		},
		{
			name:       "collision-fabric-with-mesh-IP",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Mesh.Links[0].Leaf2.BasePortName = wiringapi.NewBasePortName("leaf-02/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-fabric-with-mesh-port",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.IP = AltIP41
				conn.Spec.Mesh.Links[0].Leaf2.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-fabric-with-mesh",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Mesh.Links[0].Leaf2.BasePortName = wiringapi.NewBasePortName("leaf-02/E1/3")
				conn.Spec.Mesh.Links[0].Leaf1.IP = AltIP41
				conn.Spec.Mesh.Links[0].Leaf2.IP = AltIP40
			})),
		},
		{
			name:       "collision-fabric-with-gateway-IP",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.BasePortName = wiringapi.NewBasePortName("gateway-1/enp2s2")
				conn.Spec.Gateway.Links[0].Switch.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-fabric-with-gateway-port",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.IP = AltIP41
				conn.Spec.Gateway.Links[0].Switch.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-fabric-with-gateway",
			conn:       fabricConnGen("fabric-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.BasePortName = wiringapi.NewBasePortName("gateway-1/enp2s2")
				conn.Spec.Gateway.Links[0].Switch.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
				conn.Spec.Gateway.Links[0].Gateway.IP = AltIP41
				conn.Spec.Gateway.Links[0].Switch.IP = AltIP40
			})),
		},
		{
			name:       "collision-mesh-with-fabric-IP",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Fabric.Links[0].Spine.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-mesh-with-fabric-port",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.IP = AltIP41
				conn.Spec.Fabric.Links[0].Spine.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-mesh-with-fabric",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Fabric.Links[0].Spine.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
				conn.Spec.Fabric.Links[0].Leaf.IP = AltIP41
				conn.Spec.Fabric.Links[0].Spine.IP = AltIP40
			})),
		},
		{
			name:       "collision-mesh-with-mesh-IP",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Mesh.Links[0].Leaf2.BasePortName = wiringapi.NewBasePortName("leaf-02/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-mesh-with-mesh-port",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.IP = AltIP41
				conn.Spec.Mesh.Links[0].Leaf2.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-mesh-with-mesh",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Mesh.Links[0].Leaf2.BasePortName = wiringapi.NewBasePortName("leaf-02/E1/3")
				conn.Spec.Mesh.Links[0].Leaf1.IP = AltIP41
				conn.Spec.Mesh.Links[0].Leaf2.IP = AltIP40
			})),
		},
		{
			name:       "collision-mesh-with-gateway-IP",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects:    withObjs(base, gwConnGen("gw-2")), // mesh and gw conns have no ports in common
			err:        true,
		},
		{
			name:       "no-collision-mesh-with-gateway",
			conn:       meshConnGen("mesh-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.IP = AltIP41
				conn.Spec.Gateway.Links[0].Switch.IP = AltIP40
			})),
		},
		{
			name:       "collision-gateway-with-fabric-IP",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Fabric.Links[0].Spine.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-gateway-with-fabric-port",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.IP = AltIP41
				conn.Spec.Fabric.Links[0].Spine.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-gateway-with-fabric",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, fabricConnGen("fabric-2", func(conn *wiringapi.Connection) {
				conn.Spec.Fabric.Links[0].Leaf.BasePortName = wiringapi.NewBasePortName("leaf-01/E1/2")
				conn.Spec.Fabric.Links[0].Spine.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
				conn.Spec.Fabric.Links[0].Leaf.IP = AltIP41
				conn.Spec.Fabric.Links[0].Spine.IP = AltIP40
			})),
		},
		{
			name:       "collision-gateway-with-mesh-IP",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects:    withObjs(base, meshConnGen("mesh-2")), // mesh and gw conns have no ports in common
			err:        true,
		},
		{
			name:       "no-collision-gateway-with-mesh",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, meshConnGen("mesh-2", func(conn *wiringapi.Connection) {
				conn.Spec.Mesh.Links[0].Leaf1.IP = AltIP41
				conn.Spec.Mesh.Links[0].Leaf2.IP = AltIP40
			})),
		},
		{
			name:       "collision-gateway-with-gateway-IP",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.BasePortName = wiringapi.NewBasePortName("gateway-1/enp2s2")
				conn.Spec.Gateway.Links[0].Switch.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
			})),
			err: true,
		},
		{
			name:       "collision-gateway-with-gateway-port",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.IP = AltIP41
				conn.Spec.Gateway.Links[0].Switch.IP = AltIP40
			})),
			err: true,
		},
		{
			name:       "no-collision-gateway-with-gateway",
			conn:       gwConnGen("gw-1"),
			withClient: true,
			objects: withObjs(base, gwConnGen("gw-2", func(conn *wiringapi.Connection) {
				conn.Spec.Gateway.Links[0].Gateway.BasePortName = wiringapi.NewBasePortName("gateway-1/enp2s2")
				conn.Spec.Gateway.Links[0].Switch.BasePortName = wiringapi.NewBasePortName("spine-01/E1/3")
				conn.Spec.Gateway.Links[0].Gateway.IP = AltIP41
				conn.Spec.Gateway.Links[0].Switch.IP = AltIP40
			})),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &meta.FabricConfig{
				ReservedSubnets: []string{"172.30.1.0/24"},
				FabricSubnet:    "172.30.128.0/24",
				FabricMode:      meta.FabricModeSpineLeaf,
			}
			err := cfg.WithReservedSubnets()
			require.NoError(t, err)
			ctx := t.Context()

			var kube kclient.Client
			if tt.withClient {
				scheme := runtime.NewScheme()
				require.NoError(t, wiringapi.AddToScheme(scheme))
				kube = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(tt.objects...).
					Build()
				profiles := switchprofile.NewDefaultSwitchProfiles()
				require.NoError(t, profiles.RegisterAll(ctx, kube, cfg))
				require.NoError(t, profiles.Enforce(ctx, kube, cfg, false))
			}

			_, err = tt.conn.Validate(ctx, kube, cfg)

			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
