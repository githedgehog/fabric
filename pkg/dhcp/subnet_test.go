// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"math/rand/v2"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestSubnetSet(t *testing.T) {
	for _, tt := range []struct {
		name string
		uid  ktypes.UID
		rv   string
		want bool
	}{
		{"older", "uid-a", "4", false},
		{"equal", "uid-a", "5", false},
		{"newer", "uid-a", "6", true},
		{"newer with more digits", "uid-a", "10", true},
		{"malformed is accepted", "uid-a", "garbage", true},
		{"other uid", "uid-b", "6", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cur := testSubnet("a", "Vlan1000", "5")
			sn := newSubnet(cur)

			obj := testSubnet("a", "Vlan1000", tt.rv)
			obj.UID = tt.uid

			assert.Equal(t, tt.want, sn.set(obj))
			if tt.want {
				assert.Same(t, obj, sn.obj.Load())
			} else {
				assert.Same(t, cur, sn.obj.Load())
			}
		})
	}
}

func TestSubnetSetConcurrent(t *testing.T) {
	sn := newSubnet(testSubnet("a", "Vlan1000", "1"))

	rvs := make([]int, 0, 200)
	for rv := 2; rv <= 201; rv++ {
		rvs = append(rvs, rv)
	}
	rand.Shuffle(len(rvs), func(i, j int) { rvs[i], rvs[j] = rvs[j], rvs[i] })

	wg := sync.WaitGroup{}
	for _, rv := range rvs {
		wg.Go(func() { sn.set(testSubnet("a", "Vlan1000", strconv.Itoa(rv))) })
	}
	wg.Wait()

	assert.Equal(t, "201", sn.obj.Load().ResourceVersion)
}

type fakeKubeCalls struct {
	gets    atomic.Int32
	updates atomic.Int32
}

// newFakeKube returns a fake client holding the given subnet and the wrapper for it built from the stored object,
// beforeUpdate is called with the underlying client on every status update before it's applied
func newFakeKube(t *testing.T, obj *dhcpapi.DHCPSubnet, beforeUpdate func(ctx context.Context, kube kclient.Client)) (kclient.Client, *subnet, *fakeKubeCalls) {
	t.Helper()

	scheme, err := kubeutil.NewScheme(dhcpapi.AddToScheme)
	require.NoError(t, err)

	calls := &fakeKubeCalls{}
	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(obj).
		WithStatusSubresource(&dhcpapi.DHCPSubnet{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, kube kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
				calls.gets.Add(1)

				return kube.Get(ctx, key, obj, opts...)
			},
			SubResourceUpdate: func(ctx context.Context, kube kclient.Client, subResource string, obj kclient.Object, opts ...kclient.SubResourceUpdateOption) error {
				calls.updates.Add(1)
				if beforeUpdate != nil {
					beforeUpdate(ctx, kube)
				}

				return kube.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}).
		Build()

	stored := &dhcpapi.DHCPSubnet{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKeyFromObject(obj), stored))
	calls.gets.Store(0)

	return kube, newSubnet(stored), calls
}

func testLeaseSubnet() *dhcpapi.DHCPSubnet {
	return &dhcpapi.DHCPSubnet{
		ObjectMeta: kmetav1.ObjectMeta{Name: "a", Namespace: kmetav1.NamespaceDefault, UID: "uid-a"},
		Spec: dhcpapi.DHCPSubnetSpec{
			LeaseTimeSeconds: 3600,
			Subnet:           "default",
			CIDRBlock:        "10.0.0.0/24",
			Gateway:          "10.0.0.1",
			StartIP:          "10.0.0.100",
			EndIP:            "10.0.0.199",
			VRF:              "VrfV1",
			CircuitID:        "Vlan1000",
		},
	}
}

func getStored(t *testing.T, kube kclient.Client) *dhcpapi.DHCPSubnet {
	t.Helper()

	stored := &dhcpapi.DHCPSubnet{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Namespace: kmetav1.NamespaceDefault, Name: "a"}, stored))

	return stored
}

func TestSubnetSpecFor(t *testing.T) {
	v1 := testLeaseSubnet()
	v1.Generation = 1
	v1.ResourceVersion = "1"
	sn := newSubnet(v1)

	sp1 := sn.specFor(v1)
	require.NoError(t, sp1.err)

	// status write: new version, same generation
	v2 := v1.DeepCopy()
	v2.ResourceVersion = "2"
	v2.Status.Allocated = map[string]dhcpapi.DHCPAllocated{mac(0): {IP: "10.0.0.100"}}
	assert.Same(t, sp1, sn.specFor(v2), "same generation reuses the spec")

	// spec change
	v3 := v2.DeepCopy()
	v3.ResourceVersion = "3"
	v3.Generation = 2
	v3.Spec.StartIP = "10.0.0.150"
	sp3 := sn.specFor(v3)
	assert.NotSame(t, sp1, sp3, "new generation rebuilds the spec")
	require.NoError(t, sp3.err)
	assert.Equal(t, netip.MustParseAddr("10.0.0.150"), sp3.parsed.Start)

	v4 := v3.DeepCopy()
	v4.ResourceVersion = "4"
	v4.Generation = 3
	v4.Spec.StartIP = "garbage"
	require.ErrorContains(t, sn.specFor(v4).err, "invalid subnet")
}

