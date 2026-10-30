// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// gateways in two fabrics, the default one with two domains:
//   - gw-1, gw-2: default/default, group g1
//   - gw-3: default/d2, group g2
//   - gw-4: other/default, group g3
func gatewayTestKube(t *testing.T, extra ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, gwapi.AddToScheme(scheme))
	// what the reconciler deploys for the gateways
	require.NoError(t, gwintapi.AddToScheme(scheme))
	require.NoError(t, appv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))

	objMeta := func(name string) kmetav1.ObjectMeta {
		return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
	}

	gw := func(name, fabric, domain, group string) *gwapi.Gateway {
		gw := &gwapi.Gateway{
			ObjectMeta: objMeta(name),
			Spec: gwapi.GatewaySpec{
				Topology: gwapi.GatewayTopology{Fabric: fabric, Domain: domain},
				Groups:   []gwapi.GatewayGroupMembership{{Name: group}},
			},
		}
		gw.Default()

		return gw
	}

	vpcInfo := func(name, fabric string, domains ...string) *gwapi.VPCInfo {
		vpc := &gwapi.VPCInfo{
			ObjectMeta: objMeta(name),
			Spec: gwapi.VPCInfoSpec{
				Topology: gwapi.VPCInfoTopology{Fabric: fabric, Domains: domains},
				VNI:      100,
			},
			Status: gwapi.VPCInfoStatus{InternalID: name},
		}
		vpc.Default()

		return vpc
	}

	peering := func(name, fabric, vpc1, vpc2 string) *gwapi.GatewayPeering {
		p := &gwapi.GatewayPeering{
			ObjectMeta: objMeta(name),
			Spec: gwapi.PeeringSpec{
				Topology: gwapi.GatewayPeeringTopology{Fabric: fabric},
				Peering:  map[string]*gwapi.PeeringEntry{vpc1: {}, vpc2: {}},
			},
		}
		p.Default()

		return p
	}

	objs := []kclient.Object{ //nolint:prealloc
		&wiringapi.Fabric{
			ObjectMeta: objMeta(wiringapi.DefaultFabric),
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}, "d2": {}}},
		},
		&wiringapi.Fabric{
			ObjectMeta: objMeta("other"),
			Spec:       wiringapi.FabricSpec{Domains: map[string]wiringapi.FabricDomainSpec{wiringapi.DefaultFabricDomain: {}}},
		},

		gw("gw-1", "", "", "g1"),
		gw("gw-2", "", "", "g1"),
		gw("gw-3", "", "d2", "g2"),
		gw("gw-4", "other", "", "g3"),

		vpcInfo("vpc-1", "", wiringapi.DefaultFabricDomain),
		vpcInfo("vpc-2", "", wiringapi.DefaultFabricDomain),
		vpcInfo("vpc-3", "", "d2"),
		// in both domains of the default fabric
		vpcInfo("vpc-4", "", wiringapi.DefaultFabricDomain, "d2"),
		vpcInfo("vpc-5", "other", wiringapi.DefaultFabricDomain),

		peering("vpc-1--vpc-2", "", "vpc-1", "vpc-2"),
		peering("vpc-1--vpc-4", "", "vpc-1", "vpc-4"),
		peering("vpc-3--vpc-4", "", "vpc-3", "vpc-4"),
		peering("vpc-5--vpc-6", "other", "vpc-5", "vpc-6"),
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objs, extra...)...).Build()
}

