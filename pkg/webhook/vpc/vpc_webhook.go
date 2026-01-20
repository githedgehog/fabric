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
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
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

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr, &vpcapi.VPC{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup vpc webhook")
}

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-vpc,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update,versions=v1beta1,name=mvpc.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-vpc,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update;delete,versions=v1beta1,name=vvpc.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpc-webhook")

func (w *Webhook) Default(_ context.Context, vpc *vpcapi.VPC) error {
	vpc.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, vpc *vpcapi.VPC) (admission.Warnings, error) {
	warns, err := vpc.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate vpc")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ *vpcapi.VPC, newVPC *vpcapi.VPC) (admission.Warnings, error) {
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

func (w *Webhook) ValidateDelete(ctx context.Context, vpc *vpcapi.VPC) (admission.Warnings, error) {
	vpcAttachments := &vpcapi.VPCAttachmentList{}
	if err := w.Client.List(ctx, vpcAttachments, kclient.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc attachments") // TODO hide internal error
	}
	if len(vpcAttachments.Items) > 0 {
		return nil, errors.Errorf("VPC has attachments")
	}

	vpcPeerings := &vpcapi.VPCPeeringList{}
	if err := w.Client.List(ctx, vpcPeerings, kclient.MatchingLabels{
		vpcapi.ListLabelVPC(vpc.Name): vpcapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc peerings") // TODO hide internal error
	}
	if len(vpcPeerings.Items) > 0 {
		return nil, errors.Errorf("VPC has peerings")
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := w.Client.List(ctx, extPeerings, kclient.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external peerings") // TODO hide internal error
	}
	if len(extPeerings.Items) > 0 {
		return nil, errors.Errorf("VPC has external peerings")
	}

	staticExts := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, staticExts, kclient.MatchingLabels{
		wiringapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(staticExts.Items) > 0 {
		return nil, errors.Errorf("VPC has static external connections (using withingVPC option)")
	}

	return nil, nil
}
