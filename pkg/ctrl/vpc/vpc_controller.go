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
	"sync"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/ctrl/common"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	VPC_CTRL_CONFIG = "vpc-ctrl-config.yaml"
)

type VPCControllerConfig struct {
	ControlVIP     string           `json:"controlVIP,omitempty"`
	VPCVLANRange   common.VLANRange `json:"vpcVLANRange,omitempty"`
	DHCPDConfigMap string           `json:"dhcpdConfigMap,omitempty"`
	DHCPDConfigKey string           `json:"dhcpdConfigKey,omitempty"`
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

	if cfg.ControlVIP == "" {
		return errors.Errorf("config: controlVIP is required")
	}
	if err := cfg.VPCVLANRange.Validate(); err != nil {
		return errors.Wrapf(err, "config: vpcVLANRange is invalid")
	}
	if cfg.DHCPDConfigMap == "" {
		return errors.Errorf("config: dhcpdConfigMap is required")
	}
	if cfg.DHCPDConfigKey == "" {
		return errors.Errorf("config: dhcpdConfigKey is required")
	}

	r := &VPCReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcapi.VPC{}).
		Watches(&vpcapi.VPCAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForAttach)).
		Complete(r)
}

func (r *VPCReconciler) enqueueForAttach(ctx context.Context, obj client.Object) []reconcile.Request {
	attach, ok := obj.(*vpcapi.VPCAttachment)
	if !ok {
		panic(fmt.Sprintf("enqueueVPCByAttachName got not a VPCAttachment: %#v", obj))
	}

	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Name:      attach.Spec.VPC,
			Namespace: attach.Namespace,
		},
	}}
}

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/finalizers,verbs=update

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments/finalizers,verbs=update

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcsummaries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcsummaries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcsummaries/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

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

	summaryLabels := map[string]string{}
	for _, connName := range connNames {
		conn := &wiringapi.Connection{}
		err := r.Get(ctx, client.ObjectKey{Name: connName, Namespace: vpc.Namespace}, conn) // TODO ns
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting connection %s", connName)
		}

		maps.Copy(summaryLabels, conn.Spec.ConnectionLabels())
	}

	summary := &vpcapi.VPCSummary{ObjectMeta: metav1.ObjectMeta{Name: vpc.Name, Namespace: vpc.Namespace}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, summary, func() error {
		summary.Spec.VPC = vpc.Spec
		summary.Spec.VLAN = vpc.Status.VLAN
		summary.Spec.Connections = connNames
		summary.Labels = summaryLabels

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
	err := r.List(ctx, vpcs)
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
