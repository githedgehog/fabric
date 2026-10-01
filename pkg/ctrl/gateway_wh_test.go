// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGatewayDeleteGuard(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	gwConn := &wiringapi.Connection{
		ObjectMeta: kmetav1.ObjectMeta{Name: "spine-01--gateway--gw-1", Namespace: kmetav1.NamespaceDefault},
		Spec: wiringapi.ConnectionSpec{Gateway: &wiringapi.ConnGateway{
			Links: []wiringapi.GatewayLink{{
				Switch:  wiringapi.ConnFabricLinkSwitch{BasePortName: wiringapi.BasePortName{Port: "spine-01/E1/1"}},
				Gateway: wiringapi.ConnGatewayLinkGateway{BasePortName: wiringapi.BasePortName{Port: "gw-1/enp2s1"}},
			}},
		}},
	}

	for _, tt := range []struct {
		name    string
		gw      string
		objects []kclient.Object
		err     string
	}{
		{name: "no connections", gw: "gw-1"},
		{name: "connection to another gateway", gw: "gw-2", objects: []kclient.Object{gwConn}},
		{name: "connection to the gateway", gw: "gw-1", objects: []kclient.Object{gwConn}, err: "connection spine-01--gateway--gw-1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := &GatewayWebhook{Reader: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()}

			_, err := w.ValidateDelete(t.Context(), &gwapi.Gateway{
				ObjectMeta: kmetav1.ObjectMeta{Name: tt.gw, Namespace: kmetav1.NamespaceDefault},
			})

			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tt.err)
		})
	}
}
