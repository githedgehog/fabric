package switchh

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type SwitchWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	KubeClient client.Reader
	Cfg        *meta.FabricConfig
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *meta.FabricConfig) error {
	w := &SwitchWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
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
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-switch,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=switches,verbs=create;update;delete,versions=v1alpha2,name=vswitch.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("switch-webhook")

func (w *SwitchWebhook) Default(ctx context.Context, obj runtime.Object) error {
	sw := obj.(*wiringapi.Switch)

	sw.Default()

	return nil
}

func (w *SwitchWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	sw := obj.(*wiringapi.Switch)

	warns, err := sw.Validate(ctx, w.KubeClient, w.Cfg)
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

	warns, err := newSw.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, err
	}

	return nil, nil
}

func (w *SwitchWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	sw := obj.(*wiringapi.Switch)

	conns := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, conns, client.MatchingLabels{
		wiringapi.ListLabelSwitch(sw.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(conns.Items) > 0 {
		return nil, errors.Errorf("switch has connections")
	}

	return nil, nil
}
