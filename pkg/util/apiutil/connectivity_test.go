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

package apiutil_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsServerReachable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	base := []meta.Object{
		&wiringapi.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: "source",
			},
		},
		&wiringapi.Connection{
			ObjectMeta: metav1.ObjectMeta{
				Name: "source-conn",
			},
			Spec: wiringapi.ConnectionSpec{
				Unbundled: &wiringapi.ConnUnbundled{
					Link: wiringapi.ServerToSwitchLink{
						Server: wiringapi.NewBasePortName("source/port-1"),
						Switch: wiringapi.NewBasePortName("switch/port-1"),
					},
				},
			},
		},
		&wiringapi.Server{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dest",
			},
		},
		&wiringapi.Connection{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dest-conn",
			},
			Spec: wiringapi.ConnectionSpec{
				Unbundled: &wiringapi.ConnUnbundled{
					Link: wiringapi.ServerToSwitchLink{
						Server: wiringapi.NewBasePortName("dest/port-2"),
						Switch: wiringapi.NewBasePortName("switch/port-2"),
					},
				},
			},
		},
		&vpcapi.VPC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vpc-1",
			},
			Spec: vpcapi.VPCSpec{
				Subnets: map[string]*vpcapi.VPCSubnet{
					"subnet-1": {},
					"subnet-2": {},
				},
			},
		},
		&vpcapi.VPC{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vpc-2",
			},
			Spec: vpcapi.VPCSpec{
				Subnets: map[string]*vpcapi.VPCSubnet{
					"subnet-1": {},
				},
			},
		},
	}

	tests := []struct {
		name      string
		existing  []meta.Object
		source    string
		dest      string
		reachable bool
		err       bool
	}{
		{
			name:      "no servers",
			existing:  []meta.Object{},
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       true,
		},
		{
			name: "source-is-control",
			existing: []meta.Object{
				&wiringapi.Server{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source",
					},
					Spec: wiringapi.ServerSpec{
						Type: wiringapi.ServerTypeControl,
					},
				},
			},
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       true,
		},
		{
			name: "only-servers",
			existing: []meta.Object{
				&wiringapi.Server{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source",
					},
				},
				&wiringapi.Server{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest",
					},
				},
			},
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name:      "no-attachments",
			existing:  base,
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name: "same-subnet",
			existing: append(base,
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-1/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-1/subnet-1",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: true,
			err:       false,
		},
		{
			name: "different-subnet-same-vpc",
			existing: append(base,
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-1/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-1/subnet-2",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: true,
			err:       false,
		},
		{
			name: "different-vpc-no-peering",
			existing: append(base,
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-1/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-2/subnet-1",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name: "different-vpc-peering",
			existing: append(base,
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-1/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-2/subnet-1",
					},
				},
				&vpcapi.VPCPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1-vpc-2",
					},
					Spec: vpcapi.VPCPeeringSpec{
						Permit: []map[string]vpcapi.VPCPeer{
							{
								"vpc-1": {Subnets: []string{"subnet-1"}},
								"vpc-2": {},
							},
						},
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: true,
			err:       false,
		},
		{
			name: "different-subnet-same-vpc-default-isolated",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						DefaultIsolated: true,
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {},
							"subnet-2": {},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-2",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name: "different-subnet-same-vpc-isolated",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {
								Isolated: pointer.To(true),
							},
							"subnet-2": {},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-2",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name: "different-subnet-same-vpc-default-isolated-not-isolated",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						DefaultIsolated: true,
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {
								Isolated: pointer.To(false),
							},
							"subnet-2": {
								Isolated: pointer.To(false),
							},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-2",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: true,
			err:       false,
		},
		{
			name: "different-subnet-same-vpc-default-isolated-permit",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						DefaultIsolated: true,
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {},
							"subnet-2": {},
						},
						Permit: [][]string{
							{"subnet-1", "subnet-2"},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-2",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: true,
			err:       false,
		},
		{
			name: "same-subnet-default-restricted",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						DefaultRestricted: true,
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name: "same-subnet-restricted",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {
								Restricted: pointer.To(true),
							},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: false,
			err:       false,
		},
		{
			name: "same-subnet-default-restricted-not-restricted",
			existing: append(base,
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						DefaultRestricted: true,
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {
								Restricted: pointer.To(false),
							},
						},
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "source-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "source-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
				&vpcapi.VPCAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dest-attach",
					},
					Spec: vpcapi.VPCAttachmentSpec{
						Connection: "dest-conn",
						Subnet:     "vpc-3/subnet-1",
					},
				},
			),
			source:    "source",
			dest:      "dest",
			reachable: true,
			err:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawObjs := make([]client.Object, len(tt.existing))
			for idx, obj := range tt.existing {
				obj.Default()
				rawObjs[idx] = obj
			}

			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rawObjs...).Build()
			reachable, err := apiutil.IsServerReachable(context.Background(), kube, tt.source, tt.dest)

			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.reachable, reachable)
		})
	}
}
