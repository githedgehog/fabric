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
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	dhcpcfg "go.githedgehog.com/fabric/pkg/dhcp/config"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
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
// beforeUpdate is called with the underlying client on every status update before it's applied and can fail it
func newFakeKube(t *testing.T, obj *dhcpapi.DHCPSubnet, beforeUpdate func(ctx context.Context, kube kclient.Client) error) (kclient.Client, *subnet, *fakeKubeCalls) {
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
					if err := beforeUpdate(ctx, kube); err != nil {
						return err
					}
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

var testCfg = func() *dhcpcfg.ServerConfig {
	cfg := &dhcpcfg.ServerConfig{}
	cfg.Default()

	return cfg
}()

func TestSubnetLease(t *testing.T) {
	withHostname := func(req *dhcpv4.DHCPv4, hostname string) *dhcpv4.DHCPv4 {
		req.UpdateOption(dhcpv4.OptHostName(hostname))

		return req
	}

	t.Run("discovers are offered different IPs without writes", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)

		ip0, _, err := sn.lease(t.Context(), kube, testCfg, req(0))
		require.NoError(t, err)
		ip1, _, err := sn.lease(t.Context(), kube, testCfg, req(1))
		require.NoError(t, err)
		again, _, err := sn.lease(t.Context(), kube, testCfg, req(0))
		require.NoError(t, err)

		assert.Equal(t, "10.0.0.100", ip0.String())
		assert.Equal(t, "10.0.0.101", ip1.String())
		assert.Equal(t, ip0, again, "retransmitted discover gets the same offer")
		assert.EqualValues(t, 0, calls.updates.Load())
		assert.Len(t, sn.offers, 2)
	})

	t.Run("expired offer is free for others", func(t *testing.T) {
		kube, sn, _ := newFakeKube(t, testLeaseSubnet(), nil)
		cfg := &dhcpcfg.ServerConfig{OfferTTL: kmetav1.Duration{Duration: time.Nanosecond}}

		ip0, _, err := sn.lease(t.Context(), kube, cfg, req(0))
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
		ip1, _, err := sn.lease(t.Context(), kube, cfg, req(1))
		require.NoError(t, err)

		assert.Equal(t, ip0, ip1)
		assert.NotContains(t, sn.offers, mac(0))
	})

	t.Run("request for another client's offer is NAKed", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)

		ip0, _, err := sn.lease(t.Context(), kube, testCfg, req(0))
		require.NoError(t, err)
		_, _, err = sn.lease(t.Context(), kube, testCfg, request(t, 1, ip0.String(), ""))
		require.ErrorIs(t, err, ErrRequestedIPUnavailable)
		assert.EqualValues(t, 0, calls.updates.Load())
	})

	t.Run("request writes the lease", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)

		ip, _, err := sn.lease(t.Context(), kube, testCfg, req(0))
		require.NoError(t, err)
		start := time.Now()
		got, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, ip.String(), ""))
		require.NoError(t, err)

		assert.Equal(t, ip, got)
		assert.EqualValues(t, 1, calls.updates.Load())
		assert.Equal(t, ip, sn.offers[mac(0)].ip)

		stored := getStored(t, kube).Status.Allocated[mac(0)]
		assert.Equal(t, ip.String(), stored.IP)
		assert.False(t, stored.Discover)
		assert.WithinDuration(t, start.Add(time.Hour+5*time.Minute), stored.Expiry.Time, 2*time.Second)
	})

	t.Run("failed request write keeps the IP for the client", func(t *testing.T) {
		fail := atomic.Bool{}
		fail.Store(true)
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), func(context.Context, kclient.Client) error {
			if fail.Load() {
				return errUpdateBudget
			}

			return nil
		})

		ip, _, err := sn.lease(t.Context(), kube, testCfg, req(0))
		require.NoError(t, err)
		_, _, err = sn.lease(t.Context(), kube, testCfg, request(t, 0, ip.String(), ""))
		require.ErrorIs(t, err, errUpdateBudget)

		other, _, err := sn.lease(t.Context(), kube, testCfg, req(1))
		require.NoError(t, err)
		assert.NotEqual(t, ip, other, "the IP is still held for the client")

		fail.Store(false)
		got, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, ip.String(), ""))
		require.NoError(t, err, "retransmitted request is ACKed")
		assert.Equal(t, ip, got)
		assert.EqualValues(t, 2, calls.updates.Load())
	})

	t.Run("renewals", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			stored dhcpapi.DHCPAllocated
			static bool
			req    func(t *testing.T) *dhcpv4.DHCPv4
			write  bool
		}{
			{
				name:   "covered by the stored expiry",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.100", Expiry: kmetav1.NewTime(time.Now().Add(time.Hour + 4*time.Minute))},
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "", "10.0.0.100") },
			},
			{
				name:   "stored expiry before the end of the new lease",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.100", Expiry: kmetav1.NewTime(time.Now().Add(30 * time.Minute))},
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "", "10.0.0.100") },
				write:  true,
			},
			{
				name:   "hostname changed",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.100", Expiry: kmetav1.NewTime(time.Now().Add(time.Hour + 4*time.Minute))},
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return withHostname(request(t, 0, "", "10.0.0.100"), "server-1") },
				write:  true,
			},
			{
				name:   "left from a discover before the upgrade",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.100", Expiry: kmetav1.NewTime(time.Now().Add(time.Hour + 4*time.Minute)), Discover: true},
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "10.0.0.100", "") },
				write:  true,
			},
			{
				name:   "static",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.50"},
				static: true,
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "", "10.0.0.50") },
			},
			{
				name:   "becomes static",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.50", Expiry: kmetav1.NewTime(time.Now().Add(2 * time.Hour))},
				static: true,
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "", "10.0.0.50") },
				write:  true,
			},
			{
				name:   "no longer static",
				stored: dhcpapi.DHCPAllocated{IP: "10.0.0.150"},
				req:    func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "", "10.0.0.150") },
				write:  true,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				obj := testLeaseSubnet()
				obj.Status.Allocated = map[string]dhcpapi.DHCPAllocated{mac(0): tt.stored}
				if tt.static {
					obj.Spec.Static = map[string]dhcpapi.DHCPSubnetStatic{mac(0): {IP: "10.0.0.50"}}
				}
				kube, sn, calls := newFakeKube(t, obj, nil)

				ip, _, err := sn.lease(t.Context(), kube, testCfg, tt.req(t))
				require.NoError(t, err)
				assert.Equal(t, tt.stored.IP, ip.String())

				if !tt.write {
					assert.EqualValues(t, 0, calls.updates.Load())

					return
				}

				assert.EqualValues(t, 1, calls.updates.Load())
				stored := getStored(t, kube).Status.Allocated[mac(0)]
				assert.False(t, stored.Discover)
				assert.Equal(t, tt.static, stored.Expiry.IsZero())
			})
		}
	})
}

