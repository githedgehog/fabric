package nat

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

type NATWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	w := &NATWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.NAT{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*NATWebhook)(nil)
	_ admission.CustomValidator = (*NATWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-nat-githedgehog-com-v1alpha2-nat,mutating=true,failurePolicy=fail,sideEffects=None,groups=nat.githedgehog.com,resources=nats,verbs=create;update,versions=v1alpha2,name=mnat.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-nat-githedgehog-com-v1alpha2-nat,mutating=false,failurePolicy=fail,sideEffects=None,groups=nat.githedgehog.com,resources=nats,verbs=create;update,versions=v1alpha2,name=vnat.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("nat-webhook")

func (w *NATWebhook) Default(ctx context.Context, obj runtime.Object) error {
	nat := obj.(*vpcapi.NAT)

	nat.Default()

	return nil
}

func (w *NATWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	nat := obj.(*vpcapi.NAT)

	warns, err := nat.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *NATWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newNAT := newObj.(*vpcapi.NAT)
	oldNAT := oldObj.(*vpcapi.NAT)

	if !equality.Semantic.DeepEqual(oldNAT.Spec.Subnet, newNAT.Spec.Subnet) {
		return nil, errors.Errorf("nat subnet is immutable")
	}

	warns, err := newNAT.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *NATWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
