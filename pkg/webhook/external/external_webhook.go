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

package external

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Webhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupWithManager(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	w := &Webhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.External{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup external webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-external,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externals,verbs=create;update,versions=v1beta1,name=mexternal.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-external,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externals,verbs=create;update;delete,versions=v1beta1,name=vexternal.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("external-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	external := obj.(*vpcapi.External)

	external.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	external := obj.(*vpcapi.External)

	warns, err := external.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating external")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newExternal := newObj.(*vpcapi.External)
	// oldExternal := oldObj.(*vpcapi.External)

	// if !equality.Semantic.DeepEqual(oldExternal.Spec, newExternal.Spec) {
	// 	return nil, errors.Errorf("external spec is immutable")
	// }

	warns, err := newExternal.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating external")
	}

	return warns, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ext := obj.(*vpcapi.External)

	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := w.Client.List(ctx, extAttachments, kclient.MatchingLabels{
		vpcapi.LabelExternal: ext.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external attachments") // TODO hide internal error
	}
	if len(extAttachments.Items) > 0 {
		return nil, errors.Errorf("external has attachments")
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := w.Client.List(ctx, extPeerings, kclient.MatchingLabels{
		vpcapi.LabelExternal: ext.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external peerings") // TODO hide internal error
	}
	if len(extPeerings.Items) > 0 {
		return nil, errors.Errorf("external has peerings")
	}

	return nil, nil
}
