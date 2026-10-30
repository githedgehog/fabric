// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"slices"

	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-gatewaygroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=create;update;delete,versions=v1alpha1,name=mgatewaygroup.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-gatewaygroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=create;update;delete,versions=v1alpha1,name=vgatewaygroup.kb.io,admissionReviewVersions=v1

type GatewayGroupWebhook struct {
	kclient.Reader
	cfg *meta.FabricConfig
}

func SetupGatewayGroupWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &GatewayGroupWebhook{
		Reader: mgr.GetClient(),
		cfg:    cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &gwapi.GatewayGroup{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*gwapi.GatewayGroup](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err)
	}

	return nil
}

func (w *GatewayGroupWebhook) Default(_ context.Context, gwGr *gwapi.GatewayGroup) error {
	gwGr.Default()

	return nil
}

func (w *GatewayGroupWebhook) ValidateCreate(ctx context.Context, gwGr *gwapi.GatewayGroup) (admission.Warnings, error) {
	return nil, gwGr.Validate(ctx, w.Reader, w.cfg) //nolint:wrapcheck
}

func (w *GatewayGroupWebhook) ValidateUpdate(ctx context.Context, oldGwGr *gwapi.GatewayGroup, newGwGr *gwapi.GatewayGroup) (admission.Warnings, error) {
	if fabricChanged(oldGwGr.Spec.Topology.Fabric, newGwGr.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}
	// so that a GatewayGroup update never has to re-check its gateways and peerings
	if domainChanged(oldGwGr.Spec.Topology.Domain, newGwGr.Spec.Topology.Domain) {
		return nil, fmt.Errorf("topology.domain is immutable") //nolint:err113
	}

	// TODO validate diff between oldObj and newObj if needed

	return nil, newGwGr.Validate(ctx, w.Reader, w.cfg) //nolint:wrapcheck
}

// ValidateDelete refuses deleting a group that's still in use: its gateways would fail validation on any update and
// its peerings would be processed by no gateway.
func (w *GatewayGroupWebhook) ValidateDelete(ctx context.Context, gwGr *gwapi.GatewayGroup) (admission.Warnings, error) {
	// only the gateways and peerings of its own fabric can use it
	inFabric := kclient.MatchingLabels{
		wiringapi.ListLabelFabric(gwGr.Spec.Topology.Fabric): gwapi.ListLabelValue,
	}

	gws := &gwapi.GatewayList{}
	if err := w.List(ctx, gws, inFabric); err != nil {
		return nil, fmt.Errorf("listing gateways: %w", err)
	}
	for _, gw := range gws.Items {
		// so that a gateway and its group can be deleted together, it's only waiting for its cleanup
		if gw.DeletionTimestamp != nil {
			continue
		}
		if slices.ContainsFunc(gw.Spec.Groups, func(m gwapi.GatewayGroupMembership) bool { return m.Name == gwGr.Name }) {
			return nil, fmt.Errorf("gateway group is used by gateway %s", gw.Name) //nolint:err113
		}
	}

	peerings := &gwapi.GatewayPeeringList{}
	if err := w.List(ctx, peerings, inFabric); err != nil {
		return nil, fmt.Errorf("listing peerings: %w", err)
	}
	for _, peering := range peerings.Items {
		if peering.Spec.GatewayGroup == gwGr.Name {
			return nil, fmt.Errorf("gateway group is used by peering %s", peering.Name) //nolint:err113
		}
	}

	return nil, nil
}
