// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/testutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaulting sorts the subnets, so the errors can't point at the index the user wrote
func TestIPv4NamespaceValidateSubnets(t *testing.T) {
	reservedCfg := &meta.FabricConfig{ReservedSubnets: []string{"192.168.0.0/16"}}
	require.NoError(t, reservedCfg.WithReservedSubnets())

	for _, tt := range []struct {
		name    string
		subnets []string
		err     string
	}{
		{name: "invalid subnet", subnets: []string{"10.2.0.0/16", "10.1.0.0/33"}, err: `spec.subnets: Invalid value: "10.1.0.0/33"`},
		{name: "reserved subnet", subnets: []string{"192.168.1.0/24", "10.1.0.0/16"}, err: `spec.subnets: Invalid value: "192.168.1.0/24": overlaps with reserved subnet 192.168.0.0/16`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ns := defaulted(&v1beta1.IPv4Namespace{
				Name: "ns-1", Namespace: kmetav1.NamespaceDefault,
				Spec: v1beta1.IPv4NamespaceSpec{Subnets: tt.subnets},
			})

			_, err := ns.Validate(t.Context(), nil, reservedCfg)
			require.ErrorContains(t, err, tt.err)
			testutil.RequireFieldError(t, err, ns)
		})
	}
}
