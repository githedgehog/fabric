package external

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ExternalWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	KubeClient client.Reader
	Cfg        *meta.FabricConfig
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *meta.FabricConfig) error {
	w := &ExternalWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		KubeClient: mgr.GetClient(),
		Cfg:        cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.External{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*ExternalWebhook)(nil)
	_ admission.CustomValidator = (*ExternalWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-external,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externals,verbs=create;update,versions=v1alpha2,name=mexternal.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-external,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=externals,verbs=create;update;delete,versions=v1alpha2,name=vexternal.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("external-webhook")

func (w *ExternalWebhook) Default(ctx context.Context, obj runtime.Object) error {
	external := obj.(*vpcapi.External)

	external.Default()

	return nil
}

func (w *ExternalWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	external := obj.(*vpcapi.External)

	warns, err := external.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ExternalWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newExternal := newObj.(*vpcapi.External)
	// oldExternal := oldObj.(*vpcapi.External)

	// if !equality.Semantic.DeepEqual(oldExternal.Spec, newExternal.Spec) {
	// 	return nil, errors.Errorf("external spec is immutable")
	// }

	warns, err := newExternal.Validate(ctx, w.KubeClient, w.Cfg)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ExternalWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	ext := obj.(*vpcapi.External)

	extAttachments := &vpcapi.ExternalAttachmentList{}
	if err := w.Client.List(ctx, extAttachments, client.MatchingLabels{
		vpcapi.LabelExternal: ext.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external attachments") // TODO hide internal error
	}
	if len(extAttachments.Items) > 0 {
		return nil, errors.Errorf("external has attachments")
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := w.Client.List(ctx, extPeerings, client.MatchingLabels{
		vpcapi.LabelExternal: ext.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external peerings") // TODO hide internal error
	}
	if len(extPeerings.Items) > 0 {
		return nil, errors.Errorf("external has peerings")
	}

	return nil, nil
}
