package connection

import (
	"context"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ConnectionWebhook struct {
	client.Client
	Scheme *runtime.Scheme
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	w := &ConnectionWebhook{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
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

// var log = ctrl.Log.WithName("connection-webhook")

func (w *ConnectionWebhook) Default(ctx context.Context, obj runtime.Object) error {
	conn := obj.(*wiringapi.Connection)

	conn.GenerateLabels()

	return nil
}

func (w *ConnectionWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO validate it points to existing devices of correct types
	// TODO validate there are no duplicates
	// TODO validate all ref structure

	return nil, nil
}

func (w *ConnectionWebhook) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (warnings admission.Warnings, err error) {
	oldConn := oldObj.(*wiringapi.Connection)
	newConn := newObj.(*wiringapi.Connection)

	// TODO some connections could be mutable probably
	if !equality.Semantic.DeepEqual(oldConn.Spec, newConn.Spec) {
		return nil, errors.New("connection spec is immutable")
	}

	return nil, nil
}

func (w *ConnectionWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (warnings admission.Warnings, err error) {
	// TODO prevent deleting connections that are in use
	// TODO prevent deleting control node connections

	conn := obj.(*wiringapi.Connection)

	if conn.Spec.Management != nil {
		return nil, errors.New("cannot delete management connection")
	}

	return nil, nil
}
