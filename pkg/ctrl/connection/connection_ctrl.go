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

package controlagent

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Reconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Cfg     *meta.FabricConfig
	LibMngr *librarian.Manager
}

func SetupWithManager(mgr ctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager) error {
	r := &Reconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Cfg:     cfg,
		LibMngr: libMngr,
	}

	return errors.Wrapf(ctrl.NewControllerManagedBy(mgr).
		Named("connection").
		For(&wiringapi.Connection{}).
		Complete(r), "failed to setup connection controller")
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	if err := r.LibMngr.UpdateConnections(ctx, r.Client); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating connections catalog")
	}

	conn := &wiringapi.Connection{}
	err := r.Get(ctx, req.NamespacedName, conn)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, errors.Wrapf(err, "error getting connection")
	}

	l.Info("connection reconciled")

	return ctrl.Result{}, nil
}
