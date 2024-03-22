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

package vpc

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
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
		For(&vpcapi.VPC{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup vpc webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-vpc,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update,versions=v1alpha2,name=mvpc.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpc,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update;delete,versions=v1alpha2,name=vvpc.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpc-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	vpc := obj.(*vpcapi.VPC)

	vpc.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vpc := obj.(*vpcapi.VPC)

	warns, err := vpc.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate vpc")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	// oldVPC := oldObj.(*vpcapi.VPC)
	newVPC := newObj.(*vpcapi.VPC)

	warns, err := newVPC.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate vpc")
	}

	// TODO check that you can only add subnets, or edit/remove unused ones

	// for subnetName, oldSubnet := range oldVPC.Spec.Subnets {
	// 	newSubnet, ok := newVPC.Spec.Subnets[subnetName]
	// 	if !ok {
	// 		continue
	// 	}

	// 	if !equality.Semantic.DeepEqual(oldSubnet, newSubnet) {
	// 		return nil, errors.Errorf("subnets are immutable, but %s changed", subnetName)
	// 	}
	// }

	return nil, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vpc := obj.(*vpcapi.VPC)

	vpcAttachments := &vpcapi.VPCAttachmentList{}
	if err := w.Client.List(ctx, vpcAttachments, client.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc attachments") // TODO hide internal error
	}
	if len(vpcAttachments.Items) > 0 {
		return nil, errors.Errorf("VPC has attachments")
	}

	vpcPeerings := &vpcapi.VPCPeeringList{}
	if err := w.Client.List(ctx, vpcPeerings, client.MatchingLabels{
		vpcapi.ListLabelVPC(vpc.Name): vpcapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc peerings") // TODO hide internal error
	}
	if len(vpcPeerings.Items) > 0 {
		return nil, errors.Errorf("VPC has peerings")
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := w.Client.List(ctx, extPeerings, client.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external peerings") // TODO hide internal error
	}
	if len(extPeerings.Items) > 0 {
		return nil, errors.Errorf("VPC has external peerings")
	}

	staticExts := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, staticExts, client.MatchingLabels{
		wiringapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(staticExts.Items) > 0 {
		return nil, errors.Errorf("VPC has static external connections (using withingVPC option)")
	}

	return nil, nil
}
