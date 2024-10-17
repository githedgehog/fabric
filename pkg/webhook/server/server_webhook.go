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

package server

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Webhook struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    *meta.FabricConfig
}

func SetupWithManager(mgr ctrl.Manager, cfg *meta.FabricConfig) error {
	w := &Webhook{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return errors.Wrapf(ctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.Server{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup server webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-server,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=servers,verbs=create;update,versions=v1beta1,name=mserver.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-server,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=servers,verbs=create;update;delete,versions=v1beta1,name=vserver.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("server-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	server := obj.(*wiringapi.Server)

	server.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	server := obj.(*wiringapi.Server)

	warns, err := server.Validate(ctx, w.Client, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating server")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newServer := newObj.(*wiringapi.Server)

	warns, err := newServer.Validate(ctx, w.Client, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating server")
	}

	return warns, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	server := obj.(*wiringapi.Server)

	conns := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, conns, client.MatchingLabels{
		wiringapi.ListLabelServer(server.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(conns.Items) > 0 {
		return nil, errors.Errorf("server has connections")
	}

	return nil, nil
}
