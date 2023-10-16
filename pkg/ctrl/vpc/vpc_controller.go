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
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"golang.org/x/exp/maps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// VPCReconciler reconciles a VPC object
type VPCReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Cfg        *config.Fabric
	vlanAssign sync.Mutex
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	r := &VPCReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcapi.VPC{}).
		Watches(&vpcapi.VPCAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForAttach)).
		Watches(&vpcapi.VPCPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForPeering)).
		Complete(r)
}

func (r *VPCReconciler) enqueueForAttach(ctx context.Context, obj client.Object) []reconcile.Request {
	attach, ok := obj.(*vpcapi.VPCAttachment)
	if !ok {
		panic(fmt.Sprintf("enqueueForAttach got not a VPCAttachment: %#v", obj))
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Name:      attach.Spec.VPC,
			Namespace: attach.Namespace,
		},
	}}
}

func (r *VPCReconciler) enqueueForPeering(ctx context.Context, obj client.Object) []reconcile.Request {
	peering, ok := obj.(*vpcapi.VPCPeering)
	if !ok {
		panic(fmt.Sprintf("enqueueForPeering got not a VPCPeering: %#v", obj))
	}

	res := []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      peering.Spec.VPCs[0],
				Namespace: peering.Namespace, // TODO ns
			},
		},
		{
			NamespacedName: client.ObjectKey{
				Name:      peering.Spec.VPCs[1],
				Namespace: peering.Namespace, // TODO ns
			},
		},
	}

	return res
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments/finalizers,verbs=update

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcpeerings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcpeerings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcpeerings/finalizers,verbs=update

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcsummaries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcsummaries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcsummaries/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	vpc := &vpcapi.VPC{}
	err := r.Get(ctx, req.NamespacedName, vpc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.Delete(ctx, &vpcapi.VPCSummary{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}) // TODO ns
			if err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Wrapf(err, "error deleting summary for vpc %s after its being deleted", req.NamespacedName)
			}
		}

		return ctrl.Result{}, errors.Wrapf(err, "error getting vpc %s", req.NamespacedName)
	}

	if vpc.Status.VLAN == 0 {
		err = r.setNextFreeVLAN(ctx, vpc)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error assigning vlan to vpc %s", vpc.Name)
		}
		l.Info("vpc vlan assigned", "vpc", vpc.Name, "vlan", vpc.Status.VLAN)
	}

	attaches := &vpcapi.VPCAttachmentList{}
	err = r.List(ctx, attaches, client.InNamespace(req.Namespace), client.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing vpc attachments for vpc %s", vpc.Name)
	}

	connNames := []string{}
	for _, attach := range attaches.Items {
		connNames = append(connNames, attach.Spec.Connection)
	}
	sort.Slice(connNames, func(i, j int) bool {
		return connNames[i] < connNames[j]
	})

	summaryLabels := map[string]string{}
	for _, connName := range connNames {
		conn := &wiringapi.Connection{}
		err := r.Get(ctx, client.ObjectKey{Name: connName, Namespace: vpc.Namespace}, conn) // TODO ns
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting connection %s", connName)
		}

		maps.Copy(summaryLabels, conn.Spec.ConnectionLabels())
	}

	peerings := &vpcapi.VPCPeeringList{}
	err = r.List(ctx, peerings, client.InNamespace(req.Namespace), client.MatchingLabels{
		vpcapi.ListLabelVPC(vpc.Name): vpcapi.ListLabelValue,
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing vpc peerings for vpc %s", vpc.Name)
	}

	peers := []string{}
	for _, peering := range peerings.Items {
		for _, peer := range peering.Spec.VPCs {
			if peer == vpc.Name {
				continue
			}
			vpc := &vpcapi.VPC{}
			err := r.Get(ctx, client.ObjectKey{Name: peer, Namespace: peering.Namespace}, vpc) // TODO ns
			if err != nil {
				if apierrors.IsNotFound(err) {
					l.Info("vpc peering to non-existing vpc, ignoring", "vpc", peer)
					continue
				}

				return ctrl.Result{}, errors.Wrapf(err, "error getting vpc %s to check peering", peer)
			}

			peers = append(peers, vpc.Name) // TODO ns
		}
	}
	sort.Slice(peers, func(i, j int) bool {
		return peers[i] < peers[j]
	})

	nat := &vpcapi.NAT{}
	err = r.Get(ctx, client.ObjectKey{Name: "default", Namespace: vpc.Namespace}, nat) // TODO ns and multiple nats
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "error getting nat for vpc %s", vpc.Name)
	}

	dnat := map[string]string{}
	if nat.Spec.Subnet != "" && len(nat.Spec.DNATPool) > 0 {
		vpcs := &vpcapi.VPCSummaryList{}
		err = r.List(ctx, vpcs)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error listing vpc summaries")
		}

		_, natNet, err := net.ParseCIDR(nat.Spec.Subnet)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error parsing nat subnet %s", nat.Spec.Subnet)
		}

		_, vpcNet, err := net.ParseCIDR(vpc.Spec.Subnet)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error parsing vpc subnet %s", vpc.Spec.Subnet)
		}

		internalIPs := map[string]bool{}
		externalIPs := map[string]bool{}
		dnats := map[string]string{}
		for _, some := range vpcs.Items {
			for internalIP, externalIP := range some.Spec.DNAT {
				internalIPs[internalIP] = true
				if !strings.HasPrefix(externalIP, "@") {
					externalIPs[externalIP] = true
				}
				dnats[internalIP] = externalIP
			}
		}

		for internalIP, externalIP := range vpc.Spec.DNATRequests {
			if dnats[internalIP] == externalIP {
				continue
			}

			result := ""
			if internalIP == "" {
				result = "internal IP is empty"
			} else if externalIP == "" {
				result = "external IP is empty"
			} else if internalIPs[internalIP] {
				result = "internal IP already used in DNAT"
			} else if externalIPs[externalIP] {
				result = "external IP already used in DNAT"
			} else {
				ip := net.ParseIP(externalIP)
				if ip == nil {
					result = "external IP is not a valid IP"
				} else if !natNet.Contains(ip) {
					result = "external IP is not in NAT subnet"
				}

				ip = net.ParseIP(internalIP)
				if ip == nil {
					result = "internal IP is not a valid IP"
				} else if !vpcNet.Contains(ip) {
					result = "internal IP is not in NAT subnet"
				}
			}

			if result != "" {
				result = "@" + result
			} else {
				externalIPs[externalIP] = true
				internalIPs[internalIP] = true
				result = externalIP
			}

			dnat[internalIP] = result
		}
	}

	summary := &vpcapi.VPCSummary{ObjectMeta: metav1.ObjectMeta{Name: vpc.Name, Namespace: vpc.Namespace}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, summary, func() error {
		summary.Spec.Name = vpc.Name
		summary.Spec.VPC = vpc.Spec
		summary.Spec.VLAN = vpc.Status.VLAN
		summary.Spec.Connections = connNames
		summary.Spec.Peers = peers
		summary.Labels = summaryLabels
		summary.Spec.DNAT = dnat

		return nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error creating summary for vpc %s", vpc.Name)
	}

	err = r.updateDHCPConfig(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating dhcp config")
	}

	l.Info("vpc reconciled")

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
	err := r.List(ctx, vpcs) // we have to query all vpcs to find next free vlan
	if err != nil {
		return errors.Wrapf(err, "error listing vpcs")
	}

	used := make([]bool, r.Cfg.VPCVLANRange.Max-r.Cfg.VPCVLANRange.Min+1)
	for _, v := range vpcs.Items {
		if v.Status.VLAN == 0 {
			continue
		}
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

	err = r.Status().Update(ctx, vpc)
	if err != nil {
		return errors.Wrapf(err, "error updating vpc status %s", vpc.Name)
	}

	return nil
}
