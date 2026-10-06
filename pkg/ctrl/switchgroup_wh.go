// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"slices"

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

func SetupSwitchGroupWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &SwitchGroupWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &wiringapi.SwitchGroup{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*wiringapi.SwitchGroup](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup switchgroup webhook: %w", err)
	}

	return nil
}

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-switchgroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchgroups,verbs=create;update,versions=v1beta1,name=mswitchgroup.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-switchgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchgroups,verbs=create;update;delete,versions=v1beta1,name=vswitchgroup.kb.io,admissionReviewVersions=v1

func (w *SwitchGroupWebhook) Default(_ context.Context, sg *wiringapi.SwitchGroup) error {
	sg.Default()

	return nil
}

func (w *SwitchGroupWebhook) ValidateCreate(ctx context.Context, sg *wiringapi.SwitchGroup) (admission.Warnings, error) {
	warns, err := sg.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate switchgroup: %w", err)
	}

	return warns, nil
}

func (w *SwitchGroupWebhook) ValidateUpdate(ctx context.Context, oldSg *wiringapi.SwitchGroup, newSg *wiringapi.SwitchGroup) (admission.Warnings, error) {
	if fabricChanged(oldSg.Spec.Topology.Fabric, newSg.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}

	warns, err := newSg.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate switchgroup: %w", err)
	}

	return warns, nil
}

// ValidateDelete refuses deleting a group that switches still reference: the catalog of a redundancy group is garbage
// collected with its SwitchGroup, and its members would then get their IRB VLANs and port channel IDs reallocated.
// The specs are checked rather than the group labels, which switches written before them don't have.
func (w *SwitchGroupWebhook) ValidateDelete(ctx context.Context, sg *wiringapi.SwitchGroup) (admission.Warnings, error) {
	sws := &wiringapi.SwitchList{}
	if err := w.KubeClient.List(ctx, sws, kclient.InNamespace(sg.Namespace)); err != nil {
		return nil, fmt.Errorf("listing switches: %w", err)
	}

	for _, sw := range sws.Items {
		if sw.Spec.Redundancy.Group == sg.Name || slices.Contains(sw.Spec.Groups, sg.Name) {
			return nil, fmt.Errorf("switch group is used by switch %s", sw.Name) //nolint:err113
		}
	}

	return nil, nil
}
