package vlanns

import (
	"context"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VLANNamespaceWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
	Cfg        *config.Fabric
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	w := &VLANNamespaceWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
		Cfg:        cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.VLANNamespace{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*VLANNamespaceWebhook)(nil)
	_ admission.CustomValidator = (*VLANNamespaceWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1alpha2-vlannamespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=create;update,versions=v1alpha2,name=mvlannamespace.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-vlannamespace,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=create;update;delete,versions=v1alpha2,name=vvlannamespace.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vlannamespace-webhook")

func (w *VLANNamespaceWebhook) Default(ctx context.Context, obj runtime.Object) error {
	ns := obj.(*wiringapi.VLANNamespace)

	ns.Default()

	return nil
}

func (w *VLANNamespaceWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	ns := obj.(*wiringapi.VLANNamespace)

	warns, err := ns.Validate(ctx, w.Validation, w.Cfg.VPCIRBVLANRanges)
	if err != nil {
		return warns, err
	}

	return nil, nil
}

func (w *VLANNamespaceWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	oldNs := oldObj.(*wiringapi.VLANNamespace)
	newNs := newObj.(*wiringapi.VLANNamespace)

	if !equality.Semantic.DeepEqual(oldNs.Spec, newNs.Spec) {
		return nil, errors.Errorf("VLANNamespace spec is immutable")
	}

	return nil, nil
}

func (w *VLANNamespaceWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	ns := obj.(*wiringapi.VLANNamespace)

	switches := &wiringapi.SwitchList{}
	if err := w.Client.List(ctx, switches, client.MatchingLabels{
		wiringapi.ListLabelVLANNamespace(ns.Name): wiringapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing switches") // TODO hide internal error
	}
	if len(switches.Items) > 0 {
		return nil, errors.Errorf("VLANNamespace has switches")
	}

	vpcs := &vpcapi.VPCList{}
	if err := w.Client.List(ctx, vpcs, client.MatchingLabels{
		vpcapi.LabelVLANNS: ns.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpcs") // TODO hide internal error
	}
	if len(vpcs.Items) > 0 {
		return nil, errors.Errorf("VLANNamespace has VPCs")
	}

	return nil, nil
}
