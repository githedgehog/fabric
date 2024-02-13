package vpcattachment

import (
	"context"

	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type VPCAttachmentWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	w := &VPCAttachmentWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.VPCAttachment{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*VPCAttachmentWebhook)(nil)
	_ admission.CustomValidator = (*VPCAttachmentWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-vpcattachment,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcattachments,verbs=create;update,versions=v1alpha2,name=mvpcattachment.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpcattachment,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcattachments,verbs=create;update;delete,versions=v1alpha2,name=vvpcattachment.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpcattachment-webhook")

func (w *VPCAttachmentWebhook) Default(ctx context.Context, obj runtime.Object) error {
	attach := obj.(*vpcapi.VPCAttachment)

	attach.Default()

	return nil
}

func (w *VPCAttachmentWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	attach := obj.(*vpcapi.VPCAttachment)

	warns, err := attach.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCAttachmentWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	newAttach := newObj.(*vpcapi.VPCAttachment)
	// oldAttach := oldObj.(*vpcapi.VPCAttachment)

	// if !equality.Semantic.DeepEqual(oldAttach.Spec, newAttach.Spec) {
	// 	return nil, errors.Errorf("vpc attachment is immutable")
	// }

	warns, err := newAttach.Validate(ctx, w.Validation)
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCAttachmentWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}
