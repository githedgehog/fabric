// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCheckUpgradeFresh(t *testing.T) {
	cfg := initFabricConfig()
	kube := fake.NewClientBuilder().WithScheme(initScheme(t)).Build()

	problems, err := CheckUpgrade(t.Context(), kube, cfg)
	require.NoError(t, err)
	require.Empty(t, problems, "the built-in switch profiles are valid on their own")

	profiles := switchprofile.NewDefaultSwitchProfiles()
	require.NoError(t, profiles.RegisterAll(t.Context(), kube, cfg))
	sps := &wiringapi.SwitchProfileList{}
	require.NoError(t, kube.List(t.Context(), sps))
	require.Len(t, sps.Items, len(profiles.List()))

	err = kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}, &wiringapi.Fabric{})
	require.True(t, kapierrors.IsNotFound(err), "expected no default fabric, got %v", err)
}

// valid objects of every refreshed kind pass without a client, as the refresh validates them, which keeps upgrades
// from failing on a Validate that starts to require one
func TestCheckUpgradeValid(t *testing.T) {
	const ns = kmetav1.NamespaceDefault

	cfg := initFabricConfig()
	cfg.EnableGateway = true

	scheme := initScheme(t)
	objs := []kclient.Object{
		&wiringapi.Fabric{Namespace: ns, Name: wiringapi.DefaultFabric, Spec: wiringapi.DefaultFabricSpec(cfg)},
		&wiringapi.VLANNamespace{Namespace: ns, Name: "default"},
		&vpcapi.IPv4Namespace{Namespace: ns, Name: "default"},
		&wiringapi.SwitchGroup{Namespace: ns, Name: "mclag-1"},
		&wiringapi.Switch{Namespace: ns, Name: "leaf-01", Spec: wiringapi.SwitchSpec{
			Role: wiringapi.SwitchRoleServerLeaf, Profile: "vs", ASN: 65101,
			IP: "172.30.0.8/21", ProtocolIP: "172.30.8.2/32", VTEPIP: "172.30.12.0/32",
		}},
		&wiringapi.Server{Namespace: ns, Name: "server-01"},
		&wiringapi.Connection{Namespace: ns, Name: "server-01--unbundled--leaf-01", Spec: wiringapi.ConnectionSpec{
			Unbundled: &wiringapi.ConnUnbundled{Link: wiringapi.ServerToSwitchLink{
				Server: wiringapi.NewBasePortName("server-01/enp2s1"), Switch: wiringapi.NewBasePortName("leaf-01/E1/1"),
			}},
		}},
		&vpcapi.VPC{Namespace: ns, Name: "vpc-01", Spec: vpcapi.VPCSpec{
			Subnets: map[string]*vpcapi.VPCSubnet{"default": {Subnet: "10.0.1.0/24", VLAN: 1001}},
		}},
		&vpcapi.VPCAttachment{Namespace: ns, Name: "server-01--vpc-01--default", Spec: vpcapi.VPCAttachmentSpec{
			Subnet: "vpc-01/default", Connection: "server-01--unbundled--leaf-01",
		}},
		&vpcapi.VPCPeering{Namespace: ns, Name: "vpc-01--vpc-02", Spec: vpcapi.VPCPeeringSpec{
			Permit: []map[string]vpcapi.VPCPeer{{"vpc-01": {}, "vpc-02": {}}},
		}},
		&vpcapi.External{Namespace: ns, Name: "ext"},
		&vpcapi.ExternalAttachment{Namespace: ns, Name: "border-01--ext", Spec: vpcapi.ExternalAttachmentSpec{
			External: "ext", Connection: "border-01--external--ext",
			Switch:   vpcapi.ExternalAttachmentSwitch{VLAN: 100, IP: "100.100.0.1/24"},
			Neighbor: vpcapi.ExternalAttachmentNeighbor{ASN: 64102, IP: "100.100.0.6"},
		}},
		&vpcapi.ExternalPeering{Namespace: ns, Name: "vpc-01--ext", Spec: vpcapi.ExternalPeeringSpec{
			Permit: vpcapi.ExternalPeeringSpecPermit{
				VPC: vpcapi.ExternalPeeringSpecVPC{Name: "vpc-01", Subnets: []string{"default"}},
				External: vpcapi.ExternalPeeringSpecExternal{
					Name: "ext", Prefixes: []vpcapi.ExternalPeeringSpecPrefix{{Prefix: "0.0.0.0/0"}},
				},
			},
		}},
		&gwapi.GatewayGroup{Namespace: ns, Name: "default"},
		&gwapi.Gateway{Namespace: ns, Name: "gw-01", Spec: gwapi.GatewaySpec{
			ProtocolIP: "172.30.8.10/32", VTEPIP: "172.30.12.10/32", VTEPMAC: "ca:fe:ba:be:00:01", ASN: 65534,
			Interfaces: map[string]gwapi.GatewayInterface{"enp2s1": {IPs: []string{"172.30.128.1/31"}}},
			Neighbors:  []gwapi.GatewayBGPNeighbor{{Source: "enp2s1", IP: "172.30.128.0", ASN: 65100}},
		}},
		&gwapi.VPCInfo{Namespace: ns, Name: "vpc-01", Spec: gwapi.VPCInfoSpec{
			VNI: 100, Subnets: map[string]*gwapi.VPCInfoSubnet{"default": {CIDR: "10.0.1.0/24"}},
		}},
		&gwapi.GatewayPeering{Namespace: ns, Name: "vpc-01--vpc-02", Spec: gwapi.PeeringSpec{
			GatewayGroup: "default",
			Peering: map[string]*gwapi.PeeringEntry{
				"vpc-01": {Expose: []gwapi.PeeringEntryExpose{{IPs: []gwapi.PeeringEntryIP{{CIDR: "10.0.1.0/24"}}}}},
				"vpc-02": {Expose: []gwapi.PeeringEntryExpose{{IPs: []gwapi.PeeringEntryIP{{CIDR: "10.0.2.0/24"}}}}},
			},
		}},
	}

	// switch profiles are the enforced built-in ones
	kinds := make([]string, 0, len(objs)+1)
	kinds = append(kinds, "SwitchProfile")
	for _, obj := range objs {
		gvk, err := apiutil.GVKForObject(obj, scheme)
		require.NoError(t, err)
		kinds = append(kinds, gvk.Kind)
	}
	for _, k := range refreshKinds {
		require.Contains(t, kinds, k.kind, "a valid %s is missing", k.kind)
	}

	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

	problems, err := CheckUpgrade(t.Context(), kube, cfg)
	require.NoError(t, err)
	require.Empty(t, problems)
}

