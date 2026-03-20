// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
)

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-gatewaypeering,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gatewaypeerings,verbs=create;update;delete,versions=v1alpha1,name=mgatewaypeering.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-gatewaypeering,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gatewaypeerings,verbs=create;update;delete,versions=v1alpha1,name=vgatewaypeering.kb.io,admissionReviewVersions=v1

type GatewayPeeringWebhook struct {
	kclient.Reader
}

func SetupGatewayPeeringWebhookWith(mgr kctrl.Manager) error {
	w := &GatewayPeeringWebhook{
		Reader: mgr.GetClient(),
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &gwapi.GatewayPeering{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *GatewayPeeringWebhook) Default(_ context.Context, peer *gwapi.GatewayPeering) error {
	peer.Default()

	return nil
}

func (w *GatewayPeeringWebhook) ValidateCreate(ctx context.Context, peer *gwapi.GatewayPeering) (admission.Warnings, error) {
	return nil, peer.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *GatewayPeeringWebhook) ValidateUpdate(ctx context.Context, _ *gwapi.GatewayPeering, newPeer *gwapi.GatewayPeering) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed

	return nil, newPeer.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *GatewayPeeringWebhook) ValidateDelete(_ context.Context, _ *gwapi.GatewayPeering) (admission.Warnings, error) {
	return nil, nil
}