func TestSubnetPersist(t *testing.T) {
	t.Run("writes and stores the result", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)
		before := sn.obj.Load()

		ip, sp, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
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
			_, _, err := sn.lease(t.Context(), kube, testCfg, request(t, idx, "", ""))
			require.NoError(t, err)
		}

		assert.EqualValues(t, 5, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
		assert.Len(t, getStored(t, kube).Status.Allocated, 5)
	})

	t.Run("outside write is refetched and kept", func(t *testing.T) {
		outside := true
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), func(ctx context.Context, kube kclient.Client) error {
			if !outside {
				return nil
			}
			outside = false

			latest := &dhcpapi.DHCPSubnet{}
			require.NoError(t, kube.Get(ctx, kclient.ObjectKey{Namespace: kmetav1.NamespaceDefault, Name: "a"}, latest))
			latest.Status.Allocated = map[string]dhcpapi.DHCPAllocated{mac(9): {IP: "10.0.0.100"}}
			require.NoError(t, kube.Status().Update(ctx, latest))

			return nil
		})

		ip, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
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
		reqs := make([]*dhcpv4.DHCPv4, len(ips))
		for idx := range reqs {
			reqs[idx] = request(t, idx, "", "")
		}
		wg := sync.WaitGroup{}
		for idx := range ips {
			wg.Go(func() {
				ip, _, err := sn.lease(t.Context(), kube, testCfg, reqs[idx])
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

		_, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
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

		_, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
		require.ErrorContains(t, err, "UID mismatch")
		assert.EqualValues(t, 1, calls.updates.Load())
		assert.Empty(t, getStored(t, kube).Status.Allocated)
	})

	t.Run("transient errors are retried without refetching", func(t *testing.T) {
		failures := 2
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), func(context.Context, kclient.Client) error {
			if failures == 0 {
				return nil
			}
			failures--

			return kapierrors.NewTooManyRequests("slow down", 0)
		})

		_, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
		require.NoError(t, err)
		assert.EqualValues(t, 3, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
		assert.Len(t, getStored(t, kube).Status.Allocated, 1)
	})

	t.Run("other errors aren't retried", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), func(context.Context, kclient.Client) error {
			return kapierrors.NewForbidden(dhcpapi.GroupVersion.WithResource("dhcpsubnets").GroupResource(), "a", nil)
		})

		_, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
		require.True(t, kapierrors.IsForbidden(err))
		assert.EqualValues(t, 1, calls.updates.Load())
	})

	t.Run("update timing out isn't reported as success", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), func(context.Context, kclient.Client) error {
			return context.DeadlineExceeded
		})

		_, sp, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, sp)
		assert.EqualValues(t, 1, calls.updates.Load())
	})

	t.Run("expired request makes no API calls", func(t *testing.T) {
		kube, sn, calls := newFakeKube(t, testLeaseSubnet(), nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
		defer cancel()
		<-ctx.Done()

		_, _, err := sn.lease(ctx, kube, testCfg, request(t, 0, "", ""))
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.EqualValues(t, 0, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
	})

	t.Run("invalid subnet makes no API calls", func(t *testing.T) {
		obj := testLeaseSubnet()
		obj.Spec.StartIP = "garbage"
		kube, sn, calls := newFakeKube(t, obj, nil)

		_, _, err := sn.lease(t.Context(), kube, testCfg, request(t, 0, "", ""))
		require.Error(t, err)
		assert.EqualValues(t, 0, calls.updates.Load())
		assert.EqualValues(t, 0, calls.gets.Load())
	})
}
