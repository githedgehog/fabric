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
	"fmt"

	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type RemotePeeringWebhook struct {
	kclient.Client
	Scheme     *runtime.Scheme
	KubeClient kclient.Reader
	Cfg        *meta.FabricConfig
}

func SetupRemotePeeringWebhookWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	w := &RemotePeeringWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	if err := kctrl.NewWebhookManagedBy(mgr, &vpcapi.RemotePeering{}).
		WithDefaulter(w).
		WithValidator(newLockedValidator[*vpcapi.RemotePeering](w, lock)).
		Complete(); err != nil {
		return fmt.Errorf("failed to setup remote peering webhook: %w", err)
	}

	return nil
}

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1beta1-remotepeering,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=remotepeerings,verbs=create;update,versions=v1beta1,name=mremotepeering.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1beta1-remotepeering,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=remotepeerings,verbs=create;update;delete,versions=v1beta1,name=vremotepeering.kb.io,admissionReviewVersions=v1

func (w *RemotePeeringWebhook) Default(_ context.Context, rp *vpcapi.RemotePeering) error {
	rp.Default()

	return nil
}

func (w *RemotePeeringWebhook) ValidateCreate(ctx context.Context, rp *vpcapi.RemotePeering) (admission.Warnings, error) {
	warns, err := rp.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate remote peering: %w", err)
	}

	return warns, nil
}

func (w *RemotePeeringWebhook) ValidateUpdate(ctx context.Context, oldRP *vpcapi.RemotePeering, newRP *vpcapi.RemotePeering) (admission.Warnings, error) {
	if fabricChanged(oldRP.Spec.Topology.Fabric, newRP.Spec.Topology.Fabric) {
		return nil, fmt.Errorf("topology.fabric is immutable") //nolint:err113
	}
	if domainChanged(oldRP.Spec.Topology.Domain, newRP.Spec.Topology.Domain) {
		return nil, fmt.Errorf("topology.domain is immutable") //nolint:err113
	}

	warns, err := newRP.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, fmt.Errorf("failed to validate remote peering: %w", err)
	}

	return warns, nil
}

func (w *RemotePeeringWebhook) ValidateDelete(_ context.Context, _ *vpcapi.RemotePeering) (admission.Warnings, error) {
	return nil, nil
}
