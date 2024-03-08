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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ServerWebhook struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    *meta.FabricConfig
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *meta.FabricConfig) error {
	w := &ServerWebhook{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.Server{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*ServerWebhook)(nil)
	_ admission.CustomValidator = (*ServerWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1alpha2-server,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=servers,verbs=create;update,versions=v1alpha2,name=mserver.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-server,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=servers,verbs=create;update;delete,versions=v1alpha2,name=vserver.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("server-webhook")

func (w *ServerWebhook) Default(ctx context.Context, obj runtime.Object) error {
	server := obj.(*wiringapi.Server)

	server.Default()

	return nil
}

func (w *ServerWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	server := obj.(*wiringapi.Server)

	warns, err := server.Validate(ctx, w.Client, w.Cfg)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ServerWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newServer := newObj.(*wiringapi.Server)

	warns, err := newServer.Validate(ctx, w.Client, w.Cfg)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ServerWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
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
