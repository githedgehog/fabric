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
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var base = []meta.Object{
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

func TestIsServerReachable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

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

func TestIsExternalSubnetReachable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	tests := []struct {
		name      string
		existing  []meta.Object
		source    string
		dest      string
		reachable bool
		err       bool
	}{
		{
			name: "simple-reachable",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/0",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "source",
			dest:      "0.0.0.0/0",
			reachable: true,
			err:       false,
		},
		{
			name: "no-ext-attach",
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
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/0",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "source",
			dest:      "0.0.0.0/0",
			reachable: false,
			err:       false,
		},
		{
			name: "no-ext-peering",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
			),
			source:    "source",
			dest:      "0.0.0.0/0",
			reachable: false,
			err:       false,
		},
		{
			name: "ext-peering-wrong-subnet",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/0",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-2"},
							},
						},
					},
				},
			),
			source:    "source",
			dest:      "0.0.0.0/0",
			reachable: false,
			err:       false,
		},
		{
			name: "ext-peering-wrong-prefix",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/24",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "source",
			dest:      "0.0.0.0/0",
			reachable: false,
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
			reachable, err := apiutil.IsExternalSubnetReachable(context.Background(), kube, tt.source, tt.dest)

			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.reachable, reachable)
		})
	}
}

func TestGetReacheableFrom(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	for _, tt := range []struct {
		name      string
		existing  []meta.Object
		vpc       string
		reachable map[string]*apiutil.ReachableFromSubnet
		err       bool
	}{
		{
			name: "simple",
			existing: []meta.Object{
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1",
					},
					Spec: vpcapi.VPCSpec{
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {Subnet: "10.0.1.1/24"},
							"subnet-2": {Subnet: "10.0.1.2/24", Restricted: pointer.To(true)},
							"subnet-3": {Subnet: "10.0.1.3/24", Isolated: pointer.To(true)},
							"subnet-4": {Subnet: "10.0.1.4/24", Isolated: pointer.To(true)},
						},
						Permit: [][]string{
							{"subnet-1", "subnet-3"},
						},
					},
				},
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-2",
					},
					Spec: vpcapi.VPCSpec{
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {Subnet: "10.0.2.1/24"},
							"subnet-2": {Subnet: "10.0.2.2/24"},
							"subnet-3": {Subnet: "10.0.2.3/24"},
						},
					},
				},
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-3",
					},
					Spec: vpcapi.VPCSpec{
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {Subnet: "10.0.3.1/24"},
						},
					},
				},
				&vpcapi.VPC{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-4",
					},
					Spec: vpcapi.VPCSpec{
						Subnets: map[string]*vpcapi.VPCSubnet{
							"subnet-1": {Subnet: "10.0.4.1/24"},
						},
					},
				},
				&vpcapi.VPCPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--vpc-2",
					},
					Spec: vpcapi.VPCPeeringSpec{
						Permit: []map[string]vpcapi.VPCPeer{
							{
								"vpc-1": {Subnets: []string{"subnet-1"}},
								"vpc-2": {Subnets: []string{"subnet-2", "subnet-1"}},
							},
							{
								"vpc-1": {Subnets: []string{"subnet-2"}},
								"vpc-2": {},
							},
							{
								"vpc-1": {},
								"vpc-2": {Subnets: []string{"subnet-3"}},
							},
						},
					},
				},
				&vpcapi.VPCPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--vpc-3",
					},
					Spec: vpcapi.VPCPeeringSpec{
						Permit: []map[string]vpcapi.VPCPeer{
							{
								"vpc-1": {Subnets: []string{"subnet-1"}},
								"vpc-3": {Subnets: []string{"subnet-1"}},
							},
						},
					},
				},
				&vpcapi.VPCPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--vpc-4",
					},
					Spec: vpcapi.VPCPeeringSpec{
						Remote: "border",
						Permit: []map[string]vpcapi.VPCPeer{
							{
								"vpc-1": {Subnets: []string{"subnet-1"}},
								"vpc-4": {Subnets: []string{"subnet-1"}},
							},
						},
					},
				},
				&wiringapi.SwitchGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "border",
					},
				},
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1--attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						External: "ext-1",
					},
				},
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-2--attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						External: "ext-2",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1", "subnet-2"},
							},
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{Prefix: "172.1.2.0/24"},
									{Prefix: "172.1.1.0/24"},
								},
							},
						},
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-2",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-4", "subnet-1"},
							},
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-2",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{Prefix: "172.2.0.0/24"},
								},
							},
						},
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-3",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-3"},
							},
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-3",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{Prefix: "172.3.0.0/24"},
								},
							},
						},
					},
				},
			},
			vpc: "vpc-1",
			reachable: map[string]*apiutil.ReachableFromSubnet{
				"subnet-1": {
					WithinSameSubnet: &apiutil.ReachableSubnet{
						Name:   "subnet-1",
						Subnet: "10.0.1.1/24",
					},
					SameVPCSubnets: []apiutil.ReachableSubnet{
						{
							Name:   "subnet-2",
							Subnet: "10.0.1.2/24",
						},
						{
							Name:   "subnet-3",
							Subnet: "10.0.1.3/24",
						},
					},
					OtherVPCSubnets: map[string][]apiutil.ReachableSubnet{
						"vpc-2": {
							{
								Name:   "subnet-1",
								Subnet: "10.0.2.1/24",
							},
							{
								Name:   "subnet-2",
								Subnet: "10.0.2.2/24",
							},
							{
								Name:   "subnet-3",
								Subnet: "10.0.2.3/24",
							},
						},
						"vpc-3": {
							{
								Name:   "subnet-1",
								Subnet: "10.0.3.1/24",
							},
						},
					},
					ExternalPrefixes: map[string][]string{
						"ext-1": {"172.1.1.0/24", "172.1.2.0/24"},
						"ext-2": {"172.2.0.0/24"},
					},
				},
				"subnet-2": {
					WithinSameSubnet: nil,
					SameVPCSubnets: []apiutil.ReachableSubnet{
						{
							Name:   "subnet-1",
							Subnet: "10.0.1.1/24",
						},
					},
					OtherVPCSubnets: map[string][]apiutil.ReachableSubnet{
						"vpc-2": {
							{
								Name:   "subnet-1",
								Subnet: "10.0.2.1/24",
							},
							{
								Name:   "subnet-2",
								Subnet: "10.0.2.2/24",
							},
							{
								Name:   "subnet-3",
								Subnet: "10.0.2.3/24",
							},
						},
					},
					ExternalPrefixes: map[string][]string{
						"ext-1": {"172.1.1.0/24", "172.1.2.0/24"},
					},
				},
				"subnet-3": {
					WithinSameSubnet: &apiutil.ReachableSubnet{
						Name:   "subnet-3",
						Subnet: "10.0.1.3/24",
					},
					SameVPCSubnets: []apiutil.ReachableSubnet{
						{
							Name:   "subnet-1",
							Subnet: "10.0.1.1/24",
						},
					},
					OtherVPCSubnets: map[string][]apiutil.ReachableSubnet{
						"vpc-2": {
							{
								Name:   "subnet-3",
								Subnet: "10.0.2.3/24",
							},
						},
					},
					ExternalPrefixes: map[string][]string{},
				},
				"subnet-4": {
					WithinSameSubnet: &apiutil.ReachableSubnet{
						Name:   "subnet-4",
						Subnet: "10.0.1.4/24",
					},
					SameVPCSubnets: nil,
					OtherVPCSubnets: map[string][]apiutil.ReachableSubnet{
						"vpc-2": {
							{
								Name:   "subnet-3",
								Subnet: "10.0.2.3/24",
							},
						},
					},
					ExternalPrefixes: map[string][]string{
						"ext-2": {"172.2.0.0/24"},
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rawObjs := make([]client.Object, len(tt.existing))
			for idx, obj := range tt.existing {
				obj.Default()
				rawObjs[idx] = obj
			}

			kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rawObjs...).Build()
			reachable, err := apiutil.GetReachableFrom(context.Background(), kube, tt.vpc)

			if tt.err {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.reachable, reachable)
		})
	}
}

