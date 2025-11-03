// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"strings"

	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type GwVPCSync struct {
	kclient.Client
	cfg  *meta.FabricConfig
	libr *librarian.Manager
}

func SetupGwVPCSyncReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:goerr113
	}
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:goerr113
	}

	r := &GwVPCSync{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("GwVPCSync").
		For(&vpcapi.VPC{}).
		// TODO consider relying on the owner reference
		Watches(&gwapi.VPCInfo{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForVPCInfo)).
		Complete(r); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	return nil
}

func (r *GwVPCSync) enqueueForVPCInfo(ctx context.Context, obj kclient.Object) []reconcile.Request {
	vpcInfo, ok := obj.(*gwapi.VPCInfo)
	if !ok {
		kctrllog.FromContext(ctx).Info("Enqueue: object is not a VPCInfo", "obj", obj)

		return nil
	}

	return []reconcile.Request{
		{NamespacedName: ktypes.NamespacedName{
			Namespace: vpcInfo.Namespace,
			Name:      vpcInfo.Name,
		}},
	}
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/finalizers,verbs=update

func (r *GwVPCSync) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	vpc := &vpcapi.VPC{}
	if err := r.Get(ctx, req.NamespacedName, vpc); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting VPC %s: %w", req.NamespacedName, err)
	}

	vni, err := r.libr.GetVPCVNI(ctx, r.Client, vpc.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting VPC %s VNI: %w", vpc.Name, err)
	}

	subnets := map[string]*gwapi.VPCInfoSubnet{}
	for subnetName, subnet := range vpc.Spec.Subnets {
		subnets[subnetName] = &gwapi.VPCInfoSubnet{
			CIDR: subnet.Subnet,
		}
	}

	vpcInfo := &gwapi.VPCInfo{ObjectMeta: kmetav1.ObjectMeta{
		Name:      vpc.Name,
		Namespace: vpc.Namespace,
	}}
	if op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, vpcInfo, func() error {
		if err := ctrlutil.SetControllerReference(vpc, vpcInfo, r.Scheme(),
			ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
			return fmt.Errorf("setting controller reference: %w", err)
		}

		vpcInfo.Spec = gwapi.VPCInfoSpec{
			VNI:     vni,
			Subnets: subnets,
		}

		return nil
	}); err != nil {
		return kctrl.Result{}, fmt.Errorf("creating/updating VPCInfo %s: %w", req.NamespacedName, err)
	} else if op == ctrlutil.OperationResultCreated || op == ctrlutil.OperationResultUpdated {
		l.Info("Gateway VPCInfo synced", "op", op)
	}

	return kctrl.Result{}, nil
}

// External equivalent of the above code

type GwExternalSync struct {
	kclient.Client
	cfg  *meta.FabricConfig
	libr *librarian.Manager
}

func SetupGwExternalSyncReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:goerr113
	}
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:goerr113
	}

	r := &GwExternalSync{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("GwExternalSync").
		For(&vpcapi.External{}).
		// TODO consider relying on the owner reference
		Watches(&gwapi.VPCInfo{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForVPCInfo)).
		Complete(r); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	return nil
}

func (r *GwExternalSync) enqueueForVPCInfo(ctx context.Context, obj kclient.Object) []reconcile.Request {
	vpcInfo, ok := obj.(*gwapi.VPCInfo)
	if !ok {
		kctrllog.FromContext(ctx).Info("Enqueue: object is not a VPCInfo", "obj", obj)

		return nil
	}

	if !strings.HasPrefix(vpcInfo.Name, vpcapi.VPCInfoExtPrefix) {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: ktypes.NamespacedName{
			Namespace: vpcInfo.Namespace,
			Name:      strings.TrimPrefix(vpcInfo.Name, vpcapi.VPCInfoExtPrefix),
		}},
	}
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/finalizers,verbs=update

func (r *GwExternalSync) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	external := &vpcapi.External{}
	if err := r.Get(ctx, req.NamespacedName, external); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting External %s: %w", req.NamespacedName, err)
	}

	vni, err := r.libr.GetExternalVNI(ctx, r.Client, external.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting External %s VNI: %w", external.Name, err)
	}

	subnets := map[string]*gwapi.VPCInfoSubnet{}
	// FIXME: the external spec does not have the prefixes we are importing, they are part of the externalPeering
	subnets["external"] = &gwapi.VPCInfoSubnet{
		CIDR: "0.0.0.0/0",
	}

	vpcInfo := &gwapi.VPCInfo{ObjectMeta: kmetav1.ObjectMeta{
		Name:      vpcapi.VPCInfoExtPrefix + external.Name,
		Namespace: external.Namespace,
	}}
	if op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, vpcInfo, func() error {
		if err := ctrlutil.SetControllerReference(external, vpcInfo, r.Scheme(),
			ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
			return fmt.Errorf("setting controller reference: %w", err)
		}

		vpcInfo.Spec = gwapi.VPCInfoSpec{
			VNI:     vni,
			Subnets: subnets,
		}

		return nil
	}); err != nil {
		return kctrl.Result{}, fmt.Errorf("creating/updating VPCInfo %s: %w", req.NamespacedName, err)
	} else if op == ctrlutil.OperationResultCreated || op == ctrlutil.OperationResultUpdated {
		l.Info("Gateway VPCInfo synced", "op", op)
	}

	return kctrl.Result{}, nil
}
