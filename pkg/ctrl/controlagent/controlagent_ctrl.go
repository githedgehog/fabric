package controlagent

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AgentReconciler reconciles a Agent object
type ControlAgentReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Cfg     *config.Fabric
	Version string
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric, version string) error {
	r := &ControlAgentReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Cfg:     cfg,
		Version: version,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("control-agent").
		For(&wiringapi.Server{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByServerListLabels)).
		Complete(r)
}

func (r *ControlAgentReconciler) enqueueByServerListLabels(ctx context.Context, obj client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	labels := obj.GetLabels()

	// TODO extract to lib
	serverConnPrefix := wiringapi.ListLabelPrefix(wiringapi.ConnectionLabelTypeServer)

	for label, val := range labels {
		if val != wiringapi.ListLabelValue {
			continue
		}

		if strings.HasPrefix(label, serverConnPrefix) {
			serverName := strings.TrimPrefix(label, serverConnPrefix)
			res = append(res, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      serverName,
			}})
		}
	}

	return res
}

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=controlagents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=controlagents/status,verbs=get;get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=controlagents/finalizers,verbs=update

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

func (r *ControlAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	server := &wiringapi.Server{}
	err := r.Get(ctx, req.NamespacedName, server)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting server")
	}

	if server.Spec.Type != wiringapi.ServerTypeControl {
		return ctrl.Result{}, nil
	}

	agent := &agentapi.ControlAgent{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, agent, func() error {
		agent.Spec.ControlVIP = r.Cfg.ControlVIP
		agent.Spec.Version.Default = r.Version
		agent.Spec.Version.Repo = r.Cfg.AgentRepo
		agent.Spec.Version.CA = r.Cfg.AgentRepoCA

		return nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error creating/updating control agent")
	}

	l.Info("control agent reconciled")

	return ctrl.Result{}, nil
}
