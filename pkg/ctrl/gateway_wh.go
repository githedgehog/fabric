// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
)

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-gateway,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gateways,verbs=create;update;delete,versions=v1alpha1,name=mgateway.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-gateway,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gateways,verbs=create;update;delete,versions=v1alpha1,name=vgateway.kb.io,admissionReviewVersions=v1

type GatewayWebhook struct {
	kclient.Reader
	cfg *meta.FabricConfig
	v   *GatewayValidator
}

func SetupGatewayWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, v *GatewayValidator, lock *Lock) error {
	if cfg.EnableGateway && v == nil {
		return fmt.Errorf("validator is nil") //nolint:err113
	}

	w := &GatewayWebhook{
		Reader: mgr.GetClient(),
		cfg:    cfg,
		v:      v,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &gwapi.Gateway{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*gwapi.Gateway](w, lock)).
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

	gwAg, err := BuildGatewayAgent(ctx, w.Reader, w.cfg, gw)
	if err != nil {
		return nil, fmt.Errorf("building gateway agent: %w", err)
	}

	if w.v != nil {
		return nil, w.v.Validate(ctx, gwAg)
	}

	return nil, nil
}

func (w *GatewayWebhook) ValidateUpdate(ctx context.Context, oldGw *gwapi.Gateway, newGw *gwapi.Gateway) (admission.Warnings, error) {
	// a gateway being deleted only gets its finalizer removed, which must not depend on it still being valid
	if newGw.DeletionTimestamp != nil {
		return nil, nil
	}

	// nothing to validate in a metadata only update, e.g. the controller adding its finalizer, which must not be
	// refused for a gateway that doesn't pass the current validation anymore
	if equality.Semantic.DeepEqual(oldGw.Spec, newGw.Spec) {
		return nil, nil
	}

	if fabricChanged(oldGw.Spec.Topology.Fabric, newGw.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}
	// so that a Gateway update never has to re-check its connections
	if domainChanged(oldGw.Spec.Topology.Domain, newGw.Spec.Topology.Domain) {
		return nil, fmt.Errorf("topology.domain is immutable") //nolint:err113
	}

	// TODO validate diff between oldObj and newObj if needed
	if err := newGw.Validate(ctx, w.Reader, w.cfg); err != nil {
		return nil, err //nolint:wrapcheck
	}

	gwAg, err := BuildGatewayAgent(ctx, w.Reader, w.cfg, newGw)
	if err != nil {
		return nil, fmt.Errorf("building gateway agent: %w", err)
	}

	if w.v != nil {
		return nil, w.v.Validate(ctx, gwAg)
	}

	return nil, nil
}

// recreating a cabled gateway in another domain would otherwise bypass the connection checks
func (w *GatewayWebhook) ValidateDelete(ctx context.Context, gw *gwapi.Gateway) (admission.Warnings, error) {
	conns := &wiringapi.ConnectionList{}
	if err := w.List(ctx, conns, kclient.InNamespace(gw.Namespace)); err != nil {
		return nil, fmt.Errorf("listing connections: %w", err)
	}
	for _, conn := range conns.Items {
		if conn.Spec.Gateway == nil {
			continue
		}
		for _, link := range conn.Spec.Gateway.Links {
			if link.Gateway.DeviceName() == gw.Name {
				return nil, fmt.Errorf("gateway is used by connection %s", conn.Name) //nolint:err113
			}
		}
	}

	return nil, nil
}
