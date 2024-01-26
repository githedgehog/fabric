package controlagent

import (
	"context"
	"math"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ConnectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    *config.Fabric
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric) error {
	r := &ConnectionReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("connection").
		For(&wiringapi.Connection{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

func (r *ConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	conn := &wiringapi.Connection{}
	err := r.Get(ctx, req.NamespacedName, conn)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "error getting connection")
	}

	if conn.Spec.ESLAG != nil {
		return r.reconcileESLAG(ctx, conn)
	}

	l.Info("connection reconciled")

	return ctrl.Result{}, nil
}

func (r *ConnectionReconciler) reconcileESLAG(ctx context.Context, conn *wiringapi.Connection) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	if conn.Status.SystemID != 0 {
		return ctrl.Result{}, nil
	}

	connList := &wiringapi.ConnectionList{}
	if err := r.List(ctx, connList, client.MatchingLabels{
		wiringapi.LabelConnectionType: wiringapi.CONNECTION_TYPE_ESLAG,
	}); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing ESLAG connections")
	}

	taken := map[uint32]bool{}
	for _, c := range connList.Items {
		if c.Status.SystemID != 0 {
			taken[c.Status.SystemID] = true
		}
	}

	for id := uint32(1); id < math.MaxUint32; id++ {
		if !taken[id] {
			conn.Status.SystemID = id
			break
		}
	}

	if err := r.Status().Update(ctx, conn); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error updating connection status")
	}

	l.Info("assigned system ID to ESLAG connection", "systemID", conn.Status.SystemID, "connection", conn.Name)

	return ctrl.Result{}, nil
}
