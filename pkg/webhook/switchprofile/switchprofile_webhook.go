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

package switchprofile

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"k8s.io/apimachinery/pkg/api/equality"
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
	Profiles   *switchprofile.Default
}

func SetupWithManager(mgr ctrl.Manager, cfg *meta.FabricConfig, profiles *switchprofile.Default) error {
	w := &Webhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
		Profiles:   profiles,
	}

	return errors.Wrapf(ctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.SwitchProfile{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup switch profile webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1alpha2-switchprofile,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchprofiles,verbs=create;update,versions=v1alpha2,name=mswitchprofile.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-switchprofile,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchprofiles,verbs=create;update;delete,versions=v1alpha2,name=vswitchprofile.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switchprofile-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	sw := obj.(*wiringapi.SwitchProfile)

	sw.Default()

	return nil
}

func (w *Webhook) Validate(ctx context.Context, sp *wiringapi.SwitchProfile) (admission.Warnings, error) {
	if sp.Name == "" {
		return nil, errors.Errorf("switch profile name must be set")
	}

	warns, err := sp.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating switchprofile")
	}

	if dsp := w.Profiles.Get(sp.Name); dsp != nil {
		if !equality.Semantic.DeepEqual(dsp.Spec, sp.Spec) {
			return nil, errors.Errorf("default switch profiles are immutable")
		}
	}

	return warns, nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	sp := obj.(*wiringapi.SwitchProfile)

	if dsp := w.Profiles.Get(sp.Name); dsp == nil && !w.Cfg.AllowExtraSwitchProfiles {
		return nil, errors.Errorf("only default switch profiles are allowed")
	}

	return w.Validate(ctx, sp)
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	sp := newObj.(*wiringapi.SwitchProfile)

	return w.Validate(ctx, sp)
}

func (w *Webhook) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	sp := obj.(*wiringapi.SwitchProfile)

	if dsp := w.Profiles.Get(sp.Name); dsp != nil {
		return nil, errors.Errorf("default switch profiles are immutable")
	}

	return nil, nil
}
