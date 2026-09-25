// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFabricChanged(t *testing.T) {
	for _, tt := range []struct {
		old, new string
		changed  bool
	}{
		{old: "", new: ""},
		{old: "backend", new: "backend"},
		// the stored object predates the reference and the incoming one has just been defaulted,
		// so this is every update to every object on the first upgrade and must not be refused
		{old: "", new: "default"},
		{old: "default", new: ""},
		{old: "", new: "backend", changed: true},
		{old: "default", new: "backend", changed: true},
		{old: "backend", new: "", changed: true},
		{old: "backend", new: "frontend", changed: true},
	} {
		t.Run(tt.old+"->"+tt.new, func(t *testing.T) {
			require.Equal(t, tt.changed, fabricChanged(tt.old, tt.new))
		})
	}
}

func TestFabricDeleteGuard(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	inFabric := func(fabricName string) []kclient.Object {
		return []kclient.Object{
			&wiringapi.Switch{
				ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-01", Namespace: kmetav1.NamespaceDefault},
				Spec:       wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Fabric: fabricName}},
			},
			&vpcapi.VPC{
				ObjectMeta: kmetav1.ObjectMeta{Name: "vpc-01", Namespace: kmetav1.NamespaceDefault},
				Spec:       vpcapi.VPCSpec{Topology: vpcapi.VPCTopology{Fabric: fabricName}},
			},
			&gwapi.GatewayGroup{
				ObjectMeta: kmetav1.ObjectMeta{Name: "gwg-01", Namespace: kmetav1.NamespaceDefault},
				Spec:       gwapi.GatewayGroupSpec{Topology: gwapi.GatewayGroupTopology{Fabric: fabricName}},
			},
		}
	}

	for _, tt := range []struct {
		name    string
		fabric  string
		objects []kclient.Object
		err     string
	}{
		{name: "empty fabric", fabric: "backend", objects: inFabric("frontend")},
		{name: "default fabric, nothing in it", fabric: "default", err: "default Fabric can not be deleted"},
		{name: "fabric with a switch", fabric: "backend", objects: inFabric("backend")[:1], err: "switch leaf-01"},
		{name: "fabric with a vpc", fabric: "backend", objects: inFabric("backend")[1:2], err: "VPC vpc-01"},
		{name: "fabric with a gateway group", fabric: "backend", objects: inFabric("backend")[2:], err: "gateway group gwg-01"},
		// reserved whether or not anything is in it; an object written before the reference
		// existed belongs to it implicitly and carries an empty topology.fabric
		{name: "default fabric, populated", fabric: "default", objects: inFabric("")[1:2], err: "default Fabric can not be deleted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := &FabricWebhook{Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()}

			_, err := w.ValidateDelete(t.Context(), &wiringapi.Fabric{
				ObjectMeta: kmetav1.ObjectMeta{Name: tt.fabric, Namespace: kmetav1.NamespaceDefault},
			})

			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func TestFabricUpdateASNs(t *testing.T) {
	fabricGen := func(asnStart, asnEnd, spineASN uint32) *wiringapi.Fabric {
		return &wiringapi.Fabric{
			ObjectMeta: kmetav1.ObjectMeta{Name: "backend", Namespace: kmetav1.NamespaceDefault},
			Spec: wiringapi.FabricSpec{
				ASNStart: asnStart,
				ASNEnd:   asnEnd,
				Domains:  map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {SpineASN: spineASN}},
			},
		}
	}

	for _, tt := range []struct {
		name     string
		old, new *wiringapi.Fabric
		err      string
	}{
		{name: "unchanged", old: fabricGen(64100, 64200, 64100), new: fabricGen(64100, 64200, 64100)},
		// written by a build that kept the ASNs in the status
		{name: "filling in unset ASNs", old: fabricGen(0, 0, 0), new: fabricGen(64100, 64200, 64100)},
		{name: "changing asnStart", old: fabricGen(64100, 64200, 64100), new: fabricGen(64000, 64200, 64100), err: "ASN range can not be changed"},
		{name: "changing asnEnd", old: fabricGen(64100, 64200, 64100), new: fabricGen(64100, 64300, 64100), err: "ASN range can not be changed"},
		{name: "changing spineASN", old: fabricGen(64100, 64200, 64100), new: fabricGen(64100, 64200, 64101), err: "spineASN of domain default can not be changed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := &FabricWebhook{}
			_, err := w.ValidateUpdate(t.Context(), tt.old, tt.new)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
