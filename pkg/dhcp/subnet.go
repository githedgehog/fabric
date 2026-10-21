// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	dhcpcfg "go.githedgehog.com/fabric/pkg/dhcp/config"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/resourceversion"
	"k8s.io/apimachinery/pkg/util/wait"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var errSubnetDeleted = errors.New("subnet deleted")

// short, as the sleeps hold the subnet lock: it only rides out blips, the client retransmitting is the longer retry
var updateBackoff = wait.Backoff{
	Steps:    5,
	Duration: 10 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
	Cap:      100 * time.Millisecond,
}

// retriable errors are conflicts with other writers and transient API server errors
func retriable(err error) bool {
	return kapierrors.IsConflict(err) || kapierrors.IsTooManyRequests(err) || kapierrors.IsServerTimeout(err) ||
		kapierrors.IsTimeout(err) || kapierrors.IsServiceUnavailable(err)
}

// subnet tracks a single DHCPSubnet. All API writes for it go through persist:
//   - persist holds mu from reading obj until the API update returns, so writes for a subnet are one at a time and
//     never conflict with each other, other subnets aren't blocked
//   - obj is never modified in place and only replaced with a newer version, so it's read without any lock (the
//     informer never waits for an in-flight write)
//   - Server.mu (subnets map) is never held while taking mu, so the two can't deadlock
type subnet struct {
	uid     ktypes.UID
	mu      sync.Mutex
	obj     atomic.Pointer[dhcpapi.DHCPSubnet]
	deleted atomic.Bool
	spec    *subnetSpec      // guarded by mu
	offers  map[string]offer // by MAC, guarded by mu
}

// offer is an IP held for the client in memory after a DISCOVER or REQUEST until it expires, a stored lease takes over
type offer struct {
	ip     netip.Addr
	expiry time.Time
}

func newSubnet(obj *dhcpapi.DHCPSubnet) *subnet {
	sn := &subnet{uid: obj.UID, offers: map[string]offer{}}
	sn.obj.Store(obj)

	return sn
}

// set replaces obj if the given one is a newer version of the same subnet, so the echo of our own write or a late
// informer event never takes it back
func (sn *subnet) set(obj *dhcpapi.DHCPSubnet) bool {
	if obj.UID != sn.uid {
		return false
	}

	for {
		cur := sn.obj.Load()
		if cmp, err := resourceversion.CompareResourceVersion(obj.ResourceVersion, cur.ResourceVersion); err != nil {
			slog.Debug("Can't compare subnet resource versions, replacing", "subnet", obj.Name,
				"new", obj.ResourceVersion, "cur", cur.ResourceVersion, "err", err)
		} else if cmp <= 0 {
			return false
		}

		if sn.obj.CompareAndSwap(cur, obj) {
			return true
		}
	}
}

// errNoChange is returned by a decideFunc when nothing needs to be written
var errNoChange = errors.New("no change")

// decideFunc looks at the latest known subnet, which must not be modified, and returns the change to write
type decideFunc func(cur *dhcpapi.DHCPSubnet) (apply func(upd *dhcpapi.DHCPSubnet), err error)

