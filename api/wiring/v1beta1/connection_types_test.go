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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConnectionValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		conn *wiringapi.ConnectionSpec
		err  bool
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &meta.FabricConfig{
				ReservedSubnets: []string{"172.30.1.0/24"},
			}
			err := cfg.WithReservedSubnets()
			require.NoError(t, err)

			_, err = (&wiringapi.Connection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: *tt.conn,
			}).Validate(t.Context(), nil, cfg)

			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
