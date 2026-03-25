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
	"go.githedgehog.com/fabric/api/meta"
)

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-gateway,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gateways,verbs=create;update;delete,versions=v1alpha1,name=mgateway.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-gateway,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gateways,verbs=create;update;delete,versions=v1alpha1,name=vgateway.kb.io,admissionReviewVersions=v1

type GatewayWebhook struct {
	kclient.Reader
	cfg *meta.FabricConfig
	v   *GatewayValidator
}

func SetupGatewayWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, v *GatewayValidator) error {
	w := &GatewayWebhook{
		Reader: mgr.GetClient(),
		cfg:    cfg,
		v:      v,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &gwapi.Gateway{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *GatewayWebhook) Default(_ context.Context, obj *gwapi.Gateway) error {
	obj.Default()

	return nil
}

func (w *GatewayWebhook) ValidateCreate(ctx context.Context, gw *gwapi.Gateway) (admission.Warnings, error) {
	if err := gw.Validate(ctx, w.Reader, w.cfg); err != nil {
		return nil, err //nolint:wrapcheck
	}

	return nil, w.v.Validate()
}

func (w *GatewayWebhook) ValidateUpdate(ctx context.Context, _ *gwapi.Gateway, newGw *gwapi.Gateway) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed
	if err := newGw.Validate(ctx, w.Reader, w.cfg); err != nil {
		return nil, err //nolint:wrapcheck
	}

	return nil, w.v.Validate()
}

func (w *GatewayWebhook) ValidateDelete(_ context.Context, _ *gwapi.Gateway) (admission.Warnings, error) {
	return nil, nil
}
