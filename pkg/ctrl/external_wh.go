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

	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ExternalWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupExternalWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &ExternalWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &vpcapi.External{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*vpcapi.External](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup external webhook: %w", err)
	}

	return nil
}

// +kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-external,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externals,verbs=create;update,versions=v1beta1,name=mexternal.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-external,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externals,verbs=create;update;delete,versions=v1beta1,name=vexternal.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("external-webhook")

func (w *ExternalWebhook) Default(_ context.Context, ext *vpcapi.External) error {
	ext.Default()

	return nil
}

func (w *ExternalWebhook) ValidateCreate(ctx context.Context, ext *vpcapi.External) (admission.Warnings, error) {
	warns, err := ext.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating external: %w", err)
	}

	return warns, nil
}

func (w *ExternalWebhook) ValidateUpdate(ctx context.Context, oldExt *vpcapi.External, newExt *vpcapi.External) (admission.Warnings, error) {
	if err := checkFabricUnchanged(oldExt.Spec.Topology.Fabric, newExt.Spec.Topology.Fabric); err != nil {
		return nil, err
	}
	// so that an External update never has to re-check its attachments
	if err := checkDomainUnchanged(oldExt.Spec.Topology.Domain, newExt.Spec.Topology.Domain); err != nil {
		return nil, err
	}

	// if !equality.Semantic.DeepEqual(oldExt.Spec, newExt.Spec) {
	// 	return nil, errors.Errorf("external spec is immutable")
	// }

	warns, err := newExt.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating external: %w", err)
	}

	// static attachments install one route per spec.static.prefixes, so dropping the prefixes
	// while they exist would leave them with nothing to point at
	if oldExt.Spec.Static != nil && newExt.Spec.Static == nil {
		extAttachments := &vpcapi.ExternalAttachmentList{}
		if err := w.Client.List(ctx, extAttachments, kclient.MatchingLabels{
			vpcapi.LabelExternal: newExt.Name,
		}); err != nil {
			return warns, fmt.Errorf("error listing external attachments: %w", err) // TODO hide internal error
		}
		for _, attach := range extAttachments.Items {
			if attach.Spec.Static != nil {
				return warns, field.Forbidden(field.NewPath("spec", "static"), fmt.Sprintf("external attachment %s is still static, convert it to BGP before removing static from the external", attach.Name))
			}
		}
	}

	return warns, nil
}

func (w *ExternalWebhook) ValidateDelete(ctx context.Context, ext *vpcapi.External) (admission.Warnings, error) {
	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := w.Client.List(ctx, extAttachments, kclient.MatchingLabels{
		vpcapi.LabelExternal: ext.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing external attachments: %w", err) // TODO hide internal error
	}
	if len(extAttachments.Items) > 0 {
		return nil, fmt.Errorf("external has attachments") //nolint:err113
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := w.Client.List(ctx, extPeerings, kclient.MatchingLabels{
		vpcapi.LabelExternal: ext.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing external peerings: %w", err) // TODO hide internal error
	}
	if len(extPeerings.Items) > 0 {
		return nil, fmt.Errorf("external has peerings") //nolint:err113
	}

	return nil, nil
}
