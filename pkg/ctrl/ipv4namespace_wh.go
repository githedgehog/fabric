// Copyright 2023 Hedgehog
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
	"maps"
	"net/netip"
	"slices"

	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/iputil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type IPv4NamespaceWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupIPv4NamespaceWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &IPv4NamespaceWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &vpcapi.IPv4Namespace{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*vpcapi.IPv4Namespace](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup ipv4namespace webhook: %w", err)
	}

	return nil
}

// +kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-ipv4namespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=create;update,versions=v1beta1,name=mipv4namespace.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-ipv4namespace,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=create;update;delete,versions=v1beta1,name=vipv4namespace.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("ipv4namespace-webhook")

func (w *IPv4NamespaceWebhook) Default(_ context.Context, ns *vpcapi.IPv4Namespace) error {
	ns.Default()

	return nil
}

func (w *IPv4NamespaceWebhook) ValidateCreate(ctx context.Context, ns *vpcapi.IPv4Namespace) (admission.Warnings, error) {
	warns, err := ns.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate ipv4namespace: %w", err)
	}

	return warns, nil
}

func (w *IPv4NamespaceWebhook) ValidateUpdate(ctx context.Context, oldNs *vpcapi.IPv4Namespace, newNs *vpcapi.IPv4Namespace) (admission.Warnings, error) {
	if fabricChanged(oldNs.Spec.Topology.Fabric, newNs.Spec.Topology.Fabric) {
		return nil, field.Invalid(field.NewPath("spec", "topology", "fabric"), newNs.Spec.Topology.Fabric, "field is immutable")
	}

	if warn, err := newNs.Validate(ctx, w.Client, w.Cfg); err != nil {
		return warn, fmt.Errorf("failed to validate ipv4namespace: %w", err)
	}

	nsSubnets := []netip.Prefix{}
	for _, subnet := range newNs.Spec.Subnets {
		ipNet, err := netip.ParsePrefix(subnet)
		if err != nil {
			return nil, fmt.Errorf("failed to parse cidr %s: %w", subnet, err)
		}

		nsSubnets = append(nsSubnets, ipNet)
	}

	exts := &vpcapi.ExternalList{}
	if err := w.Client.List(ctx, exts, kclient.InNamespace(newNs.Namespace), kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: newNs.Name,
	}); err != nil {
		return nil, fmt.Errorf("listing externals: %w", err) // TODO hide internal error
	}
	for _, ext := range exts.Items {
		if err := newNs.CheckOutside("inbound prefix", slices.Sorted(maps.Keys(ext.Spec.InboundPrefixes))); err != nil {
			return nil, field.Invalid(field.NewPath("spec", "subnets"), field.OmitValueType{}, fmt.Sprintf("external %s: %s", ext.Name, err))
		}
	}

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: newNs.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing vpcs: %w", err) // TODO hide internal error
	}

	for _, vpc := range vpcs.Items {
		for subnetName, subnetCfg := range vpc.Spec.Subnets {
			vpcSubnet, err := netip.ParsePrefix(subnetCfg.Subnet)
			if err != nil {
				return nil, fmt.Errorf("failed to parse vpc subnet %s: %w", subnetCfg.Subnet, err)
			}

			ok := false
			for _, nsSubnet := range nsSubnets {
				if iputil.IsSubset(vpcSubnet, nsSubnet) {
					ok = true

					break
				}
			}

			if !ok {
				return nil, field.Invalid(field.NewPath("spec", "subnets"), field.OmitValueType{}, fmt.Sprintf("existing vpc %s subnet %s (cidr %s) doesn't fit updated IPv4 namespace", vpc.Name, subnetName, subnetCfg.Subnet))
			}
		}
	}

	return nil, nil
}

func (w *IPv4NamespaceWebhook) ValidateDelete(ctx context.Context, ns *vpcapi.IPv4Namespace) (admission.Warnings, error) {
	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: ns.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing vpcs: %w", err) // TODO hide internal error
	}
	if len(vpcs.Items) > 0 {
		return nil, fmt.Errorf("IPv4Namespace has VPCs") //nolint:err113
	}

	externals := &vpcapi.ExternalList{}
	if err := w.Client.List(ctx, externals, kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: ns.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing externals: %w", err) // TODO hide internal error
	}
	if len(externals.Items) > 0 {
		return nil, fmt.Errorf("IPv4Namespace has externals") //nolint:err113
	}

	return nil, nil
}
