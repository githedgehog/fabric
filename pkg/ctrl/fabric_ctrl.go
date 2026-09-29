// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"time"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type FabricInitializer struct {
	kclient.Client
	cfg *meta.FabricConfig
}

func SetupFabricInitializerWith(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:err113
	}

	if err := mgr.Add(&FabricInitializer{
		Client: mgr.GetClient(),
		cfg:    cfg,
	}); err != nil {
		return fmt.Errorf("failed to add fabric initializer: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=fabrics,verbs=get;list;watch;create;update

var (
	_ manager.Runnable               = (*FabricInitializer)(nil)
	_ manager.LeaderElectionRunnable = (*FabricInitializer)(nil)
)

func (i *FabricInitializer) Start(ctx context.Context) error {
	l := kctrllog.FromContext(ctx).WithValues("initializer", "fabric")
	l.Info("Fabric initial setup")

	var err error
	for attempt := 0; attempt < 60; attempt++ { // TODO think about more graceful way to handle this
		if err = i.ensureDefaultFabric(ctx); err == nil {
			return nil
		}

		l.Info("Failed to ensure the default fabric", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("fabric initializer cancelled: %w", ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}

	return err
}

func (i *FabricInitializer) NeedLeaderElection() bool {
	return true
}

// TODO backfill the fabric labels onto objects written before they existed. Wiring is applied on
// install and never re-applied on upgrade, so those objects carry no fabric label and drop out of
// a label-filtered list. Doing it here would need the manager to hold update on every declaring
// type, so it belongs on fabricator's upgrade path instead, where the credentials already exist.
func (i *FabricInitializer) ensureDefaultFabric(ctx context.Context) error {
	fabric := &wiringapi.Fabric{ObjectMeta: kmetav1.ObjectMeta{
		Name:      wiringapi.DefaultFabric,
		Namespace: kmetav1.NamespaceDefault,
	}}

	spec := wiringapi.DefaultFabricSpec(i.cfg)

	err := i.Get(ctx, kclient.ObjectKeyFromObject(fabric), fabric)
	if kapierrors.IsNotFound(err) {
		fabric.Spec = spec
		if err := i.Create(ctx, fabric); err != nil {
			return fmt.Errorf("failed to create fabric %s: %w", fabric.Name, err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get fabric %s: %w", fabric.Name, err)
	}

	if fabric.Spec.LeafASNStart != 0 {
		return nil
	}

	fabric.Spec = spec
	if err := i.Update(ctx, fabric); err != nil {
		return fmt.Errorf("failed to update fabric %s: %w", fabric.Name, err)
	}

	return nil
}
