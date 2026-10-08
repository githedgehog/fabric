// Copyright 2026 Hedgehog
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
	"encoding/json"
	"fmt"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VPCInterconnectWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupVPCInterconnectWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &VPCInterconnectWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &vpcapi.VPCInterconnect{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*vpcapi.VPCInterconnect](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup VPC interconnect webhook: %w", err)
	}

	return nil
}

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-vpcinterconnect,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcinterconnects,verbs=create;update,versions=v1beta1,name=mvpcinterconnect.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-vpcinterconnect,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcinterconnects,verbs=create;update;delete,versions=v1beta1,name=vvpcinterconnect.kb.io,admissionReviewVersions=v1

func (w *VPCInterconnectWebhook) Default(_ context.Context, ic *vpcapi.VPCInterconnect) error {
	ic.Default()

	return nil
}

func (w *VPCInterconnectWebhook) ValidateCreate(ctx context.Context, ic *vpcapi.VPCInterconnect) (admission.Warnings, error) {
	warns, err := ic.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate VPC interconnect: %w", err)
	}

	return warns, nil
}

func (w *VPCInterconnectWebhook) ValidateUpdate(ctx context.Context, oldIC *vpcapi.VPCInterconnect, newIC *vpcapi.VPCInterconnect) (admission.Warnings, error) {
	if fabricChanged(oldIC.Spec.Topology.Fabric, newIC.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}
	if domainChanged(oldIC.Spec.Topology.Domain, newIC.Spec.Topology.Domain) {
		return nil, fmt.Errorf("topology.domain is immutable") //nolint:err113
	}

	warns, err := newIC.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate VPC interconnect: %w", err)
	}

	return warns, nil
}

func (w *VPCInterconnectWebhook) ValidateDelete(ctx context.Context, ic *vpcapi.VPCInterconnect) (admission.Warnings, error) {
	// the webhooks refuse the garbage collector's removal of the owner references, which would leave the delete hanging
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting admission request: %w", err)
	}
	opts := &kmetav1.DeleteOptions{}
	if len(req.Options.Raw) > 0 {
		if err := json.Unmarshal(req.Options.Raw, opts); err != nil {
			return nil, fmt.Errorf("parsing delete options: %w", err)
		}
	}
	if opts.PropagationPolicy != nil && *opts.PropagationPolicy == kmetav1.DeletePropagationOrphan ||
		opts.OrphanDependents != nil && *opts.OrphanDependents { //nolint:staticcheck // still honored by the API server
		return nil, fmt.Errorf("VPC interconnect can't be deleted with orphan propagation, its generated objects go with it") //nolint:err113
	}

	// the gateway reaches it through the External generated with its name
	gwPeerings := &gwapi.GatewayPeeringList{}
	if err := w.Client.List(ctx, gwPeerings, kclient.MatchingLabels{
		gwapi.ListLabelVPC(vpcapi.VPCInfoExtPrefix + ic.Name): gwapi.ListLabelValue,
	}); err != nil {
		return nil, fmt.Errorf("error listing gateway peerings: %w", err) // TODO hide internal error
	}
	if len(gwPeerings.Items) > 0 {
		return nil, fmt.Errorf("VPC interconnect is used by gateway peering %s", gwPeerings.Items[0].Name) //nolint:err113
	}

	return nil, nil
}
