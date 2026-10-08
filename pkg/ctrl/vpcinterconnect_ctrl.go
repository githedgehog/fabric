// Copyright 2026 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ctrl

import (
	"context"
	"fmt"
	"time"

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

// VPCInterconnectReconciler generates the External, ExternalAttachments and ExternalPeerings a VPCInterconnect stands
// for, and keeps them as specified there. They're garbage collected with it.
type VPCInterconnectReconciler struct {
	kclient.Client
	cfg  *meta.FabricConfig
	libr *librarian.Manager
	lock *Lock
}

func SetupVPCInterconnectReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager, lock *Lock) error {
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:err113
	}
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:err113
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &VPCInterconnectReconciler{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
		lock:   lock,
	}

	// recreated if deleted and reverted if their spec is changed, but not reconciled on their status updates
	if err := kctrl.NewControllerManagedBy(mgr).
		Named("VPCInterconnect").
		For(&vpcapi.VPCInterconnect{}).
		Owns(&vpcapi.External{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&vpcapi.ExternalAttachment{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&vpcapi.ExternalPeering{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcinterconnects,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcinterconnects/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externalattachments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externalpeerings,verbs=get;list;watch;create;update;patch;delete

func (r *VPCInterconnectReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	ic := &vpcapi.VPCInterconnect{}
	if err := r.Get(ctx, req.NamespacedName, ic); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting VPC interconnect %s: %w", req.NamespacedName, err)
	}
	// what it generated is garbage collected with it
	if ic.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}
	// copied into the generated objects, whose defaulting would put them into the default fabric and domain otherwise
	if ic.Spec.Topology.Fabric == "" || ic.Spec.Topology.Domain == "" {
		return kctrl.Result{}, fmt.Errorf("VPC interconnect %s has no fabric or domain", ic.Name) //nolint:err113
	}

	// The outbound community replaces the routes' VPC communities, which are only unique per controller, so that none
	// reaches the other side. It comes from the External's VNI, which can only be allocated once the External exists,
	// and the attachments wait for it so that no route goes out without it.
	comm := ""
	ext := &vpcapi.External{}
	if err := r.Get(ctx, kclient.ObjectKey{Namespace: ic.Namespace, Name: ic.Name}, ext); err == nil {
		vni, err := r.libr.GetOrEnsureExternalVNI(ctx, r.Client, ic.Name)
		if err != nil {
			return kctrl.Result{}, fmt.Errorf("getting External %s VNI: %w", ic.Name, err)
		}
		if comm, err = librarian.VNICommunity(r.cfg.BaseVPCCommunity, vni); err != nil {
			return kctrl.Result{}, fmt.Errorf("getting External %s community: %w", ic.Name, err)
		}
	} else if !kapierrors.IsNotFound(err) {
		return kctrl.Result{}, fmt.Errorf("getting External %s: %w", ic.Name, err)
	}

	// remote prefixes match as in an ExternalPeering, including the longer prefixes within them
	inboundPrefixes := map[string]vpcapi.ExternalInboundPrefix{}
	for _, prefix := range ic.Spec.Remote.Prefixes {
		inboundPrefixes[prefix] = vpcapi.ExternalInboundPrefix{MaxPrefixLen: 32}
	}

	ext = &vpcapi.External{ObjectMeta: kmetav1.ObjectMeta{Name: ic.Name, Namespace: ic.Namespace}}
	if err := r.createOrUpdate(ctx, ic, ext, func() {
		ext.Spec = vpcapi.ExternalSpec{
			Topology:          vpcapi.ExternalTopology{Fabric: ic.Spec.Topology.Fabric, Domain: ic.Spec.Topology.Domain},
			IPv4Namespace:     ic.Spec.IPv4Namespace,
			OutboundCommunity: comm,
			InboundPrefixes:   inboundPrefixes,
		}
		ext.Default()
	}); err != nil {
		return kctrl.Result{}, err
	}
	if comm == "" {
		return kctrl.Result{RequeueAfter: time.Second}, nil
	}

	attachNames := map[string]bool{}
	for _, link := range ic.Spec.Links {
		attach := &vpcapi.ExternalAttachment{ObjectMeta: kmetav1.ObjectMeta{
			Name:      vpcapi.VPCInterconnectAttachmentName(ic.Name, link),
			Namespace: ic.Namespace,
		}}
		attachNames[attach.Name] = true
		if err := r.createOrUpdate(ctx, ic, attach, func() {
			attach.Spec = vpcapi.ExternalAttachmentSpec{
				Topology:   vpcapi.ExternalAttachmentTopology{Fabric: ic.Spec.Topology.Fabric},
				External:   ic.Name,
				Connection: link.Connection,
				Switch:     vpcapi.ExternalAttachmentSwitch{VLAN: link.VLAN},
				Neighbor:   vpcapi.ExternalAttachmentNeighbor{ASN: link.RemoteASN},
				BFD:        link.BFD.DeepCopy(),
				InboundACL: link.InboundACL.DeepCopy(),
			}
			attach.Default()
		}); err != nil {
			return kctrl.Result{}, err
		}
	}

	peeringNames := map[string]bool{}
	for vpcName, local := range ic.Spec.Local {
		peering := &vpcapi.ExternalPeering{ObjectMeta: kmetav1.ObjectMeta{
			Name:      vpcapi.VPCInterconnectPeeringName(ic.Name, vpcName),
			Namespace: ic.Namespace,
		}}
		peeringNames[peering.Name] = true
		prefixes := []vpcapi.ExternalPeeringSpecPrefix{}
		for _, prefix := range ic.Spec.Remote.Prefixes {
			prefixes = append(prefixes, vpcapi.ExternalPeeringSpecPrefix{Prefix: prefix})
		}
		if err := r.createOrUpdate(ctx, ic, peering, func() {
			peering.Spec = vpcapi.ExternalPeeringSpec{
				Topology: vpcapi.ExternalPeeringTopology{Fabric: ic.Spec.Topology.Fabric},
				Permit: vpcapi.ExternalPeeringSpecPermit{
					VPC:      vpcapi.ExternalPeeringSpecVPC{Name: vpcName, Subnets: append([]string{}, local.Subnets...)},
					External: vpcapi.ExternalPeeringSpecExternal{Name: ic.Name, Prefixes: prefixes},
				},
			}
			peering.Default()
		}); err != nil {
			return kctrl.Result{}, err
		}
	}

	// the ones left from links and local VPCs removed since
	attaches := &vpcapi.ExternalAttachmentList{}
	if err := r.List(ctx, attaches, kclient.InNamespace(ic.Namespace), kclient.MatchingLabels{vpcapi.LabelExternal: ic.Name}); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing external attachments of %s: %w", ic.Name, err)
	}
	for _, attach := range attaches.Items {
		if vpcapi.VPCInterconnectOwner(&attach) == ic.Name && !attachNames[attach.Name] {
			if err := r.Delete(ctx, &attach); kclient.IgnoreNotFound(err) != nil {
				return kctrl.Result{}, fmt.Errorf("deleting external attachment %s: %w", attach.Name, err)
			}
			l.Info("Deleted external attachment of a removed link", "name", attach.Name)
		}
	}
	peerings := &vpcapi.ExternalPeeringList{}
	if err := r.List(ctx, peerings, kclient.InNamespace(ic.Namespace), kclient.MatchingLabels{vpcapi.LabelExternal: ic.Name}); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing external peerings of %s: %w", ic.Name, err)
	}
	for _, peering := range peerings.Items {
		if vpcapi.VPCInterconnectOwner(&peering) == ic.Name && !peeringNames[peering.Name] {
			if err := r.Delete(ctx, &peering); kclient.IgnoreNotFound(err) != nil {
				return kctrl.Result{}, fmt.Errorf("deleting external peering %s: %w", peering.Name, err)
			}
			l.Info("Deleted external peering of a removed VPC", "name", peering.Name)
		}
	}

	return kctrl.Result{}, nil
}

// createOrUpdate writes obj as set by mutate, which defaults it as the webhook will so that an unchanged object isn't
// written again, owned by the VPC interconnect
func (r *VPCInterconnectReconciler) createOrUpdate(ctx context.Context, ic *vpcapi.VPCInterconnect, obj kclient.Object, mutate func()) error {
	op, err := ctrlutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		// one the VPC interconnect doesn't own already would be a user object of the same name
		if owner := vpcapi.VPCInterconnectOwner(obj); obj.GetResourceVersion() != "" && owner != ic.Name {
			return fmt.Errorf("%T %s exists and isn't generated for VPC interconnect %s", obj, obj.GetName(), ic.Name) //nolint:err113
		}
		mutate()

		return setOwner(ic, obj, r.Scheme())
	})
	if err != nil {
		return fmt.Errorf("creating/updating %T %s: %w", obj, obj.GetName(), err)
	}
	if op != ctrlutil.OperationResultNone {
		kctrllog.FromContext(ctx).Info("Generated object for VPC interconnect synced", "kind", fmt.Sprintf("%T", obj), "name", obj.GetName(), "op", op)
	}

	return nil
}
