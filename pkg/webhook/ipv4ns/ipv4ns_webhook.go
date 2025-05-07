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

package ipv4ns

import (
	"context"
	"net"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Webhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupWithManager(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	w := &Webhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.IPv4Namespace{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup ipv4namespace webhook")
}

var (
	_ admission.CustomDefaulter = (*Webhook)(nil)
	_ admission.CustomValidator = (*Webhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-ipv4namespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=create;update,versions=v1beta1,name=mipv4namespace.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-ipv4namespace,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=create;update;delete,versions=v1beta1,name=vipv4namespace.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("ipv4namespace-webhook")

func (w *Webhook) Default(_ context.Context, obj runtime.Object) error {
	ns := obj.(*vpcapi.IPv4Namespace)

	ns.Default()

	return nil
}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ns := obj.(*vpcapi.IPv4Namespace)

	warns, err := ns.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "failed to validate ipv4namespace")
	}

	return warns, nil
}

func (w *Webhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	// oldNs := oldObj.(*vpcapi.IPv4Namespace)
	ns := newObj.(*vpcapi.IPv4Namespace)

	if warn, err := ns.Validate(ctx, w.Client, w.Cfg); err != nil {
		return warn, errors.Wrapf(err, "failed to validate ipv4namespace")
	}

	nsSubnets := []*net.IPNet{}
	for _, subnet := range ns.Spec.Subnets {
		_, ipNet, err := net.ParseCIDR(subnet)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse cidr %s", subnet)
		}

		nsSubnets = append(nsSubnets, ipNet)
	}

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: ns.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpcs") // TODO hide internal error
	}

	for _, vpc := range vpcs.Items {
		for subnetName, subnetCfg := range vpc.Spec.Subnets {
			_, vpcSubnet, err := net.ParseCIDR(subnetCfg.Subnet)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse vpc subnet %s", subnetCfg.Subnet)
			}

			ok := false
			for _, nsSubnet := range nsSubnets {
				if nsSubnet.Contains(vpcSubnet.IP) {
					ok = true

					break
				}
			}

			if !ok {
				return nil, errors.Errorf("existing vpc %s subnet %s (cidr %s) doesn't fit updated IPv4 namespace", vpc.Name, subnetName, subnetCfg.Subnet)
			}
		}
	}

	return nil, nil
}

func (w *Webhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ipns := obj.(*vpcapi.IPv4Namespace)

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: ipns.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpcs") // TODO hide internal error
	}
	if len(vpcs.Items) > 0 {
		return nil, errors.Errorf("IPv4Namespace has VPCs")
	}

	externals := &vpcapi.ExternalList{}
	if err := w.Client.List(ctx, externals, kclient.MatchingLabels{
		vpcapi.LabelIPv4NS: ipns.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing externals") // TODO hide internal error
	}
	if len(externals.Items) > 0 {
		return nil, errors.Errorf("IPv4Namespace has externals")
	}

	return nil, nil
}