func TestBuildGatewayAgentScope(t *testing.T) {
	cfg := &meta.FabricConfig{}

	for _, tt := range []struct {
		name     string
		gw       string
		vpcs     []string
		peerings []string
		groups   map[string][]string
	}{
		{
			name:     "default domain",
			gw:       "gw-1",
			vpcs:     []string{"vpc-1", "vpc-2", "vpc-4"},
			peerings: []string{"vpc-1--vpc-2", "vpc-1--vpc-4"},
			groups:   map[string][]string{"g1": {"gw-1", "gw-2"}},
		},
		{
			name:     "another domain",
			gw:       "gw-3",
			vpcs:     []string{"vpc-3", "vpc-4"},
			peerings: []string{"vpc-3--vpc-4"},
			groups:   map[string][]string{"g2": {"gw-3"}},
		},
		{
			// the peering of the other fabric has a VPC that doesn't exist, so it's skipped there too
			name:   "another fabric",
			gw:     "gw-4",
			vpcs:   []string{"vpc-5"},
			groups: map[string][]string{"g3": {"gw-4"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kube := gatewayTestKube(t)

			gw := &gwapi.Gateway{}
			require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: tt.gw, Namespace: kmetav1.NamespaceDefault}, gw))

			ag, err := BuildGatewayAgent(t.Context(), kube, cfg, gw)
			require.NoError(t, err)

			require.Equal(t, tt.vpcs, slices.Sorted(maps.Keys(ag.Spec.VPCs)))
			require.Equal(t, tt.peerings, slices.Sorted(maps.Keys(ag.Spec.Peerings)))

			groups := map[string][]string{}
			for name, info := range ag.Spec.Groups {
				for _, member := range info.Members {
					groups[name] = append(groups[name], member.Name)
				}
			}
			require.Equal(t, tt.groups, groups)
		})
	}
}

// selecting by an empty fabric or domain label matches nothing, so a gateway or peering the refresh on fabric-ctrl
// initialization had to leave alone would get an empty config without these errors
func TestBuildGatewayAgentTopologyRequired(t *testing.T) {
	kube := gatewayTestKube(t)
	cfg := &meta.FabricConfig{}

	gw := &gwapi.Gateway{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "gw-1", Namespace: kmetav1.NamespaceDefault}, gw))

	withoutDomain := gw.DeepCopy()
	withoutDomain.Spec.Topology.Domain = ""
	_, err := BuildGatewayAgent(t.Context(), kube, cfg, withoutDomain)
	require.ErrorContains(t, err, "gateway gw-1 has no domain")

	withoutFabric := gw.DeepCopy()
	withoutFabric.Spec.Topology.Fabric = ""
	_, err = BuildGatewayAgent(t.Context(), kube, cfg, withoutFabric)
	require.ErrorIs(t, err, wiringapi.ErrFabricNotSet)

	peering := &gwapi.GatewayPeering{Name: "vpc-1--vpc-2", Namespace: kmetav1.NamespaceDefault}
	_, err = BuildGatewayAgentForPeering(t.Context(), kube, cfg, peering)
	require.ErrorContains(t, err, "peering vpc-1--vpc-2 has no fabric")
}

func TestBuildGatewayAgentNotReadyElsewhere(t *testing.T) {
	notReady := &gwapi.VPCInfo{
		Name: "vpc-6", Namespace: kmetav1.NamespaceDefault,
		Spec: gwapi.VPCInfoSpec{
			Topology: gwapi.VPCInfoTopology{Domains: []string{"d2"}},
			VNI:      100,
		},
	}
	notReady.Default()

	kube := gatewayTestKube(t, notReady)
	cfg := &meta.FabricConfig{}

	gw := &gwapi.Gateway{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "gw-1", Namespace: kmetav1.NamespaceDefault}, gw))
	_, err := BuildGatewayAgent(t.Context(), kube, cfg, gw)
	require.NoError(t, err, "a VPCInfo of another domain doesn't block the gateway")

	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "gw-3", Namespace: kmetav1.NamespaceDefault}, gw))
	_, err = BuildGatewayAgent(t.Context(), kube, cfg, gw)
	require.ErrorIs(t, err, ErrRetryLater)
}

