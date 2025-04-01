// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dhcpd

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	Verbose         bool
	Config          string
	ListenInterface string

	kubeUpdates  chan Event
	updateStatus func(dhcpapi.DHCPSubnet) error
}

type Event struct {
	Type   EventType
	Subnet *dhcpapi.DHCPSubnet
}

type EventType string

const (
	EventTypeAdded    EventType = "ADDED"
	EventTypeModified EventType = "MODIFIED"
	EventTypeDeleted  EventType = "DELETED"
)

func (d *Service) Run(ctx context.Context) error {
	kube, err := kubeutil.NewClient(ctx, "", dhcpapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	d.kubeUpdates = make(chan Event, 100)
	d.updateStatus = func(d dhcpapi.DHCPSubnet) error {
		// TODO download latest obj and try to update its status
		return errors.Wrapf(kube.Status().Update(ctx, &d), "failed to update status")
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := d.runKubeWatcher(ctx, kube); err != nil {
			slog.Error("KubeWatcher", "error", err)
		}

		time.Sleep(1 * time.Second)
		os.Exit(1)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := d.runCoreDHCP(ctx); err != nil {
			slog.Error("CoreDHCP", "error", err)
		}

		time.Sleep(1 * time.Second)
		os.Exit(2)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()

		d.handleExpiredLeases()
	}()
	wg.Wait()

	return nil
}

func (d *Service) runKubeWatcher(ctx context.Context, kube kclient.WithWatch) error {
	var err error
	var watcher watch.Interface

	for {
		if watcher == nil {
			slog.Info("Starting K8s watcher")
			if watcher, err = kube.Watch(ctx, &dhcpapi.DHCPSubnetList{}, kclient.InNamespace(kmetav1.NamespaceDefault)); err != nil {
				return errors.Wrapf(err, "failed to start watcher")
			}
			defer watcher.Stop()
		}

		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				slog.Warn("K8s watch channel closed, restarting watcher")
				watcher = nil

				continue
			}

			if event.Object == nil {
				slog.Warn("Received nil object from K8s, restarting watcher")
				watcher = nil

				continue
			}

			if event.Type == watch.Bookmark {
				slog.Info("Received watch event, ignoring", "event", event.Type)

				continue
			}

			if event.Type == watch.Error {
				slog.Error("Received watch error", "event", event.Type, "object", event.Object)
				if err, ok := event.Object.(error); ok {
					slog.Error("Watch error", "error", err)
				}

				watcher = nil

				continue
			}

			subnet := event.Object.(*dhcpapi.DHCPSubnet)
			slog.Debug("Received watch event", "event", event.Type, "subnet", subnet.Name)
			d.kubeUpdates <- Event{
				Type:   EventType(event.Type),
				Subnet: subnet,
			}
		}
	}
}