// persist writes the change decided on the latest known subnet, retrying transient errors and refetching and deciding
// again on conflicts with other writers. No API call is made if there's nothing to write.
func (sn *subnet) persist(ctx context.Context, kube kclient.Client, decide decideFunc) error {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	refetch := false
	try := func() error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("request expired: %w", err)
		}
		if sn.deleted.Load() {
			return errSubnetDeleted
		}

		base := sn.obj.Load()
		if refetch {
			latest := &dhcpapi.DHCPSubnet{}
			if err := kube.Get(ctx, kclient.ObjectKeyFromObject(base), latest); err != nil {
				return fmt.Errorf("fetching latest subnet %s: %w", base.Name, err)
			}
			if latest.UID != sn.uid {
				return fmt.Errorf("subnet %s UID mismatch", base.Name) //nolint:err113
			}
			sn.set(latest)
			base = sn.obj.Load() // could be even newer if the informer delivered it meanwhile
			refetch = false
		}

		apply, err := decide(base)
		if errors.Is(err, errNoChange) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("deciding on subnet %s: %w", base.Name, err)
		}

		upd := base.DeepCopy()
		if upd.Status.Allocated == nil {
			upd.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
		}
		apply(upd)

		if err := kube.Status().Update(ctx, upd); err != nil {
			refetch = kapierrors.IsConflict(err)

			return fmt.Errorf("updating subnet %s (res %s gen %d): %w", upd.Name, base.ResourceVersion, upd.Generation, err)
		}

		// upd now holds the stored object
		sn.set(upd)

		return nil
	}

	// not retry.OnError, it reports success if the last error wraps a context error
	backoff := updateBackoff
	for attempt := 1; ; attempt++ {
		err := try()
		if err == nil {
			return nil
		}
		if !retriable(err) || attempt >= updateBackoff.Steps {
			return err
		}

		slog.Debug("Retrying subnet update", "subnet", sn.obj.Load().Name, "attempt", attempt, "refetch", refetch, "err", err)
		timer := time.NewTimer(backoff.Step())
		select {
		case <-ctx.Done():
			timer.Stop()

			return fmt.Errorf("request expired while retrying: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// specFor must be called with mu held
func (sn *subnet) specFor(obj *dhcpapi.DHCPSubnet) *subnetSpec {
	// status writes don't bump the generation, generation 0 is only in tests
	if sn.spec == nil || obj.Generation == 0 || sn.spec.gen != obj.Generation {
		sn.spec = newSubnetSpec(obj)
	}

	return sn.spec
}

// lease picks an IP for the client and returns it with the spec of the subnet version it was decided on. A DISCOVER
// only records an offer in memory. A REQUEST stores the lease with a padded expiry, and skips the write if the stored
// entry has the same IP and hostname and doesn't expire before the lease given to the client ends.
func (sn *subnet) lease(ctx context.Context, kube kclient.Client, cfg *dhcpcfg.ServerConfig, req *dhcpv4.DHCPv4) (netip.Addr, *subnetSpec, error) {
	var ip netip.Addr
	var sp *subnetSpec
	if err := sn.persist(ctx, kube, func(cur *dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
		sp = sn.specFor(cur)
		if sp.err != nil {
			return nil, sp.err
		}

		now := time.Now()
		sn.pruneOffers(now)

		var static bool
		var err error
		if ip, static, err = allocate(cur, sp, sn.offers, req); err != nil {
			return nil, fmt.Errorf("allocating ip: %w", err)
		}

		// a REQUEST refreshes the offer too, so the IP stays held for the client's retransmit if the write fails
		mac := req.ClientHWAddr.String()
		sn.offers[mac] = offer{ip: ip, expiry: now.Add(cfg.OfferTTL.Duration)}
		if req.MessageType() != dhcpv4.MessageTypeRequest {
			return nil, errNoChange
		}

		entry := dhcpapi.DHCPAllocated{IP: ip.String(), Hostname: req.HostName()}
		if !static {
			entry.Expiry = kmetav1.NewTime(now.Add(sp.leaseTime + cfg.LeaseExpiryPadding.Duration))
		}
		if stored, ok := cur.Status.Allocated[mac]; ok && covers(stored, entry, now.Add(sp.leaseTime)) {
			return nil, errNoChange
		}

		return func(upd *dhcpapi.DHCPSubnet) { upd.Status.Allocated[mac] = entry }, nil
	}); err != nil {
		return netip.Addr{}, nil, err
	}

	return ip, sp, nil
}

// covers reports whether the stored entry already has everything the new one would write, so the stored expiry is
// never earlier than the end of the lease given to the client
func covers(stored, entry dhcpapi.DHCPAllocated, leaseEnd time.Time) bool {
	if stored.IP != entry.IP || stored.Hostname != entry.Hostname || stored.Discover {
		return false
	}

	if entry.Expiry.IsZero() {
		return stored.Expiry.IsZero()
	}

	return !stored.Expiry.IsZero() && !stored.Expiry.Time.Before(leaseEnd)
}

// pruneOffers must be called with mu held
func (sn *subnet) pruneOffers(now time.Time) {
	for mac, o := range sn.offers {
		if now.After(o.expiry) {
			delete(sn.offers, mac)
		}
	}
}
