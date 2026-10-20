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
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/resourceversion"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var errSubnetDeleted = errors.New("subnet deleted")

var updateBackoff = wait.Backoff{
	Steps:    10,
	Duration: 10 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
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
}

func newSubnet(obj *dhcpapi.DHCPSubnet) *subnet {
	sn := &subnet{uid: obj.UID}
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

// persist applies mutate to a copy of the latest known subnet and writes it, refetching and reapplying on conflicts
// with other writers. It returns the stored subnet.
func (sn *subnet) persist(ctx context.Context, kube kclient.Client, mutate func(*dhcpapi.DHCPSubnet) error) (*dhcpapi.DHCPSubnet, error) {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	var res *dhcpapi.DHCPSubnet
	attempt := 0
	if err := retry.RetryOnConflict(updateBackoff, func() error {
		if sn.deleted.Load() {
			return errSubnetDeleted
		}

		base := sn.obj.Load()
		if attempt > 0 {
			latest := &dhcpapi.DHCPSubnet{}
			if err := kube.Get(ctx, kclient.ObjectKeyFromObject(base), latest); err != nil {
				return fmt.Errorf("fetching latest subnet %s: %w", base.Name, err)
			}
			if latest.UID != sn.uid {
				return fmt.Errorf("subnet %s UID mismatch", base.Name) //nolint:err113
			}
			sn.set(latest)
			base = sn.obj.Load() // could be even newer if the informer delivered it meanwhile
		}
		attempt++

		upd := base.DeepCopy()
		if upd.Status.Allocated == nil {
			upd.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
		}
		if err := mutate(upd); err != nil {
			return fmt.Errorf("mutating subnet %s: %w", upd.Name, err)
		}

		if err := kube.Status().Update(ctx, upd); err != nil {
			if kapierrors.IsConflict(err) {
				slog.Debug("Conflict updating subnet, refetching", "subnet", upd.Name, "rv", base.ResourceVersion)
			}

			return fmt.Errorf("updating subnet %s (res %s gen %d): %w", upd.Name, base.ResourceVersion, upd.Generation, err)
		}

		// upd now holds the stored object
		sn.set(upd)
		res = upd

		return nil
	}); err != nil {
		return nil, fmt.Errorf("retrying subnet update: %w", err)
	}

	return res, nil
}

func (sn *subnet) lease(ctx context.Context, kube kclient.Client, req *dhcpv4.DHCPv4) (netip.Addr, *dhcpapi.DHCPSubnet, error) {
	var ip netip.Addr
	obj, err := sn.persist(ctx, kube, func(obj *dhcpapi.DHCPSubnet) error {
		var err error
		if ip, err = allocate(obj, req); err != nil {
			return fmt.Errorf("allocating ip: %w", err)
		}

		return nil
	})
	if err != nil {
		return netip.Addr{}, nil, err
	}

	return ip, obj, nil
}
