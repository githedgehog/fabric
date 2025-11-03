// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) setupKube(ctx context.Context) error {
	kube, err := kubeutil.NewClient(ctx, "", dhcpapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}
	s.kube = kube

	return nil
}

func (s *Server) watchKube(ctx context.Context) error {
	slog.Debug("Starting K8s watcher")

	watcher, err := s.kube.Watch(ctx, &dhcpapi.DHCPSubnetList{}, kclient.InNamespace(kmetav1.NamespaceDefault))
	if err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil // TODO
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed") //nolint:err113
			}

			if event.Object == nil {
				return fmt.Errorf("received nil object from K8s") //nolint:err113
			}

			switch event.Type {
			case watch.Error:
				if err, ok := event.Object.(error); ok {
					return fmt.Errorf("watch error: %w", err)
				}

				return fmt.Errorf("watch error") //nolint:err113
			case watch.Bookmark:
				continue
			case watch.Added, watch.Modified:
				subnet := event.Object.(*dhcpapi.DHCPSubnet)
				if subnet.Status.Allocated == nil {
					subnet.Status.Allocated = map[string]dhcpapi.DHCPAllocated{}
				}

				key := subnetKeyFrom(subnet)
				slog.Debug("Received", "event", event.Type, "subnet", subnet.Name, "key", key)

				s.m.Lock()
				existing, ok := s.subnets[key]
				if !ok || subnet.ResourceVersion > existing.ResourceVersion {
					s.subnets[key] = subnet
				}
				s.m.Unlock()
			case watch.Deleted:
				subnet := event.Object.(*dhcpapi.DHCPSubnet)
				key := subnetKeyFrom(subnet)
				slog.Debug("Received", "event", event.Type, "subnet", subnet.Name, "key", key)

				s.m.Lock()
				delete(s.subnets, key)
				s.m.Unlock()
			default:
				slog.Warn("Unexpected", "event", event.Type)
			}
		}
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
