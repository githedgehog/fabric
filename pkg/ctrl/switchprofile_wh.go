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

package ctrl

import (
	"context"
	"fmt"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type SwitchProfileWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
	Profiles   *switchprofile.Default
}

func SetupSwitchProfileWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, profiles *switchprofile.Default, lock *Lock) error {
	w := &SwitchProfileWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
		Profiles:   profiles,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &wiringapi.SwitchProfile{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*wiringapi.SwitchProfile](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup switch profile webhook: %w", err)
	}

	return nil
}

// +kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-switchprofile,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchprofiles,verbs=create;update,versions=v1beta1,name=mswitchprofile.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-switchprofile,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switchprofiles,verbs=create;update;delete,versions=v1beta1,name=vswitchprofile.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switchprofile-webhook")

func (w *SwitchProfileWebhook) Default(_ context.Context, sp *wiringapi.SwitchProfile) error {
	sp.Default()

	return nil
}

func (w *SwitchProfileWebhook) validate(ctx context.Context, sp *wiringapi.SwitchProfile) (admission.Warnings, error) {
	if sp.Name == "" {
		return nil, fmt.Errorf("switch profile name must be set") //nolint:err113
	}

	warns, err := sp.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating switchprofile: %w", err)
	}

	if dsp := w.Profiles.Get(sp.Name); dsp != nil {
		if !equality.Semantic.DeepEqual(dsp.Spec, sp.Spec) {
			return nil, fmt.Errorf("default switch profiles are immutable") //nolint:err113
		}
	}

	return warns, nil
}

func (w *SwitchProfileWebhook) ValidateCreate(ctx context.Context, sp *wiringapi.SwitchProfile) (admission.Warnings, error) {
	if dsp := w.Profiles.Get(sp.Name); dsp == nil && !w.Cfg.AllowExtraSwitchProfiles {
		return nil, fmt.Errorf("only default switch profiles are allowed") //nolint:err113
	}

	return w.validate(ctx, sp)
}

func (w *SwitchProfileWebhook) ValidateUpdate(ctx context.Context, _ *wiringapi.SwitchProfile, sp *wiringapi.SwitchProfile) (admission.Warnings, error) {
	return w.validate(ctx, sp)
}

func (w *SwitchProfileWebhook) ValidateDelete(_ context.Context, sp *wiringapi.SwitchProfile) (admission.Warnings, error) {
	if dsp := w.Profiles.Get(sp.Name); dsp != nil {
		return nil, fmt.Errorf("default switch profiles are immutable") //nolint:err113
	}

	return nil, nil
}
