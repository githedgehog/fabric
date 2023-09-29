package vpcpeering

import (
	"context"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VPCPeeringWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	w := &VPCPeeringWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.VPCPeering{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*VPCPeeringWebhook)(nil)
	_ admission.CustomValidator = (*VPCPeeringWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-vpcpeering,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcpeerings,verbs=create;update,versions=v1alpha2,name=mvpcpeering.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpcpeering,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcpeerings,verbs=create;update,versions=v1alpha2,name=vvpcpeering.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpcpeering-webhook")

func (w *VPCPeeringWebhook) Default(ctx context.Context, obj runtime.Object) error {
	peering := obj.(*vpcapi.VPCPeering)

	peering.Default()

	return nil
}

func (w *VPCPeeringWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	peering := obj.(*vpcapi.VPCPeering)

	warns, err := peering.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCPeeringWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newPeering := newObj.(*vpcapi.VPCPeering)
	oldPeering := oldObj.(*vpcapi.VPCPeering)

	if !equality.Semantic.DeepEqual(oldPeering.Spec, newPeering.Spec) {
		return nil, errors.Errorf("vpc peering is immutable")
	}

	warns, err := newPeering.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCPeeringWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
