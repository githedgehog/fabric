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
}

func SetupVPCInfoReconcilerWith(mgr kctrl.Manager, libMngr *librarian.Manager) error {
	r := &VPCInfoReconciler{
		Client: mgr.GetClient(),
		libr:   libMngr,
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

	l.Info("Reconciling VPCInfo")

	vpcID, err := r.libr.UpdateAndGetVPCInfoID(ctx, r, VPCID.GetMaxValue(), req.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("updating vpcinfo id: %w", err)
	}

	vpc.Status.InternalID, err = VPCID.Encode(vpcID)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("encoding vpcinfo id: %w", err)
	}

	if err := r.Status().Update(ctx, vpc); err != nil {
		return kctrl.Result{}, fmt.Errorf("updating vpcinfo status: %w", err)
	}

	return kctrl.Result{}, nil
}
