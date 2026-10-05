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

	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ConnectionReconciler allocates the IDs of the ESLAG connections in the connections catalog
type ConnectionReconciler struct {
	kclient.Client
	libr *librarian.Manager
}

func SetupConnectionReconcilerWith(mgr kctrl.Manager, libMngr *librarian.Manager) error {
	if libMngr == nil {
		return fmt.Errorf("librarian manager is nil") //nolint:err113
	}

	r := &ConnectionReconciler{
		Client: mgr.GetClient(),
		libr:   libMngr,
	}

	// only ESLAG connections get IDs allocated
	eslagOnly, err := predicate.LabelSelectorPredicate(kmetav1.LabelSelector{
		MatchLabels: map[string]string{wiringapi.LabelConnectionType: wiringapi.ConnectionTypeESLAG},
	})
	if err != nil {
		return fmt.Errorf("creating ESLAG connections predicate: %w", err)
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("Connection").
		For(&wiringapi.Connection{}, builder.WithPredicates(eslagOnly)).
		Complete(r); err != nil {
		return fmt.Errorf("setting up connection controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

func (r *ConnectionReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	conn := &wiringapi.Connection{}
	if err := r.Get(ctx, req.NamespacedName, conn); err != nil {
		// the IDs of deleted connections are released with the next allocation
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting connection: %w", err)
	}

	// nothing to allocate for a connection being deleted
	if conn.DeletionTimestamp != nil || conn.Spec.ESLAG == nil {
		return kctrl.Result{}, nil
	}

	updated, err := r.libr.UpdateConnections(ctx, r.Client, conn.Name)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("updating connections catalog: %w", err)
	}

	if updated {
		l.Info("Connections catalog updated")
	}

	return kctrl.Result{}, nil
}
