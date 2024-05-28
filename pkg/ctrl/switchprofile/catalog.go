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

package switchprofile

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var defaultSwitchProfiles = []wiringapi.SwitchProfile{
	profileVS,
	profileDellS5248FON,
	profileDellS5232FON,
	profileEdgecoreEPS203,
}

type Default struct {
	store       map[string]*wiringapi.SwitchProfile
	initialized uint32
}

func NewDefaultSwitchProfiles() *Default {
	return &Default{
		store: map[string]*wiringapi.SwitchProfile{},
	}
}

func (d *Default) Register(ctx context.Context, kube client.Reader, cfg *meta.FabricConfig, sp wiringapi.SwitchProfile) error {
	if sp.Name == "" {
		return errors.Errorf("switch profile name must be set")
	}

	if _, exists := d.store[sp.Name]; exists {
		return errors.Errorf("switch profile %q already registered", sp.Name)
	}

	sp.Namespace = metav1.NamespaceDefault

	sp.Default()

	_, err := sp.Validate(ctx, kube, cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to validate switch profile")
	}

	d.store[sp.Name] = &sp

	return nil
}

func (d *Default) RegisterAll(ctx context.Context, kube client.Reader, cfg *meta.FabricConfig) error {
	for _, sp := range defaultSwitchProfiles {
		if err := d.Register(ctx, kube, cfg, sp); err != nil {
			return errors.Wrapf(err, "failed to register switch profile %q", sp.Name)
		}
	}

	return nil
}

func (d *Default) Enforce(ctx context.Context, kube client.Client, cfg *meta.FabricConfig, logs bool) error {
	if !cfg.AllowExtraSwitchProfiles {
		sps := &wiringapi.SwitchProfileList{}
		if err := kube.List(ctx, sps); err != nil {
			return errors.Wrap(err, "failed to list switch profiles")
		}

		for _, sp := range sps.Items {
			if _, exists := d.store[sp.Name]; exists {
				continue
			}

			sp := sp
			err := kube.Delete(ctx, &sp)
			if err != nil {
				return errors.Wrapf(err, "failed to delete non-default switch profile %q", sp.Name)
			}
		}
	}

	for _, defaultSp := range d.store {
		sp := &wiringapi.SwitchProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      defaultSp.Name,
				Namespace: defaultSp.Namespace,
			},
		}
		var err error
		var res ctrlutil.OperationResult
		if res, err = ctrlutil.CreateOrUpdate(ctx, kube, sp, func() error {
			sp.Spec = defaultSp.Spec

			return nil
		}); err != nil {
			return errors.Wrapf(err, "failed to create or update switch profile %q", sp.Name)
		}

		if logs && res != ctrlutil.OperationResultNone {
			l := log.FromContext(ctx)
			l.Info("switch profile reconciled", "name", sp.Name, "operation", res)
		}
	}

	atomic.StoreUint32(&d.initialized, 1)

	return nil
}

func (d *Default) Get(name string) *wiringapi.SwitchProfile {
	return d.store[name]
}

func (d *Default) IsInitialized() bool {
	return atomic.LoadUint32(&d.initialized) == 1
}

func (d *Default) List() []*wiringapi.SwitchProfile {
	return maps.Values(d.store)
}
