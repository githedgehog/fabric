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
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConnectionValidation(t *testing.T) {
	fabricSpec := &wiringapi.ConnectionSpec{
		Fabric: &wiringapi.ConnFabric{
			Links: []wiringapi.FabricLink{
				{
					Spine: wiringapi.ConnFabricLinkSwitch{
						BasePortName: wiringapi.BasePortName{
							Port: "some/fabric1",
						},
						IP: "172.30.128.0/31",
					},
					Leaf: wiringapi.ConnFabricLinkSwitch{
						BasePortName: wiringapi.BasePortName{
							Port: "some/fabric2",
						},
						IP: "172.30.128.1/31",
					},
				},
			},
		},
	}
	meshSpec := &wiringapi.ConnectionSpec{
		Mesh: &wiringapi.ConnMesh{
			Links: []wiringapi.MeshLink{
				{
					Leaf1: wiringapi.ConnFabricLinkSwitch{
						BasePortName: wiringapi.BasePortName{
							Port: "some/mesh1",
						},
						IP: "172.30.128.0/31",
					},
					Leaf2: wiringapi.ConnFabricLinkSwitch{
						BasePortName: wiringapi.BasePortName{
							Port: "some/mesh2",
						},
						IP: "172.30.128.1/31",
					},
				},
			},
		},
	}
	gatewaySpec := &wiringapi.ConnectionSpec{
		Gateway: &wiringapi.ConnGateway{
			Links: []wiringapi.GatewayLink{
				{
					Switch: wiringapi.ConnFabricLinkSwitch{
						BasePortName: wiringapi.BasePortName{
							Port: "some/gw1",
						},
						IP: "172.30.128.0/31",
					},
					Gateway: wiringapi.ConnGatewayLinkGateway{
						BasePortName: wiringapi.BasePortName{
							Port: "some/gw2",
						},
						IP: "172.30.128.1/31",
					},
				},
			},
		},
	}

	for _, tt := range []struct {
		name       string
		conn       *wiringapi.ConnectionSpec
		withClient bool
		objects    []kclient.Object
		err        bool
	}{
		{
			name: "static-ext-default-route",
			conn: &wiringapi.ConnectionSpec{
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
		},
		{
			name: "static-ext-default-route-and-not",
			conn: &wiringapi.ConnectionSpec{
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
			err: true,
		},
		{
			name: "static-ext-default-route-and-not-2",
			conn: &wiringapi.ConnectionSpec{
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
			err: true,
		},
		{
			name: "static-ext--not-default-route",
			conn: &wiringapi.ConnectionSpec{
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
		},
		{
			name: "static-ext-overlap-route",
			conn: &wiringapi.ConnectionSpec{
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
			err: true,
		},
		{
			name: "static-ext-overlap-route-2",
			conn: &wiringapi.ConnectionSpec{
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
			err: true,
		},
		{
			name: "static-ext-overlap-route-3",
			conn: &wiringapi.ConnectionSpec{
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
			err: true,
		},
		{
			name:       "collision-fabric-with-fabric",
			conn:       fabricSpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *fabricSpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-fabric-with-mesh",
			conn:       fabricSpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *meshSpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-fabric-with-gateway",
			conn:       fabricSpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *gatewaySpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-mesh-with-fabric",
			conn:       meshSpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *fabricSpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-mesh-with-mesh",
			conn:       meshSpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *meshSpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-mesh-with-gateway",
			conn:       meshSpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *gatewaySpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-gateway-with-fabric",
			conn:       gatewaySpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *fabricSpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-gateway-with-mesh",
			conn:       gatewaySpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *meshSpec,
				},
			},
			err: true,
		},
		{
			name:       "collision-gateway-with-gateway",
			conn:       gatewaySpec,
			withClient: true,
			objects: []kclient.Object{
				&wiringapi.Connection{
					ObjectMeta: kmetav1.ObjectMeta{
						Name:      "existing",
						Namespace: kmetav1.NamespaceDefault,
					},
					Spec: *gatewaySpec,
				},
			},
			err: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &meta.FabricConfig{
				ReservedSubnets: []string{"172.30.1.0/24"},
			}
			err := cfg.WithReservedSubnets()
			require.NoError(t, err)

			var kube kclient.Reader
			if tt.withClient {
				scheme := runtime.NewScheme()
				require.NoError(t, wiringapi.AddToScheme(scheme))
				kube = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(tt.objects...).
					Build()
			}

			_, err = (&wiringapi.Connection{
				ObjectMeta: kmetav1.ObjectMeta{
					Name:      "test",
					Namespace: kmetav1.NamespaceDefault,
				},
				Spec: *tt.conn,
			}).Validate(t.Context(), kube, cfg)

			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
