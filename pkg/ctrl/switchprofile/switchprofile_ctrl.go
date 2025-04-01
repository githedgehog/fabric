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
	"time"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Reconciler struct {
	kclient.Client
	Scheme   *runtime.Scheme
	Cfg      *meta.FabricConfig
	Profiles *Default
}

func SetupWithManager(mgr kctrl.Manager, cfg *meta.FabricConfig, _ *librarian.Manager, profiles *Default) error {
	r := &Reconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Cfg:      cfg,
		Profiles: profiles,
	}

	if err := mgr.Add(&Initializer{
		Client:   mgr.GetClient(),
		Cfg:      cfg,
		Profiles: profiles,
	}); err != nil {
		return errors.Wrapf(err, "failed to add switch profile initializer")
	}

	return errors.Wrapf(kctrl.NewControllerManagedBy(mgr).
		Named("switchprofile").
		For(&wiringapi.SwitchProfile{}).
		Complete(r), "failed to setup switch profile controller")
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, _ kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if !r.Profiles.IsInitialized() {
		return kctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	if err := r.Profiles.Enforce(ctx, r.Client, r.Cfg, true); err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error enforcing switch profiles")
	}

	l.Info("switch profiles reconciled")

	return kctrl.Result{}, nil
}

type Initializer struct {
	Client   kclient.Client
	Cfg      *meta.FabricConfig
	Profiles *Default
}

var (
	_ manager.Runnable               = (*Initializer)(nil)
	_ manager.LeaderElectionRunnable = (*Initializer)(nil)
)

func (i *Initializer) Start(ctx context.Context) error {
	l := kctrllog.FromContext(ctx).WithValues("initializer", "switchprofile")
	l.Info("SwitchProfile initial setup")

	var err error
	for attempt := 0; attempt < 60; attempt++ { // TODO think about more graceful way to handle this
		err = i.Profiles.Enforce(ctx, i.Client, i.Cfg, true)
		if err == nil {
			break
		}

		l.Info("Failed to enforce switch profiles", "attempt", attempt, "error", err)
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		return errors.Wrap(err, "error enforcing switch profiles")
	}

	return nil
}

func (i *Initializer) NeedLeaderElection() bool {
	return true
}
