// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"time"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/version"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// develVersion is the version of builds without a version set, they never count as initialized
const develVersion = "(devel)"

const (
	initRetryMin = 1 * time.Second
	initRetryMax = 30 * time.Second
)

// FabricControllerInitializer initializes the running fabric controller version once: the default fabric for a
// deployment that predates fabrics, the built-in switch profiles and the refresh of stored objects. Recording the
// version in the FabricController then unlocks the fabric controller on every replica. While it runs the fabric
// controller is locked, so its own writes are the only ones.
type FabricControllerInitializer struct {
	kclient.Client
	key      ktypes.NamespacedName
	version  string
	cfg      *meta.FabricConfig
	profiles *switchprofile.Default
}

func SetupFabricControllerInitializerWith(mgr kctrl.Manager, namespace string, cfg *meta.FabricConfig, profiles *switchprofile.Default) error {
	if namespace == "" {
		return fmt.Errorf("fabric controller namespace is empty") //nolint:err113
	}
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:err113
	}
	if profiles == nil {
		return fmt.Errorf("switch profiles are nil") //nolint:err113
	}

	if err := mgr.Add(&FabricControllerInitializer{
		Client:   mgr.GetClient(),
		key:      ktypes.NamespacedName{Namespace: namespace, Name: fcintapi.FabricControllerName},
		version:  version.Version,
		cfg:      cfg,
		profiles: profiles,
	}); err != nil {
		return fmt.Errorf("adding fabric controller initializer: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=fcint.githedgehog.com,resources=fabriccontrollers,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=fcint.githedgehog.com,resources=fabriccontrollers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=fabrics,verbs=get;list;watch;create

var (
	_ manager.Runnable               = (*FabricControllerInitializer)(nil)
	_ manager.LeaderElectionRunnable = (*FabricControllerInitializer)(nil)
)

func (i *FabricControllerInitializer) NeedLeaderElection() bool {
	return true
}

// Start keeps retrying until the running version is initialized: giving up would leave the fabric controller locked
func (i *FabricControllerInitializer) Start(ctx context.Context) error {
	l := kctrllog.FromContext(ctx).WithValues("initializer", "fabric-controller", "version", i.version)
	ctx = kctrllog.IntoContext(ctx, l)

	delay := initRetryMin
	for {
		err := i.initialize(ctx)
		if err == nil {
			return nil
		}

		l.Info("Failed to initialize, retrying", "error", err.Error(), "in", delay)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}

		delay = min(2*delay, initRetryMax)
	}
}

func (i *FabricControllerInitializer) initialize(ctx context.Context) error {
	l := kctrllog.FromContext(ctx)

	fc, err := i.getOrCreate(ctx)
	if err != nil {
		return err
	}

	if fc.Status.InitializedVersion == i.version && i.version != develVersion {
		l.Info("Already initialized")

		return nil
	}

	l.Info("Initializing", "initialized", fc.Status.InitializedVersion)

	if err := i.ensureFabricForUpgrade(ctx); err != nil {
		return err
	}

	if err := i.profiles.Enforce(ctx, i.Client, i.cfg, true); err != nil {
		return fmt.Errorf("enforcing switch profiles: %w", err)
	}

	fc.Status.InitializedVersion = i.version
	kmeta.SetStatusCondition(&fc.Status.Conditions, kmetav1.Condition{
		Type:               fcintapi.ConditionInitialized,
		Status:             kmetav1.ConditionTrue,
		ObservedGeneration: fc.Generation,
		Reason:             "Initialized",
		Message:            "Initialized version " + i.version,
	})
	if err := i.Status().Update(ctx, fc); err != nil {
		return fmt.Errorf("recording initialized version: %w", err)
	}

	l.Info("Initialized")

	return nil
}

func (i *FabricControllerInitializer) getOrCreate(ctx context.Context) (*fcintapi.FabricController, error) {
	fc := &fcintapi.FabricController{}
	err := i.Get(ctx, i.key, fc)
	if err == nil {
		return fc, nil
	}
	if !kapierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting fabric controller: %w", err)
	}

	fc = &fcintapi.FabricController{ObjectMeta: kmetav1.ObjectMeta{Namespace: i.key.Namespace, Name: i.key.Name}}
	if err := i.Create(ctx, fc); err != nil {
		return nil, fmt.Errorf("creating fabric controller: %w", err)
	}

	return fc, nil
}

// ensureFabricForUpgrade creates the default fabric for a deployment that predates fabrics: no fabric at all but
// objects that refer to one. Refreshing them then puts them into the default fabric and domain. A new deployment
// has nothing yet when it's initialized and gets its fabrics from the user
func (i *FabricControllerInitializer) ensureFabricForUpgrade(ctx context.Context) error {
	fabrics := &wiringapi.FabricList{}
	if err := i.List(ctx, fabrics, kclient.Limit(1)); err != nil {
		return fmt.Errorf("listing fabrics: %w", err)
	}
	if len(fabrics.Items) > 0 {
		return nil
	}

	predates := false
	for _, objs := range []kclient.ObjectList{
		&wiringapi.SwitchList{},
		&wiringapi.SwitchGroupList{},
		&wiringapi.ConnectionList{},
		&vpcapi.IPv4NamespaceList{},
		&vpcapi.VPCList{},
		&vpcapi.ExternalList{},
		&gwapi.GatewayList{},
	} {
		if err := i.List(ctx, objs, kclient.Limit(1)); err != nil {
			return fmt.Errorf("listing %T: %w", objs, err)
		}
		if kmeta.LenList(objs) > 0 {
			predates = true

			break
		}
	}
	if !predates {
		return nil
	}

	fabric := &wiringapi.Fabric{
		ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric},
		Spec:       wiringapi.DefaultFabricSpec(i.cfg),
	}
	if err := i.Create(ctx, fabric); err != nil {
		return fmt.Errorf("creating default fabric: %w", err)
	}

	kctrllog.FromContext(ctx).Info("Created the default fabric for a deployment that predates fabrics")

	return nil
}
