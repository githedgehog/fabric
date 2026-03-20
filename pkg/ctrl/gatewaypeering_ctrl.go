// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"reflect"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaypeerings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaypeerings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=get;list;watch;create;update;patch;delete

type PeeringReconciler struct {
	kclient.Client
}

func SetupPeeringReconcilerWith(mgr kctrl.Manager) error {
	r := &PeeringReconciler{
		Client: mgr.GetClient(),
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("Peering").
		For(&gwapi.GatewayPeering{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *PeeringReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	peering := &gwapi.GatewayPeering{}
	if err := r.Get(ctx, req.NamespacedName, peering); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting peering: %w", err)
	}

	if peering.DeletionTimestamp != nil {
		l.Info("Peering is being deleted, skipping")

		return kctrl.Result{}, nil
	}

	{
		defGwGr := &gwapi.GatewayGroup{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      gwapi.DefaultGatewayGroup,
				Namespace: kmetav1.NamespaceDefault,
			},
		}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, defGwGr, func() error {
			return nil
		}); err != nil {
			return kctrl.Result{}, fmt.Errorf("creating/updating default gateway group: %w", err)
		}

		orig := peering.DeepCopy()
		peering.Default()
		if !reflect.DeepEqual(orig, peering) {
			l.Info("Applying defaults to Peering")

			if err := r.Update(ctx, peering); err != nil {
				return kctrl.Result{}, fmt.Errorf("updating peering: %w", err)
			}
		}
	}

	// l.Info("Reconciling Peering")

	return kctrl.Result{}, nil
}
