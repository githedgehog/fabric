/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vpc

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/ctrl/common"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	VPC_CTRL_CONFIG = "vpc-ctrl-config.yaml"
)

type VPCControllerConfig struct {
	VPCVLANRange common.VLANRange `json:"vpcVLANRange,omitempty"`
}

// VPCReconciler reconciles a VPC object
type VPCReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Cfg        *VPCControllerConfig
	vlanAssign sync.Mutex
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	cfg := &VPCControllerConfig{}
	err := common.LoadCtrlConfig(cfgBasedir, VPC_CTRL_CONFIG, cfg)
	if err != nil {
		return err
	}

	if err := cfg.VPCVLANRange.Validate(); err != nil {
		return errors.Wrapf(err, "config: vpcVLANRange is invalid")
	}

	r := &VPCReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcapi.VPC{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vpc := &vpcapi.VPC{}
	err := r.Get(ctx, req.NamespacedName, vpc)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting vpc %s", req.NamespacedName)
	}

	if vpc.Status.VLAN == 0 {
		err = r.setNextFreeVLAN(ctx, vpc)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error assigning vlan to vpc %s", vpc.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *VPCReconciler) setNextFreeVLAN(ctx context.Context, vpc *vpcapi.VPC) error {
	if vpc.Status.VLAN != 0 {
		return nil
	}

	l := log.FromContext(ctx)
	l.Info("vpc vlan not set, assigning next free", "vpc", vpc.Name)

	r.vlanAssign.Lock()
	defer r.vlanAssign.Unlock()

	vpcs := &vpcapi.VPCList{}
	err := r.List(ctx, vpcs)
	if err != nil {
		return errors.Wrapf(err, "error listing vpcs")
	}

	used := make([]bool, r.Cfg.VPCVLANRange.Max-r.Cfg.VPCVLANRange.Min+1)
	for _, v := range vpcs.Items {
		if v.Status.VLAN > 0 && (v.Status.VLAN < r.Cfg.VPCVLANRange.Min || v.Status.VLAN > r.Cfg.VPCVLANRange.Max) {
			l.Info("vpc vlan out of range, ignoring", "vpc", v.Name, "vlan", v.Status.VLAN)
			continue
		}
		used[v.Status.VLAN-r.Cfg.VPCVLANRange.Min] = true
	}

	for idx, val := range used {
		if !val {
			vpc.Status.VLAN = uint16(idx) + r.Cfg.VPCVLANRange.Min
			break
		}
	}

	l.Info("vpc vlan assigned", "vpc", vpc.Name, "vlab", vpc.Status.VLAN)

	err = r.Status().Update(ctx, vpc)
	if err != nil {
		return errors.Wrapf(err, "error updating vpc status %s", vpc.Name)
	}

	return nil
}
