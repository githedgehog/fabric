// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"
)

func newTestServer() *Server {
	return &Server{
		subnets:        map[string]*dhcpapi.DHCPSubnet{},
		switchToIP:     map[string]string{},
		relayAllowlist: map[string]struct{}{},
	}
}

func testSubnet(name, circuitID, rv string) *dhcpapi.DHCPSubnet {
	return &dhcpapi.DHCPSubnet{
		ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault, ResourceVersion: rv},
		Spec:       dhcpapi.DHCPSubnetSpec{VRF: "VrfV1", CircuitID: circuitID},
	}
}

func testSwitch(name string, role wiringapi.SwitchRole, ip string) *wiringapi.Switch {
	return &wiringapi.Switch{
		ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
		Spec:       wiringapi.SwitchSpec{Role: role, IP: ip},
	}
}

func TestSubnetHandler(t *testing.T) {
	keyA := subnetKey("VrfV1", "Vlan1000")
	keyB := subnetKey("VrfV1", "Vlan1001")

	t.Run("add stores a copy", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		in := testSubnet("a", "Vlan1000", "1")
		h.OnAdd(in, true)

		require.Contains(t, s.subnets, keyA)
		assert.Equal(t, "1", s.subnets[keyA].ResourceVersion)
		assert.NotNil(t, s.subnets[keyA].Status.Allocated)
		assert.Nil(t, in.Status.Allocated, "informer object must not be modified")
		assert.NotSame(t, in, s.subnets[keyA])
	})

	t.Run("update replaces", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		old := testSubnet("a", "Vlan1000", "1")
		h.OnAdd(old, true)
		h.OnUpdate(old, testSubnet("a", "Vlan1000", "2"))

		require.Contains(t, s.subnets, keyA)
		assert.Equal(t, "2", s.subnets[keyA].ResourceVersion)
	})

	t.Run("update with a new key drops the old one", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		old := testSubnet("a", "Vlan1000", "1")
		h.OnAdd(old, true)
		h.OnUpdate(old, testSubnet("a", "Vlan1001", "2"))

		assert.NotContains(t, s.subnets, keyA)
		assert.Contains(t, s.subnets, keyB)
	})

	t.Run("update with a new key keeps another subnet on the old one", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		old := testSubnet("a", "Vlan1000", "1")
		h.OnAdd(old, true)
		h.OnAdd(testSubnet("b", "Vlan1000", "2"), true)
		h.OnUpdate(old, testSubnet("a", "Vlan1001", "3"))

		require.Contains(t, s.subnets, keyA)
		assert.Equal(t, "b", s.subnets[keyA].Name)
		assert.Contains(t, s.subnets, keyB)
	})

	t.Run("delete", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		in := testSubnet("a", "Vlan1000", "1")
		h.OnAdd(in, true)
		h.OnDelete(in)

		assert.NotContains(t, s.subnets, keyA)
	})

	t.Run("delete missed while disconnected", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		in := testSubnet("a", "Vlan1000", "1")
		h.OnAdd(in, true)
		h.OnDelete(toolscache.DeletedFinalStateUnknown{Key: "default/a", Obj: in})

		assert.NotContains(t, s.subnets, keyA)
	})

	t.Run("delete keeps another subnet on the same key", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		h.OnAdd(testSubnet("b", "Vlan1000", "2"), true)
		h.OnDelete(testSubnet("a", "Vlan1000", "1"))

		require.Contains(t, s.subnets, keyA)
		assert.Equal(t, "b", s.subnets[keyA].Name)
	})

	t.Run("unexpected objects are ignored", func(t *testing.T) {
		s := newTestServer()
		h := s.subnetHandler()

		h.OnAdd("garbage", true)
		h.OnDelete(toolscache.DeletedFinalStateUnknown{Key: "default/a", Obj: "garbage"})

		assert.Empty(t, s.subnets)
	})
}

func TestSwitchHandler(t *testing.T) {
	t.Run("leaf is allowlisted", func(t *testing.T) {
		s := newTestServer()
		h := s.switchHandler()

		h.OnAdd(testSwitch("leaf-01", wiringapi.SwitchRoleServerLeaf, "172.30.0.8/21"), true)

		assert.Contains(t, s.relayAllowlist, "172.30.0.8")
		assert.Equal(t, "172.30.0.8", s.switchToIP["leaf-01"])
	})

	t.Run("spine is not allowlisted", func(t *testing.T) {
		s := newTestServer()
		h := s.switchHandler()

		h.OnAdd(testSwitch("spine-01", wiringapi.SwitchRoleSpine, "172.30.0.2/21"), true)

		assert.Empty(t, s.relayAllowlist)
	})

	t.Run("ip change replaces the entry", func(t *testing.T) {
		s := newTestServer()
		h := s.switchHandler()

		old := testSwitch("leaf-01", wiringapi.SwitchRoleServerLeaf, "172.30.0.8/21")
		h.OnAdd(old, true)
		h.OnUpdate(old, testSwitch("leaf-01", wiringapi.SwitchRoleServerLeaf, "172.30.0.9/21"))

		assert.NotContains(t, s.relayAllowlist, "172.30.0.8")
		assert.Contains(t, s.relayAllowlist, "172.30.0.9")
	})

	t.Run("delete", func(t *testing.T) {
		s := newTestServer()
		h := s.switchHandler()

		in := testSwitch("leaf-01", wiringapi.SwitchRoleServerLeaf, "172.30.0.8/21")
		h.OnAdd(in, true)
		h.OnDelete(in)

		assert.Empty(t, s.relayAllowlist)
		assert.Empty(t, s.switchToIP)
	})

	t.Run("delete missed while disconnected", func(t *testing.T) {
		s := newTestServer()
		h := s.switchHandler()

		in := testSwitch("leaf-01", wiringapi.SwitchRoleServerLeaf, "172.30.0.8/21")
		h.OnAdd(in, true)
		h.OnDelete(toolscache.DeletedFinalStateUnknown{Key: "default/leaf-01", Obj: in})

		assert.Empty(t, s.relayAllowlist)
		assert.Empty(t, s.switchToIP)
	})
}