// the gateway webhooks build the agent of a gateway that isn't stored yet or is stored with its old spec
func TestBuildGatewayAgentGivenGateway(t *testing.T) {
	kube := gatewayTestKube(t)
	cfg := &meta.FabricConfig{}

	gw := &gwapi.Gateway{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "gw-1", Namespace: kmetav1.NamespaceDefault}, gw))

	members := func(gw *gwapi.Gateway) map[string][]string {
		ag, err := BuildGatewayAgent(t.Context(), kube, cfg, gw)
		require.NoError(t, err)

		res := map[string][]string{}
		for name, info := range ag.Spec.Groups {
			for _, member := range info.Members {
				res[name] = append(res[name], member.Name)
			}
		}

		return res
	}

	created := gw.DeepCopy()
	created.Name = "gw-5"
	created.Spec.Groups = []gwapi.GatewayGroupMembership{{Name: "g1", Priority: 1}, {Name: "g4"}}
	require.Equal(t, map[string][]string{"g1": {"gw-1", "gw-2", "gw-5"}, "g4": {"gw-5"}}, members(created))

	updated := gw.DeepCopy()
	updated.Spec.Groups = []gwapi.GatewayGroupMembership{{Name: "g1", Priority: 1}, {Name: "g4"}}
	require.Equal(t, map[string][]string{"g1": {"gw-2", "gw-1"}, "g4": {"gw-1"}}, members(updated))
}

func TestGatewayEnqueue(t *testing.T) {
	kube := gatewayTestKube(t)
	r := &GatewayReconciler{Client: kube, lock: unlockedLock()}

	get := func(obj kclient.Object, name string) kclient.Object {
		require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: name, Namespace: kmetav1.NamespaceDefault}, obj))

		return obj
	}
	defaulted := func(vpcInfo *gwapi.VPCInfo) kclient.Object {
		vpcInfo.Default()

		return vpcInfo
	}

	for _, tt := range []struct {
		name    string
		obj     kclient.Object
		enqueue func(context.Context, kclient.Object) []reconcile.Request
		gws     []string
	}{
		{name: "gateway", obj: get(&gwapi.Gateway{}, "gw-1"), enqueue: r.enqueueForGateway, gws: []string{"gw-1", "gw-2"}},
		{name: "peering", obj: get(&gwapi.GatewayPeering{}, "vpc-1--vpc-2"), enqueue: r.enqueueForPeering, gws: []string{"gw-1", "gw-2", "gw-3"}},
		{name: "vpcinfo in a domain", obj: get(&gwapi.VPCInfo{}, "vpc-3"), enqueue: r.enqueueForVPCInfo, gws: []string{"gw-3"}},
		{name: "vpcinfo in two domains", obj: get(&gwapi.VPCInfo{}, "vpc-4"), enqueue: r.enqueueForVPCInfo, gws: []string{"gw-1", "gw-2", "gw-3"}},
		{
			// created without a topology, so defaulted to the default fabric and domain
			name:    "vpcinfo without topology",
			obj:     defaulted(&gwapi.VPCInfo{Name: "vpc-7", Namespace: kmetav1.NamespaceDefault}),
			enqueue: r.enqueueForVPCInfo,
			gws:     []string{"gw-1", "gw-2"},
		},
		{name: "fabric", obj: get(&wiringapi.Fabric{}, "other"), enqueue: r.enqueueForFabric, gws: []string{"gw-4"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gws := []string{}
			for _, req := range tt.enqueue(t.Context(), tt.obj) {
				gws = append(gws, req.Name)
			}
			slices.Sort(gws)

			require.Equal(t, tt.gws, gws)
		})
	}
}

