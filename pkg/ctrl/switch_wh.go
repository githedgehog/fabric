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
	"reflect"

	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type SwitchWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupSwitchWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &SwitchWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &wiringapi.Switch{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*wiringapi.Switch](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup switch webhook: %w", err)
	}

	return nil
}

// +kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-switch,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update,versions=v1beta1,name=mswitch.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-switch,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update;delete,versions=v1beta1,name=vswitch.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switch-webhook")

func (w *SwitchWebhook) Default(_ context.Context, sw *wiringapi.Switch) error {
	sw.Default()

	return nil
}

func (w *SwitchWebhook) ValidateCreate(ctx context.Context, sw *wiringapi.Switch) (admission.Warnings, error) {
	warns, err := sw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating switch: %w", err)
	}

	return warns, nil
}

func (w *SwitchWebhook) ValidateUpdate(ctx context.Context, oldSw *wiringapi.Switch, newSw *wiringapi.Switch) (admission.Warnings, error) {
	if fabricChanged(oldSw.Spec.Topology.Fabric, newSw.Spec.Topology.Fabric) {
		return nil, field.Invalid(field.NewPath("spec", "topology", "fabric"), newSw.Spec.Topology.Fabric, "field is immutable")
	}
	// SONiC refuses to change the local AS of a running BGP instance, so the agent would fail on
	// every attempt, and peers would keep the old ASN as nothing regenerates their config
	if oldSw.Spec.ASN != newSw.Spec.ASN {
		return nil, field.Invalid(field.NewPath("spec", "asn"), newSw.Spec.ASN, "field is immutable, delete and recreate the switch to change it")
	}
	if domainsChanged(oldSw.Spec.Topology.Domains, newSw.Spec.Topology.Domains) {
		return nil, field.Invalid(field.NewPath("spec", "topology", "domains"), newSw.Spec.Topology.Domains, "field is immutable, delete and recreate the switch to change it")
	}

	warns, err := newSw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating switch: %w", err)
	}

	if (oldSw.Spec.RoCE || newSw.Spec.RoCE) && !reflect.DeepEqual(oldSw.Spec.PortBreakouts, newSw.Spec.PortBreakouts) {
		return warns, field.Forbidden(field.NewPath("spec", "portBreakouts"), "cannot be changed when RoCEv2 is enabled")
	}

	return warns, nil
}

func (w *SwitchWebhook) ValidateDelete(ctx context.Context, sw *wiringapi.Switch) (admission.Warnings, error) {
	conns := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, conns, kclient.MatchingLabels{
		wiringapi.ListLabelSwitch(sw.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, fmt.Errorf("error listing connections: %w", err) // TODO hide internal error
	}
	if len(conns.Items) > 0 {
		return nil, fmt.Errorf("switch has connections") //nolint:err113
	}

	return nil, nil
}
