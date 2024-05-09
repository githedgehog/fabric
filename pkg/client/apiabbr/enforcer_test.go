// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiabbr_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/client/apiabbr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnforcer(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	tests := []struct {
		in               string
		ignoreNotDefined bool
		existingObjects  testData
		expectedObjects  testData
	}{
		{
			in: `
			vpc-1 vpc-2:defI:defR:s=subnet-1=10.42.0.0/24,vlan=1042,dhcp
			vpc-1/subnet-1@server-1 vpc-2/default@server-1--eslag--switch-1
			vpc-1+vpc-2
			vpc-1~ext-1:s=10.1.1.0/24,10.1.2.0/24:s=10.1.3.0/24:p=10.99.1.0/24,10.99.2.0/24
			fallback:server-1--eslag--switch-1
			`,
			ignoreNotDefined: false,
			existingObjects: testData{
				"srv/server-1": wiringapi.ServerSpec{},
				"conn/server-1--eslag--switch-1": wiringapi.ConnectionSpec{
					ESLAG: &wiringapi.ConnESLAG{
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"conn/test-conn": wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: true,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"vpc/delme":       vpcapi.VPCSpec{},
				"vpcattach/delme": vpcapi.VPCAttachmentSpec{},
				"vpcpeer/delme":   vpcapi.VPCPeeringSpec{},
			},
			expectedObjects: testData{
				"srv/server-1": wiringapi.ServerSpec{},
				"conn/server-1--eslag--switch-1": wiringapi.ConnectionSpec{
					ESLAG: &wiringapi.ConnESLAG{
						Fallback: true,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"conn/test-conn": wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: false,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"vpc/vpc-1": vpcapi.VPCSpec{},
				"vpc/vpc-2": vpcapi.VPCSpec{
					DefaultIsolated:   true,
					DefaultRestricted: true,
					Subnets: map[string]*vpcapi.VPCSubnet{
						"subnet-1": {
							Subnet: "10.42.0.0/24",
							VLAN:   1042,
							DHCP: vpcapi.VPCDHCP{
								Enable: true,
							},
						},
					},
				},
				"vpcattach/vpc-1--subnet-1--server-1": vpcapi.VPCAttachmentSpec{
					Subnet:     "vpc-1/subnet-1",
					Connection: "server-1--eslag--switch-1",
				},
				"vpcattach/vpc-2--default--server-1--eslag--switch-1": vpcapi.VPCAttachmentSpec{
					Subnet:     "vpc-2/default",
					Connection: "server-1--eslag--switch-1",
				},
				"vpcpeer/vpc-1--vpc-2": vpcapi.VPCPeeringSpec{
					Permit: []map[string]vpcapi.VPCPeer{
						{
							"vpc-1": {},
							"vpc-2": {},
						},
					},
				},
				"extpeer/vpc-1--ext-1": vpcapi.ExternalPeeringSpec{
					Permit: vpcapi.ExternalPeeringSpecPermit{
						VPC: vpcapi.ExternalPeeringSpecVPC{
							Name:    "vpc-1",
							Subnets: []string{"10.1.1.0/24", "10.1.2.0/24", "10.1.3.0/24"},
						},
						External: vpcapi.ExternalPeeringSpecExternal{
							Name: "ext-1",
							Prefixes: []vpcapi.ExternalPeeringSpecPrefix{
								{Prefix: "10.99.1.0/24"},
								{Prefix: "10.99.2.0/24"},
							},
						},
					},
				},
			},
		},
		{
			in:               "vpc-1 fallback:server-1--eslag--switch-1 fallback:server-3--mclag--switch-1:disable",
			ignoreNotDefined: true,
			existingObjects: testData{
				"vpc/vpc-1": vpcapi.VPCSpec{
					Subnets: map[string]*vpcapi.VPCSubnet{
						"default": {
							Subnet: "10.42.0.0/24",
							VLAN:   1024,
						},
					},
				},
				"vpc/keepme":       vpcapi.VPCSpec{},
				"vpcattach/keepme": vpcapi.VPCAttachmentSpec{},
				"vpcpeer/keepme":   vpcapi.VPCPeeringSpec{},
				"conn/server-1--eslag--switch-1": wiringapi.ConnectionSpec{
					ESLAG: &wiringapi.ConnESLAG{
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"conn/server-2--mclag--switch-1": wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: true,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"conn/server-3--mclag--switch-1": wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: true,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-3/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-3/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
			},
			expectedObjects: testData{
				"vpc/vpc-1":        vpcapi.VPCSpec{},
				"vpc/keepme":       vpcapi.VPCSpec{},
				"vpcattach/keepme": vpcapi.VPCAttachmentSpec{},
				"vpcpeer/keepme":   vpcapi.VPCPeeringSpec{},
				"conn/server-1--eslag--switch-1": wiringapi.ConnectionSpec{
					ESLAG: &wiringapi.ConnESLAG{
						Fallback: true,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-1/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"conn/server-2--mclag--switch-1": wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: true,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-2/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
				"conn/server-3--mclag--switch-1": wiringapi.ConnectionSpec{
					MCLAG: &wiringapi.ConnMCLAG{
						Fallback: false,
						Links: []wiringapi.ServerToSwitchLink{
							{
								Server: wiringapi.BasePortName{Port: "server-3/port-1"},
								Switch: wiringapi.BasePortName{Port: "switch-1/port-1"},
							},
							{
								Server: wiringapi.BasePortName{Port: "server-3/port-2"},
								Switch: wiringapi.BasePortName{Port: "switch-2/port-1"},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			enf, err := apiabbr.NewEnforcer(tt.ignoreNotDefined)
			require.NoError(t, err)
			require.NotNil(t, enf)

			err = enf.Load(tt.in)
			require.NoError(t, err)

			initObjects := tt.existingObjects.toObjects(t, "existing", true)

			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(initObjects...).
				Build()

			err = enf.Enforce(context.Background(), kube)
			require.NoError(t, err)

			expectedObjects := tt.expectedObjects.toObjects(t, "expected", true)
			expectedFound := map[string]bool{}

			for _, list := range []meta.ObjectList{
				&wiringapi.ServerList{},
				&wiringapi.ConnectionList{},
				&vpcapi.VPCList{},
				&vpcapi.VPCAttachmentList{},
				&vpcapi.VPCPeeringList{},
				&vpcapi.ExternalPeeringList{},
			} {
				if err := kube.List(context.Background(), list); err != nil {
					t.Fatalf("failed to list %s: %v", list.GetObjectKind().GroupVersionKind().Kind, err)
				}

				for _, actual := range list.GetItems() {
					actual.SetResourceVersion("")
					actual.Default()

					actualName := actual.GetName()
					actualKind := actual.GetObjectKind().GroupVersionKind().Kind

					if actualKind == "" {
						actualKind = reflect.TypeOf(actual).Elem().Name()
					}

					found := false
					for _, expected := range expectedObjects {
						if expected.GetName() != actualName || expected.GetObjectKind().GroupVersionKind().Kind != actualKind {
							continue
						}

						actual.GetObjectKind().SetGroupVersionKind(expected.GetObjectKind().GroupVersionKind())

						require.Exactly(t, expected, actual, "actual != expected object: %s/%s", actualKind, actualName)
						found = true

						break
					}

					require.True(t, found, "unexpected object: %s/%s: %#v", actualKind, actualName, actual)

					expectedFound[actualKind+"/"+actualName] = true
				}
			}

			for _, expected := range expectedObjects {
				found := expectedFound[expected.GetObjectKind().GroupVersionKind().Kind+"/"+expected.GetName()]
				require.True(t, found, "expected object not found: %s/%s: %#v", expected.GetObjectKind().GroupVersionKind().Kind, expected.GetName(), expected)
			}
		})
	}
}

type testData map[string]any

func (d testData) toObjects(t *testing.T, logType string, logObj bool) []client.Object {
	t.Helper()

	objs := []client.Object{}
	for k, v := range d {
		parts := strings.Split(k, "/")
		if len(parts) != 2 {
			t.Fatalf("invalid %s object key: %s", logType, k)
		}

		kind := parts[0]
		name := parts[1]

		switch kind {
		case "srv":
			objs = append(objs, &wiringapi.Server{
				TypeMeta:   metav1.TypeMeta{APIVersion: wiringapi.GroupVersion.String(), Kind: wiringapi.KindServer},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec:       v.(wiringapi.ServerSpec),
			})
		case "conn":
			objs = append(objs, &wiringapi.Connection{
				TypeMeta:   metav1.TypeMeta{APIVersion: wiringapi.GroupVersion.String(), Kind: wiringapi.KindConnection},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec:       v.(wiringapi.ConnectionSpec),
			})
		case "vpc":
			objs = append(objs, &vpcapi.VPC{
				TypeMeta:   metav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPC},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec:       v.(vpcapi.VPCSpec),
			})
		case "vpcattach":
			objs = append(objs, &vpcapi.VPCAttachment{
				TypeMeta:   metav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPCAttachment},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec:       v.(vpcapi.VPCAttachmentSpec),
			})
		case "vpcpeer":
			objs = append(objs, &vpcapi.VPCPeering{
				TypeMeta:   metav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPCPeering},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec:       v.(vpcapi.VPCPeeringSpec),
			})
		case "extpeer":
			objs = append(objs, &vpcapi.ExternalPeering{
				TypeMeta:   metav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindExternalPeering},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: metav1.NamespaceDefault},
				Spec:       v.(vpcapi.ExternalPeeringSpec),
			})
		default:
			t.Fatalf("unknown kind for %s object: %s", logType, k)
		}
	}

	for _, obj := range objs {
		obj.(meta.Object).Default()

		if logObj {
			t.Logf("%s object: %s/%s: %#v", logType, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), obj)
		}
	}

	return objs
}
