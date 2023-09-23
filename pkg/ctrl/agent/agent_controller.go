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
	"reflect"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/ctrl/common"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	AGENT_CTRL_CONFIG = "agent-ctrl-config.yaml"
	PORT_CHAN_MIN     = 100
	PORT_CHAN_MAX     = 199
)

type AgentControllerConfig struct {
	ControlVIP   string               `json:"controlVIP,omitempty"`
	APIServer    string               `json:"apiServer,omitempty"`
	VPCVLANRange common.VLANRange     `json:"vpcVLANRange,omitempty"`
	Users        []agentapi.UserCreds `json:"users,omitempty"`
}

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    *AgentControllerConfig
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	cfg := &AgentControllerConfig{}
	err := common.LoadCtrlConfig(cfgBasedir, AGENT_CTRL_CONFIG, cfg)
	if err != nil {
		return err
	}

	if cfg.ControlVIP == "" {
		return errors.Errorf("config: controlVIP is required")
	}
	if cfg.APIServer == "" {
		return errors.Errorf("config: apiServer is required")
	}
	if err := cfg.VPCVLANRange.Validate(); err != nil {
		return errors.Wrapf(err, "config: vpcVLANRange is invalid")
	}
	// TODO reserve some VLANs?

	r := &AgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&wiringapi.Switch{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabels)).
		Watches(&vpcapi.VPCAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabels)).
		// TODO enque for rack changes?
		Complete(r)
}

