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

package vpcattachment

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Webhook struct {
	client.Client
	Scheme     *runtime.Scheme
	KubeClient client.Reader
	Cfg        *meta.FabricConfig
}

func SetupWithManager(mgr ctrl.Manager, cfg *meta.FabricConfig) error {
	w := &Webhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return errors.Wrapf(ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.VPCAttachment{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup vpc attachment webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-vpcattachment,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcattachments,verbs=create;update,versions=v1alpha2,name=mvpcattachment.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpcattachment,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcattachments,verbs=create;update;delete,versions=v1alpha2,name=vvpcattachment.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpcattachment-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	attach := obj.(*vpcapi.VPCAttachment)

	attach.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	attach := obj.(*vpcapi.VPCAttachment)

	warns, err := attach.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate vpc attachment")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newAttach := newObj.(*vpcapi.VPCAttachment)
	// oldAttach := oldObj.(*vpcapi.VPCAttachment)

	// if !equality.Semantic.DeepEqual(oldAttach.Spec, newAttach.Spec) {
	// 	return nil, errors.Errorf("vpc attachment is immutable")
	// }

	warns, err := newAttach.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate vpc attachment")
	}

	return warns, nil
}

func (w *Webhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
