package ipv4ns

import (
	"context"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type IPv4NamespaceWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
	Cfg        *config.Fabric
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	w := &IPv4NamespaceWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
		Cfg:        cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&vpcapi.IPv4Namespace{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*IPv4NamespaceWebhook)(nil)
	_ admission.CustomValidator = (*IPv4NamespaceWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-vpc-githedgehog-com-v1alpha2-ipv4namespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=create;update,versions=v1alpha2,name=mipv4namespace.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-vpc-githedgehog-com-v1alpha2-ipv4namespace,mutating=false,failurePolicy=fail,sideEffects=None,groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=create;update,versions=v1alpha2,name=vipv4namespace.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("ipv4namespace-webhook")

func (w *IPv4NamespaceWebhook) Default(ctx context.Context, obj runtime.Object) error {
	ns := obj.(*vpcapi.IPv4Namespace)

	ns.Default()

	return nil
}

func (w *IPv4NamespaceWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	ns := obj.(*vpcapi.IPv4Namespace)

	warns, err := ns.Validate(ctx, w.Validation, w.Cfg.ParsedReservedSubnets())
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *IPv4NamespaceWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	oldNs := oldObj.(*vpcapi.IPv4Namespace)
	newNs := newObj.(*vpcapi.IPv4Namespace)

	if !equality.Semantic.DeepEqual(oldNs.Spec, newNs.Spec) {
		return nil, errors.Errorf("IPv4Namespace spec is immutable")
	}

	return nil, nil
}

func (w *IPv4NamespaceWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO prevent deleting IPv4Namespace that are in use

	// ns := obj.(*vpcapi.IPv4Namespace)

	return nil, nil
}
