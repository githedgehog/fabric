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
	"reflect"
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

type SwitchWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupSwitchWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	w := &SwitchWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return errors.Wrapf(kctrl.NewWebhookManagedBy(mgr, &wiringapi.Switch{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete(), "failed to setup switch webhook")
}

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1beta1-switch,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update,versions=v1beta1,name=mswitch.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1beta1-switch,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update;delete,versions=v1beta1,name=vswitch.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switch-webhook")

func (w *SwitchWebhook) Default(_ context.Context, sw *wiringapi.Switch) error {
	sw.Default()

	return nil
}

func (w *SwitchWebhook) ValidateCreate(ctx context.Context, sw *wiringapi.Switch) (admission.Warnings, error) {
	warns, err := sw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating switch")
	}

	return warns, nil
}

func (w *SwitchWebhook) ValidateUpdate(ctx context.Context, oldSw *wiringapi.Switch, newSw *wiringapi.Switch) (admission.Warnings, error) {
	if fabricChanged(oldSw.Spec.Topology.Fabric, newSw.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}
	// SONiC refuses to change the local AS of a running BGP instance, so the agent would fail on
	// every attempt, and peers would keep the old ASN as nothing regenerates their config
	if oldSw.Spec.ASN != newSw.Spec.ASN {
		return nil, fmt.Errorf("asn is immutable, delete and recreate the switch to change it") //nolint:err113
	}
	// a spine's ASN is its domain's spine ASN, so its domain can't change either
	if newSw.Spec.Role.IsSpine() && !slices.Equal(wiringapi.DomainsOrDefault(oldSw.Spec.Topology.Domains), wiringapi.DomainsOrDefault(newSw.Spec.Topology.Domains)) {
		return nil, fmt.Errorf("the domain of a spine is immutable, delete and recreate the switch to change it") //nolint:err113
	}

	warns, err := newSw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, errors.Wrapf(err, "error validating switch")
	}

	if (oldSw.Spec.RoCE || newSw.Spec.RoCE) && !reflect.DeepEqual(oldSw.Spec.PortBreakouts, newSw.Spec.PortBreakouts) {
		return warns, errors.New("port breakouts cannot be changed when RoCEv2 is enabled")
	}

	oldDomains := slices.Sorted(slices.Values(wiringapi.DomainsOrDefault(oldSw.Spec.Topology.Domains)))
	newDomains := slices.Sorted(slices.Values(wiringapi.DomainsOrDefault(newSw.Spec.Topology.Domains)))
	if !slices.Equal(oldDomains, newDomains) {
		if err := w.validateDomainChange(ctx, newSw); err != nil {
			return warns, fmt.Errorf("can not change domains to %v: %w", newDomains, err)
		}
	}

	return warns, nil
}

