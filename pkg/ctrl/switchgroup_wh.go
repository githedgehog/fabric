// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type SwitchGroupWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupSwitchGroupWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	w := &SwitchGroupWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr, &wiringapi.SwitchGroup{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup switchgroup webhook")
}

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-switchgroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchgroups,verbs=create;update,versions=v1beta1,name=mswitchgroup.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-switchgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchgroups,verbs=create;update,versions=v1beta1,name=vswitchgroup.kb.io,admissionReviewVersions=v1

func (w *SwitchGroupWebhook) Default(_ context.Context, sg *wiringapi.SwitchGroup) error {
	sg.Default()

	return nil
}

func (w *SwitchGroupWebhook) ValidateCreate(ctx context.Context, sg *wiringapi.SwitchGroup) (admission.Warnings, error) {
	warns, err := sg.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate switchgroup")
	}

	return warns, nil
}

func (w *SwitchGroupWebhook) ValidateUpdate(ctx context.Context, oldSg *wiringapi.SwitchGroup, newSg *wiringapi.SwitchGroup) (admission.Warnings, error) {
	if fabricChanged(oldSg.Spec.Topology.Fabric, newSg.Spec.Topology.Fabric) {
		return nil, errors.Errorf("topology.fabric is immutable")
	}

	warns, err := newSg.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate switchgroup")
	}

	return warns, nil
}

// ValidateDelete is deliberately empty: deleting a group that switches still reference has always
// been allowed, and Switch.Validate already refuses to admit a switch naming a group that is gone.
func (w *SwitchGroupWebhook) ValidateDelete(_ context.Context, _ *wiringapi.SwitchGroup) (admission.Warnings, error) {
	return nil, nil
}
