// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"sync/atomic"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	dhcpcfg "go.githedgehog.com/fabric/pkg/dhcp/config"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	"golang.org/x/time/rate"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"
	kcache "sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func newKube(ctx context.Context, dhcpCfg *dhcpcfg.ServerConfig, stats *budgetStats) (kclient.Client, kcache.Cache, error) {
	cfg, err := kubeutil.NewClientConfig(ctx, "")
	if err != nil {
		return nil, nil, fmt.Errorf("creating kube config: %w", err)
	}

	scheme, err := kubeutil.NewScheme(dhcpapi.AddToScheme, wiringapi.AddToScheme)
	if err != nil {
		return nil, nil, fmt.Errorf("creating scheme: %w", err)
	}

	kube, err := kclient.New(cfg, kclient.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("creating kube client: %w", err)
	}
	kube = withUpdateBudget(kube, rate.NewLimiter(rate.Limit(dhcpCfg.UpdateQPS), dhcpCfg.UpdateBurst), stats)

	cache, err := kcache.New(cfg, kcache.Options{
		Scheme:                   scheme,
		DefaultNamespaces:        map[string]kcache.Config{kmetav1.NamespaceDefault: {}},
		DefaultWatchErrorHandler: watchErrorHandler,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating kube cache: %w", err)
	}

	return kube, cache, nil
}

var errUpdateBudget = errors.New("update budget exhausted")

// budgetStats counts the requests slowed down or dropped because of the update budget, they're logged periodically
// instead of one by one so a storm doesn't flood the logs
type budgetStats struct {
	delayed atomic.Int64 // lease writes that had to wait for the budget
	dropped atomic.Int64 // lease writes that couldn't get the budget before the request timeout
	expired atomic.Int64 // requests that timed out waiting for the subnet or retries
}

func (st *budgetStats) report(cfg *dhcpcfg.ServerConfig) {
	delayed, dropped, expired := st.delayed.Swap(0), st.dropped.Swap(0), st.expired.Swap(0)
	if delayed == 0 && dropped == 0 && expired == 0 {
		return
	}

	attrs := []any{
		"delayed", delayed, "dropped", dropped, "expired", expired, "period", budgetReportPeriod,
		"updateQPS", cfg.UpdateQPS, "updateBurst", cfg.UpdateBurst, "requestTimeout", cfg.RequestTimeout.Duration,
	}
	if dropped > 0 || expired > 0 {
		slog.Warn("Lease writes are over the update budget, dropping requests", attrs...)
	} else {
		slog.Info("Lease writes are rate limited by the update budget", attrs...)
	}
}

const budgetReportPeriod = 30 * time.Second

func (s *Server) reportBudget(ctx context.Context) {
	ticker := time.NewTicker(budgetReportPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.budget.report(&s.Config)
		}
	}
}

// withUpdateBudget makes status updates (lease writes) wait for the limiter right before they're sent, so nothing else
// (reads, skipped writes, the informers) uses the budget, while each retried update does
func withUpdateBudget(kube kclient.Client, limiter *rate.Limiter, stats *budgetStats) kclient.Client {
	return &budgetClient{Client: kube, limiter: limiter, stats: stats}
}

type budgetClient struct {
	kclient.Client
	limiter *rate.Limiter
	stats   *budgetStats
}

func (c *budgetClient) Status() kclient.SubResourceWriter {
	return &budgetStatusWriter{SubResourceWriter: c.Client.Status(), limiter: c.limiter, stats: c.stats}
}

type budgetStatusWriter struct {
	kclient.SubResourceWriter
	limiter *rate.Limiter
	stats   *budgetStats
}

func (w *budgetStatusWriter) Update(ctx context.Context, obj kclient.Object, opts ...kclient.SubResourceUpdateOption) error {
	// Allow only takes a token available right now (never one reserved by an earlier waiter), so a write that gets to Wait
	// is delayed. Wait fails right away without using the budget if the next token comes after the request timeout.
	if !w.limiter.Allow() {
		if err := w.limiter.Wait(ctx); err != nil {
			w.stats.dropped.Add(1)

			return fmt.Errorf("%w: %w", errUpdateBudget, err)
		}
		w.stats.delayed.Add(1)
	}

	return w.SubResourceWriter.Update(ctx, obj, opts...) //nolint:wrapcheck
}

func watchErrorHandler(ctx context.Context, r *toolscache.Reflector, err error) {
	switch {
	case errors.Is(err, io.EOF):
		// watch closed normally
	case kapierrors.IsResourceExpired(err) || kapierrors.IsGone(err) || errors.Is(err, io.ErrUnexpectedEOF):
		slog.Debug("Watch closed, informer will relist", "type", r.TypeDescription(), "err", err)
	default:
		slog.Warn("Watch failed, informer will retry", "type", r.TypeDescription(), "err", err)
	}

	toolscache.DefaultWatchErrorHandler(ctx, r, err)
}

