// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/version"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=fcint.githedgehog.com,resources=switchboots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=fcint.githedgehog.com,resources=switchboots/status,verbs=get;update;patch

// +kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches,verbs=get;list;watch
// +kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles,verbs=get;list;watch

type SwitchBootReconciler struct {
	kclient.Client
	lock *Lock
}

func SetupSwitchBootReconcilerWith(mgr kctrl.Manager, lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &SwitchBootReconciler{
		Client: mgr.GetClient(),
		lock:   lock,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("SwitchBoot").
		For(&wiringapi.Switch{}).
		// recreated if deleted, garbage collected with the switch otherwise
		Owns(&fcintapi.SwitchBoot{}, builder.WithPredicates(onlyDeletes)).
		Watches(&wiringapi.SwitchProfile{}, handler.EnqueueRequestsFromMapFunc(enqueueSwitchesByProfile(r, lock))).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *SwitchBootReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	sw := &wiringapi.Switch{}
	if err := r.Get(ctx, req.NamespacedName, sw); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting switch: %w", err)
	}

	if sw.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}

	if sw.Spec.Profile == "" {
		return kctrl.Result{}, fmt.Errorf("switch has no profile") //nolint:err113
	}

	sp := &wiringapi.SwitchProfile{}
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: sw.Namespace, Name: sw.Spec.Profile}, sp); err != nil {
		return kctrl.Result{}, fmt.Errorf("getting switch profile %s: %w", sw.Spec.Profile, err)
	}

	mac, err := wiringapi.NormalizeBootMAC(sw.Spec.Boot.MAC)
	if err != nil {
		// stored before the validation rejected it, it's still found by serial; any change to the switch is
		// rejected until it's fixed
		l.Info("Ignoring invalid switch boot MAC", "error", err.Error())
		mac = ""
	}

	sb := &fcintapi.SwitchBoot{Namespace: sw.Namespace, Name: sw.Name}
	res, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sb, func() error {
		if err := setOwner(sw, sb, r.Scheme()); err != nil {
			return err
		}

		sb.Spec = fcintapi.SwitchBootSpec{
			Serial:       wiringapi.NormalizeBootSerial(sw.Spec.Boot.Serial),
			MAC:          mac,
			NOSType:      sp.Spec.NOSType,
			AgentVersion: version.Version,
		}

		return nil
	})
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("creating or updating switch boot: %w", err)
	}

	if res != ctrlutil.OperationResultNone {
		l.Info("SwitchBoot reconciled", "op", res)
	}

	return kctrl.Result{}, nil
}
