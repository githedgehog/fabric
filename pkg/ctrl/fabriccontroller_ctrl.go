// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/pkg/version"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// FabricControllerReconciler locks or unlocks the fabric controller depending on whether the FabricController
// records the running version as initialized
type FabricControllerReconciler struct {
	kclient.Client
	key  ktypes.NamespacedName
	lock *Lock
}

func SetupFabricControllerReconcilerWith(mgr kctrl.Manager, namespace string, lock *Lock) error {
	if namespace == "" {
		return fmt.Errorf("fabric controller namespace is empty") //nolint:err113
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &FabricControllerReconciler{
		Client: mgr.GetClient(),
		key:    ktypes.NamespacedName{Namespace: namespace, Name: fcintapi.FabricControllerName},
		lock:   lock,
	}

	onlyOwn := predicate.NewPredicateFuncs(func(obj kclient.Object) bool {
		return obj.GetNamespace() == r.key.Namespace && obj.GetName() == r.key.Name
	})

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("FabricController").
		For(&fcintapi.FabricController{}, builder.WithPredicates(onlyOwn)).
		// webhooks are served by every replica and need the lock, not only the leader's reconcilers
		WithOptions(controller.Options{NeedLeaderElection: ptr.To(false)}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up fabric controller controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=fcint.githedgehog.com,resources=fabriccontrollers,verbs=get;list;watch

func (r *FabricControllerReconciler) Reconcile(ctx context.Context, _ kctrl.Request) (kctrl.Result, error) {
	fc := &fcintapi.FabricController{}
	if err := r.Get(ctx, r.key, fc); err != nil {
		if kapierrors.IsNotFound(err) {
			r.lock.set(ctx, true, "not initialized yet")

			return kctrl.Result{}, nil
		}

		// keep the current state, it's locked from the start and an unlocked controller shouldn't flap on a read error
		return kctrl.Result{}, fmt.Errorf("getting fabric controller: %w", err)
	}

	switch {
	case fc.Spec.ForceUnlock:
		r.lock.set(ctx, false, "force unlocked", "initialized", fc.Status.InitializedVersion, "running", version.Version)
	case fc.Status.InitializedVersion == version.Version:
		r.lock.set(ctx, false, "initialized", "version", version.Version)
	default:
		r.lock.set(ctx, true, "running version not initialized yet", "initialized", fc.Status.InitializedVersion, "running", version.Version)
	}

	return kctrl.Result{}, nil
}