func TestCheckUpgrade(t *testing.T) {
	cfg := initFabricConfig()

	longName := strings.Repeat("s", meta.MaxNameLength+1)

	deleting := &wiringapi.Server{
		Namespace: kmetav1.NamespaceDefault, Name: strings.Repeat("d", meta.MaxNameLength+1),
		Finalizers:        []string{"fabric.githedgehog.com/cleanup"},
		DeletionTimestamp: &kmetav1.Time{Time: time.Now()},
	}
	loopback := &wiringapi.Connection{
		Namespace: kmetav1.NamespaceDefault, Name: "leaf-01--vpc-loopback",
		Labels: map[string]string{wiringapi.LabelConnectionType: "vpc-loopback"},
	}

	kube := fake.NewClientBuilder().
		WithScheme(initScheme(t)).
		WithObjects(
			&wiringapi.Server{Namespace: kmetav1.NamespaceDefault, Name: longName},
			// predates fabrics and has no spec
			&wiringapi.Switch{Namespace: kmetav1.NamespaceDefault, Name: "leaf-01"},
			loopback,
			deleting,
		).
		Build()

	problems, err := CheckUpgrade(t.Context(), kube, cfg)
	require.NoError(t, err)

	require.Len(t, problems, 2)
	for idx, want := range []fcintapi.FabricControllerProblem{
		{Kind: "Switch", Namespace: kmetav1.NamespaceDefault, Name: "leaf-01", Reason: fcintapi.ProblemInvalid},
		{Kind: "Server", Namespace: kmetav1.NamespaceDefault, Name: longName, Reason: fcintapi.ProblemInvalid},
	} {
		require.NotEmpty(t, problems[idx].Message)
		problems[idx].Message = ""
		require.Equal(t, want, problems[idx])
	}

	fabric := &wiringapi.Fabric{}
	require.NoError(t, kube.Get(t.Context(), ktypes.NamespacedName{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric}, fabric))
	require.Equal(t, wiringapi.DefaultFabricSpec(cfg), fabric.Spec)

	err = kube.Get(t.Context(), kclient.ObjectKeyFromObject(loopback), &wiringapi.Connection{})
	require.True(t, kapierrors.IsNotFound(err), "expected the vpc-loopback connection to be deleted, got %v", err)
}

func TestCheckUpgradeNoConfig(t *testing.T) {
	_, err := CheckUpgrade(t.Context(), fake.NewClientBuilder().WithScheme(initScheme(t)).Build(), nil)
	require.Error(t, err)
}
