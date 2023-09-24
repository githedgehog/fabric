package vpc

import (
	"context"

	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VPCWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	w := &VPCWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.VPC{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*VPCWebhook)(nil)
	_ admission.CustomValidator = (*VPCWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-vpc,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update,versions=v1alpha2,name=mvpc.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpc,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update,versions=v1alpha2,name=vvpc.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpc-webhook")

func (w *VPCWebhook) Default(ctx context.Context, obj runtime.Object) error {
	vpc := obj.(*vpcapi.VPC)

	vpc.Default()

	return nil
}

func (w *VPCWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	vpc := obj.(*vpcapi.VPC)

	warns, err := vpc.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newVPC := newObj.(*vpcapi.VPC)

	warns, err := newVPC.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
