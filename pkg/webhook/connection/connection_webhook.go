package connection

import (
	"context"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

// validateStaticExternal checks that the static external connection is valid and it's located in a webhook to avoid circular dependency with vpcapi
func (w *ConnectionWebhook) validateStaticExternal(ctx context.Context, client validation.Client, conn *wiringapi.Connection) error {
	if conn.Spec.StaticExternal != nil && conn.Spec.StaticExternal.WithinVPC != "" {
		vpc := &vpcapi.VPC{}
		err := client.Get(ctx, types.NamespacedName{Name: conn.Spec.StaticExternal.WithinVPC, Namespace: conn.Namespace}, vpc) // TODO namespace could be different?
		if apierrors.IsNotFound(err) {
			return errors.Errorf("vpc %s not found", conn.Spec.StaticExternal.WithinVPC)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to get vpc %s", conn.Spec.StaticExternal.WithinVPC) // TODO replace with some internal error to not expose to the user
		}
	}

	return nil
}

func (w *ConnectionWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	conn := obj.(*wiringapi.Connection)

	warns, err := conn.Validate(ctx, w.Validation,
		w.Cfg.FabricMTU,
		w.Cfg.ServerFacingMTUOffset,
		w.Cfg.ParsedReservedSubnets(),
		w.Cfg.FabricMode == config.FabricModeSpineLeaf)
	if err != nil {
		return warns, err
	}

	return warns, w.validateStaticExternal(ctx, w.Validation, conn)
}

func (w *ConnectionWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO some connections or their parts should be immutable

	oldConn := oldObj.(*wiringapi.Connection)
	newConn := newObj.(*wiringapi.Connection)

	warns, err := newConn.Validate(ctx, w.Validation,
		w.Cfg.FabricMTU,
		w.Cfg.ServerFacingMTUOffset,
		w.Cfg.ParsedReservedSubnets(),
		w.Cfg.FabricMode == config.FabricModeSpineLeaf)
	if err != nil {
		return warns, err
	}

	if newConn.Spec.Unbundled != nil || newConn.Spec.Bundled != nil || newConn.Spec.MCLAG != nil {
		if !equality.Semantic.DeepEqual(oldConn.Spec, newConn.Spec) {
			return nil, errors.Errorf("connection spec is immutable")
		}
	}

	return warns, w.validateStaticExternal(ctx, w.Validation, newConn)
}

func (w *ConnectionWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO prevent deleting connections that are in use

	conn := obj.(*wiringapi.Connection)

	if conn.Spec.Management != nil {
		return nil, errors.New("cannot delete management connection")
	}

	return nil, nil
}
