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
	kapivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ConnectionWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupConnectionWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &ConnectionWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &wiringapi.Connection{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*wiringapi.Connection](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup connection webhook: %w", err)
	}

	return nil
}

// +kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-connection,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update,versions=v1beta1,name=mconnection.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-connection,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update;delete,versions=v1beta1,name=vconnection.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("connection-webhook")

func (w *ConnectionWebhook) Default(_ context.Context, conn *wiringapi.Connection) error {
	conn.Default()

	return nil
}

func (w *ConnectionWebhook) ValidateCreate(ctx context.Context, conn *wiringapi.Connection) (admission.Warnings, error) {
	warns, err := conn.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating connection: %w", err)
	}

	return warns, nil
}

func (w *ConnectionWebhook) ValidateUpdate(ctx context.Context, oldConn *wiringapi.Connection, newConn *wiringapi.Connection) (admission.Warnings, error) {
	if err := checkFabricUnchanged(oldConn.Spec.Topology.Fabric, newConn.Spec.Topology.Fabric); err != nil {
		return nil, err
	}

	// TODO some connections or their parts should be immutable

	if oldConn.Spec.Type() != newConn.Spec.Type() {
		return nil, field.Forbidden(field.NewPath("spec"), "connection type is immutable")
	}

	if newConn.Spec.StaticExternal != nil && oldConn.Spec.StaticExternal != nil {
		if newConn.Spec.StaticExternal.WithinVPC != oldConn.Spec.StaticExternal.WithinVPC {
			return nil, field.Invalid(field.NewPath("spec", "staticExternal", "withinVPC"), newConn.Spec.StaticExternal.WithinVPC, kapivalidation.FieldImmutableErrorMsg)
		}
	}

	warns, err := newConn.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("error validating connection: %w", err)
	}

	// if newConn.Spec.Unbundled != nil || newConn.Spec.Bundled != nil || newConn.Spec.MCLAG != nil || newConn.Spec.ESLAG != nil {
	// 	if !equality.Semantic.DeepEqual(oldConn.Spec, newConn.Spec) {
	// 		return nil, errors.Errorf("server-facing Connection spec is immutable")
	// 	}
	// }

	return warns, nil
}

func (w *ConnectionWebhook) ValidateDelete(ctx context.Context, conn *wiringapi.Connection) (admission.Warnings, error) {
	vpcAttachments := &vpcapi.VPCAttachmentList{}
	if err := w.Client.List(ctx, vpcAttachments, kclient.MatchingLabels{
		wiringapi.LabelConnection: conn.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing vpc attachments: %w", err) // TODO hide internal error
	}
	if len(vpcAttachments.Items) > 0 {
		return nil, fmt.Errorf("connection has attachments") //nolint:err113
	}

	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := w.Client.List(ctx, extAttachments, kclient.MatchingLabels{
		wiringapi.LabelConnection: conn.Name,
	}); err != nil {
		return nil, fmt.Errorf("error listing external attachments: %w", err) // TODO hide internal error
	}
	if len(extAttachments.Items) > 0 {
		return nil, fmt.Errorf("connection has external attachments") //nolint:err113
	}

	return nil, nil
}
