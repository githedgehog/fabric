// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package valid

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func checkPeering(peering kclient.Object) func(context.Context, kclient.Reader) error {
	return func(ctx context.Context, kube kclient.Reader) error {
		return Peering(ctx, kube, peering)
	}
}

// peeringName returns the name override if passed, otherwise generates one from the members, e.g. "vpc-1--vpc-2"
func peeringName(member1, member2 string, nameOverride ...string) string {
	if len(nameOverride) > 0 {
		return nameOverride[0]
	}

	members := []string{member1, member2}
	slices.Sort(members)

	return strings.Join(members, "--")
}

func gwExt(ext string) string {
	return gwapi.VPCInfoExtPrefix + ext
}

func vpcPeering(vpc1, vpc2 string, nameOverride ...string) *vpcapi.VPCPeering {
	peering := &vpcapi.VPCPeering{
		ObjectMeta: kmetav1.ObjectMeta{Name: peeringName(vpc1, vpc2, nameOverride...)},
		Spec: vpcapi.VPCPeeringSpec{
			Permit: []map[string]vpcapi.VPCPeer{
				{vpc1: {}, vpc2: {}},
			},
		},
	}
	peering.Default()

	return peering
}

func extPeering(vpc, ext string, nameOverride ...string) *vpcapi.ExternalPeering { //nolint:unparam
	peering := &vpcapi.ExternalPeering{
		ObjectMeta: kmetav1.ObjectMeta{Name: peeringName(vpc, ext, nameOverride...)},
		Spec: vpcapi.ExternalPeeringSpec{
			Permit: vpcapi.ExternalPeeringSpecPermit{
				VPC:      vpcapi.ExternalPeeringSpecVPC{Name: vpc},
				External: vpcapi.ExternalPeeringSpecExternal{Name: ext},
			},
		},
	}
	peering.Default()

	return peering
}

// gwPeering members are VPC names, use gwExt to pass an external
func gwPeering(member1, member2 string, nameOverride ...string) *gwapi.GatewayPeering {
	peering := &gwapi.GatewayPeering{
		ObjectMeta: kmetav1.ObjectMeta{Name: peeringName(member1, member2, nameOverride...)},
		Spec: gwapi.PeeringSpec{
			Peering: map[string]*gwapi.PeeringEntry{
				member1: {},
				member2: {},
			},
		},
	}
	peering.Default()

	return peering
}

func TestValidatePeering(t *testing.T) {
	tests := []struct {
		name    string
		check   func(context.Context, kclient.Reader) error
		objects []kclient.Object
		err     bool
	}{
		{
			name:    "vpc-peering-no-conflict",
			check:   checkPeering(vpcPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{vpcPeering("vpc-1", "vpc-3")},
		},
		{
			name:    "ext-peering-no-conflict",
			check:   checkPeering(extPeering("vpc-1", "ext-1")),
			objects: []kclient.Object{extPeering("vpc-1", "ext-2")},
		},
		{
			name:    "gw-peering-no-conflict",
			check:   checkPeering(gwPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{gwPeering("vpc-1", "vpc-3")},
		},
		{
			name:    "gw-peering-ext-no-conflict",
			check:   checkPeering(gwPeering("vpc-1", gwExt("ext-1"))),
			objects: []kclient.Object{gwPeering("vpc-1", gwExt("ext-2"))},
		},
		{
			name:    "vpc-peering-update-self",
			check:   checkPeering(vpcPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{vpcPeering("vpc-1", "vpc-2")},
		},
		{
			name:    "ext-peering-update-self",
			check:   checkPeering(extPeering("vpc-1", "ext-1")),
			objects: []kclient.Object{extPeering("vpc-1", "ext-1")},
		},
		{
			name:    "gw-peering-update-self",
			check:   checkPeering(gwPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{gwPeering("vpc-1", "vpc-2")},
		},
		{
			name:    "vpc-peering-conflicting-vpc-peering",
			check:   checkPeering(vpcPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{vpcPeering("vpc-2", "vpc-1", "other-name")},
			err:     true,
		},
		{
			name:    "ext-peering-conflicting-ext-peering",
			check:   checkPeering(extPeering("vpc-1", "ext-1")),
			objects: []kclient.Object{extPeering("vpc-1", "ext-1", "other-name")},
			err:     true,
		},
		{
			name:    "gw-peering-conflicting-gw-peering",
			check:   checkPeering(gwPeering("vpc-1", gwExt("ext-1"))),
			objects: []kclient.Object{gwPeering(gwExt("ext-1"), "vpc-1", "other-name")},
			err:     true,
		},
		{
			name:    "gw-peering-conflicting-vpc-peering",
			check:   checkPeering(gwPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{vpcPeering("vpc-1", "vpc-2")},
			err:     true,
		},
		{
			name:    "vpc-peering-conflicting-gw-peering",
			check:   checkPeering(vpcPeering("vpc-1", "vpc-2")),
			objects: []kclient.Object{gwPeering("vpc-1", "vpc-2")},
			err:     true,
		},
		{
			name:    "gw-peering-conflicting-ext-peering",
			check:   checkPeering(gwPeering("vpc-1", gwExt("ext-1"))),
			objects: []kclient.Object{extPeering("vpc-1", "ext-1")},
			err:     true,
		},
		{
			name:    "ext-peering-conflicting-gw-peering",
			check:   checkPeering(extPeering("vpc-1", "ext-1")),
			objects: []kclient.Object{gwPeering("vpc-1", gwExt("ext-1"))},
			err:     true,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(test.objects...).
				Build()

			err := test.check(t.Context(), kube)
			if test.err {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "unexpected error during validation")
			}
		})
	}
}
