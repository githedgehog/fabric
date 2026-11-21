// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSwitchASNImmutable(t *testing.T) {
	oldSw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{ASN: 65100}}
	newSw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{ASN: 65099}}

	_, err := (&SwitchWebhook{}).ValidateUpdate(t.Context(), oldSw, newSw)
	require.ErrorContains(t, err, "asn is immutable")
}

func TestSwitchRoCEBreakouts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	bcm, cmls := switchprofile.DellS5248FON.DeepCopy(), switchprofile.CumulusNvidiaSN4700.DeepCopy()
	bcm.Namespace, cmls.Namespace = kmetav1.NamespaceDefault, kmetav1.NamespaceDefault
	w := &SwitchWebhook{KubeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(bcm, cmls).Build()}

	for _, tt := range []struct {
		profile string
		roce    bool
		err     string
	}{
		{profile: bcm.Name, roce: true, err: "port breakouts cannot be changed when RoCEv2 is enabled"},
		{profile: bcm.Name},
		{profile: cmls.Name, roce: true},
	} {
		oldSw := &wiringapi.Switch{
			Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
			Spec: wiringapi.SwitchSpec{Profile: tt.profile, RoCE: tt.roce},
		}
		newSw := oldSw.DeepCopy()
		newSw.Spec.PortBreakouts = map[string]string{"E1/1": "4x25G"}

		err := w.validateRoCEBreakouts(t.Context(), oldSw, newSw)
		if tt.err != "" {
			require.ErrorContains(t, err, tt.err, tt.profile)
		} else {
			require.NoError(t, err, tt.profile)
		}
	}
}

func TestSwitchDomainsImmutable(t *testing.T) {
	for _, role := range []wiringapi.SwitchRole{wiringapi.SwitchRoleSpine, wiringapi.SwitchRoleServerLeaf} {
		oldSw := &wiringapi.Switch{Spec: wiringapi.SwitchSpec{Role: role}}
		newSw := oldSw.DeepCopy()
		newSw.Spec.Topology.Domains = []string{"plane-b"}

		_, err := (&SwitchWebhook{}).ValidateUpdate(t.Context(), oldSw, newSw)
		require.ErrorContains(t, err, "topology.domains is immutable", role)
	}
}
