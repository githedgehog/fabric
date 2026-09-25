// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type FabricWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupFabricWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	w := &FabricWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &wiringapi.Fabric{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup fabric webhook: %w", err)
	}

	return nil
}

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-fabric,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=fabrics,verbs=create;update,versions=v1beta1,name=mfabric.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-fabric,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=fabrics,verbs=create;update;delete,versions=v1beta1,name=vfabric.kb.io,admissionReviewVersions=v1

// fabricChanged reports whether an update moves an object to another fabric. It resolves both
// sides because the stored object may predate the reference and still hold an empty value, while
// the incoming one has just been defaulted to "default".
func fabricChanged(oldName, newName string) bool {
	return wiringapi.FabricNameOrDefault(oldName) != wiringapi.FabricNameOrDefault(newName)
}

func (w *FabricWebhook) Default(_ context.Context, fabric *wiringapi.Fabric) error {
	fabric.Default()

	// ASNs have no sensible default, but these are the same in most deployments
	if w.Cfg != nil {
		if fabric.Spec.FabricMTU == 0 {
			fabric.Spec.FabricMTU = w.Cfg.FabricMTU
		}
		if fabric.Spec.ServerFacingMTUOffset == 0 {
			fabric.Spec.ServerFacingMTUOffset = w.Cfg.ServerFacingMTUOffset
		}
		if fabric.Spec.DefaultMaxPathsEBGP == 0 {
			fabric.Spec.DefaultMaxPathsEBGP = w.Cfg.DefaultMaxPathsEBGP
		}
	}

	return nil
}

func (w *FabricWebhook) ValidateCreate(ctx context.Context, fabric *wiringapi.Fabric) (admission.Warnings, error) {
	warns, err := fabric.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate fabric: %w", err)
	}

	return warns, nil
}

func (w *FabricWebhook) ValidateUpdate(ctx context.Context, oldFabric *wiringapi.Fabric, fabric *wiringapi.Fabric) (admission.Warnings, error) {
	// switches are validated against these ASNs and configured with them, so changing them would
	// silently invalidate what is already admitted. Unset values can still be filled in
	if oldFabric.Spec.LeafASNStart != 0 && oldFabric.Spec.LeafASNStart != fabric.Spec.LeafASNStart ||
		oldFabric.Spec.LeafASNEnd != 0 && oldFabric.Spec.LeafASNEnd != fabric.Spec.LeafASNEnd {
		return nil, fmt.Errorf("fabric leaf ASN range can not be changed") //nolint:err113
	}
	for name, oldDomain := range oldFabric.Spec.Domains {
		domain, exists := fabric.Spec.Domains[name]
		if !exists && oldDomain != (wiringapi.FabricDomainSpec{}) {
			return nil, fmt.Errorf("domain %s can not be removed or renamed", name) //nolint:err113
		}
		if oldDomain.SpineASN != 0 && oldDomain.SpineASN != domain.SpineASN {
			return nil, fmt.Errorf("spineASN of domain %s can not be changed", name) //nolint:err113
		}
		if oldDomain.GatewayASN != 0 && oldDomain.GatewayASN != domain.GatewayASN {
			return nil, fmt.Errorf("gatewayASN of domain %s can not be changed", name) //nolint:err113
		}
	}

	warns, err := fabric.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate fabric: %w", err)
	}

	return warns, nil
}

func (w *FabricWebhook) ValidateDelete(ctx context.Context, fabric *wiringapi.Fabric) (admission.Warnings, error) {
	// The default fabric is reserved. Every object written before the reference existed belongs
	// to it implicitly, admission exempts it from the fabric-exists check so nothing would refuse
	// to admit more, and the initializer only recreates it at startup.
	if fabric.Name == wiringapi.DefaultFabric {
		return nil, fmt.Errorf("the default Fabric can not be deleted") //nolint:err113
	}

	// Deleting a fabric out from under its objects does not just orphan them: every one of these
	// types checks that its fabric exists, so they would keep working but could never be updated
	// again. Filtering on the spec and not on the fabric label, which may never have been
	// backfilled.
	for _, ref := range []struct {
		kind string
		list kclient.ObjectList
	}{
		{"switch", &wiringapi.SwitchList{}},
		{"switch group", &wiringapi.SwitchGroupList{}},
		{"connection", &wiringapi.ConnectionList{}},
		{"IPv4 namespace", &vpcapi.IPv4NamespaceList{}},
		{"VPC", &vpcapi.VPCList{}},
		{"VPC attachment", &vpcapi.VPCAttachmentList{}},
		{"VPC peering", &vpcapi.VPCPeeringList{}},
		{"external", &vpcapi.ExternalList{}},
		{"external attachment", &vpcapi.ExternalAttachmentList{}},
		{"external peering", &vpcapi.ExternalPeeringList{}},
		{"gateway", &gwapi.GatewayList{}},
		{"gateway group", &gwapi.GatewayGroupList{}},
		{"gateway peering", &gwapi.GatewayPeeringList{}},
	} {
		if err := w.Client.List(ctx, ref.list); err != nil {
			return nil, fmt.Errorf("error listing %ss: %w", ref.kind, err) // TODO hide internal error
		}

		if err := kmeta.EachListItem(ref.list, func(item runtime.Object) error {
			var declared string
			switch o := item.(type) {
			case *wiringapi.Switch:
				declared = o.Spec.Topology.Fabric
			case *wiringapi.SwitchGroup:
				declared = o.Spec.Topology.Fabric
			case *wiringapi.Connection:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.IPv4Namespace:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.VPC:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.VPCAttachment:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.VPCPeering:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.External:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.ExternalAttachment:
				declared = o.Spec.Topology.Fabric
			case *vpcapi.ExternalPeering:
				declared = o.Spec.Topology.Fabric
			case *gwapi.Gateway:
				declared = o.Spec.Topology.Fabric
			case *gwapi.GatewayGroup:
				declared = o.Spec.Topology.Fabric
			case *gwapi.GatewayPeering:
				declared = o.Spec.Topology.Fabric
			default:
				return fmt.Errorf("unexpected type %T", item) //nolint:err113
			}

			if wiringapi.FabricNameOrDefault(declared) != fabric.Name {
				return nil
			}

			obj, ok := item.(kclient.Object)
			if !ok {
				return fmt.Errorf("unexpected type %T", item) //nolint:err113
			}

			return fmt.Errorf("fabric is still used by %s %s", ref.kind, obj.GetName()) //nolint:err113
		}); err != nil {
			return nil, err //nolint:wrapcheck // the error is ours, EachListItem only passes it through
		}
	}

	return nil, nil
}
