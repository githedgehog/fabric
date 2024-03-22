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
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	VPCVNIOffset = 100
	VPCVNIMax    = (16_777_215 - VPCVNIOffset) / VPCVNIOffset * VPCVNIOffset
)

// Reconciler reconciles a VPC object
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

	// TODO only enqueue related VPCs
	return errors.Wrapf(ctrl.NewControllerManagedBy(mgr).
		For(&vpcapi.VPC{}).
		// It's enough to trigger just a single VPC update in this case as it'll update DHCP config for all VPCs
		Watches(&wiringapi.Switch{}, handler.EnqueueRequestsFromMapFunc(r.enqueueOneVPC)).
		Complete(r), "failed to setup vpc controller")
}

func (r *Reconciler) enqueueOneVPC(ctx context.Context, _ client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	vpcs := &vpcapi.VPCList{}
	err := r.List(ctx, vpcs, client.Limit(1))
	if err != nil {
		log.FromContext(ctx).Error(err, "error listing vpcs")

		return res
	}
	if len(vpcs.Items) > 0 {
		res = append(res, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&vpcs.Items[0]),
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

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	if err := r.LibMngr.UpdateVPCs(ctx, r.Client); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating vpcs catalog")
	}

	err := r.updateISCDHCPConfig(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating dhcp config")
	}

	vpc := &vpcapi.VPC{}
	err = r.Get(ctx, req.NamespacedName, vpc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("vpc deleted, cleaning up dhcp subnets")
			err = r.deleteDHCPSubnets(ctx, req.NamespacedName, map[string]*vpcapi.VPCSubnet{})
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error deleting dhcp subnets for removed vpc")
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, errors.Wrapf(err, "error getting vpc %s", req.NamespacedName)
	}

	err = r.updateDHCPSubnets(ctx, vpc)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating dhcp subnets")
	}

	l.Info("vpc reconciled")

	return ctrl.Result{}, nil
}
