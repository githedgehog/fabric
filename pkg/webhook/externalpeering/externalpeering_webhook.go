package externalpeering

import (
	"context"

	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ExternalPeeringWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
	Cfg        *config.Fabric
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	w := &ExternalPeeringWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
		Cfg:        cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.ExternalPeering{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*ExternalPeeringWebhook)(nil)
	_ admission.CustomValidator = (*ExternalPeeringWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-externalpeering,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externalpeerings,verbs=create;update,versions=v1alpha2,name=mexternalpeering.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-externalpeering,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externalpeerings,verbs=create;update;delete,versions=v1alpha2,name=vexternalpeering.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("externalpeering-webhook")

func (w *ExternalPeeringWebhook) Default(ctx context.Context, obj runtime.Object) error {
	peering := obj.(*vpcapi.ExternalPeering)

	peering.Default()

	return nil
}

func (w *ExternalPeeringWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	peering := obj.(*vpcapi.ExternalPeering)

	warns, err := peering.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ExternalPeeringWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newPeering := newObj.(*vpcapi.ExternalPeering)
	// oldPeering := oldObj.(*vpcapi.ExternalPeering)

	// if !equality.Semantic.DeepEqual(oldPeering.Spec, newPeering.Spec) {
	// 	return nil, errors.Errorf("external peering spec is immutable")
	// }

	warns, err := newPeering.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ExternalPeeringWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
