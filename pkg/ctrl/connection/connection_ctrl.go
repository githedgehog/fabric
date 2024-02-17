package controlagent

import (
	"context"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ConnectionReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Cfg     *meta.FabricConfig
	LibMngr *librarian.Manager
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *meta.FabricConfig, libMngr *librarian.Manager) error {
	r := &ConnectionReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Cfg:     cfg,
		LibMngr: libMngr,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("connection").
		For(&wiringapi.Connection{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

func (r *ConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	if err := r.LibMngr.UpdateConnections(ctx, r.Client); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating connections catalog")
	}

	conn := &wiringapi.Connection{}
	err := r.Get(ctx, req.NamespacedName, conn)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "error getting connection")
	}

	l.Info("connection reconciled")

	return ctrl.Result{}, nil
}
