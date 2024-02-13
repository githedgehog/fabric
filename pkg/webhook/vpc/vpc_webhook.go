package vpc

import (
	"context"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
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
	Cfg        *config.Fabric
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	w := &VPCWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
		Cfg:        cfg,
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
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-vpc,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=vpcs,verbs=create;update;delete,versions=v1alpha2,name=vvpc.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("vpc-webhook")

func (w *VPCWebhook) Default(ctx context.Context, obj runtime.Object) error {
	vpc := obj.(*vpcapi.VPC)

	vpc.Default()

	return nil
}

func (w *VPCWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	vpc := obj.(*vpcapi.VPC)

	warns, err := vpc.Validate(ctx, w.Validation, w.Cfg.ParsedReservedSubnets(), w.Cfg.DHCPMode.IsMultiNSDHCP())
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *VPCWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	// oldVPC := oldObj.(*vpcapi.VPC)
	newVPC := newObj.(*vpcapi.VPC)

	warns, err := newVPC.Validate(ctx, w.Validation, w.Cfg.ParsedReservedSubnets(), w.Cfg.DHCPMode.IsMultiNSDHCP())
	if err != nil {
		return warns, err
	}

	// TODO check that you can only add subnets, or edit/remove unused ones

	// for subnetName, oldSubnet := range oldVPC.Spec.Subnets {
	// 	newSubnet, ok := newVPC.Spec.Subnets[subnetName]
	// 	if !ok {
	// 		continue
	// 	}

	// 	if !equality.Semantic.DeepEqual(oldSubnet, newSubnet) {
	// 		return nil, errors.Errorf("subnets are immutable, but %s changed", subnetName)
	// 	}
	// }

	return nil, nil
}

func (w *VPCWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	vpc := obj.(*vpcapi.VPC)

	vpcAttachments := &vpcapi.VPCAttachmentList{}
	if err := w.Client.List(ctx, vpcAttachments, client.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc attachments") // TODO hide internal error
	}
	if len(vpcAttachments.Items) > 0 {
		return nil, errors.Errorf("VPC has attachments")
	}

	vpcPeerings := &vpcapi.VPCPeeringList{}
	if err := w.Client.List(ctx, vpcPeerings, client.MatchingLabels{
		vpcapi.ListLabelVPC(vpc.Name): vpcapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing vpc peerings") // TODO hide internal error
	}
	if len(vpcPeerings.Items) > 0 {
		return nil, errors.Errorf("VPC has peerings")
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := w.Client.List(ctx, extPeerings, client.MatchingLabels{
		vpcapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing external peerings") // TODO hide internal error
	}
	if len(extPeerings.Items) > 0 {
		return nil, errors.Errorf("VPC has external peerings")
	}

	staticExts := &wiringapi.ConnectionList{}
	if err := w.Client.List(ctx, staticExts, client.MatchingLabels{
		wiringapi.LabelVPC: vpc.Name,
	}); err != nil {
		return nil, errors.Wrapf(err, "error listing connections") // TODO hide internal error
	}
	if len(staticExts.Items) > 0 {
		return nil, errors.Errorf("VPC has static external connections (using withingVPC option)")
	}

	return nil, nil
}
