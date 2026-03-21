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

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-gatewaygroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=create;update;delete,versions=v1alpha1,name=mgatewaygroup.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-gatewaygroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=create;update;delete,versions=v1alpha1,name=vgatewaygroup.kb.io,admissionReviewVersions=v1

type GatewayGroupWebhook struct {
	kclient.Reader
}

func SetupGatewayGroupWebhookWith(mgr kctrl.Manager) error {
	w := &GatewayGroupWebhook{
		Reader: mgr.GetClient(),
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &gwapi.GatewayGroup{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *GatewayGroupWebhook) Default(_ context.Context, gwGr *gwapi.GatewayGroup) error {
	gwGr.Default()

	return nil
}

func (w *GatewayGroupWebhook) ValidateCreate(ctx context.Context, gwGr *gwapi.GatewayGroup) (admission.Warnings, error) {
	return nil, gwGr.Validate(ctx, w.Reader, nil) //nolint:wrapcheck
}

func (w *GatewayGroupWebhook) ValidateUpdate(ctx context.Context, _ *gwapi.GatewayGroup, newGwGr *gwapi.GatewayGroup) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed

	return nil, newGwGr.Validate(ctx, w.Reader, nil) //nolint:wrapcheck
}

func (w *GatewayGroupWebhook) ValidateDelete(_ context.Context, _ *gwapi.GatewayGroup) (admission.Warnings, error) {
	return nil, nil
}
