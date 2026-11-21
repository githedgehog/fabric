// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVLANNamespaceSwitchOverlaps(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	vlanNS := func(name string, from, to uint16) *wiringapi.VLANNamespace {
		return &wiringapi.VLANNamespace{
			Name: name, Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.VLANNamespaceSpec{Ranges: []meta.VLANRange{{From: from, To: to}}},
		}
	}
	vlanSwitch := func(name string, namespaces ...string) *wiringapi.Switch {
		sw := &wiringapi.Switch{
			Name: name, Namespace: kmetav1.NamespaceDefault, Labels: map[string]string{},
			Spec: wiringapi.SwitchSpec{VLANNamespaces: namespaces},
		}
		for _, ns := range namespaces {
			sw.Labels[wiringapi.ListLabelVLANNamespace(ns)] = wiringapi.ListLabelValue
		}

		return sw
	}

	// a and b share leaf-1, c is only on leaf-2 and overlaps them
	w := &VLANNamespaceWebhook{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		vlanNS("a", 1000, 1999), vlanNS("b", 2000, 2999), vlanNS("c", 1000, 2999),
		vlanSwitch("leaf-1", "a", "b"), vlanSwitch("leaf-2", "c"),
	).Build()}

	for _, tt := range []struct {
		name string
		ns   *wiringapi.VLANNamespace
		err  string
	}{
		{name: "no-overlap", ns: vlanNS("a", 1000, 1499)},
		{name: "overlap-on-switch", ns: vlanNS("a", 1000, 2499), err: "ranges overlap another VLAN namespace of switch leaf-1"},
		{name: "overlap-other-switch", ns: vlanNS("c", 1000, 3999)},
		{name: "unused", ns: vlanNS("d", 1000, 3999)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := w.validateSwitchOverlaps(t.Context(), tt.ns)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