func (s *Server) addInformerHandlers(ctx context.Context, cache kcache.Cache) ([]toolscache.InformerSynced, error) {
	synced := []toolscache.InformerSynced{}

	for _, h := range []struct {
		obj     kclient.Object
		handler toolscache.ResourceEventHandler
	}{
		{&dhcpapi.DHCPSubnet{}, s.subnetHandler()},
		{&wiringapi.Switch{}, s.switchHandler()},
	} {
		informer, err := cache.GetInformer(ctx, h.obj)
		if err != nil {
			return nil, fmt.Errorf("getting %T informer: %w", h.obj, err)
		}

		// no periodic resync, it would only redeliver the objects we already have
		reg, err := informer.AddEventHandlerWithResyncPeriod(h.handler, 0)
		if err != nil {
			return nil, fmt.Errorf("adding %T event handler: %w", h.obj, err)
		}

		synced = append(synced, reg.HasSynced)
	}

	return synced, nil
}

func unwrapTombstone(obj any) any {
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		return tombstone.Obj
	}

	return obj
}

func (s *Server) subnetHandler() toolscache.ResourceEventHandlerFuncs {
	return toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { s.upsertSubnet(nil, obj) },
		UpdateFunc: s.upsertSubnet,
		DeleteFunc: s.deleteSubnet,
	}
}

func (s *Server) upsertSubnet(oldObj, newObj any) {
	obj, ok := newObj.(*dhcpapi.DHCPSubnet)
	if !ok {
		slog.Warn("Unexpected object in DHCPSubnet informer, ignoring", "type", fmt.Sprintf("%T", newObj))

		return
	}

	// objects are shared with the informer cache and must not be modified
	obj = obj.DeepCopy()
	if obj.Status.Allocated == nil {
		obj.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
	}
	key := subnetKeyFrom(obj)
	slog.Debug("DHCPSubnet updated", "subnet", obj.Name, "key", key, "rv", obj.ResourceVersion)

	s.mu.Lock()
	defer s.mu.Unlock()

	var sn *subnet

	// VRF or circuit ID changed, so the subnet isn't reachable under the old key anymore
	if old, ok := oldObj.(*dhcpapi.DHCPSubnet); ok {
		if oldKey := subnetKeyFrom(old); oldKey != key {
			if cur := s.subnets[oldKey]; cur != nil && cur.uid == old.UID {
				delete(s.subnets, oldKey)
				if cur.uid == obj.UID {
					sn = cur
				} else {
					cur.deleted.Store(true)
				}
			}
		}
	}

	if cur := s.subnets[key]; cur != nil && cur != sn {
		if sn == nil && cur.uid == obj.UID {
			sn = cur
		} else {
			// recreated with the same name or another subnet now uses the same VRF and circuit ID
			cur.deleted.Store(true)
		}
	}

	if sn == nil {
		s.subnets[key] = newSubnet(obj)

		return
	}

	s.subnets[key] = sn
	sn.set(obj)
}

func (s *Server) deleteSubnet(deletedObj any) {
	deletedObj = unwrapTombstone(deletedObj)
	obj, ok := deletedObj.(*dhcpapi.DHCPSubnet)
	if !ok {
		slog.Warn("Unexpected deleted object in DHCPSubnet informer, ignoring", "type", fmt.Sprintf("%T", deletedObj))

		return
	}

	key := subnetKeyFrom(obj)
	slog.Debug("DHCPSubnet deleted", "subnet", obj.Name, "key", key)

	s.mu.Lock()
	defer s.mu.Unlock()

	if cur := s.subnets[key]; cur != nil && cur.uid == obj.UID {
		delete(s.subnets, key)
		cur.deleted.Store(true)
	}
}

func (s *Server) switchHandler() toolscache.ResourceEventHandlerFuncs {
	return toolscache.ResourceEventHandlerFuncs{
		AddFunc:    s.upsertSwitch,
		UpdateFunc: func(_, obj any) { s.upsertSwitch(obj) },
		DeleteFunc: s.deleteSwitch,
	}
}

func (s *Server) upsertSwitch(obj any) {
	sw, ok := obj.(*wiringapi.Switch)
	if !ok {
		slog.Warn("Unexpected object in Switch informer, ignoring", "type", fmt.Sprintf("%T", obj))

		return
	}
	slog.Debug("Switch updated", "switch", sw.Name)

	newIP := ""
	if sw.Spec.Role.IsLeaf() && sw.Spec.IP != "" {
		prefix, err := netip.ParsePrefix(sw.Spec.IP)
		if err != nil {
			slog.Warn("Switch has unparseable IP, removing from relay allowlist", "switch", sw.Name, "ip", sw.Spec.IP)
		} else {
			newIP = prefix.Addr().String()
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if oldIP := s.switchToIP[sw.Name]; oldIP != "" && oldIP != newIP {
		delete(s.switchToIP, sw.Name)
		delete(s.relayAllowlist, oldIP)
	}
	if newIP != "" {
		s.switchToIP[sw.Name] = newIP
		s.relayAllowlist[newIP] = struct{}{}
	}
}

func (s *Server) deleteSwitch(obj any) {
	obj = unwrapTombstone(obj)
	sw, ok := obj.(*wiringapi.Switch)
	if !ok {
		slog.Warn("Unexpected deleted object in Switch informer, ignoring", "type", fmt.Sprintf("%T", obj))

		return
	}
	slog.Debug("Switch deleted", "switch", sw.Name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if oldIP := s.switchToIP[sw.Name]; oldIP != "" {
		delete(s.switchToIP, sw.Name)
		delete(s.relayAllowlist, oldIP)
	}
}