func (r *AgentReconciler) enqueueBySwitchListLabels(ctx context.Context, obj client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	labels := obj.GetLabels()

	// extract to var
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

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments/status,verbs=get

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// TODO make some queueing for switch updates as we're calling reconsile for every switch and its ports changes
	// seems like it's doing a good job compacting reconcilation queue, so, not high priority

	// TODO handle Updates more carefully (e.g. if got updated in parallel)
	// if apierrors.IsConflict(err) {
	//     return ctrl.Result{Requeue: true}, nil
	// }
	// if apierrors.IsNotFound(err) {
	//     return ctrl.Result{Requeue: true}, nil
	// }

	sw := &wiringapi.Switch{}
	err := r.Get(ctx, req.NamespacedName, sw)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch")
	}

	connList := &wiringapi.ConnectionList{}
	err = r.List(ctx, connList, client.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(sw.Name))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch connections")
	}

	peerName := ""
	conns := []agentapi.ConnectionInfo{}
	for _, conn := range connList.Items {
		conns = append(conns, agentapi.ConnectionInfo{
			Name: conn.Name,
			Spec: conn.Spec,
		})
		if conn.Spec.MCLAGDomain != nil {
			// TODO add some helpers
			for _, link := range conn.Spec.MCLAGDomain.PeerLinks {
				if link.Switch1.DeviceName() == sw.Name {
					peerName = link.Switch2.DeviceName()
				} else if link.Switch2.DeviceName() == sw.Name {
					peerName = link.Switch1.DeviceName()
				}
			}
		}
	}
	sort.Slice(conns, func(i, j int) bool {
		return conns[i].Name < conns[j].Name
	})

	vpcAtts := &vpcapi.VPCAttachmentList{}
	err = r.List(ctx, vpcAtts, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch vpc attachments")
	}

	vpcOk := map[string]bool{}
	vpcs := []agentapi.VPCInfo{}
	for _, att := range vpcAtts.Items {
		vpcName := att.Spec.VPC

		if vpcOk[vpcName] {
			continue
		}

		conn := &wiringapi.Connection{}
		err := r.Get(ctx, types.NamespacedName{Namespace: sw.Namespace, Name: att.Spec.Connection}, conn)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting connection %s for attach %s", att.Spec.Connection, att.Name)
		}

		ok := false
		if conn.Spec.Unbundled != nil {
			if conn.Spec.Unbundled.Link.Server.DeviceName() == sw.Name {
				ok = true
			}
		} else if conn.Spec.MCLAG != nil {
			for _, link := range conn.Spec.MCLAG.Links {
				if link.Switch.DeviceName() == sw.Name {
					ok = true
					break
				}
			}
		}
		if !ok {
			continue
		}

		vpc := &vpcapi.VPC{}
		err = r.Get(ctx, types.NamespacedName{Namespace: sw.Namespace, Name: vpcName}, vpc)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting vpc %s for attach %s", vpcName, att.Name)
		}

		if vpc.Status.VLAN == 0 {
			l.Info("vpc doesn't have vlan assigned, skipping", "vpc", vpcName)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil // errors.Errorf("vpc %s doesn't have vlan assigned", vpcName)
		}
		if vpc.Status.VLAN < r.Cfg.VPCVLANRange.Min || vpc.Status.VLAN > r.Cfg.VPCVLANRange.Max {
			return ctrl.Result{}, errors.Errorf("vpc %s vlan %d is out of range %d..%d", vpcName, vpc.Status.VLAN, r.Cfg.VPCVLANRange.Min, r.Cfg.VPCVLANRange.Max)
		}

		vpcs = append(vpcs, agentapi.VPCInfo{
			Name: vpcName,
			VLAN: vpc.Status.VLAN,
			Spec: vpc.Spec,
		})

		vpcOk[vpcName] = true
	}
	sort.Slice(vpcs, func(i, j int) bool {
		return vpcs[i].Name < vpcs[j].Name
	})

	// We only support MCLAG switch pairs for now
	// It means that 2 switches would have the same MCLAG connections and same set of PortChannels

	agent := &agentapi.Agent{}
	err = r.Get(ctx, agentKey(req.NamespacedName), agent)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "error getting agent")
	}

	peer := &agentapi.Agent{}
	err = r.Get(ctx, agentKey(types.NamespacedName{Namespace: sw.Namespace, Name: peerName}), peer)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrapf(err, "error getting peer agent")
	}

	// conn name -> port channel name
	portChannels := map[string]uint16{}
	taken := make([]bool, PORT_CHAN_MAX-PORT_CHAN_MIN+1)
	for _, conn := range conns {
		if conn.Spec.MCLAG != nil {
			pc1 := agent.Spec.PortChannels[conn.Name]
			pc2 := peer.Spec.PortChannels[conn.Name]

			if pc1 != 0 {
				portChannels[conn.Name] = pc1
			}
			if pc2 != 0 {
				portChannels[conn.Name] = pc2
			}
			if pc1 != 0 && pc2 != 0 && pc1 != pc2 {
				return ctrl.Result{}, errors.Errorf("port channel mismatch for conn %s on %s, %s", conn.Name, sw.Name, peerName)
			}
			if portChannels[conn.Name] == 0 {
				continue
			}
			if portChannels[conn.Name] < PORT_CHAN_MIN || portChannels[conn.Name] > PORT_CHAN_MAX {
				return ctrl.Result{}, errors.Errorf("port channel %d for conn %s on %s is out of range %d..%d", portChannels[conn.Name], conn.Name, sw.Name, PORT_CHAN_MIN, PORT_CHAN_MAX)
			}

			taken[portChannels[conn.Name]-PORT_CHAN_MIN] = true
		}
	}

	for _, conn := range conns {
		if conn.Spec.MCLAG != nil {
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
		}
	}

	// We'll only save it for the current agent, as our peer can always take it from us
	// TODO is it good?

	agent = &agentapi.Agent{}
	err = Enforce(l, r, ctx, agentKey(req.NamespacedName), agent, func(agent *agentapi.Agent) {
		if agent.Labels == nil {
			agent.Labels = map[string]string{}
		}

		agent.Spec.ControlVIP = r.Cfg.ControlVIP

		agent.Labels[wiringapi.LabelRack] = sw.Labels[wiringapi.LabelRack]
		agent.Labels[wiringapi.LabelSwitch] = sw.Name

		agent.Spec.Switch = sw.Spec
		agent.Spec.Connections = conns
		agent.Spec.VPCs = vpcs
		agent.Spec.VPCVLANRange = fmt.Sprintf("%d..%d", r.Cfg.VPCVLANRange.Min, r.Cfg.VPCVLANRange.Max)
		agent.Spec.Users = r.Cfg.Users
		agent.Spec.PortChannels = portChannels
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	sa := &corev1.ServiceAccount{}
	err = Enforce(l, r, ctx, saKey(req.NamespacedName), sa, func(sa *corev1.ServiceAccount) {})
	if err != nil {
		return ctrl.Result{}, err
	}

	role := &rbacv1.Role{}
	err = Enforce(l, r, ctx, roleKey(req.NamespacedName), role, func(role *rbacv1.Role) {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents"},
				ResourceNames: []string{agent.Name},
				Verbs:         []string{"get", "watch"},
			},
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents/status"},
				ResourceNames: []string{agent.Name},
				Verbs:         []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		}
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	roleBinding := &rbacv1.RoleBinding{}
	err = Enforce(l, r, ctx, roleBindingKey(req.NamespacedName), roleBinding, func(roleBinding *rbacv1.RoleBinding) {
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
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	kubeconfig := &corev1.Secret{}
	err = Enforce(l, r, ctx, kubeconfigKey(req.NamespacedName), kubeconfig, func(secret *corev1.Secret) {
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}

		secret.Annotations[corev1.ServiceAccountNameKey] = req.Name
		secret.Type = corev1.SecretTypeServiceAccountToken
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO
	for attempt := 0; attempt < 5 && len(kubeconfig.Data) < 3; attempt++ {
		time.Sleep(1 * time.Second)

		err = r.Get(ctx, kubeconfigKey(req.NamespacedName), kubeconfig)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	if len(kubeconfig.Data) < 3 {
		return ctrl.Result{}, errors.New("error getting token for sa")
	}

	genKubeconfig, err := r.genKubeconfig(kubeconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	kubeconfig.StringData = map[string]string{
		KubeconfigKey: genKubeconfig,
	}
	// TODO avoid re-generating kubeconfig if it's not required
	err = r.Update(ctx, kubeconfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func Enforce[T client.Object](log logr.Logger, r *AgentReconciler, ctx context.Context, key client.ObjectKey, obj T, set func(T)) error {
	create := false
	err := r.Get(ctx, key, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			create = true
		} else {
			return errors.Wrapf(err, "error getting %T", obj)
		}
	}

	original := obj.DeepCopyObject()

	obj.SetName(key.Name)
	obj.SetNamespace(key.Namespace)

	// TODO add some labels automatically for tracking?

	set(obj)

	// TODO think about replacing reflect with something else, maybe generate Equal?
	if !create && reflect.DeepEqual(original, obj) {
		// log.Info("Skipping object update", "type", fmt.Sprintf("%T", obj))
		return nil
	}

	if create {
		log.Info("Creating object", "type", fmt.Sprintf("%T", obj))
		err = r.Create(ctx, obj)
	} else {
		log.Info("Updating object", "type", fmt.Sprintf("%T", obj))
		err = r.Update(ctx, obj)
	}

	return errors.Wrapf(err, "error creating/updating %T", obj)
}

func agentKey(switchKey client.ObjectKey) client.ObjectKey {
	return client.ObjectKey{
		Namespace: switchKey.Namespace,
		Name:      switchKey.Name,
	}
}

func saKey(switchKey client.ObjectKey) client.ObjectKey {
	return client.ObjectKey{
		Namespace: switchKey.Namespace,
		Name:      switchKey.Name,
	}
}

func roleKey(switchKey client.ObjectKey) client.ObjectKey {
	return client.ObjectKey{
		Namespace: switchKey.Namespace,
		Name:      switchKey.Name,
	}
}

func roleBindingKey(switchKey client.ObjectKey) client.ObjectKey {
	return client.ObjectKey{
		Namespace: switchKey.Namespace,
		Name:      switchKey.Name,
	}
}

func kubeconfigKey(switchKey client.ObjectKey) client.ObjectKey {
	return client.ObjectKey{
		Namespace: switchKey.Namespace,
		Name:      switchKey.Name,
	}
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
