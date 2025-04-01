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

package vpc

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	VPCVNIOffset = 100
	VPCVNIMax    = (16_777_215 - VPCVNIOffset) / VPCVNIOffset * VPCVNIOffset
)

// Reconciler reconciles a VPC object
type Reconciler struct {
	kclient.Client
	Scheme  *runtime.Scheme
	Cfg     *meta.FabricConfig
	LibMngr *librarian.Manager
}

func SetupWithManager(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager) error {
	r := &Reconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Cfg:     cfg,
		LibMngr: libMngr,
	}

	// TODO only enqueue related VPCs
	return errors.Wrapf(kctrl.NewControllerManagedBy(mgr).
		Named("vpc").
		For(&vpcapi.VPC{}).
		// It's enough to trigger just a single VPC update in this case as it'll update DHCP config for all VPCs
		Watches(&wiringapi.Switch{}, handler.EnqueueRequestsFromMapFunc(r.enqueueOneVPC)).
		Complete(r), "failed to setup vpc controller")
}

func (r *Reconciler) enqueueOneVPC(ctx context.Context, _ kclient.Object) []reconcile.Request {
	res := []reconcile.Request{}

	vpcs := &vpcapi.VPCList{}
	err := r.List(ctx, vpcs, kclient.Limit(1))
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing vpcs")

		return res
	}
	if len(vpcs.Items) > 0 {
		res = append(res, reconcile.Request{
			NamespacedName: kclient.ObjectKeyFromObject(&vpcs.Items[0]),
		})
	}

	return res
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=dhcp.githedgehog.com,resources=dhcpsubnets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dhcp.githedgehog.com,resources=dhcpsubnets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dhcp.githedgehog.com,resources=dhcpsubnets/finalizers,verbs=update

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

func (r *Reconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if err := r.LibMngr.UpdateVPCs(ctx, r.Client); err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error updating vpcs catalog")
	}

	vpc := &vpcapi.VPC{}
	err := r.Get(ctx, req.NamespacedName, vpc)
	if err != nil {
		if kapierrors.IsNotFound(err) {
			l.Info("vpc deleted, cleaning up dhcp subnets")
			err = r.deleteDHCPSubnets(ctx, req.NamespacedName, map[string]*vpcapi.VPCSubnet{})
			if err != nil {
				return kctrl.Result{}, errors.Wrapf(err, "error deleting dhcp subnets for removed vpc")
			}

			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, errors.Wrapf(err, "error getting vpc %s", req.NamespacedName)
	}

	err = r.updateDHCPSubnets(ctx, vpc)
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error updating dhcp subnets")
	}

	l.Info("vpc reconciled")

	return kctrl.Result{}, nil
}
