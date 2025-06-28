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

package switchh

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
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

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.Switch{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup switch webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-switch,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update,versions=v1beta1,name=mswitch.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-switch,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update;delete,versions=v1beta1,name=vswitch.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switch-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	sw := obj.(*wiringapi.Switch)

	sw.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	sw := obj.(*wiringapi.Switch)

	warns, err := sw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating switch")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	oldSw := oldObj.(*wiringapi.Switch)
	newSw := newObj.(*wiringapi.Switch)

	warns, err := newSw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating switch")
	}

	if (oldSw.Spec.RoCEv2 || newSw.Spec.RoCEv2) && !reflect.DeepEqual(oldSw.Spec.PortBreakouts, newSw.Spec.PortBreakouts) {
		return warns, errors.New("port breakouts cannot be changed when RoCEv2 is enabled")
	}

	return warns, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	sw := obj.(*wiringapi.Switch)

	conns := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, conns, kclient.MatchingLabels{
		wiringapi.ListLabelSwitch(sw.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(conns.Items) > 0 {
		return nil, errors.Errorf("switch has connections")
	}

	return nil, nil
}
