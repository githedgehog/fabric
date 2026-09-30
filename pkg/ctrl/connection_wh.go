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
	"slices"

	"github.com/pkg/errors"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ConnectionWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupConnectionWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	w := &ConnectionWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr, &wiringapi.Connection{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup connection webhook")
}

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-connection,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update,versions=v1beta1,name=mconnection.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-connection,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update;delete,versions=v1beta1,name=vconnection.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("connection-webhook")

func (w *ConnectionWebhook) Default(_ context.Context, conn *wiringapi.Connection) error {
	conn.Default()

	return nil
}

// validateStaticExternal checks that the static external connection is valid and it's located in a webhook to avoid circular dependency with vpcapi
func (w *ConnectionWebhook) validateStaticExternal(ctx context.Context, kube kclient.Reader, conn *wiringapi.Connection) error {
	if conn.Spec.StaticExternal != nil && conn.Spec.StaticExternal.WithinVPC != "" {
		vpc := &vpcapi.VPC{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: conn.Spec.StaticExternal.WithinVPC, Namespace: conn.Namespace}, vpc) // TODO namespace could be different?
		if kapierrors.IsNotFound(err) {
			return errors.Errorf("vpc %s not found", conn.Spec.StaticExternal.WithinVPC)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to get vpc %s", conn.Spec.StaticExternal.WithinVPC) // TODO replace with some internal error to not expose to the user
		}

		connFabric := wiringapi.FabricNameOrDefault(conn.Spec.Topology.Fabric)
		if vpcFabric := wiringapi.FabricNameOrDefault(vpc.Spec.Topology.Fabric); vpcFabric != connFabric {
			return fmt.Errorf("connection is in fabric %s but vpc %s is in fabric %s", connFabric, vpc.Name, vpcFabric) //nolint:err113
		}

		// the VPC's VRF is built on the switch
		swName := conn.Spec.StaticExternal.Link.Switch.DeviceName()
		sw := &wiringapi.Switch{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: swName, Namespace: conn.Namespace}, sw); err != nil {
			return fmt.Errorf("failed to get switch %s: %w", swName, err) // TODO replace with some internal error to not expose to the user
		}
		vpcDomains, swDomains := wiringapi.DomainsOrDefault(vpc.Spec.Topology.Domains), wiringapi.DomainsOrDefault(sw.Spec.Topology.Domains)
		if slices.ContainsFunc(vpcDomains, func(domain string) bool { return !slices.Contains(swDomains, domain) }) {
			return fmt.Errorf("vpc %s is in domains %v but switch %s is in domains %v", vpc.Name, vpcDomains, swName, swDomains) //nolint:err113
		}
	}

	return nil
}

// validateGateway checks that the gateways of a gateway connection exist and are in its fabric.
// It's located in a webhook to avoid circular dependency with gwapi
func (w *ConnectionWebhook) validateGateway(ctx context.Context, kube kclient.Reader, conn *wiringapi.Connection) error {
	if conn.Spec.Gateway == nil {
		return nil
	}

	connFabric := wiringapi.FabricNameOrDefault(conn.Spec.Topology.Fabric)
	for _, link := range conn.Spec.Gateway.Links {
		gw := &gwapi.Gateway{}
		err := kube.Get(ctx, ktypes.NamespacedName{Name: link.Gateway.DeviceName(), Namespace: conn.Namespace}, gw)
		// a connection admitted before its gateway would never have its fabric checked
		if kapierrors.IsNotFound(err) {
			return fmt.Errorf("gateway %s not found", link.Gateway.DeviceName()) //nolint:err113
		}
		if err != nil {
			return fmt.Errorf("failed to get gateway %s: %w", link.Gateway.DeviceName(), err) // TODO replace with some internal error to not expose to the user
		}

		if gwFabric := wiringapi.FabricNameOrDefault(gw.Spec.Topology.Fabric); gwFabric != connFabric {
			return fmt.Errorf("connection is in fabric %s but gateway %s is in fabric %s", connFabric, gw.Name, gwFabric) //nolint:err113
		}

		sw := &wiringapi.Switch{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: link.Switch.DeviceName(), Namespace: conn.Namespace}, sw); err != nil {
			return fmt.Errorf("failed to get switch %s: %w", link.Switch.DeviceName(), err) // TODO replace with some internal error to not expose to the user
		}
		gwDomain := wiringapi.DomainNameOrDefault(gw.Spec.Topology.Domain)
		if swDomains := wiringapi.DomainsOrDefault(sw.Spec.Topology.Domains); !slices.Equal(swDomains, []string{gwDomain}) {
			return fmt.Errorf("gateway %s is in domain %s but switch %s is in domains %v", gw.Name, gwDomain, sw.Name, swDomains) //nolint:err113
		}
	}

	return nil
}

func (w *ConnectionWebhook) ValidateCreate(ctx context.Context, conn *wiringapi.Connection) (admission.Warnings, error) {
	warns, err := conn.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating connection")
	}

	if err := w.validateStaticExternal(ctx, w.KubeClient, conn); err != nil {
		return warns, err
	}

	return warns, w.validateGateway(ctx, w.KubeClient, conn)
}

func (w *ConnectionWebhook) ValidateUpdate(ctx context.Context, oldConn *wiringapi.Connection, newConn *wiringapi.Connection) (admission.Warnings, error) {
	if fabricChanged(oldConn.Spec.Topology.Fabric, newConn.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}

	// TODO some connections or their parts should be immutable

	if oldConn.Spec.Type() != newConn.Spec.Type() {
		return nil, errors.Errorf("connection type is immutable")
	}

	if newConn.Spec.StaticExternal != nil && oldConn.Spec.StaticExternal != nil {
		if newConn.Spec.StaticExternal.WithinVPC != oldConn.Spec.StaticExternal.WithinVPC {
			return nil, errors.Errorf("StaticExternal.WithinVPC is immutable")
		}
	}

	warns, err := newConn.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating connection")
	}

	// if newConn.Spec.Unbundled != nil || newConn.Spec.Bundled != nil || newConn.Spec.MCLAG != nil || newConn.Spec.ESLAG != nil {
	// 	if !equality.Semantic.DeepEqual(oldConn.Spec, newConn.Spec) {
	// 		return nil, errors.Errorf("server-facing Connection spec is immutable")
	// 	}
	// }

	if err := w.validateStaticExternal(ctx, w.KubeClient, newConn); err != nil {
		return warns, err
	}

	return warns, w.validateGateway(ctx, w.KubeClient, newConn)
}

func (w *ConnectionWebhook) ValidateDelete(ctx context.Context, conn *wiringapi.Connection) (admission.Warnings, error) {
	vpcAttachments := &vpcapi.VPCAttachmentList{}
	if err := w.Client.List(ctx, vpcAttachments, kclient.MatchingLabels{
		wiringapi.LabelConnection: conn.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc attachments") // TODO hide internal error
	}
	if len(vpcAttachments.Items) > 0 {
		return nil, errors.Errorf("connection has attachments")
	}

	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := w.Client.List(ctx, extAttachments, kclient.MatchingLabels{
		wiringapi.LabelConnection: conn.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external attachments") // TODO hide internal error
	}
	if len(extAttachments.Items) > 0 {
		return nil, errors.Errorf("connection has external attachments")
	}

	return nil, nil
}