func TestDataplaneIfaceArgs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		driver gwapi.GatewayDriver
		ifaces map[string]gwapi.GatewayInterface
		want   []string
	}{
		{
			name:   "dpdk",
			driver: gwapi.GatewayDriverDPDK,
			ifaces: map[string]gwapi.GatewayInterface{"port1": {PCI: "0000:21:00.1"}, "port0": {PCI: "0000:21:00.0", Kernel: "eth0"}},
			want:   []string{"--interface", "port0=pci@0000:21:00.0", "--interface", "port1=pci@0000:21:00.1", "--driver", "dpdk"},
		},
		{
			name:   "kernel",
			driver: gwapi.GatewayDriverKernel,
			ifaces: map[string]gwapi.GatewayInterface{"port0": {Kernel: "eth0", PCI: "0000:21:00.0"}, "eth1": {}, "eth2": {PCI: "0000:21:00.2"}, "eth3": {Kernel: "eth3"}},
			want:   []string{"--interface", "eth1", "--interface", "eth2", "--interface", "eth3", "--interface", "port0=kernel@eth0", "--driver", "kernel"},
		},
		{
			name:   "mtu",
			driver: gwapi.GatewayDriverDPDK,
			ifaces: map[string]gwapi.GatewayInterface{"port0": {PCI: "0000:21:00.0", MTU: 9036}},
			want:   []string{"--interface", "port0=pci@0000:21:00.0/mtu=9036", "--driver", "dpdk"},
		},
		{
			name:   "mtu-bare-name",
			driver: gwapi.GatewayDriverKernel,
			ifaces: map[string]gwapi.GatewayInterface{"enp2s1": {MTU: 9036}},
			want:   []string{"--interface", "enp2s1/mtu=9036", "--driver", "kernel"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gw := &gwapi.Gateway{Spec: gwapi.GatewaySpec{Driver: tt.driver, Interfaces: tt.ifaces}}
			require.Equal(t, tt.want, dataplaneIfaceArgs(gw))
		})
	}
}

func TestDataplaneHugePages(t *testing.T) {
	quantities := func(list corev1.ResourceList) map[string]string {
		res := map[string]string{}
		for name, q := range list {
			res[string(name)] = q.String()
		}

		return res
	}

	for _, tt := range []struct {
		name   string
		hp1G   int64
		hp2M   int64
		driver gwapi.GatewayDriver
		limits map[string]string
		args   []string
	}{
		{
			name:   "none",
			driver: gwapi.GatewayDriverDPDK,
		},
		{
			name:   "1G-dpdk",
			hp1G:   4,
			driver: gwapi.GatewayDriverDPDK,
			limits: map[string]string{"hugepages-1Gi": "4Gi"},
			args:   []string{"--hugepages-gib", "4"},
		},
		{
			name:   "1G-kernel",
			hp1G:   4,
			driver: gwapi.GatewayDriverKernel,
			limits: map[string]string{"hugepages-1Gi": "4Gi"},
		},
		{
			name:   "2M-only",
			hp2M:   512,
			driver: gwapi.GatewayDriverDPDK,
			limits: map[string]string{"hugepages-2Mi": "1Gi"},
		},
		{
			name:   "both",
			hp1G:   2,
			hp2M:   8,
			driver: gwapi.GatewayDriverDPDK,
			limits: map[string]string{"hugepages-1Gi": "2Gi", "hugepages-2Mi": "16Mi"},
			args:   []string{"--hugepages-gib", "2"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &meta.FabricConfig{GatewayHugePages1G: tt.hp1G, GatewayHugePages2M: tt.hp2M}
			gw := &gwapi.Gateway{Spec: gwapi.GatewaySpec{Driver: tt.driver}}

			resources, args := dataplaneHugePages(cfg, gw)
			require.Equal(t, tt.args, args)
			if tt.limits == nil {
				require.Equal(t, corev1.ResourceRequirements{}, resources)

				return
			}

			requests := maps.Clone(tt.limits)
			requests["memory"] = "1Gi"
			require.Equal(t, tt.limits, quantities(resources.Limits))
			require.Equal(t, requests, quantities(resources.Requests))
		})
	}
}

