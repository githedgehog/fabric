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
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
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

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("VPC").
		For(&vpcapi.VPC{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up vpc controller: %w", err)
	}

	return nil
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=dhcp.githedgehog.com,resources=dhcpsubnets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dhcp.githedgehog.com,resources=dhcpsubnets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dhcp.githedgehog.com,resources=dhcpsubnets/finalizers,verbs=update

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

func (r *VPCReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	vpc := &vpcapi.VPC{}
	if err := r.Get(ctx, req.NamespacedName, vpc); err != nil {
		if kapierrors.IsNotFound(err) {
			// the VNIs of deleted VPCs are released with the next allocation
			l.Info("vpc deleted, cleaning up dhcp subnets")
			if err := r.deleteDHCPSubnets(ctx, req.NamespacedName, map[string]*vpcapi.VPCSubnet{}); err != nil {
				return kctrl.Result{}, fmt.Errorf("deleting dhcp subnets for removed vpc: %w", err)
			}

			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting vpc %s: %w", req.NamespacedName, err)
	}

	updated, err := r.libr.EnsureVNIs(ctx, r.Client, map[string]vpcapi.VPCSpec{vpc.Name: vpc.Spec}, nil)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("updating VNIs catalog: %w", err)
	}
	if updated {
		l.Info("VNIs catalog updated")
	}

	if err := r.updateDHCPSubnets(ctx, vpc); err != nil {
		return kctrl.Result{}, fmt.Errorf("updating dhcp subnets: %w", err)
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
				vrf = strings.ToLower(vpc.Name)
			case vpcapi.VPCModeL3Flat:
				vrf = "default"
			}

			statics := map[string]dhcpapi.DHCPSubnetStatic{}
			for mac, static := range subnet.DHCP.Static {
				statics[mac] = dhcpapi.DHCPSubnetStatic{
					IP: static.IP,
				}
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
				CircuitID:           fmt.Sprintf("vlan%d", subnet.VLAN), // TODO move to utils
				PXEURL:              pxeURL,
				DNSServers:          dnsServers,
				TimeServers:         timeServers,
				InterfaceMTU:        mtu,
				L3Mode:              vpc.Spec.Mode == vpcapi.VPCModeL3Flat || vpc.Spec.Mode == vpcapi.VPCModeL3VNI,
				DisableDefaultRoute: disableDefaultRoute,
				AdvertisedRoutes:    advertisedRoutes,
				Static:              statics,
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
