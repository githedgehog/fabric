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
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	kcache "sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func newKube(ctx context.Context) (kclient.Client, kcache.Cache, error) {
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

func (s *Server) upsertSubnet(oldObj, obj any) {
	subnet, ok := obj.(*dhcpapi.DHCPSubnet)
	if !ok {
		slog.Warn("Unexpected object in DHCPSubnet informer, ignoring", "type", fmt.Sprintf("%T", obj))

		return
	}

	// objects are shared with the informer cache and must not be modified
	subnet = subnet.DeepCopy()
	if subnet.Status.Allocated == nil {
		subnet.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
	}
	key := subnetKeyFrom(subnet)
	slog.Debug("DHCPSubnet updated", "subnet", subnet.Name, "key", key, "rv", subnet.ResourceVersion)

	s.m.Lock()
	defer s.m.Unlock()

	// VRF or circuit ID changed, so the subnet isn't reachable under the old key anymore
	if old, ok := oldObj.(*dhcpapi.DHCPSubnet); ok {
		if oldKey := subnetKeyFrom(old); oldKey != key {
			s.deleteSubnetKeyLocked(oldKey, old.Name)
		}
	}
	s.subnets[key] = subnet
}

func (s *Server) deleteSubnet(obj any) {
	obj = unwrapTombstone(obj)
	subnet, ok := obj.(*dhcpapi.DHCPSubnet)
	if !ok {
		slog.Warn("Unexpected deleted object in DHCPSubnet informer, ignoring", "type", fmt.Sprintf("%T", obj))

		return
	}

	key := subnetKeyFrom(subnet)
	slog.Debug("DHCPSubnet deleted", "subnet", subnet.Name, "key", key)

	s.m.Lock()
	defer s.m.Unlock()

	s.deleteSubnetKeyLocked(key, subnet.Name)
}

func (s *Server) deleteSubnetKeyLocked(key, name string) {
	if cur, ok := s.subnets[key]; ok && cur.Name == name {
		delete(s.subnets, key)
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

	s.m.Lock()
	defer s.m.Unlock()

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

	s.m.Lock()
	defer s.m.Unlock()

	if oldIP := s.switchToIP[sw.Name]; oldIP != "" {
		delete(s.switchToIP, sw.Name)
		delete(s.relayAllowlist, oldIP)
	}
}

func (s *Server) updateSubnet(ctx context.Context, subnet *dhcpapi.DHCPSubnet, mutate func(subnet *dhcpapi.DHCPSubnet) error) error {
	uid := subnet.UID
	subnetName := subnet.Name

	attempt := 0
	if err := retry.RetryOnConflict(wait.Backoff{
		Steps:    10,
		Duration: 10 * time.Millisecond,
		Factor:   1.0,
		Jitter:   0.1,
	}, func() error {
		// skip log on a first retry
		if attempt > 1 {
			slog.Debug("Fetching latest to update status", "subnet", subnetName)
		}
		// fetch latest subnet if it's a retry
		if attempt > 0 {
			if err := s.kube.Get(ctx, kclient.ObjectKeyFromObject(subnet), subnet); err != nil {
				return fmt.Errorf("fetching latest subnet %s: %w", subnetName, err)
			}

			if subnet.UID != uid {
				return fmt.Errorf("subnet %s UID mismatch", subnetName) //nolint:err113
			}
		}
		attempt++

		if err := mutate(subnet); err != nil {
			return fmt.Errorf("mutating subnet %s: %w", subnetName, err)
		}

		if err := s.kube.Status().Update(ctx, subnet); err != nil {
			return fmt.Errorf("updating subnet %s (res %s gen %d): %w", subnetName, subnet.ResourceVersion, subnet.Generation, err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("retrying subnet %s update: %w", subnetName, err)
	}

	return nil
}
