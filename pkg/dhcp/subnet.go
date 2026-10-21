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

// short, as the sleeps hold updateMu: it only rides out blips, the client retransmitting is the longer retry
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

// subnet tracks a single DHCPSubnet. All decisions and API writes for it go through persist, so writes for a subnet
// are one at a time and never conflict with each other, while requests that write nothing never wait for them.
// Server.mu (subnets map) is never held while taking the subnet locks, so they can't deadlock.
type subnet struct {
	uid     ktypes.UID
	deleted atomic.Bool
	// obj is never modified in place and only replaced with a newer version, so it's read without any lock (the
	// informer never waits for a write in progress)
	obj atomic.Pointer[dhcpapi.DHCPSubnet]

	// updateMu serializes the API updates: persist holds it for a change that needs a status update, from deciding it
	// again on the latest obj through the update budget wait, the update and its retries. Taken before decideMu,
	// never while holding it.
	updateMu sync.Mutex

	// decideMu guards the in-memory state below and is held for every decide call (CPU only, never across API calls
	// or waits). persist takes it on its own first, so requests that write nothing never wait for updateMu.
	decideMu sync.Mutex
	spec     *subnetSpec
	offers   map[string]offer // by MAC
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

// decideFunc looks at the latest known subnet, which must not be modified, and returns the change to write. It's
// called with decideMu held, possibly more than once per persist, so its side effects on the in-memory state must be
// repeatable, and it must not call persist.
type decideFunc func(cur *dhcpapi.DHCPSubnet) (apply func(upd *dhcpapi.DHCPSubnet), err error)

// persist writes the change decided on the latest known subnet, retrying transient errors and refetching and deciding
// again on conflicts with other writers. It decides without updateMu first, so a request that writes nothing (or
// fails) doesn't wait for updates in progress. A change that needs a write is decided again after taking updateMu, as
// an update made while waiting for it can make the write unnecessary, e.g. a retransmitted REQUEST whose first copy
// has just stored the same lease.
func (sn *subnet) persist(ctx context.Context, kube kclient.Client, decide decideFunc) error {
	decideNow := func(base *dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("request expired: %w", err)
		}
		if sn.deleted.Load() {
			return nil, errSubnetDeleted
		}

		sn.decideMu.Lock()
		defer sn.decideMu.Unlock()

		apply, err := decide(base)
		if err != nil && !errors.Is(err, errNoChange) {
			return nil, fmt.Errorf("deciding on subnet %s: %w", base.Name, err)
		}

		return apply, err
	}

	_, err := decideNow(sn.obj.Load())
	if errors.Is(err, errNoChange) {
		return nil
	}
	if err != nil {
		return err
	}

	sn.updateMu.Lock()
	defer sn.updateMu.Unlock()

	refetch := false
	try := func() error {
		base := sn.obj.Load()
		if refetch {
			latest := &dhcpapi.DHCPSubnet{}
			if err := kube.Get(ctx, kclient.ObjectKeyFromObject(base), latest); kapierrors.IsNotFound(err) {
				return fmt.Errorf("fetching latest subnet %s: %w", base.Name, errSubnetDeleted)
			} else if err != nil {
				return fmt.Errorf("fetching latest subnet %s: %w", base.Name, err)
			}
			// the informer marks it deleted soon, nothing is written to the new object
			if latest.UID != sn.uid {
				return fmt.Errorf("subnet %s recreated: %w", base.Name, errSubnetDeleted)
			}
			sn.set(latest)
			base = sn.obj.Load() // could be even newer if the informer delivered it meanwhile
			refetch = false
		}

		apply, err := decideNow(base)
		if errors.Is(err, errNoChange) {
			return nil
		}
		if err != nil {
			return err
		}

		upd := base.DeepCopy()
		if upd.Status.Allocated == nil {
			upd.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
		}
		apply(upd)

		if err := kube.Status().Update(ctx, upd); err != nil {
			if kapierrors.IsNotFound(err) {
				return fmt.Errorf("updating subnet %s: %w", upd.Name, errSubnetDeleted)
			}
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

// specFor must be called with decideMu held
func (sn *subnet) specFor(obj *dhcpapi.DHCPSubnet) *subnetSpec {
	// status writes don't bump the generation, generation 0 is only in tests
	if sn.spec == nil || obj.Generation == 0 || sn.spec.gen != obj.Generation {
		sn.spec = newSubnetSpec(obj)
	}

	return sn.spec
}

// leaseInfo is how a lease was decided, for the reply and the request log
type leaseInfo struct {
	spec      *subnetSpec // of the subnet version it was decided on
	written   bool        // the lease was stored, it isn't if the stored entry already covers it
	notWanted string      // why the IP the client asked for wasn't given (DISCOVER)
}

// lease picks an IP for the client and returns it with how it was decided. A DISCOVER only records an offer in memory.
// A REQUEST stores the lease with a padded expiry, and skips the write if the stored entry has the same IP and
// hostname and doesn't expire before the lease given to the client ends.
func (sn *subnet) lease(ctx context.Context, kube kclient.Client, cfg *dhcpcfg.ServerConfig, req *dhcpv4.DHCPv4) (netip.Addr, leaseInfo, error) {
	var ip netip.Addr
	info := leaseInfo{}
	if err := sn.persist(ctx, kube, func(cur *dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
		info.written = false
		sp := sn.specFor(cur)
		info.spec = sp
		if sp.err != nil {
			return nil, sp.err
		}

		now := time.Now()
		sn.pruneOffers(now)

		var static bool
		var err error
		if ip, static, info.notWanted, err = allocate(cur, sp, sn.offers, req); err != nil {
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

		return func(upd *dhcpapi.DHCPSubnet) {
			upd.Status.Allocated[mac] = entry
			info.written = true
		}, nil
	}); err != nil {
		return netip.Addr{}, leaseInfo{}, err
	}

	return ip, info, nil
}

// cleanup removes the expired leases and gives a normal expiry to the entries stored without one that aren't the MAC's
// static IP anymore (the static was removed or changed), so they don't hold the IP forever. Expired offers are dropped
// too. Nothing is written if nothing changed. It returns the MACs it removed and gave an expiry to.
func (sn *subnet) cleanup(ctx context.Context, kube kclient.Client, cfg *dhcpcfg.ServerConfig) ([]string, []string, error) {
	var expired, unstatic []string
	if err := sn.persist(ctx, kube, func(cur *dhcpapi.DHCPSubnet) (func(*dhcpapi.DHCPSubnet), error) {
		expired, unstatic = nil, nil

		now := time.Now()
		sn.pruneOffers(now)

		sp := sn.specFor(cur)
		for mac, entry := range cur.Status.Allocated {
			if !entry.Expiry.IsZero() {
				if entry.Expiry.Time.Before(now) {
					expired = append(expired, mac)
				}

				continue
			}

			// statics are unknown for an invalid subnet
			if sp.err != nil {
				continue
			}
			if ip, err := netip.ParseAddr(entry.IP); err != nil || sp.statics[mac] != ip {
				unstatic = append(unstatic, mac)
			}
		}
		if len(expired) == 0 && len(unstatic) == 0 {
			return nil, errNoChange
		}

		expiry := kmetav1.NewTime(now.Add(sp.leaseTime + cfg.LeaseExpiryPadding.Duration))

		return func(upd *dhcpapi.DHCPSubnet) {
			for _, mac := range expired {
				delete(upd.Status.Allocated, mac)
			}
			for _, mac := range unstatic {
				entry := upd.Status.Allocated[mac]
				entry.Expiry = expiry
				upd.Status.Allocated[mac] = entry
			}
		}, nil
	}); err != nil {
		return nil, nil, err
	}

	return expired, unstatic, nil
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

// pruneOffers must be called with decideMu held
func (sn *subnet) pruneOffers(now time.Time) {
	for mac, o := range sn.offers {
		if now.After(o.expiry) {
			delete(sn.offers, mac)
		}
	}
}
