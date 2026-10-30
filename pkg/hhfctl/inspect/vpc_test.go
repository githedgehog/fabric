// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVPCSubnetAttachments(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	// stored attachments are always defaulted, which sets the vpc and subnet labels
	attachGen := func(name, subnet string) kclient.Object {
		attach := &vpcapi.VPCAttachment{Spec: vpcapi.VPCAttachmentSpec{Subnet: subnet, Connection: name}}
		attach.Name, attach.Namespace = name, kmetav1.NamespaceDefault
		attach.Default()

		return attach
	}

	vpc := &vpcapi.VPC{Spec: vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{
		"subnet-1": {Subnet: "10.0.1.0/24"},
		"subnet-2": {Subnet: "10.0.2.0/24"},
	}}}
	vpc.Name, vpc.Namespace = "vpc-1", kmetav1.NamespaceDefault

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		vpc,
		attachGen("attach-1", "vpc-1/subnet-1"),
		attachGen("attach-2", "vpc-1/subnet-2"),
		attachGen("attach-3", "vpc-2/subnet-1"),
	).Build()

	attaches := func(out *VPCOut) []string {
		return slices.Sorted(maps.Keys(out.VPCAttachments))
	}

	out, err := VPC(t.Context(), kube, VPCIn{Name: "vpc-1"})
	require.NoError(t, err)
	require.Equal(t, []string{"attach-1", "attach-2"}, attaches(out))

	out, err = VPC(t.Context(), kube, VPCIn{Name: "vpc-1", Subnet: "subnet-2"})
	require.NoError(t, err)
	require.Equal(t, []string{"attach-2"}, attaches(out))
}

func TestVPCSubnetPeerings(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	// stored peerings are always defaulted, which sets the vpc list labels
	peeringGen := func(name string, subnets ...string) kclient.Object {
		peering := &vpcapi.VPCPeering{Spec: vpcapi.VPCPeeringSpec{Permit: []map[string]vpcapi.VPCPeer{{
			"vpc-1": {Subnets: subnets},
			name:    {},
		}}}}
		peering.Name, peering.Namespace = "vpc-1--"+name, kmetav1.NamespaceDefault
		peering.Default()

		return peering
	}

	vpcGen := func(name string) kclient.Object {
		vpc := &vpcapi.VPC{Spec: vpcapi.VPCSpec{Subnets: map[string]*vpcapi.VPCSubnet{
			"subnet-1": {Subnet: "10.0.1.0/24"},
			"subnet-2": {Subnet: "10.0.2.0/24"},
		}}}
		vpc.Name, vpc.Namespace = name, kmetav1.NamespaceDefault

		return vpc
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		vpcGen("vpc-1"),
		vpcGen("vpc-2"),
		vpcGen("vpc-3"),
		vpcGen("vpc-4"),
		peeringGen("vpc-2"),
		peeringGen("vpc-3", "subnet-1"),
		peeringGen("vpc-4", "subnet-2"),
	).Build()

	peerings := func(out *VPCOut) []string {
		return slices.Sorted(maps.Keys(out.VPCPeerings))
	}

	out, err := VPC(t.Context(), kube, VPCIn{Name: "vpc-1"})
	require.NoError(t, err)
	require.Equal(t, []string{"vpc-1--vpc-2", "vpc-1--vpc-3", "vpc-1--vpc-4"}, peerings(out))

	out, err = VPC(t.Context(), kube, VPCIn{Name: "vpc-1", Subnet: "subnet-2"})
	require.NoError(t, err)
	require.Equal(t, []string{"vpc-1--vpc-2", "vpc-1--vpc-4"}, peerings(out))
}
