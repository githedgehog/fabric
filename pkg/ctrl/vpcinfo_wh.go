// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-vpcinfo,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=vpcinfos,verbs=create;update;delete,versions=v1alpha1,name=mvpcinfo.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-vpcinfo,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=vpcinfos,verbs=create;update;delete,versions=v1alpha1,name=vvpcinfo.kb.io,admissionReviewVersions=v1

type VPCInfoWebhook struct {
	kclient.Reader
}

func SetupVPCInfoWebhookWith(mgr kctrl.Manager) error {
	w := &VPCInfoWebhook{
		Reader: mgr.GetClient(),
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &gwapi.VPCInfo{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *VPCInfoWebhook) Default(_ context.Context, vpc *gwapi.VPCInfo) error {
	vpc.Default()

	return nil
}

func (w *VPCInfoWebhook) ValidateCreate(ctx context.Context, vpc *gwapi.VPCInfo) (admission.Warnings, error) {
	return nil, vpc.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *VPCInfoWebhook) ValidateUpdate(ctx context.Context, _ *gwapi.VPCInfo, newVPC *gwapi.VPCInfo) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed

	return nil, newVPC.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *VPCInfoWebhook) ValidateDelete(_ context.Context, _ *gwapi.VPCInfo) (admission.Warnings, error) {
	return nil, nil
}
