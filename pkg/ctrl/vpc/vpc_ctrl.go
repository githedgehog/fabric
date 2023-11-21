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
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	VPC_VNI_OFFSET = 100
	VPC_VNI_MAX    = (16_777_215 - VPC_VNI_OFFSET) / VPC_VNI_OFFSET * VPC_VNI_OFFSET
)

// VPCReconciler reconciles a VPC object
type VPCReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Cfg       *config.Fabric
	vniAssign sync.Mutex
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	r := &VPCReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	// TODO only enqueue related VPCs
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcapi.VPC{}).
		// It's enough to trigger just a single VPC update in this case as it'll update DHCP config for all VPCs
		Watches(&wiringapi.Switch{}, handler.EnqueueRequestsFromMapFunc(r.enqueueOneVPC)).
		Complete(r)
}

func (r *VPCReconciler) enqueueOneVPC(ctx context.Context, obj client.Object) []reconcile.Request {
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

func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	err := r.updateDHCPConfig(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating dhcp config")
	}

	vpc := &vpcapi.VPC{}
	err = r.Get(ctx, req.NamespacedName, vpc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "error getting vpc %s", req.NamespacedName)
	}

	if err := r.ensureVNIs(ctx, vpc); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error ensuring vpc vnis")
	}

	l.Info("vpc reconciled")

	return ctrl.Result{}, nil
}

func (r *VPCReconciler) ensureVNIs(ctx context.Context, vpc *vpcapi.VPC) error {
	l := log.FromContext(ctx)

	if vpc.Status.VNI == 0 {
		l.Info("VPC VNI not set, assigning next free", "vpc", vpc.Name)

		r.vniAssign.Lock()
		defer r.vniAssign.Unlock()

		vpcs := &vpcapi.VPCList{}
		err := r.List(ctx, vpcs) // we have to query all vpcs to find next free vni
		if err != nil {
			return errors.Wrapf(err, "error listing vpcs")
		}

		used := map[uint32]bool{}
		for _, other := range vpcs.Items {
			if other.Status.VNI == 0 {
				continue
			}
			if other.Status.VNI/VPC_VNI_OFFSET < 1 {
				continue
			}
			if other.Status.VNI%VPC_VNI_OFFSET != 0 {
				continue
			}

			used[other.Status.VNI] = true
		}

		ok := false
		for id := uint32(1) * VPC_VNI_OFFSET; id < VPC_VNI_MAX; id += VPC_VNI_OFFSET {
			if !used[id] {
				vpc.Status.VNI = id
				l.Info("VPC VNI assigned", "vpc", vpc.Name, "vni", vpc.Status.VNI)
				ok = true
				break
			}
		}
		if !ok {
			return errors.Errorf("no free vni for vpc %s", vpc.Name)
		}
	}

	if vpc.Status.SubnetVNIs != nil {
		for subnet, vni := range vpc.Status.SubnetVNIs {
			if _, exists := vpc.Spec.Subnets[subnet]; !exists {
				delete(vpc.Status.SubnetVNIs, subnet)
			}
			if vni <= vpc.Status.VNI || vni >= vpc.Status.VNI+VPC_VNI_OFFSET {
				delete(vpc.Status.SubnetVNIs, subnet)
			}
		}
	} else {
		vpc.Status.SubnetVNIs = map[string]uint32{}
	}

	used := map[uint32]bool{}
	for _, vni := range vpc.Status.SubnetVNIs {
		used[vni] = true
	}

	for subnet := range vpc.Spec.Subnets {
		if vpc.Status.SubnetVNIs[subnet] != 0 {
			continue
		}

		ok := false
		for id := vpc.Status.VNI + 1; id < vpc.Status.VNI+VPC_VNI_OFFSET; id++ {
			if !used[id] {
				vpc.Status.SubnetVNIs[subnet] = id
				l.Info("VPC Subnet VNI assigned", "vpc", vpc.Name, "subnet", subnet, "vni", vpc.Status.VNI)
				ok = true
				break
			}
		}

		if !ok {
			return errors.Errorf("no free vni for subnet %s", subnet)
		}
	}

	return errors.Wrapf(r.Status().Update(ctx, vpc), "error updating vpc status %s", vpc.Name)
}