func TestIsStaticExternalIPReachable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	tests := []struct {
		name      string
		existing  []meta.Object
		source    string
		dest      string
		reachable bool
		err       bool
	}{
		{
			name: "simple-reachable-ip",
			existing: append(base,
				&wiringapi.Connection{
					ObjectMeta: metav1.ObjectMeta{
						Name: "static-ext",
					},
					Spec: wiringapi.ConnectionSpec{
						StaticExternal: &wiringapi.ConnStaticExternal{
							WithinVPC: "vpc-1",
							Link: wiringapi.ConnStaticExternalLink{
								Switch: wiringapi.ConnStaticExternalLinkSwitch{
									IP: "10.10.10.1/24",
								},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "10.10.10.10",
			reachable: true,
			err:       false,
		},
		{
			name: "simple-reachable-subnet",
			existing: append(base,
				&wiringapi.Connection{
					ObjectMeta: metav1.ObjectMeta{
						Name: "static-ext",
					},
					Spec: wiringapi.ConnectionSpec{
						StaticExternal: &wiringapi.ConnStaticExternal{
							WithinVPC: "vpc-1",
							Link: wiringapi.ConnStaticExternalLink{
								Switch: wiringapi.ConnStaticExternalLinkSwitch{
									IP: "10.10.10.1/24",
									Subnets: []string{
										"10.10.20.1/24",
									},
								},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "10.10.20.10",
			reachable: true,
			err:       false,
		},
		{
			name: "simple-reachable-no-subnet",
			existing: append(base,
				&wiringapi.Connection{
					ObjectMeta: metav1.ObjectMeta{
						Name: "static-ext",
					},
					Spec: wiringapi.ConnectionSpec{
						StaticExternal: &wiringapi.ConnStaticExternal{
							WithinVPC: "vpc-1",
							Link: wiringapi.ConnStaticExternalLink{
								Switch: wiringapi.ConnStaticExternalLinkSwitch{
									IP: "10.10.10.1/24",
									Subnets: []string{
										"10.10.20.1/24",
									},
								},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "10.10.30.10",
			reachable: false,
			err:       false,
		},
		{
			name: "simple-reachable-no-in-subnet",
			existing: append(base,
				&wiringapi.Connection{
					ObjectMeta: metav1.ObjectMeta{
						Name: "static-ext",
					},
					Spec: wiringapi.ConnectionSpec{
						StaticExternal: &wiringapi.ConnStaticExternal{
							WithinVPC: "vpc-1",
							Link: wiringapi.ConnStaticExternalLink{
								Switch: wiringapi.ConnStaticExternalLinkSwitch{
									IP: "10.10.10.1/24",
									Subnets: []string{
										"10.10.20.1/32",
									},
								},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "10.10.20.10",
			reachable: false,
			err:       false,
		},
		{
			name: "not-within-vpc",
			existing: append(base,
				&wiringapi.Connection{
					ObjectMeta: metav1.ObjectMeta{
						Name: "static-ext",
					},
					Spec: wiringapi.ConnectionSpec{
						StaticExternal: &wiringapi.ConnStaticExternal{
							Link: wiringapi.ConnStaticExternalLink{
								Switch: wiringapi.ConnStaticExternalLinkSwitch{
									IP: "10.10.10.1/24",
								},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: false,
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
			reachable, err := apiutil.IsStaticExternalIPReachable(context.Background(), kube, tt.source, tt.dest)

			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.reachable, reachable)
		})
	}
}

func TestIsExternalIPReachable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	tests := []struct {
		name      string
		existing  []meta.Object
		source    string
		dest      string
		reachable bool
		err       bool
	}{
		{
			name: "simple-reachable",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/0",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: true,
			err:       false,
		},
		{
			name: "simple-reachable-32",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "8.8.8.8/32",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: true,
			err:       false,
		},
		{
			name: "no-ext-attach",
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
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/0",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: false,
			err:       false,
		},
		{
			name: "no-ext-peering",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: false,
			err:       false,
		},
		{
			name: "ext-peering-wrong-subnet",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "0.0.0.0/0",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-2"},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: false,
			err:       false,
		},
		{
			name: "ext-peering-wrong-prefix",
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
				&vpcapi.ExternalAttachment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ext-1-attach",
					},
					Spec: vpcapi.ExternalAttachmentSpec{
						Connection: "ext",
						External:   "ext-1",
					},
				},
				&vpcapi.ExternalPeering{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vpc-1--ext-1",
					},
					Spec: vpcapi.ExternalPeeringSpec{
						Permit: vpcapi.ExternalPeeringSpecPermit{
							External: vpcapi.ExternalPeeringSpecExternal{
								Name: "ext-1",
								Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
									{
										Prefix: "10.0.0.0/24",
									},
								},
							},
							VPC: vpcapi.ExternalPeeringSpecVPC{
								Name:    "vpc-1",
								Subnets: []string{"subnet-1"},
							},
						},
					},
				},
			),
			source:    "vpc-1/subnet-1",
			dest:      "8.8.8.8",
			reachable: false,
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
			reachable, err := apiutil.IsExternalIPReachable(context.Background(), kube, tt.source, tt.dest)

			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.reachable, reachable)
		})
	}
}
