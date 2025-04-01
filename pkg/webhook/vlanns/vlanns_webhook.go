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

package vlanns

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
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
		For(&wiringapi.VLANNamespace{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup vlannamespace webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-vlannamespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=create;update,versions=v1beta1,name=mvlannamespace.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-vlannamespace,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=create;update;delete,versions=v1beta1,name=vvlannamespace.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vlannamespace-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	ns := obj.(*wiringapi.VLANNamespace)

	ns.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ns := obj.(*wiringapi.VLANNamespace)

	warns, err := ns.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate vlannamespace")
	}

	return nil, nil
}

func (w *Webhook) ValidateUpdate(_ context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	oldNs := oldObj.(*wiringapi.VLANNamespace)
	newNs := newObj.(*wiringapi.VLANNamespace)

	if !equality.Semantic.DeepEqual(oldNs.Spec, newNs.Spec) {
		return nil, errors.Errorf("VLANNamespace spec is immutable")
	}

	return nil, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ns := obj.(*wiringapi.VLANNamespace)

	switches := &wiringapi.SwitchList{}
	if err := w.Client.List(ctx, switches, kclient.MatchingLabels{
		wiringapi.ListLabelVLANNamespace(ns.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing switches") // TODO hide internal error
	}
	if len(switches.Items) > 0 {
		return nil, errors.Errorf("VLANNamespace has switches")
	}

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelVLANNS: ns.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpcs") // TODO hide internal error
	}
	if len(vpcs.Items) > 0 {
		return nil, errors.Errorf("VLANNamespace has VPCs")
	}

	return nil, nil
}
