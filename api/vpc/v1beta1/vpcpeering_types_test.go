// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/testutil"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVPCPeeringValidateLimits(t *testing.T) {
	subnets := func(n int) []string {
		out := make([]string, 0, n)
		for i := range n {
			out = append(out, fmt.Sprintf("subnet-%d", i))
		}

		return out
	}

	for _, tt := range []struct {
		name string
		f    func(peering *v1beta1.VPCPeering)
		err  string
	}{
		{
			name: "max permits and subnets",
			f: func(peering *v1beta1.VPCPeering) {
				for range 9 {
					peering.Spec.Permit = append(peering.Spec.Permit, map[string]v1beta1.VPCPeer{"vpc-1": {}, "vpc-2": {}})
				}
				peering.Spec.Permit[0]["vpc-1"] = v1beta1.VPCPeer{Subnets: subnets(10)}
			},
		},
		{
			name: "too many permits",
			f: func(peering *v1beta1.VPCPeering) {
				for range 10 {
					peering.Spec.Permit = append(peering.Spec.Permit, map[string]v1beta1.VPCPeer{"vpc-1": {}, "vpc-2": {}})
				}
			},
			err: "spec.permit: Too many: 11: must have at most 10 items",
		},
		{
			name: "too many subnets",
			f: func(peering *v1beta1.VPCPeering) {
				peering.Spec.Permit[0]["vpc-2"] = v1beta1.VPCPeer{Subnets: subnets(11)}
			},
			err: "spec.permit[0][vpc-2].subnets: Too many: 11: must have at most 10 items",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			peering := vpcPeeringGen("peering", "vpc-1", "vpc-2")
			tt.f(peering)

			_, err := peering.Validate(t.Context(), nil, nil)
			if tt.err == "" {
				require.NoError(t, err)

				return
			}
			require.EqualError(t, err, tt.err)
			testutil.RequireFieldError(t, err, peering)
		})
	}
}

func TestVPCPeeringValidateMissingSubnet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wiringapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(otherFabricObjs(), vpcGen("vpc-02"))...).Build()

	peering := vpcPeeringGen("peering", "vpc-01", "vpc-02")
	peering.Spec.Permit[0]["vpc-02"] = v1beta1.VPCPeer{Subnets: []string{"default", "typo"}}

	_, err := peering.Validate(t.Context(), kube, &meta.FabricConfig{
		ExtraValidators: meta.ExtraValidators{
			Peering: func(context.Context, kclient.Reader, kclient.Object) error { return nil },
		},
	})
	require.EqualError(t, err, `spec.permit[0][vpc-02].subnets[1]: Not found: "typo"`)
	testutil.RequireFieldError(t, err, peering)
}
