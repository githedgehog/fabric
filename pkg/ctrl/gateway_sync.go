// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"slices"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type GwVPCSync struct {
	kclient.Client
	cfg  *meta.FabricConfig
	libr *librarian.Manager
	lock *Lock
}

func SetupGwVPCSyncReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager, lock *Lock) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:goerr113
	}
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:goerr113
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &GwVPCSync{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
		lock:   lock,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("GwVPCSync").
		For(&vpcapi.VPC{}).
		// recreated if deleted and reverted if its spec is changed, but not reconciled on its status updates
		Owns(&gwapi.VPCInfo{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/finalizers,verbs=update

func (r *GwVPCSync) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	vpc := &vpcapi.VPC{}
	if err := r.Get(ctx, req.NamespacedName, vpc); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting VPC %s: %w", req.NamespacedName, err)
	}
	// its VPCInfo is garbage collected with it
	if vpc.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}
	// copied into the VPCInfo, whose defaulting would put it into the default fabric and domain otherwise
	if vpc.Spec.Topology.Fabric == "" || len(vpc.Spec.Topology.Domains) == 0 {
		return kctrl.Result{}, fmt.Errorf("VPC %s has no fabric or domains", vpc.Name) //nolint:err113
	}

	vni, err := r.libr.GetOrEnsureVPCVNI(ctx, r.Client, vpc)
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
		if err := setOwner(vpc, vpcInfo, r.Scheme()); err != nil {
			return err
		}

		vpcInfo.Spec = gwapi.VPCInfoSpec{
			Topology: gwapi.VPCInfoTopology{
				Fabric:  vpc.Spec.Topology.Fabric,
				Domains: slices.Clone(vpc.Spec.Topology.Domains),
			},
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
	lock *Lock
}

func SetupGwExternalSyncReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager, lock *Lock) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:goerr113
	}
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:goerr113
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &GwExternalSync{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
		lock:   lock,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("GwExternalSync").
		For(&vpcapi.External{}).
		// recreated if deleted and reverted if its spec is changed, but not reconciled on its status updates
		Owns(&gwapi.VPCInfo{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals/finalizers,verbs=update

//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos/finalizers,verbs=update

func (r *GwExternalSync) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	external := &vpcapi.External{}
	if err := r.Get(ctx, req.NamespacedName, external); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting External %s: %w", req.NamespacedName, err)
	}
	// its VPCInfo is garbage collected with it
	if external.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}
	// copied into the VPCInfo, whose defaulting would put it into the default fabric and domain otherwise
	if external.Spec.Topology.Fabric == "" || external.Spec.Topology.Domain == "" {
		return kctrl.Result{}, fmt.Errorf("external %s has no fabric or domain", external.Name) //nolint:err113
	}

	// an External attached to no switch only gets its VNI allocated here
	vni, err := r.libr.GetOrEnsureExternalVNI(ctx, r.Client, external.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting External %s VNI: %w", external.Name, err)
	}

	// FIXME: the external spec does not have the prefixes we are importing, they are part of the externalPeering
	if err := syncVRFVPCInfo(ctx, r.Client, external, vpcapi.VPCInfoExtPrefix+external.Name, "external",
		external.Spec.Topology.Fabric, external.Spec.Topology.Domain, vni); err != nil {
		return kctrl.Result{}, err
	}

	return kctrl.Result{}, nil
}

// syncVRFVPCInfo creates or updates the VPCInfo of an External or RemotePeering, owned by it. Both are reached through
// their VRF, which holds whatever the other side advertises, so the VPCInfo gets a single 0.0.0.0/0 subnet
func syncVRFVPCInfo(ctx context.Context, kube kclient.Client, owner kclient.Object, name, subnetName, fabric, domain string, vni uint32) error {
	vpcInfo := &gwapi.VPCInfo{ObjectMeta: kmetav1.ObjectMeta{
		Name:      name,
		Namespace: owner.GetNamespace(),
	}}
	op, err := ctrlutil.CreateOrUpdate(ctx, kube, vpcInfo, func() error {
		if err := setOwner(owner, vpcInfo, kube.Scheme()); err != nil {
			return err
		}

		vpcInfo.Spec = gwapi.VPCInfoSpec{
			Topology: gwapi.VPCInfoTopology{
				Fabric:  fabric,
				Domains: []string{domain},
			},
			VNI: vni,
			Subnets: map[string]*gwapi.VPCInfoSubnet{
				subnetName: {CIDR: "0.0.0.0/0"},
			},
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("creating/updating VPCInfo %s: %w", name, err)
	}
	if op == ctrlutil.OperationResultCreated || op == ctrlutil.OperationResultUpdated {
		kctrllog.FromContext(ctx).Info("Gateway VPCInfo synced", "op", op)
	}

	return nil
}

// RemotePeering equivalent of the above code

type GwRemotePeeringSync struct {
	kclient.Client
	cfg  *meta.FabricConfig
	libr *librarian.Manager
	lock *Lock
}

func SetupGwRemotePeeringSyncReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager, lock *Lock) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:goerr113
	}
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:goerr113
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &GwRemotePeeringSync{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
		lock:   lock,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("GwRemotePeeringSync").
		For(&vpcapi.RemotePeering{}).
		// recreated if deleted and reverted if its spec is changed, but not reconciled on its status updates
		Owns(&gwapi.VPCInfo{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=remotepeerings,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=remotepeerings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=remotepeerings/finalizers,verbs=update

func (r *GwRemotePeeringSync) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	rp := &vpcapi.RemotePeering{}
	if err := r.Get(ctx, req.NamespacedName, rp); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting RemotePeering %s: %w", req.NamespacedName, err)
	}
	// its VPCInfo is garbage collected with it
	if rp.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}
	// copied into the VPCInfo, whose defaulting would put it into the default fabric and domain otherwise
	if rp.Spec.Topology.Fabric == "" || rp.Spec.Topology.Domain == "" {
		return kctrl.Result{}, fmt.Errorf("remote peering %s has no fabric or domain", rp.Name) //nolint:err113
	}

	vni, err := r.libr.GetOrEnsureRemotePeeringVNI(ctx, r.Client, rp.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting RemotePeering %s VNI: %w", rp.Name, err)
	}

	if err := syncVRFVPCInfo(ctx, r.Client, rp, vpcapi.VPCInfoRPPrefix+rp.Name, "remote",
		rp.Spec.Topology.Fabric, rp.Spec.Topology.Domain, vni); err != nil {
		return kctrl.Result{}, err
	}

	return kctrl.Result{}, nil
}
