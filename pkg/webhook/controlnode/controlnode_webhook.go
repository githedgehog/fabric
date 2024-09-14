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

package controlnode

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
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
		For(&wiringapi.ControlNode{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup controlnode webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1alpha2-controlnode,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=controlnodes,verbs=create;update,versions=v1alpha2,name=mcontrolnode.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-controlnode,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=controlnodes,verbs=create;update;delete,versions=v1alpha2,name=vcontrolnode.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("controlnode-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	control := obj.(*wiringapi.ControlNode)

	control.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	control := obj.(*wiringapi.ControlNode)

	warns, err := control.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating control node")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	// oldControl := oldObj.(*wiringapi.ControlNode)
	newControl := newObj.(*wiringapi.ControlNode)

	// TODO

	warns, err := newControl.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating control node")
	}

	return nil, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// control := obj.(*wiringapi.ControlNode)

	// TODO

	return nil, nil
}