func TestGatewayDeploy(t *testing.T) {
	oldDS := func(name string) *appv1.DaemonSet {
		return &appv1.DaemonSet{Namespace: "fab", Name: name, Finalizers: []string{testFinalizer}}
	}
	kube := gatewayTestKube(t, oldDS(entityName("gw-1", "dataplane")), oldDS(entityName("gw-1", "frr")))
	r := &GatewayReconciler{
		Client: kube,
		cfg: &meta.FabricConfig{
			GatewayNamespace:     "fab",
			DataplaneRef:         "dataplane:v1",
			DataplaneMetricsPort: 9442,
			FRRMetricsPort:       9342,
			GatewayHugePages1G:   4,
		},
		lock: unlockedLock(),
	}

	// the old daemonsets are deleted and waited for before the new one is created
	for range 2 {
		res, err := r.Reconcile(t.Context(), reconcileReq("gw-1"))
		require.NoError(t, err)
		require.NotZero(t, res.RequeueAfter)
		requireNotFound(t, kube, &appv1.DaemonSet{}, "fab", gwapi.DaemonSetName("gw-1"))
	}
	for _, name := range oldGatewayDSNames("gw-1") {
		ds := &appv1.DaemonSet{}
		require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Namespace: "fab", Name: name}, ds))
		require.NotNil(t, ds.DeletionTimestamp, name)
		ds.Finalizers = nil
		require.NoError(t, kube.Update(t.Context(), ds))
	}

	res, err := r.Reconcile(t.Context(), reconcileReq("gw-1"))
	require.NoError(t, err)
	require.Zero(t, res.RequeueAfter)

	ds := &appv1.DaemonSet{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Namespace: "fab", Name: gwapi.DaemonSetName("gw-1")}, ds))
	require.Equal(t, map[string]string{"app.kubernetes.io/name": "gw--gw-1"}, ds.Spec.Template.Labels)

	spec := ds.Spec.Template.Spec
	require.Len(t, spec.InitContainers, 1)
	require.Equal(t, "flush-vtepip", spec.InitContainers[0].Name)
	require.Equal(t, "dataplane:v1", spec.InitContainers[0].Image)
	require.True(t, spec.HostNetwork)
	require.Len(t, spec.Containers, 2)

	dp := spec.Containers[0]
	require.Equal(t, "dataplane", dp.Name)
	require.Equal(t, "dataplane:v1", dp.Image)
	require.Empty(t, dp.Command)
	require.Contains(t, dp.Args, "--supervise-frr")
	require.NotContains(t, dp.Args, "--hugepages-gib", "kernel driver doesn't use hugepages")
	hp1G := dp.Resources.Limits["hugepages-1Gi"]
	require.Equal(t, "4Gi", hp1G.String())
	require.NotContains(t, dp.Resources.Limits, corev1.ResourceMemory)
	require.Equal(t, []corev1.VolumeMount{
		{Name: frrRunVolumeName, MountPath: "/run/frr"},
		{Name: dataplaneTmpVolumeName, MountPath: "/tmp"},
		{Name: frrTmpVolumeName, MountPath: "/var/tmp/frr"},
	}, dp.VolumeMounts)

	exporter := spec.Containers[1]
	require.Equal(t, "frr-exporter", exporter.Name)
	require.Equal(t, "dataplane:v1", exporter.Image)
	require.Equal(t, []corev1.VolumeMount{{Name: frrRunVolumeName, MountPath: "/run/frr"}}, exporter.VolumeMounts)

	for _, vol := range spec.Volumes {
		require.NotNil(t, vol.EmptyDir, vol.Name)
	}
}

func TestBuildGatewayAgentPeeringAnnotations(t *testing.T) {
	kube := gatewayTestKube(t)

	peering := &gwapi.GatewayPeering{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Name: "vpc-1--vpc-2", Namespace: kmetav1.NamespaceDefault}, peering))
	peering.Spec.GatewayGroup = "g1"
	peering.Annotations = map[string]string{
		"fabric.githedgehog.com/foo":  "bar",
		"gateway.githedgehog.com/baz": "qux",
		"example.com/skip":            "me",
		gwapi.LabelVPCs:               "vpc-1,vpc-2",
		gwapi.LabelNAT:                "masquerade",
	}

	ag, err := BuildGatewayAgentForPeering(t.Context(), kube, &meta.FabricConfig{}, peering)
	require.NoError(t, err)

	require.Equal(t, map[string]string{
		"fabric.githedgehog.com/foo":  "bar",
		"gateway.githedgehog.com/baz": "qux",
	}, ag.Spec.Peerings["vpc-1--vpc-2"].Annotations)
	require.Equal(t, peering.Spec, ag.Spec.Peerings["vpc-1--vpc-2"].PeeringSpec)
	// only has the ones set by the peering defaulting
	require.Nil(t, ag.Spec.Peerings["vpc-1--vpc-4"].Annotations)
}
