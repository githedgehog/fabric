// Copyright 2026 Hedgehog
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

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// an IPv4Namespace can't grow around an External's inbound prefix, as routes within it are dropped
func TestIPv4NamespaceExternalInboundPrefixes(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	fabric := &wiringapi.Fabric{
		ObjectMeta: kmetav1.ObjectMeta{Name: wiringapi.DefaultFabric, Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.DefaultFabricSpec(&meta.FabricConfig{}),
	}
	ns := &vpcapi.IPv4Namespace{
		ObjectMeta: kmetav1.ObjectMeta{Name: vpcapi.DefaultIPv4Namespace, Namespace: kmetav1.NamespaceDefault},
		Spec:       vpcapi.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0/16"}},
	}
	ext := &vpcapi.External{
		ObjectMeta: kmetav1.ObjectMeta{Name: "ext-01", Namespace: kmetav1.NamespaceDefault},
		Spec: vpcapi.ExternalSpec{
			IPv4Namespace:   vpcapi.DefaultIPv4Namespace,
			InboundPrefixes: map[string]vpcapi.ExternalInboundPrefix{"10.1.1.0/24": {}, "0.0.0.0/0": {}},
		},
	}
	for _, obj := range []interface{ Default() }{ns, ext} {
		obj.Default()
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fabric, ns, ext).Build()
	w := &IPv4NamespaceWebhook{Client: kube, Cfg: &meta.FabricConfig{}}

	for _, tt := range []struct {
		name    string
		subnets []string
		err     string
	}{
		{name: "subnet added next to the inbound prefixes", subnets: []string{"10.0.0.0/16", "10.2.0.0/16"}},
		{name: "subnet added around an inbound prefix", subnets: []string{"10.0.0.0/16", "10.1.0.0/16"}, err: "external ext-01: inbound prefix 10.1.1.0/24 is inside subnet 10.1.0.0/16"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			newNs := ns.DeepCopy()
			newNs.Spec.Subnets = tt.subnets
			newNs.Default()
			_, err := w.ValidateUpdate(t.Context(), ns, newNs)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
