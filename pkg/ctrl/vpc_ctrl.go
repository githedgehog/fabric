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
	"strings"

	"github.com/pkg/errors"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	VPCVNIOffset     = 100
	VPCVNIMax        = (16_777_215 - VPCVNIOffset) / VPCVNIOffset * VPCVNIOffset
	DefaultMTU       = 9036
	DefaultLeaseTime = 3600
)

type VPCReconciler struct {
	kclient.Client
	cfg  *meta.FabricConfig
	libr *librarian.Manager
}

func SetupVPCReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager) error {
	r := &VPCReconciler{
		Client: mgr.GetClient(),
		cfg:    cfg,
		libr:   libMngr,
	}

	// TODO only enqueue related VPCs
	return errors.Wrapf(kctrl.NewControllerManagedBy(mgr).
		Named("VPC").
		For(&vpcapi.VPC{}).
		// It's enough to trigger just a single VPC update in this case as it'll update DHCP config for all VPCs
		Watches(&wiringapi.Switch{}, handler.EnqueueRequestsFromMapFunc(r.enqueueOneVPC)).
		Complete(r), "failed to setup vpc controller")
}

func (r *VPCReconciler) enqueueOneVPC(ctx context.Context, _ kclient.Object) []reconcile.Request {
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

func (r *VPCReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if err := r.libr.UpdateVPCs(ctx, r.Client); err != nil {
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

func (r *VPCReconciler) updateDHCPSubnets(ctx context.Context, vpc *vpcapi.VPC) error {
	err := r.deleteDHCPSubnets(ctx, kclient.ObjectKey{Name: vpc.Name, Namespace: vpc.Namespace}, vpc.Spec.Subnets)
	if err != nil {
		return errors.Wrapf(err, "error deleting obsolete dhcp subnets")
	}

	for subnetName, subnet := range vpc.Spec.Subnets {
		if !subnet.DHCP.Enable || subnet.VLAN == 0 || subnet.DHCP.Range == nil {
			continue
		}

		dhcp := &dhcpapi.DHCPSubnet{ObjectMeta: kmetav1.ObjectMeta{Name: fmt.Sprintf("%s--%s", vpc.Name, subnetName), Namespace: vpc.Namespace}}
		_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, dhcp, func() error {
			pxeURL := ""
			dnsServers := []string{}
			timeServers := []string{}
			mtu := uint16(DefaultMTU)
			leaseTime := uint32(DefaultLeaseTime)
			advertisedRoutes := []dhcpapi.DHCPRoute{}
			disableDefaultRoute := false

			if subnet.DHCP.Options != nil {
				pxeURL = subnet.DHCP.Options.PXEURL
				dnsServers = subnet.DHCP.Options.DNSServers
				timeServers = subnet.DHCP.Options.TimeServers
				for _, route := range subnet.DHCP.Options.AdvertisedRoutes {
					advertisedRoutes = append(advertisedRoutes, dhcpapi.DHCPRoute{
						Destination: route.Destination,
						Gateway:     route.Gateway,
					})
				}
				disableDefaultRoute = subnet.DHCP.Options.DisableDefaultRoute

				if subnet.DHCP.Options.InterfaceMTU > 0 {
					mtu = subnet.DHCP.Options.InterfaceMTU
				}

				if subnet.DHCP.Options.LeaseTimeSeconds > 0 {
					leaseTime = subnet.DHCP.Options.LeaseTimeSeconds
				}
			}

			vrf := ""
			switch vpc.Spec.Mode {
			case vpcapi.VPCModeL2VNI, vpcapi.VPCModeL3VNI:
				vrf = fmt.Sprintf("VrfV%s", vpc.Name) // TODO move to utils
			case vpcapi.VPCModeL3Flat:
				vrf = "default"
			}

			dhcp.Labels = map[string]string{
				vpcapi.LabelVPC:    vpc.Name,
				vpcapi.LabelSubnet: subnetName,
			}
			dhcp.Spec = dhcpapi.DHCPSubnetSpec{
				Subnet:              fmt.Sprintf("%s/%s", vpc.Name, subnetName),
				CIDRBlock:           subnet.Subnet,
				Gateway:             subnet.Gateway,
				StartIP:             subnet.DHCP.Range.Start,
				EndIP:               subnet.DHCP.Range.End,
				LeaseTimeSeconds:    leaseTime,
				VRF:                 vrf,
				CircuitID:           fmt.Sprintf("Vlan%d", subnet.VLAN), // TODO move to utils
				PXEURL:              pxeURL,
				DNSServers:          dnsServers,
				TimeServers:         timeServers,
				InterfaceMTU:        mtu,
				L3Mode:              vpc.Spec.Mode == vpcapi.VPCModeL3Flat || vpc.Spec.Mode == vpcapi.VPCModeL3VNI,
				DisableDefaultRoute: disableDefaultRoute,
				AdvertisedRoutes:    advertisedRoutes,
			}

			return nil
		})
		if err != nil {
			return errors.Wrapf(err, "error creating dhcp subnet for %s/%s", vpc.Name, subnetName)
		}
	}

	return nil
}

func (r *VPCReconciler) deleteDHCPSubnets(ctx context.Context, vpcKey kclient.ObjectKey, subnets map[string]*vpcapi.VPCSubnet) error {
	dhcpSubnets := &dhcpapi.DHCPSubnetList{}
	err := r.List(ctx, dhcpSubnets, kclient.MatchingLabels{vpcapi.LabelVPC: vpcKey.Name})
	if err != nil {
		return errors.Wrapf(err, "error listing dhcp subnets")
	}

	for _, subnet := range dhcpSubnets.Items {
		subnetName := "default"
		parts := strings.Split(subnet.Spec.Subnet, "/")
		if len(parts) == 2 {
			subnetName = parts[1]
		}

		if _, exists := subnets[subnetName]; exists {
			continue
		}

		err = r.Delete(ctx, &subnet)
		if kclient.IgnoreNotFound(err) != nil {
			return errors.Wrapf(err, "error deleting dhcp subnet %s", subnet.Name)
		}
	}

	return nil
}
