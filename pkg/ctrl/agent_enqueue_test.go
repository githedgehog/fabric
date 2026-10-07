// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func enqueueTestMeta(name string) kmetav1.ObjectMeta {
	return kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault}
}

func enqueueTestSwitch(name string, role wiringapi.SwitchRole, fabric string, domains ...string) *wiringapi.Switch {
	sw := &wiringapi.Switch{
		ObjectMeta: enqueueTestMeta(name),
		Spec: wiringapi.SwitchSpec{
			Role:     role,
			Topology: wiringapi.SwitchTopology{Fabric: fabric, Domains: domains},
		},
	}
	// sets the fabric and domain labels the switches are selected by
	sw.Default()

	return sw
}

func enqueueTestConn(name string, spec wiringapi.ConnectionSpec) *wiringapi.Connection {
	conn := &wiringapi.Connection{ObjectMeta: enqueueTestMeta(name), Spec: spec}
	// sets the switch, type and VPC labels
	conn.Default()

	return conn
}

// enqueuedNames returns the names the map function enqueues
func enqueuedNames(t *testing.T, enqueue func(context.Context, kclient.Object) []reconcile.Request, obj kclient.Object) []string {
	t.Helper()

	names := []string{}
	for _, req := range enqueue(t.Context(), obj) {
		names = append(names, req.Name)
	}

	return names
}

// agentEnqueueTestKube builds the fixture for the agent map functions:
//   - leaf-1, leaf-2: default fabric, default domain
//   - leaf-3: default fabric, plane-b domain
//   - spine-1: default fabric, default domain
//   - leaf-4: backend fabric
//   - server-1 unbundled on leaf-1, server-2 eslag on leaf-1 and leaf-2, external on leaf-3, static external on leaf-4
func agentEnqueueTestKube(t *testing.T, extra ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))
	require.NoError(t, vpcapi.AddToScheme(scheme))

	link := func(server, sw string) wiringapi.ServerToSwitchLink {
		return wiringapi.ServerToSwitchLink{Server: wiringapi.NewBasePortName(server), Switch: wiringapi.NewBasePortName(sw)}
	}

	objs := []kclient.Object{
		enqueueTestSwitch("leaf-1", wiringapi.SwitchRoleServerLeaf, wiringapi.DefaultFabric, wiringapi.DefaultFabricDomain),
		enqueueTestSwitch("leaf-2", wiringapi.SwitchRoleServerLeaf, wiringapi.DefaultFabric, wiringapi.DefaultFabricDomain),
		enqueueTestSwitch("leaf-3", wiringapi.SwitchRoleServerLeaf, wiringapi.DefaultFabric, "plane-b"),
		enqueueTestSwitch("spine-1", wiringapi.SwitchRoleSpine, wiringapi.DefaultFabric, wiringapi.DefaultFabricDomain),
		enqueueTestSwitch("leaf-4", wiringapi.SwitchRoleServerLeaf, "backend", wiringapi.DefaultFabricDomain),

		enqueueTestConn("server-1--unbundled--leaf-1", wiringapi.ConnectionSpec{
			Unbundled: &wiringapi.ConnUnbundled{Link: link("server-1/enp2s1", "leaf-1/E1/1")},
		}),
		enqueueTestConn("server-2--eslag--leaf-1--leaf-2", wiringapi.ConnectionSpec{
			ESLAG: &wiringapi.ConnESLAG{Links: []wiringapi.ServerToSwitchLink{
				link("server-2/enp2s1", "leaf-1/E1/2"),
				link("server-2/enp2s2", "leaf-2/E1/2"),
			}},
		}),
		enqueueTestConn("leaf-3--external", wiringapi.ConnectionSpec{
			External: &wiringapi.ConnExternal{Link: wiringapi.ConnExternalLink{Switch: wiringapi.NewBasePortName("leaf-3/E1/5")}},
		}),
		enqueueTestConn("leaf-4--static-external", wiringapi.ConnectionSpec{
			StaticExternal: &wiringapi.ConnStaticExternal{Link: wiringapi.ConnStaticExternalLink{
				Switch: wiringapi.ConnStaticExternalLinkSwitch{BasePortName: wiringapi.NewBasePortName("leaf-4/E1/6")},
			}},
		}),
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(append(objs, extra...)...).Build()
}

func TestAgentEnqueueByConnection(t *testing.T) {
	kube := agentEnqueueTestKube(t)
	r := &AgentReconciler{Client: kube, lock: unlockedLock()}

	get := func(name string) *wiringapi.Connection {
		conn := &wiringapi.Connection{}
		require.NoError(t, kube.Get(t.Context(), objKey(name), conn))

		return conn
	}

	require.Equal(t, []string{"leaf-1"}, enqueuedNames(t, r.enqueueBySwitchListLabelsAndSpines, get("server-1--unbundled--leaf-1")))
	require.Equal(t, []string{"leaf-1", "leaf-2"}, enqueuedNames(t, r.enqueueBySwitchListLabelsAndSpines, get("server-2--eslag--leaf-1--leaf-2")))
	// a static external outside of a VPC is configured on all spines too
	require.Equal(t, []string{"leaf-4", "spine-1"}, enqueuedNames(t, r.enqueueBySwitchListLabelsAndSpines, get("leaf-4--static-external")))
}

func TestAgentEnqueueByAttachment(t *testing.T) {
	vpcAttach := func(name, conn string) *vpcapi.VPCAttachment {
		a := &vpcapi.VPCAttachment{
			ObjectMeta: enqueueTestMeta(name),
			Spec:       vpcapi.VPCAttachmentSpec{Subnet: "vpc-1/subnet-1", Connection: conn},
		}
		a.Default()

		return a
	}

	kube := agentEnqueueTestKube(t)
	r := &AgentReconciler{Client: kube, lock: unlockedLock()}

	// only the switches of the attachment's connection get it
	require.Equal(t, []string{"leaf-1"}, enqueuedNames(t, r.enqueueByAttachment, vpcAttach("attach-1", "server-1--unbundled--leaf-1")))
	require.Equal(t, []string{"leaf-1", "leaf-2"}, enqueuedNames(t, r.enqueueByAttachment, vpcAttach("attach-2", "server-2--eslag--leaf-1--leaf-2")))
	require.Equal(t, []string{"leaf-3"}, enqueuedNames(t, r.enqueueByAttachment, &vpcapi.ExternalAttachment{
		ObjectMeta: enqueueTestMeta("ext-attach-1"),
		Spec:       vpcapi.ExternalAttachmentSpec{External: "ext-1", Connection: "leaf-3--external"},
	}))

	// without its connection it isn't on any switch
	require.Empty(t, enqueuedNames(t, r.enqueueByAttachment, vpcAttach("attach-3", "server-9--unbundled--leaf-1")))
}