// validateDomainChange re-runs the domain checks of everything cabled to or attached through the
// switch, as they were run against its old domains. It's located in a webhook to avoid circular
// dependency with vpcapi and gwapi
func (w *SwitchWebhook) validateDomainChange(ctx context.Context, sw *wiringapi.Switch) error {
	swDomains := wiringapi.DomainsOrDefault(sw.Spec.Topology.Domains)

	conns := &wiringapi.ConnectionList{}
	if err := w.KubeClient.List(ctx, conns, kclient.InNamespace(sw.Namespace), kclient.MatchingLabels{
		wiringapi.ListLabelSwitch(sw.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return fmt.Errorf("failed to list connections: %w", err) // TODO replace with some internal error to not expose to the user
	}
	swConns := []*wiringapi.Connection{}
	for i := range conns.Items {
		switchNames, _, _, _, err := conns.Items[i].Spec.Endpoints()
		if err == nil && slices.Contains(switchNames, sw.Name) {
			swConns = append(swConns, &conns.Items[i])
		}
	}

	connNames := make([]string, 0, len(swConns))
	for _, conn := range swConns {
		connNames = append(connNames, conn.Name)
	}
	peers, err := vpcapi.ConnectionSwitches(ctx, w.KubeClient, sw.Namespace, connNames)
	if err != nil {
		return err //nolint:wrapcheck
	}
	peers[sw.Name] = sw

	vpcNames := map[string]bool{}
	extNames := map[string]bool{}
	for _, conn := range swConns {
		if err := conn.Spec.ValidateDomains(peers); err != nil {
			return fmt.Errorf("connection %s: %w", conn.Name, err)
		}

		if conn.Spec.Gateway != nil {
			for _, link := range conn.Spec.Gateway.Links {
				gw := &gwapi.Gateway{}
				err := w.KubeClient.Get(ctx, ktypes.NamespacedName{Name: link.Gateway.DeviceName(), Namespace: sw.Namespace}, gw)
				if kapierrors.IsNotFound(err) {
					continue
				}
				if err != nil {
					return fmt.Errorf("failed to get gateway %s: %w", link.Gateway.DeviceName(), err) // TODO replace with some internal error to not expose to the user
				}
				if gwDomain := wiringapi.DomainNameOrDefault(gw.Spec.Topology.Domain); !slices.Equal(swDomains, []string{gwDomain}) {
					return fmt.Errorf("connection %s cables it to gateway %s in domain %s", conn.Name, gw.Name, gwDomain) //nolint:err113
				}
			}
		}

		if conn.Spec.StaticExternal != nil && conn.Spec.StaticExternal.WithinVPC != "" {
			vpcNames[conn.Spec.StaticExternal.WithinVPC] = true
		}

		vpcAttaches := &vpcapi.VPCAttachmentList{}
		if err := w.KubeClient.List(ctx, vpcAttaches, kclient.InNamespace(sw.Namespace), kclient.MatchingLabels{wiringapi.LabelConnection: conn.Name}); err != nil {
			return fmt.Errorf("failed to list vpc attachments: %w", err) // TODO replace with some internal error to not expose to the user
		}
		for _, attach := range vpcAttaches.Items {
			if attach.Spec.Connection == conn.Name {
				vpcNames[attach.Spec.VPCName()] = true
			}
		}

		extAttaches := &vpcapi.ExternalAttachmentList{}
		if err := w.KubeClient.List(ctx, extAttaches, kclient.InNamespace(sw.Namespace), kclient.MatchingLabels{wiringapi.LabelConnection: conn.Name}); err != nil {
			return fmt.Errorf("failed to list external attachments: %w", err) // TODO replace with some internal error to not expose to the user
		}
		for _, attach := range extAttaches.Items {
			if attach.Spec.Connection == conn.Name {
				extNames[attach.Spec.External] = true
			}
		}
	}

	for vpcName := range vpcNames {
		vpc := &vpcapi.VPC{}
		err := w.KubeClient.Get(ctx, ktypes.NamespacedName{Name: vpcName, Namespace: sw.Namespace}, vpc)
		if kapierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get vpc %s: %w", vpcName, err) // TODO replace with some internal error to not expose to the user
		}
		if vpcDomains := wiringapi.DomainsOrDefault(vpc.Spec.Topology.Domains); slices.ContainsFunc(vpcDomains, func(domain string) bool { return !slices.Contains(swDomains, domain) }) {
			return fmt.Errorf("it is attached to vpc %s in domains %v", vpcName, vpcDomains) //nolint:err113
		}
	}

	for extName := range extNames {
		ext := &vpcapi.External{}
		err := w.KubeClient.Get(ctx, ktypes.NamespacedName{Name: extName, Namespace: sw.Namespace}, ext)
		if kapierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to get external %s: %w", extName, err) // TODO replace with some internal error to not expose to the user
		}
		if extDomain := wiringapi.DomainNameOrDefault(ext.Spec.Topology.Domain); !slices.Contains(swDomains, extDomain) {
			return fmt.Errorf("it is attached to external %s in domain %s", extName, extDomain) //nolint:err113
		}
	}

	return nil
}

func (w *SwitchWebhook) ValidateDelete(ctx context.Context, sw *wiringapi.Switch) (admission.Warnings, error) {
	conns := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, conns, kclient.MatchingLabels{
		wiringapi.ListLabelSwitch(sw.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(conns.Items) > 0 {
		return nil, errors.Errorf("switch has connections")
	}

	return nil, nil
}