func TestSubnetPersist(t *testing.T) {
	t.Run("writes and stores the result", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)
		before := sn.obj.Load()

		ip, sp, err := sn.lease(t.Context(), kube, req(0))
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.100", ip.String())
		assert.NotNil(t, sp)
		assert.EqualValues(t, 1, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())

		stored := getStored(t, kube)
		obj := sn.obj.Load()
		assert.Equal(t, stored.ResourceVersion, obj.ResourceVersion)
		assert.Equal(t, "10.0.0.100", stored.Status.Allocated[mac(0)].IP)
		assert.Empty(t, before.Status.Allocated, "previous object must not be modified")

		assert.False(t, sn.set(before), "late event for the previous version is ignored")
		assert.False(t, sn.set(stored), "echo of our own write is ignored")
		assert.Same(t, obj, sn.obj.Load())
	})

	t.Run("consecutive writes don't conflict", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)

		for idx := range 5 {
			_, _, err := sn.lease(t.Context(), kube, req(idx))
			require.NoError(t, err)
		}

		assert.EqualValues(t, 5, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
		assert.Len(t, getStored(t, kube).Status.Allocated, 5)
	})

	t.Run("outside write is refetched and kept", func(t *testing.T) {
		outside := true
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), func(ctx context.Context, kube kclient.Client) {
			if !outside {
				return
			}
			outside = false

			latest := &dhcpapi.DHCPSubnet{}
			require.NoError(t, kube.Get(ctx, kclient.ObjectKey{Namespace: kmetav1.NamespaceDefault, Name: "a"}, latest))
			latest.Status.Allocated = map[string]dhcpapi.DHCPAllocated{mac(9): {IP: "10.0.0.100"}}
			require.NoError(t, kube.Status().Update(ctx, latest))
		})

		ip, _, err := sn.lease(t.Context(), kube, req(0))
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.101", ip.String(), "decision is redone on the refetched subnet")
		assert.EqualValues(t, 2, calls.updates.Load())
		assert.EqualValues(t, 1, calls.gets.Load())

		stored := getStored(t, kube)
		assert.Equal(t, stored.ResourceVersion, sn.obj.Load().ResourceVersion)
		assert.Len(t, stored.Status.Allocated, 2)
	})

	t.Run("concurrent writes with late events don't conflict", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)
		initial := sn.obj.Load()

		stop := make(chan struct{})
		replayed := sync.WaitGroup{}
		replayed.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					sn.set(initial)       // late event
					sn.set(sn.obj.Load()) // echo
				}
			}
		})

		ips := make([]string, 50)
		wg := sync.WaitGroup{}
		for idx := range ips {
			wg.Go(func() {
				ip, _, err := sn.lease(t.Context(), kube, req(idx))
				assert.NoError(t, err)
				ips[idx] = ip.String()
			})
		}
		wg.Wait()
		close(stop)
		replayed.Wait()

		assert.EqualValues(t, len(ips), calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load(), "no conflicts")

		stored := getStored(t, kube)
		assert.Len(t, stored.Status.Allocated, len(ips))
		assert.Equal(t, stored.ResourceVersion, sn.obj.Load().ResourceVersion)
		unique := map[string]bool{}
		for _, ip := range ips {
			unique[ip] = true
		}
		assert.Len(t, unique, len(ips), "no IP handed out twice")
	})

	t.Run("deleted subnet isn't written", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)
		sn.deleted.Store(true)

		_, _, err := sn.lease(t.Context(), kube, req(0))
		require.ErrorIs(t, err, errSubnetDeleted)
		assert.EqualValues(t, 0, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
	})

	t.Run("recreated subnet isn't written", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)

		// wrapper for an older incarnation, so the first update conflicts and the refetch finds another UID
		old := sn.obj.Load().DeepCopy()
		old.UID = "uid-old"
		old.ResourceVersion = "1"
		sn = newSubnet(old)

		_, _, err := sn.lease(t.Context(), kube, req(0))
		require.ErrorContains(t, err, "UID mismatch")
		assert.EqualValues(t, 1, calls.updates.Load())
		assert.Empty(t, getStored(t, kube).Status.Allocated)
	})

	t.Run("invalid subnet makes no API calls", func(t *testing.T) {
		obj := testLeaseSubnet()
		obj.Spec.StartIP = "garbage"
		kube, sn, calls := newFakeKube(t, obj, nil)

		_, _, err := sn.lease(t.Context(), kube, req(0))
		require.Error(t, err)
		assert.EqualValues(t, 0, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
	})
}
