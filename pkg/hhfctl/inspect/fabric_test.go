// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFabricFilter(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))

	meta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}
	switchGen := func(name, fabricName string) kclient.Object {
		return &wiringapi.Switch{ObjectMeta: meta(name), Spec: wiringapi.SwitchSpec{Topology: wiringapi.SwitchTopology{Fabric: fabricName}, Profile: "vs"}}
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&wiringapi.SwitchProfile{ObjectMeta: meta("vs")},
		&wiringapi.Fabric{ObjectMeta: meta("default")},
		&wiringapi.Fabric{ObjectMeta: meta("backend")},
		// written before the fabric reference existed
		switchGen("leaf-01", ""),
		switchGen("leaf-02", "backend"),
	).Build()

	names := func(out *FabricOut) ([]string, []string) {
		fabrics, switches := []string{}, []string{}
		for _, fab := range out.Fabrics {
			fabrics = append(fabrics, fab.Name)
		}
		for _, sw := range out.Switches {
			switches = append(switches, sw.Name+"@"+sw.Fabric)
		}

		return fabrics, switches
	}

	out, err := Fabric(t.Context(), kube, FabricIn{})
	require.NoError(t, err)
	fabrics, switches := names(out)
	require.Equal(t, []string{"backend", "default"}, fabrics)
	require.Equal(t, []string{"leaf-01@default", "leaf-02@backend"}, switches)

	out, err = Fabric(t.Context(), kube, FabricIn{Name: "default"})
	require.NoError(t, err)
	fabrics, switches = names(out)
	require.Equal(t, []string{"default"}, fabrics)
	require.Equal(t, []string{"leaf-01@default"}, switches)

	_, err = Fabric(t.Context(), kube, FabricIn{Name: "frontend"})
	require.ErrorContains(t, err, "fabric frontend not found")
}
