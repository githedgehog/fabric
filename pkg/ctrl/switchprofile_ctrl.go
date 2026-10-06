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

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type SwitchProfileReconciler struct {
	kclient.Client
	cfg      *meta.FabricConfig
	profiles *switchprofile.Default
	lock     *Lock
}

func SetupSwitchProfileReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, profiles *switchprofile.Default, lock *Lock) error {
	if cfg == nil {
		return errors.New("fabric config is nil")
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &SwitchProfileReconciler{
		Client:   mgr.GetClient(),
		cfg:      cfg,
		profiles: profiles,
		lock:     lock,
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

	// the initialization enforces the profiles on every start while locked
	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	if err := r.profiles.Enforce(ctx, r.Client, r.cfg, true); err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error enforcing switch profiles")
	}

	l.Info("switch profiles reconciled")

	return kctrl.Result{}, nil
}
