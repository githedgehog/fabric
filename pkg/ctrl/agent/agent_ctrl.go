/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/config"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

const (
	PORT_CHAN_MIN = 100
	PORT_CHAN_MAX = 199
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Cfg     *config.Fabric
	Version string
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager, cfg *config.Fabric, version string) error {
	r := &AgentReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Cfg:     cfg,
		Version: version,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&wiringapi.Switch{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabels)).
		Watches(&vpcapi.VPCSummary{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabels)).
		// TODO enque for rack changes?
		Complete(r)
}

func (r *AgentReconciler) enqueueBySwitchListLabels(ctx context.Context, obj client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	labels := obj.GetLabels()

	// TODO extract to lib
	switchConnPrefix := wiringapi.ListLabelPrefix(wiringapi.ConnectionLabelTypeSwitch)

	for label, val := range labels {
		if val != wiringapi.ListLabelValue {
			continue
		}

		if strings.HasPrefix(label, switchConnPrefix) {
			switchName := strings.TrimPrefix(label, switchConnPrefix)
			res = append(res, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      switchName,
			}})
		}
	}

	return res
}

// func (r *AgentReconciler) enqueueSwitchForRack(obj client.Object) []reconcile.Request {
// 	switches := &wiringapi.SwitchList{}
// 	selector, err := labels.ValidatedSelectorFromSet(map[string]string{
// 		wiringapi.LabelRack: obj.GetName(),
// 	})
// 	if err != nil {
// 		// return ctrl.Result{}, errors.Wrapf(err, "error creating switch selector")
// 		panic("error creating switch selector") // TODO replace with log error
// 	}
// 	err = r.List(context.TODO(), switches, client.InNamespace(obj.GetNamespace()), client.MatchingLabelsSelector{
// 		Selector: selector,
// 	})
// 	if err != nil {
// 		// return ctrl.Result{}, errors.Wrapf(err, "error getting switches")
// 		panic("error getting switches") // TODO replace with log error
// 	}

// 	requests := []reconcile.Request{}
// 	for _, sw := range switches.Items {
// 		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
// 			Namespace: obj.GetNamespace(),
// 			Name:      sw.Name,
// 		}})
// 	}

