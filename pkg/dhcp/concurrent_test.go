// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"math/rand/v2"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TestConcurrentLeases guards the subnet locking rules (see subnet): clients booting at once in several subnets, with
// retransmits, while the informer replaces the objects (replaying old versions too) and cleanup runs, must never get
// an IP held by another client, must cost one write per client, and must not conflict with our own writes. It's also
// the regression test for the "concurrent map writes" crash of the Oct 2025 prototype, so run it with the race
// detector:
//
//	GOTOOLCHAIN=go1.26.8 go test -race -run TestConcurrentLeases -count=10 ./pkg/dhcp/
func TestConcurrentLeases(t *testing.T) {
	const clientsPerSubnet = 40

	objs := []*dhcpapi.DHCPSubnet{testLeaseSubnet(), testLeaseSubnet()}
	objs[1].Name, objs[1].UID = "b", "uid-b"
	kube, subnets, calls := newFakeKubeSubnets(t, objs, nil)

	ctx, cancel := context.WithCancel(t.Context())
	background := sync.WaitGroup{}

	// the informer delivers the latest versions and replays older ones, which must be ignored
	background.Go(func() {
		seen := make([][]*dhcpapi.DHCPSubnet, len(subnets))
		for ctx.Err() == nil {
			for idx, sn := range subnets {
				latest := &dhcpapi.DHCPSubnet{}
				if err := kube.Get(ctx, kclient.ObjectKeyFromObject(sn.obj.Load()), latest); err != nil {
					continue
				}
				sn.set(latest.DeepCopy())
				if len(seen[idx]) > 0 {
					sn.set(seen[idx][rand.IntN(len(seen[idx]))].DeepCopy()) //nolint:gosec // not for security
				}
				seen[idx] = append(seen[idx], latest)
			}
			time.Sleep(100 * time.Microsecond)
		}
	})

	background.Go(func() {
		for ctx.Err() == nil {
			for _, sn := range subnets {
				if _, _, err := sn.cleanup(ctx, kube, testCfg); ctx.Err() == nil {
					assert.NoError(t, err)
				}
			}
			time.Sleep(time.Millisecond)
		}
	})

	// two copies of every request at once, like a retransmit arriving while the first is handled, after a random delay
	// so the clients interleave with the informer and cleanup
	twice := func(sn *subnet, newReq func() *dhcpv4.DHCPv4) (netip.Addr, bool) {
		time.Sleep(rand.N(2 * time.Millisecond)) //nolint:gosec // not for security

		ips := make([]netip.Addr, 2)
		wg := sync.WaitGroup{}
		for idx := range ips {
			req := newReq()
			wg.Go(func() {
				ip, _, err := sn.lease(t.Context(), kube, testCfg, req)
				assert.NoError(t, err)
				ips[idx] = ip
			})
		}
		wg.Wait()

		return ips[0], assert.Equal(t, ips[0], ips[1], "same reply to both copies") && ips[0].IsValid()
	}

	acked := make([]map[string]netip.Addr, len(subnets))
	ackedMu := sync.Mutex{}
	clients := sync.WaitGroup{}
	for snIdx, sn := range subnets {
		acked[snIdx] = map[string]netip.Addr{}
		for client := range clientsPerSubnet {
			id := snIdx*clientsPerSubnet + client
			clients.Go(func() {
				offered, ok := twice(sn, func() *dhcpv4.DHCPv4 { return req(id) })
				if !ok {
					return
				}
				ip, ok := twice(sn, func() *dhcpv4.DHCPv4 { return request(t, id, offered.String(), "") })
				if !ok || !assert.Equal(t, offered, ip, "acked the offered IP") {
					return
				}
				if renewed, ok := twice(sn, func() *dhcpv4.DHCPv4 { return request(t, id, "", ip.String()) }); ok {
					assert.Equal(t, ip, renewed, "renewal keeps the IP")
				}

				ackedMu.Lock()
				defer ackedMu.Unlock()
				acked[snIdx][mac(id)] = ip
			})
		}
	}
	clients.Wait()
	cancel()
	background.Wait()

	assert.EqualValues(t, len(subnets)*clientsPerSubnet, calls.updates.Load(), "one write per client")
	assert.EqualValues(t, 0, calls.conflicts.Load(), "no conflicts with our own writes")

	for idx, obj := range objs {
		stored := &dhcpapi.DHCPSubnet{}
		require.NoError(t, kube.Get(t.Context(), kclient.ObjectKeyFromObject(obj), stored))
		require.Len(t, acked[idx], clientsPerSubnet)
		require.Len(t, stored.Status.Allocated, clientsPerSubnet)

		owners := map[string]string{}
		for mac, entry := range stored.Status.Allocated {
			if owner, ok := owners[entry.IP]; ok {
				assert.Failf(t, "IP held by two clients", "%s: %s and %s", entry.IP, owner, mac)
			}
			owners[entry.IP] = mac
			assert.Equal(t, acked[idx][mac].String(), entry.IP, "stored lease of %s", mac)
		}
	}
}
