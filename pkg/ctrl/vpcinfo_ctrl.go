// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

type VPCInfoReconciler struct {
	kclient.Client
	libr *librarian.Manager
	lock *Lock
}

func SetupVPCInfoReconcilerWith(mgr kctrl.Manager, libMngr *librarian.Manager, lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &VPCInfoReconciler{
		Client: mgr.GetClient(),
		libr:   libMngr,
		lock:   lock,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("VPCInfo").
		For(&gwapi.VPCInfo{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *VPCInfoReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	vpc := &gwapi.VPCInfo{}
	if err := r.Get(ctx, req.NamespacedName, vpc); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting vpcinfo: %w", err)
	}

	if vpc.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}

	vpcID, err := r.libr.GetOrEnsureVPCInfoID(ctx, r, VPCID.GetMaxValue(), req.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting vpcinfo id: %w", err)
	}

	internalID, err := VPCID.Encode(vpcID)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("encoding vpcinfo id: %w", err)
	}

	if vpc.Status.InternalID == internalID {
		return kctrl.Result{}, nil
	}

	vpc.Status.InternalID = internalID
	if err := r.Status().Update(ctx, vpc); err != nil {
		return kctrl.Result{}, fmt.Errorf("updating vpcinfo status: %w", err)
	}

	l.Info("VPCInfo internal ID updated", "id", internalID)

	return kctrl.Result{}, nil
}
