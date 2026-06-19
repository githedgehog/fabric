// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) setupKube(ctx context.Context) error {
	kube, err := kubeutil.NewClient(ctx, "", dhcpapi.AddToScheme, wiringapi.AddToScheme)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}
	s.kube = kube

	return nil
}

// watchList is the shared event loop for all K8s watches. handle is called for
// Added, Modified and Deleted events with the event type and object.
func (s *Server) watchList(
	ctx context.Context,
	name string,
	list kclient.ObjectList,
	handle func(watch.EventType, kruntime.Object),
) error {
	watcher, err := s.kube.Watch(ctx, list, kclient.InNamespace(kmetav1.NamespaceDefault))
	if err != nil {
		return fmt.Errorf("starting %s watcher: %w", name, err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("%s watch channel closed", name)
			}
			if event.Object == nil {
				return fmt.Errorf("received nil object from %s watch", name)
			}

			switch event.Type {
			case watch.Error:
				if err, ok := event.Object.(error); ok {
					return fmt.Errorf("%s watch error: %w", name, err)
				}

				return fmt.Errorf("%s watch error", name)
			case watch.Bookmark:
				continue
			case watch.Added, watch.Modified, watch.Deleted:
				handle(event.Type, event.Object)
			default:
				slog.Warn("Unexpected watch event", "resource", name, "event", event.Type)
			}
		}
	}
}

func (s *Server) watchDHCPSubnets(ctx context.Context) error {
	slog.Debug("Starting DHCPSubnet watcher")

	return s.watchList(ctx, "DHCPSubnet", &dhcpapi.DHCPSubnetList{}, func(et watch.EventType, obj kruntime.Object) {
		subnet := obj.(*dhcpapi.DHCPSubnet)
		key := subnetKeyFrom(subnet)
		slog.Debug("Received", "event", et, "subnet", subnet.Name, "key", key)

		s.m.Lock()
		if et == watch.Deleted {
			delete(s.subnets, key)
		} else {
			if subnet.Status.Allocated == nil {
				subnet.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
			}
			s.subnets[key] = subnet
		}
		s.m.Unlock()
	})
}

func (s *Server) watchSwitches(ctx context.Context) error {
	slog.Debug("Starting Switch watcher")

	return s.watchList(ctx, "Switch", &wiringapi.SwitchList{}, func(et watch.EventType, obj kruntime.Object) {
		sw := obj.(*wiringapi.Switch)
		slog.Debug("Switch received", "event", et, "switch", sw.Name)

		if et == watch.Deleted {
			s.m.Lock()
			if oldIP := s.switchToIP[sw.Name]; oldIP != "" {
				delete(s.switchToIP, sw.Name)
				delete(s.relayAllowlist, oldIP)
			}
			s.m.Unlock()

			return
		}

		if !sw.Spec.Role.IsLeaf() || sw.Spec.IP == "" {
			return
		}

		prefix, err := netip.ParsePrefix(sw.Spec.IP)
		if err != nil {
			slog.Warn("Switch has unparseable IP, skipping relay allowlist update",
				"switch", sw.Name, "ip", sw.Spec.IP)

			return
		}
		newIP := prefix.Addr().String()

		s.m.Lock()
		if oldIP := s.switchToIP[sw.Name]; oldIP != "" && oldIP != newIP {
			delete(s.relayAllowlist, oldIP)
		}
		s.switchToIP[sw.Name] = newIP
		s.relayAllowlist[newIP] = struct{}{}
		s.m.Unlock()
	})
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
				return fmt.Errorf("subnet %s UID mismatch", subnetName)
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
