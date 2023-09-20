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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
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
	"sigs.k8s.io/yaml"
)

type AgentControllerConfig struct {
	ControlVIP string `json:"controlVIP,omitempty"`
	APIServer  string `json:"apiServer,omitempty"`
}

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    *AgentControllerConfig
}

func SetupWithManager(cfgBasedir string, mgr ctrl.Manager) error {
	cfg := &AgentControllerConfig{}

	data, err := os.ReadFile(filepath.Join(cfgBasedir, "agent-ctrl-config.yaml"))
	if err != nil {
		return errors.Wrapf(err, "error reading config")
	}

	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling config")
	}

	if cfg.APIServer == "" {
		cfg.APIServer = "127.0.0.1:6443"
	}

	r := &AgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&wiringapi.Switch{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForConnectionByLabel)).
		Complete(r)
}

func (r *AgentReconciler) enqueueForConnectionByLabel(ctx context.Context, obj client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	labels := obj.GetLabels()

	// extract to var
	switchConnPrefix := wiringapi.ConnectionLabelPrefix(wiringapi.ConnectionLabelTypeSwitch)

	for label, val := range labels {
		if val != wiringapi.ConnectionLabelValue {
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

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents/status,verbs=get;get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents/finalizers,verbs=update

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

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

	conns := &wiringapi.ConnectionList{}
	err = r.List(ctx, conns, client.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForSwitchConnections(sw.Name))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch connections")
	}

	agent := &agentapi.Agent{}
	err = Enforce(log, r, ctx, agentKey(req.NamespacedName), agent, func(agent *agentapi.Agent) {
		if agent.Labels == nil {
			agent.Labels = map[string]string{}
		}

		agent.Spec.ControlVIP = r.Cfg.ControlVIP

		agent.Labels[wiringapi.LabelRack] = sw.Labels[wiringapi.LabelRack]
		agent.Labels[wiringapi.LabelSwitch] = sw.Name

		// TODO would it change all the time b/c of the map order?
		agent.Spec.Switch = sw.Spec
		agent.Spec.Connections = map[string]wiringapi.ConnectionSpec{}
		for _, conn := range conns.Items {
			agent.Spec.Connections[conn.Name] = conn.Spec
		}
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	sa := &corev1.ServiceAccount{}
	err = Enforce(log, r, ctx, saKey(req.NamespacedName), sa, func(sa *corev1.ServiceAccount) {})
	if err != nil {
		return ctrl.Result{}, err
	}

	role := &rbacv1.Role{}
	err = Enforce(log, r, ctx, roleKey(req.NamespacedName), role, func(role *rbacv1.Role) {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents"}, // TODO extract to agent repo
				ResourceNames: []string{agent.Name},
				Verbs:         []string{"get", "watch"},
			},
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents/status"}, // TODO extract to agent repo
				ResourceNames: []string{agent.Name},
				Verbs:         []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		}
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	roleBinding := &rbacv1.RoleBinding{}
	err = Enforce(log, r, ctx, roleBindingKey(req.NamespacedName), roleBinding, func(roleBinding *rbacv1.RoleBinding) {
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
	err = Enforce(log, r, ctx, kubeconfigKey(req.NamespacedName), kubeconfig, func(secret *corev1.Secret) {
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}

		secret.Annotations[corev1.ServiceAccountNameKey] = req.Name
		secret.Type = corev1.SecretTypeServiceAccountToken
	})
	if err != nil {
		return ctrl.Result{}, err
	}

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
    server: {{ .Server }}
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
