package switchh

import (
	"context"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type SwitchWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	w := &SwitchWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.Switch{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*SwitchWebhook)(nil)
	_ admission.CustomValidator = (*SwitchWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1alpha2-switch,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update,versions=v1alpha2,name=mswitch.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-switch,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update,versions=v1alpha2,name=vswitch.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switch-webhook")

func (w *SwitchWebhook) Default(ctx context.Context, obj runtime.Object) error {
	sw := obj.(*wiringapi.Switch)

	sw.Default()

	return nil
}

func (w *SwitchWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	sw := obj.(*wiringapi.Switch)

	warns, err := sw.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *SwitchWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	oldSw := oldObj.(*wiringapi.Switch)
	newSw := newObj.(*wiringapi.Switch)

	if !equality.Semantic.DeepEqual(oldSw.Spec.Location, newSw.Spec.Location) {
		return nil, errors.Errorf("switch location is immutable")
	}

	warns, err := newSw.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return nil, nil
}

func (w *SwitchWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO prevent deleting switches that are in use

	return nil, nil
}
