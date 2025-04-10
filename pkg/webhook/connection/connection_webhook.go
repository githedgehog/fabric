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

package connection

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
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
		For(&wiringapi.Connection{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup connection webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-connection,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update,versions=v1beta1,name=mconnection.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-connection,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update;delete,versions=v1beta1,name=vconnection.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("connection-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	conn := obj.(*wiringapi.Connection)

	conn.Default()

	return nil
}

// validateStaticExternal checks that the static external connection is valid and it's located in a webhook to avoid circular dependency with vpcapi
func (w *Webhook) validateStaticExternal(ctx context.Context, kube kclient.Reader, conn *wiringapi.Connection) error {
	if conn.Spec.StaticExternal != nil && conn.Spec.StaticExternal.WithinVPC != "" {
		vpc := &vpcapi.VPC{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: conn.Spec.StaticExternal.WithinVPC, Namespace: conn.Namespace}, vpc) // TODO namespace could be different?
		if kapierrors.IsNotFound(err) {
			return errors.Errorf("vpc %s not found", conn.Spec.StaticExternal.WithinVPC)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to get vpc %s", conn.Spec.StaticExternal.WithinVPC) // TODO replace with some internal error to not expose to the user
		}
	}

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	conn := obj.(*wiringapi.Connection)

	warns, err := conn.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating connection")
	}

	return warns, w.validateStaticExternal(ctx, w.KubeClient, conn)
}

func (w *Webhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	// TODO some connections or their parts should be immutable

	oldConn := oldObj.(*wiringapi.Connection)
	newConn := newObj.(*wiringapi.Connection)

	if oldConn.Spec.Type() != newConn.Spec.Type() {
		return nil, errors.Errorf("connection type is immutable")
	}

	warns, err := newConn.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating connection")
	}

	// if newConn.Spec.Unbundled != nil || newConn.Spec.Bundled != nil || newConn.Spec.MCLAG != nil || newConn.Spec.ESLAG != nil {
	// 	if !equality.Semantic.DeepEqual(oldConn.Spec, newConn.Spec) {
	// 		return nil, errors.Errorf("server-facing Connection spec is immutable")
	// 	}
	// }

	return warns, w.validateStaticExternal(ctx, w.KubeClient, newConn)
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	conn := obj.(*wiringapi.Connection)

	vpcAttachments := &vpcapi.VPCAttachmentList{}
	if err := w.Client.List(ctx, vpcAttachments, kclient.MatchingLabels{
		wiringapi.LabelConnection: conn.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc attachments") // TODO hide internal error
	}
	if len(vpcAttachments.Items) > 0 {
		return nil, errors.Errorf("connection has attachments")
	}

	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := w.Client.List(ctx, extAttachments, kclient.MatchingLabels{
		wiringapi.LabelConnection: conn.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external attachments") // TODO hide internal error
	}
	if len(extAttachments.Items) > 0 {
		return nil, errors.Errorf("connection has external attachments")
	}

	// This is a light check to make sure that no mclags links transit this domain before we delete the domain
	if conn.Spec.MCLAGDomain != nil {
		labels := conn.Spec.ConnectionLabels()
		// overwrite the MCLAG-Domain ConnectionType with just an MCLAG type
		labels[wiringapi.LabelConnectionType] = wiringapi.ConnectionTypeMCLAG
		mclagList := &wiringapi.ConnectionList{}
		// The matching here, will logically and the key/vals in labels together
		// giving just the relevant connections
		if err := w.Client.List(ctx, mclagList, kclient.MatchingLabels(labels)); err != nil {
			return nil, errors.Errorf("error listing MCLAG connections")
		}
		if len(mclagList.Items) > 0 {
			return nil, errors.Errorf("%d MCLAG connection(s) present. Delete those before the domain", len(mclagList.Items))
		}
	}

	return nil, nil
}