// 	return requests
// }

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents/status,verbs=get;get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents/finalizers,verbs=update

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=nats,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=nats/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	sw := &wiringapi.Switch{}
	err := r.Get(ctx, req.NamespacedName, sw)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch")
	}

	statusUpdates := appendUpdate(nil, sw)

	switchNsName := metav1.ObjectMeta{Name: sw.Name, Namespace: sw.Namespace}
	res, err := r.prepareAgentInfra(ctx, switchNsName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if res != nil {
		return *res, nil
	}

	connList := &wiringapi.ConnectionList{}
	err = r.List(ctx, connList, client.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(sw.Name))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch connections")
	}

	mclagPeerName := ""
	conns := []agentapi.ConnectionInfo{}
	for _, conn := range connList.Items {
		conns = append(conns, agentapi.ConnectionInfo{
			Name: conn.Name,
			Spec: conn.Spec,
		})

		statusUpdates = appendUpdate(statusUpdates, &conn)

		if conn.Spec.MCLAGDomain != nil {
			// TODO add some helpers
			for _, link := range conn.Spec.MCLAGDomain.PeerLinks {
				if link.Switch1.DeviceName() == sw.Name {
					mclagPeerName = link.Switch2.DeviceName()
				} else if link.Switch2.DeviceName() == sw.Name {
					mclagPeerName = link.Switch1.DeviceName()
				}
			}
		}
	}
	sort.Slice(conns, func(i, j int) bool {
		return conns[i].Name < conns[j].Name
	})

	// TODO always provision all VPCs to all switches
	vpcs := []vpcapi.VPCSummarySpec{}
	vpcSummaries := &vpcapi.VPCSummaryList{}
	err = r.List(ctx, vpcSummaries, client.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(sw.Name))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch vpc summaries")
	}
	for _, vpcSummary := range vpcSummaries.Items {
		vpcs = append(vpcs, vpcSummary.Spec)

		// TODO do we need to update VPCSummary status?
		statusUpdates = appendUpdate(statusUpdates, &vpcSummary)
		statusUpdates = appendUpdate(statusUpdates, &vpcapi.VPC{
			TypeMeta: metav1.TypeMeta{
				APIVersion: vpcapi.GroupVersion.String(),
				Kind:       "VPC",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vpcSummary.Name,
				Namespace: vpcSummary.Namespace,
			},
		})
	}
	sort.Slice(vpcs, func(i, j int) bool {
		return vpcs[i].Name < vpcs[j].Name
	})

	// handle MCLAG things if we see a peer switch
	// We only support MCLAG switch pairs for now
	// It means that 2 switches would have the same MCLAG connections and same set of PortChannels
	var mclagPeer *agentapi.Agent
	if mclagPeerName != "" {
		mclagPeer = &agentapi.Agent{}
		err = r.Get(ctx, types.NamespacedName{Namespace: sw.Namespace, Name: mclagPeerName}, mclagPeer)
		if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "error getting peer agent")
		}
	}

	nat := &vpcapi.NAT{}
	err = r.Get(ctx, types.NamespacedName{Namespace: sw.Namespace, Name: "default"}, nat) // TODO support multiple NATs
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "error getting NAT")
	}

	agent := &agentapi.Agent{ObjectMeta: switchNsName}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, agent, func() error {
		agent.Labels = sw.Labels
		agent.Spec.Config.ControlVIP = r.Cfg.ControlVIP
		agent.Spec.Switch = sw.Spec
		agent.Spec.Connections = conns
		agent.Spec.VPCs = vpcs
		agent.Spec.VPCVLANRange = fmt.Sprintf("%d..%d", r.Cfg.VPCVLANRange.Min, r.Cfg.VPCVLANRange.Max)
		agent.Spec.Users = r.Cfg.Users
		agent.Spec.Version.Default = r.Version
		agent.Spec.Version.Repo = r.Cfg.AgentRepo
		agent.Spec.Version.CA = r.Cfg.AgentRepoCA
		agent.Spec.StatusUpdates = statusUpdates

		if r.Cfg.FabricMode == config.FabricModeCollapsedCore {
			agent.Spec.Config.CollapsedCore = &agentapi.AgentSpecConfigCollapsedCore{
				VPCBackend:  r.Cfg.VPCBackend,
				SNATAllowed: r.Cfg.SNATAllowed,
			}
		} else if r.Cfg.FabricMode == config.FabricModeSpineLeaf {
			agent.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		}

		if mclagPeer != nil {
			agent.Spec.PortChannels, err = r.calculatePortChannels(ctx, agent, mclagPeer, conns)
			if err != nil {
				return errors.Wrapf(err, "error calculating port channels")
			}
		}

		agent.Spec.NAT = nat.Spec

		return nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error creating agent")
	}

	l.Info("agent reconciled")

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) prepareAgentInfra(ctx context.Context, agentMeta metav1.ObjectMeta) (*ctrl.Result, error) {
	l := log.FromContext(ctx)

	sa := &corev1.ServiceAccount{ObjectMeta: agentMeta}
	_, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error { return nil })
	if err != nil {
		return nil, errors.Wrapf(err, "error creating service account")
	}

	role := &rbacv1.Role{ObjectMeta: agentMeta}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents"},
				ResourceNames: []string{agentMeta.Name},
				Verbs:         []string{"get", "watch"},
			},
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents/status"},
				ResourceNames: []string{agentMeta.Name},
				Verbs:         []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating role")
	}

	roleBinding := &rbacv1.RoleBinding{ObjectMeta: agentMeta}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		}
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     role.Name,
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating role binding")
	}

	tokenSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: agentMeta.Namespace, Name: agentMeta.Name + "-satoken"}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, tokenSecret, func() error {
		if tokenSecret.Annotations == nil {
			tokenSecret.Annotations = map[string]string{}
		}

		tokenSecret.Annotations[corev1.ServiceAccountNameKey] = agentMeta.Name
		tokenSecret.Type = corev1.SecretTypeServiceAccountToken

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating token secret")
	}

	// we don't yet have service account token for the agent
	if len(tokenSecret.Data) < 3 {
		// TODO is it the best we can do? or should we do few in-place retries?
		l.Info("requeue to wait for service account token")
		return &ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	kubeconfig, err := r.genKubeconfig(tokenSecret)
	if err != nil {
		return nil, errors.Wrapf(err, "error generating kubeconfig")
	}

	kubeconfigSecret := &corev1.Secret{ObjectMeta: agentMeta}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, kubeconfigSecret, func() error {
		kubeconfigSecret.StringData = map[string]string{
			KubeconfigKey: kubeconfig,
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating kubeconfig secret")
	}

	return nil, nil
}

func (r *AgentReconciler) calculatePortChannels(ctx context.Context, agent, peer *agentapi.Agent, conns []agentapi.ConnectionInfo) (map[string]uint16, error) {
	portChannels := map[string]uint16{}

	taken := make([]bool, PORT_CHAN_MAX-PORT_CHAN_MIN+1)
	for _, conn := range conns {
		if conn.Spec.MCLAG != nil {
			pc1 := agent.Spec.PortChannels[conn.Name]
			pc2 := peer.Spec.PortChannels[conn.Name]

			if pc1 == 0 && pc2 == 0 {
				continue
			}

			if pc1 != 0 && pc2 != 0 && pc1 != pc2 {
				return nil, errors.Errorf("port channel mismatch for conn %s on %s and %s", conn.Name, agent.Name, peer.Name)
			}

			if pc1 != 0 {
				if pc1 < PORT_CHAN_MIN || pc1 > PORT_CHAN_MAX {
					return nil, errors.Errorf("port channel %d for conn %s on %s is out of range %d..%d", portChannels[conn.Name], conn.Name, agent.Name, PORT_CHAN_MIN, PORT_CHAN_MAX)
				}
				if taken[pc1-PORT_CHAN_MIN] {
					return nil, errors.Errorf("port channel %d for conn %s assigned on %s is already taken", pc2, conn.Name, agent.Name)
				}

				portChannels[conn.Name] = pc1
			}
			if pc2 != 0 {
				if pc2 < PORT_CHAN_MIN || pc2 > PORT_CHAN_MAX {
					return nil, errors.Errorf("port channel %d for conn %s on peer %s is out of range %d..%d", portChannels[conn.Name], conn.Name, peer.Name, PORT_CHAN_MIN, PORT_CHAN_MAX)
				}
				if taken[pc2-PORT_CHAN_MIN] {
					return nil, errors.Errorf("port channel %d for conn %s assigned on peer %s is already taken", pc2, conn.Name, peer.Name)
				}

				portChannels[conn.Name] = pc2
			}

			taken[portChannels[conn.Name]-PORT_CHAN_MIN] = true
		} else if conn.Spec.Bundled != nil {
			pc := agent.Spec.PortChannels[conn.Name]
			if pc == 0 {
				continue
			}

			if taken[pc-PORT_CHAN_MIN] {
				return nil, errors.Errorf("port channel %d for conn %s on %s is already taken", portChannels[conn.Name], conn.Name, agent.Name)
			}
			portChannels[conn.Name] = pc

			taken[pc-PORT_CHAN_MIN] = true
		}
	}

	// mark all port channels on the peer as taken so we don't assign them to other connections
	for _, pc := range peer.Spec.PortChannels {
		if pc == 0 {
			continue
		}

		taken[pc-PORT_CHAN_MIN] = true
	}

	for _, conn := range conns {
		if conn.Spec.MCLAG != nil || conn.Spec.Bundled != nil {
			if portChannels[conn.Name] != 0 {
				continue
			}

			for i := PORT_CHAN_MIN; i <= PORT_CHAN_MAX; i++ {
				if !taken[i-PORT_CHAN_MIN] {
					portChannels[conn.Name] = uint16(i)
					taken[i-PORT_CHAN_MIN] = true
					break
				}
			}

			if portChannels[conn.Name] == 0 {
				return nil, errors.Errorf("no port channel available for conn %s on %s", conn.Name, agent.Name)
			}
		}
	}

	return portChannels, nil
}

const (
	KubeconfigKey = "kubeconfig"
)

var genKubeconfigTmpl *template.Template

type genKubeconfigTmplCfg struct {
	CA     string
	Server string
	Token  string
}

func init() {
	var err error
	genKubeconfigTmpl, err = template.New("kubeconfig").Parse(`
apiVersion: v1
kind: Config
current-context: default
contexts:
- context:
    cluster: default
    user: default
  name: default
clusters:
- cluster:
    certificate-authority-data: {{ .CA }}
    server: https://{{ .Server }}
  name: default
users:
- name: default
  user:
    token: {{ .Token }}
`)

	if err != nil {
		panic(err)
	}
}

func (r *AgentReconciler) genKubeconfig(secret *corev1.Secret) (string, error) {
	buf := &bytes.Buffer{}
	err := genKubeconfigTmpl.Execute(buf, genKubeconfigTmplCfg{
		Server: r.Cfg.APIServer,
		CA:     base64.StdEncoding.EncodeToString(secret.Data[corev1.ServiceAccountRootCAKey]),
		Token:  string(secret.Data[corev1.ServiceAccountTokenKey]),
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func appendUpdate(statusUpdates []agentapi.ApplyStatusUpdate, obj client.Object) []agentapi.ApplyStatusUpdate {
	return append(statusUpdates, agentapi.ApplyStatusUpdate{
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Generation: obj.GetGeneration(),
	})
}
