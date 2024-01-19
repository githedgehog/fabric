package connection

import (
	"context"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ConnectionWebhook struct {
	client.Client
	Scheme     *runtime.Scheme
	Validation validation.Client
	Cfg        *config.Fabric
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	w := &ConnectionWebhook{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Validation: validation.WithCtrlRuntime(mgr.GetClient()),
		Cfg:        cfg,
	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(&wiringapi.Connection{}).
		WithDefaulter(w).
		WithValidator(w).
		Complete()
}

var (
	_ admission.CustomDefaulter = (*ConnectionWebhook)(nil)
	_ admission.CustomValidator = (*ConnectionWebhook)(nil)
)

//+kubebuilder:webhook:path=/mutate-wiring-githedgehog-com-v1alpha2-connection,mutating=true,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update,versions=v1alpha2,name=mconnection.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-wiring-githedgehog-com-v1alpha2-connection,mutating=false,failurePolicy=fail,sideEffects=None,groups=wiring.githedgehog.com,resources=connections,verbs=create;update,versions=v1alpha2,name=vconnection.kb.io,admissionReviewVersions=v1

// var log = ctrl.Log.WithName("connection-webhook")

func (w *ConnectionWebhook) Default(ctx context.Context, obj runtime.Object) error {
	conn := obj.(*wiringapi.Connection)

	conn.Default()

	return nil
}

func (w *ConnectionWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	conn := obj.(*wiringapi.Connection)

	warns, err := conn.Validate(ctx, w.Validation, w.Cfg.FabricMTU, w.Cfg.ServerFacingMTUOffset, w.Cfg.ParsedReservedSubnets())
	if err != nil {
		return warns, err
	}

	return warns, nil
}

func (w *ConnectionWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO some connections or their parts should be immutable

	oldConn := oldObj.(*wiringapi.Connection)
	newConn := newObj.(*wiringapi.Connection)

	warns, err := newConn.Validate(ctx, w.Validation, w.Cfg.FabricMTU, w.Cfg.ServerFacingMTUOffset, w.Cfg.ParsedReservedSubnets())
	if err != nil {
		return warns, err
	}

	if newConn.Spec.Unbundled != nil || newConn.Spec.Bundled != nil || newConn.Spec.MCLAG != nil {
		if !equality.Semantic.DeepEqual(oldConn.Spec, newConn.Spec) {
			return nil, errors.Errorf("connection spec is immutable")
		}
	}

	return warns, nil
}

func (w *ConnectionWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO prevent deleting connections that are in use

	conn := obj.(*wiringapi.Connection)

	if conn.Spec.Management != nil {
		return nil, errors.New("cannot delete management connection")
	}
	if conn.Spec.NAT != nil {
		return nil, errors.New("cannot delete NAT connection")
	}

	return nil, nil
}
