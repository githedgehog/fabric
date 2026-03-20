// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/status,verbs=get;update;patch

type VPCInfoReconciler struct {
	kclient.Client
}

func SetupVPCInfoReconcilerWith(mgr kctrl.Manager) error {
	r := &VPCInfoReconciler{
		Client: mgr.GetClient(),
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

		return kctrl.Result{}, fmt.Errorf("getting vpc info: %w", err)
	}

	if vpc.DeletionTimestamp != nil {
		l.Info("VPCInfo is being deleted, skipping")

		return kctrl.Result{}, nil
	}

	if vpc.IsReady() {
		return kctrl.Result{}, nil
	}

	l.Info("Reconciling VPCInfo")

	// TODO actually generate a unique ID in a reliable way
	// this is a temporary solution, we should use a proper way of generating unique IDs
	vpcs := &gwapi.VPCInfoList{}
	if err := r.List(ctx, vpcs); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing vpc info: %w", err)
	}

	taken := map[uint32]bool{}
	for _, v := range vpcs.Items {
		if v.Status.InternalID == "" {
			continue
		}

		id, err := VPCID.Decode(v.Status.InternalID)
		if err != nil {
			return kctrl.Result{}, fmt.Errorf("decoding vpc id: %w", err)
		}
		taken[id] = true
	}

	for i := range VPCID.GetMaxValue() {
		if !taken[i] {
			vpc.Status.InternalID, _ = VPCID.Encode(i)

			break
		}
	}

	if vpc.Status.InternalID == "" {
		return kctrl.Result{}, fmt.Errorf("no available vpc id") //nolint:err113
	}

	if err := r.Status().Update(ctx, vpc); err != nil {
		return kctrl.Result{}, fmt.Errorf("updating vpc status: %w", err)
	}

	return kctrl.Result{}, nil
}
