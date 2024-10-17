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

package externalattachment

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
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
		For(&vpcapi.ExternalAttachment{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup external attachment webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-externalattachment,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externalattachments,verbs=create;update,versions=v1beta1,name=mexternalattachment.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-externalattachment,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externalattachments,verbs=create;update;delete,versions=v1beta1,name=vexternalattachment.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("externalattachment-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	attach := obj.(*vpcapi.ExternalAttachment)

	attach.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	attach := obj.(*vpcapi.ExternalAttachment)

	warns, err := attach.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate external attachment")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newAttach := newObj.(*vpcapi.ExternalAttachment)
	// oldAttach := oldObj.(*vpcapi.ExternalAttachment)

	// if !equality.Semantic.DeepEqual(oldAttach.Spec, newAttach.Spec) {
	// 	return nil, errors.Errorf("external attachment spec is immutable")
	// }

	warns, err := newAttach.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate external attachment")
	}

	return warns, nil
}

func (w *Webhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
