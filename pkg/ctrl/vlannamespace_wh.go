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
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VLANNamespaceWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupVLANNamespaceWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &VLANNamespaceWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &wiringapi.VLANNamespace{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*wiringapi.VLANNamespace](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup vlannamespace webhook: %w", err)
	}

	return nil
}

// +kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-vlannamespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=create;update,versions=v1beta1,name=mvlannamespace.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-vlannamespace,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=create;update;delete,versions=v1beta1,name=vvlannamespace.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vlannamespace-webhook")

func (w *VLANNamespaceWebhook) Default(_ context.Context, ns *wiringapi.VLANNamespace) error {
	ns.Default()

	return nil
}

func (w *VLANNamespaceWebhook) ValidateCreate(ctx context.Context, ns *wiringapi.VLANNamespace) (admission.Warnings, error) {
	warns, err := ns.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate vlannamespace: %w", err)
	}

	return nil, nil
}

func (w *VLANNamespaceWebhook) ValidateUpdate(ctx context.Context, _ *wiringapi.VLANNamespace, ns *wiringapi.VLANNamespace) (admission.Warnings, error) {
	if warn, err := ns.Validate(ctx, w.Client, w.Cfg); err != nil {
		return warn, fmt.Errorf("failed to validate new vlannamespace: %w", err)
	}

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelVLANNS: ns.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing vpcs: %w", err) // TODO hide internal error
	}

	for _, vpc := range vpcs.Items {
		for subnetName, subnetCfg := range vpc.Spec.Subnets {
			if !subnetCfg.HostBGP && !ns.Spec.Contains(subnetCfg.VLAN) {
				return nil, field.Invalid(field.NewPath("spec", "ranges"), field.OmitValueType{}, fmt.Sprintf("existing vpc %s subnet %s (vlan %d) doesn't fit updated VLAN namespace", vpc.Name, subnetName, subnetCfg.VLAN))
			}
		}
	}

	return nil, nil
}

func (w *VLANNamespaceWebhook) ValidateDelete(ctx context.Context, ns *wiringapi.VLANNamespace) (admission.Warnings, error) {
	switches := &wiringapi.SwitchList{}
	if err := w.Client.List(ctx, switches, kclient.MatchingLabels{
		wiringapi.ListLabelVLANNamespace(ns.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, fmt.Errorf("error listing switches: %w", err) // TODO hide internal error
	}
	if len(switches.Items) > 0 {
		return nil, fmt.Errorf("VLANNamespace has switches") //nolint:err113
	}

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelVLANNS: ns.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing vpcs: %w", err) // TODO hide internal error
	}
	if len(vpcs.Items) > 0 {
		return nil, fmt.Errorf("VLANNamespace has VPCs") //nolint:err113
	}

	return nil, nil
}
