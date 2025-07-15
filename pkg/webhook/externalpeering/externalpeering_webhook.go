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

package externalpeering

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
		For(&vpcapi.ExternalPeering{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup external peering webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

// +kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-externalpeering,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externalpeerings,verbs=create;update,versions=v1beta1,name=mexternalpeering.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-externalpeering,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externalpeerings,verbs=create;update;delete,versions=v1beta1,name=vexternalpeering.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("externalpeering-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	peering := obj.(*vpcapi.ExternalPeering)

	peering.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	peering := obj.(*vpcapi.ExternalPeering)

	warns, err := peering.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate external peering")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newPeering := newObj.(*vpcapi.ExternalPeering)
	// oldPeering := oldObj.(*vpcapi.ExternalPeering)

	// if !equality.Semantic.DeepEqual(oldPeering.Spec, newPeering.Spec) {
	// 	return nil, errors.Errorf("external peering spec is immutable")
	// }

	warns, err := newPeering.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate external peering")
	}

	return warns, nil
}

func (w *Webhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
