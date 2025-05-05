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
	"time"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type SwitchProfileReconciler struct {
	kclient.Client
	cfg      *meta.FabricConfig
	profiles *switchprofile.Default
}

func SetupSwitchProfileReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, profiles *switchprofile.Default) error {
	if cfg == nil {
		return errors.New("fabric config is nil")
	}

	r := &SwitchProfileReconciler{
		Client:   mgr.GetClient(),
		cfg:      cfg,
		profiles: profiles,
	}

	if err := mgr.Add(r); err != nil {
		return errors.Wrapf(err, "failed to add switch profile initializer")
	}

	return errors.Wrapf(kctrl.NewControllerManagedBy(mgr).
		Named("SwitchProfile").
		For(&wiringapi.SwitchProfile{}).
		Complete(r), "failed to setup switch profile controller")
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles/status,verbs=get;update;patch

func (r *SwitchProfileReconciler) Reconcile(ctx context.Context, _ kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if !r.profiles.IsInitialized() {
		return kctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	if err := r.profiles.Enforce(ctx, r.Client, r.cfg, true); err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error enforcing switch profiles")
	}

	l.Info("switch profiles reconciled")

	return kctrl.Result{}, nil
}

var (
	_ manager.Runnable               = (*SwitchProfileReconciler)(nil)
	_ manager.LeaderElectionRunnable = (*SwitchProfileReconciler)(nil)
)

func (r *SwitchProfileReconciler) Start(ctx context.Context) error {
	l := kctrllog.FromContext(ctx).WithValues("initializer", "switchprofile")
	l.Info("SwitchProfile initial setup")

	var err error
	for attempt := 0; attempt < 60; attempt++ { // TODO think about more graceful way to handle this
		err = r.profiles.Enforce(ctx, r.Client, r.cfg, true)
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

func (r *SwitchProfileReconciler) NeedLeaderElection() bool {
	return true
}
