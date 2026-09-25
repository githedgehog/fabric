// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"

	"github.com/pkg/errors"
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

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr, &wiringapi.Fabric{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup fabric webhook")
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

	return nil
}

func (w *FabricWebhook) ValidateCreate(ctx context.Context, fabric *wiringapi.Fabric) (admission.Warnings, error) {
	warns, err := fabric.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate fabric")
	}

	return warns, nil
}

func (w *FabricWebhook) ValidateUpdate(ctx context.Context, oldFabric *wiringapi.Fabric, fabric *wiringapi.Fabric) (admission.Warnings, error) {
	// switches are validated against these ASNs and configured with them, so changing them would
	// silently invalidate what is already admitted. Unset values can still be filled in
	if oldFabric.Spec.ASNStart != 0 && oldFabric.Spec.ASNStart != fabric.Spec.ASNStart ||
		oldFabric.Spec.ASNEnd != 0 && oldFabric.Spec.ASNEnd != fabric.Spec.ASNEnd {
		return nil, errors.Errorf("fabric ASN range can not be changed")
	}
	for name, oldDomain := range oldFabric.Spec.Domains {
		if oldDomain.SpineASN != 0 && oldDomain.SpineASN != fabric.Spec.Domains[name].SpineASN {
			return nil, errors.Errorf("spineASN of domain %s can not be changed", name)
		}
	}

	warns, err := fabric.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate fabric")
	}

	return warns, nil
}

func (w *FabricWebhook) ValidateDelete(ctx context.Context, fabric *wiringapi.Fabric) (admission.Warnings, error) {
	// The default fabric is reserved. Every object written before the reference existed belongs
	// to it implicitly, admission exempts it from the fabric-exists check so nothing would refuse
	// to admit more, and the initializer only recreates it at startup.
	if fabric.Name == wiringapi.DefaultFabric {
		return nil, errors.Errorf("the default Fabric can not be deleted")
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
			return nil, errors.Wrapf(err, "error listing %ss", ref.kind) // TODO hide internal error
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
				return errors.Errorf("unexpected type %T", item)
			}

			if wiringapi.FabricNameOrDefault(declared) != fabric.Name {
				return nil
			}

			obj, ok := item.(kclient.Object)
			if !ok {
				return errors.Errorf("unexpected type %T", item)
			}

			return errors.Errorf("Fabric is still used by %s %s", ref.kind, obj.GetName())
		}); err != nil {
			return nil, err //nolint:wrapcheck // the error is ours, EachListItem only passes it through
		}
	}

	return nil, nil
}
