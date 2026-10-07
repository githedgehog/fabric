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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	admissionv1 "k8s.io/api/admission/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// what a VPC interconnect relies on in its VPCs and IPv4Namespace can't be changed from under it
func TestVPCInterconnectReferences(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	fabric := &wiringapi.Fabric{
		ObjectMeta: kmetav1.ObjectMeta{Name: wiringapi.DefaultFabric, Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.DefaultFabricSpec(&meta.FabricConfig{}),
	}
	ns := &vpcapi.IPv4Namespace{
		ObjectMeta: kmetav1.ObjectMeta{Name: vpcapi.DefaultIPv4Namespace, Namespace: kmetav1.NamespaceDefault},
		Spec:       vpcapi.IPv4NamespaceSpec{Subnets: []string{"10.0.0.0/16"}},
	}
	vpc := &vpcapi.VPC{
		ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-01", Namespace: kmetav1.NamespaceDefault},
		Spec: vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{
			"default": {Subnet: "10.0.1.0/24", VLAN: 1001},
			"other":   {Subnet: "10.0.2.0/24", VLAN: 1002},
		}},
	}
	ic := &vpcapi.VPCInterconnect{
		ObjectMeta: kmetav1.ObjectMeta{Name: "ic-01", Namespace: kmetav1.NamespaceDefault},
		Spec: vpcapi.VPCInterconnectSpec{
			Links:  []vpcapi.VPCInterconnectLink{{Connection: "leaf-01--external", VLAN: 101}},
			Local:  map[string]vpcapi.VPCInterconnectVPC{"vpc-01": {Subnets: []string{"default"}}},
			Remote: vpcapi.VPCInterconnectRemote{Prefixes: []string{"10.1.1.0/24"}},
		},
	}
	for _, obj := range []interface{ Default() }{ns, vpc, ic} {
		obj.Default()
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fabric, ns, vpc, ic).Build()

	t.Run("vpc", func(t *testing.T) {
		// validated without a client, so only the VPC interconnect check looks up other objects
		w := &VPCWebhook{Client: kube, Cfg: &meta.FabricConfig{}}
		for _, tt := range []struct {
			name   string
			change func(*vpcapi.VPC)
			err    string
		}{
			{name: "unrelated subnet removed", change: func(vpc *vpcapi.VPC) { delete(vpc.Spec.Subnets, "other") }},
			{name: "local subnet removed", change: func(vpc *vpcapi.VPC) { delete(vpc.Spec.Subnets, "default") }, err: "vpc vpc-01 does not have subnet default"},
			{name: "IPv4Namespace changed", change: func(vpc *vpcapi.VPC) { vpc.Spec.IPv4Namespace = "ns-02" }, err: "vpc vpc-01 is in IPv4Namespace ns-02, not default"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				newVPC := vpc.DeepCopy()
				tt.change(newVPC)
				newVPC.Default()
				_, err := w.ValidateUpdate(t.Context(), vpc, newVPC)
				if tt.err != "" {
					require.ErrorContains(t, err, tt.err)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	// with nothing else using them, so that these guards are the ones refusing
	t.Run("delete connection and ipv4 namespace", func(t *testing.T) {
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ic.DeepCopy()).Build()
		conn := &wiringapi.Connection{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01--external", Namespace: kmetav1.NamespaceDefault}}
		connW := &ConnectionWebhook{Client: kube}
		nsW := &IPv4NamespaceWebhook{Client: kube}

		_, err := connW.ValidateDelete(t.Context(), conn)
		require.ErrorContains(t, err, "connection has VPC interconnects")
		_, err = nsW.ValidateDelete(t.Context(), ns)
		require.ErrorContains(t, err, "IPv4Namespace has VPC interconnects")

		require.NoError(t, kube.Delete(t.Context(), ic.DeepCopy()))
		_, err = connW.ValidateDelete(t.Context(), conn)
		require.NoError(t, err)
		_, err = nsW.ValidateDelete(t.Context(), ns)
		require.NoError(t, err)
	})

	t.Run("ipv4 namespace", func(t *testing.T) {
		w := &IPv4NamespaceWebhook{Client: kube, Cfg: &meta.FabricConfig{}}
		for _, tt := range []struct {
			name    string
			subnets []string
			err     string
		}{
			{name: "subnet added next to the remote prefixes", subnets: []string{"10.0.0.0/16", "10.2.0.0/16"}},
			{name: "subnet added around a remote prefix", subnets: []string{"10.0.0.0/16", "10.1.0.0/16"}, err: "remote prefix 10.1.1.0/24 is inside subnet 10.1.0.0/16"},
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
	})

	w := &VPCInterconnectWebhook{Client: kube}
	ctxWith := func(opts *kmetav1.DeleteOptions) context.Context {
		raw, err := json.Marshal(opts)
		require.NoError(t, err)

		return admission.NewContextWithRequest(t.Context(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Options: runtime.RawExtension{Raw: raw},
		}})
	}

	t.Run("delete with orphan propagation", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			opts *kmetav1.DeleteOptions
			err  bool
		}{
			{name: "default", opts: &kmetav1.DeleteOptions{}},
			{name: "background", opts: &kmetav1.DeleteOptions{PropagationPolicy: pointer.To(kmetav1.DeletePropagationBackground)}},
			{name: "foreground", opts: &kmetav1.DeleteOptions{PropagationPolicy: pointer.To(kmetav1.DeletePropagationForeground)}},
			{name: "orphan", opts: &kmetav1.DeleteOptions{PropagationPolicy: pointer.To(kmetav1.DeletePropagationOrphan)}, err: true},
			{name: "orphan dependents", opts: &kmetav1.DeleteOptions{OrphanDependents: pointer.To(true)}, err: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := w.ValidateDelete(ctxWith(tt.opts), ic)
				if tt.err {
					require.ErrorContains(t, err, "orphan propagation")
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("delete in use", func(t *testing.T) {
		ctx := ctxWith(&kmetav1.DeleteOptions{})
		_, err := w.ValidateDelete(ctx, ic)
		require.NoError(t, err)

		gwPeering := &gwapi.GatewayPeering{
			ObjectMeta: kmetav1.ObjectMeta{Name: "ic-01--ext-01", Namespace: kmetav1.NamespaceDefault},
			Spec: gwapi.PeeringSpec{Peering: map[string]*gwapi.PeeringEntry{
				vpcapi.VPCInfoExtPrefix + "ic-01":  {},
				vpcapi.VPCInfoExtPrefix + "ext-01": {},
			}},
		}
		gwPeering.Default()
		require.NoError(t, kube.Create(t.Context(), gwPeering))
		_, err = w.ValidateDelete(ctx, ic)
		require.ErrorContains(t, err, "VPC interconnect is used by gateway peering ic-01--ext-01")
	})
}
