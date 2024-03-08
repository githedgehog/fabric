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

package vpcpeering

import (
	"context"

	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VPCPeeringWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	KubeClient client.Reader
	Cfg        *meta.FabricConfig
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *meta.FabricConfig) error {
	w := &VPCPeeringWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.VPCPeering{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*VPCPeeringWebhook)(nil)
	_ admission.CustomValidator = (*VPCPeeringWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-vpcpeering,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcpeerings,verbs=create;update,versions=v1alpha2,name=mvpcpeering.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpcpeering,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcpeerings,verbs=create;update;delete,versions=v1alpha2,name=vvpcpeering.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpcpeering-webhook")

func (w *VPCPeeringWebhook) Default(ctx context.Context, obj runtime.Object) error {
	peering := obj.(*vpcapi.VPCPeering)

	peering.Default()

	return nil
}

func (w *VPCPeeringWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	peering := obj.(*vpcapi.VPCPeering)

	warns, err := peering.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCPeeringWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newPeering := newObj.(*vpcapi.VPCPeering)
	// oldPeering := oldObj.(*vpcapi.VPCPeering)

	// if !equality.Semantic.DeepEqual(oldPeering.Spec.Permit, newPeering.Spec.Permit) {
	// 	return nil, errors.Errorf("vpc peering permit list is immutable")
	// }

	warns, err := newPeering.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCPeeringWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
