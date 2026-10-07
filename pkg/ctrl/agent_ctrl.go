// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ctrl

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fmeta "go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"go.githedgehog.com/fabric/pkg/version"
	"go.githedgehog.com/libmeta/pkg/alloy"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	ktypes "k8s.io/apimachinery/pkg/types"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	AgentPrefix        = "agent--"
	AgentKubeconfigKey = "kubeconfig"
)

func AgentServiceAccount(agent string) string {
	return AgentPrefix + agent
}

func AgentKubeconfigSecret(agent string) string {
	return AgentPrefix + agent
}

const (
	PortChanMin = 100
	PortChanMax = 199
)

type AgentReconciler struct {
	kclient.Client
	cfg         *fmeta.FabricConfig
	libr        *librarian.Manager
	regCA       string
	regUsername string
	regPassword string
	lock        *Lock
}

func SetupAgentReconsilerWith(mgr kctrl.Manager, cfg *fmeta.FabricConfig, libMngr *librarian.Manager, ca, username, password string, lock *Lock) error {
	if cfg == nil {
		return errors.New("fabric config is nil")
	}
	if libMngr == nil {
		return errors.New("librarian manager is nil")
	}
	if ca == "" {
		return errors.New("reg ca is empty")
	}
	if username == "" {
		return errors.New("reg username is empty")
	}
	if password == "" {
		return errors.New("reg password is empty")
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &AgentReconciler{
		Client:      mgr.GetClient(),
		cfg:         cfg,
		libr:        libMngr,
		regCA:       ca,
		regUsername: username,
		regPassword: password,
		lock:        lock,
	}

	return errors.Wrapf(kctrl.NewControllerManagedBy(mgr).
		Named("Agent").
		For(&wiringapi.Switch{}).
		Watches(&wiringapi.Switch{}, handler.EnqueueRequestsFromMapFunc(r.enqueueNeighbors), builder.WithPredicates(neighborChanged)).
		// recreated if deleted, garbage collected with the switch otherwise
		Owns(&agentapi.Agent{}, builder.WithPredicates(onlyDeletes)).
		Owns(&corev1.ServiceAccount{}, builder.WithPredicates(onlyDeletes)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(onlyDeletes)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(onlyDeletes)).
		Owns(&corev1.Secret{}, builder.WithPredicates(onlyDeletes)).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabelsAndSpines)).
		Watches(&wiringapi.SwitchProfile{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchProfileLabel)).
		Watches(&vpcapi.VPC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByFabricWide)).
		Watches(&vpcapi.VPCAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByAttachment)).
		Watches(&vpcapi.VPCPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByFabricWide)).
		Watches(&vpcapi.External{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByFabricWide)).
		Watches(&vpcapi.ExternalAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByAttachment)).
		Watches(&vpcapi.ExternalPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByExternalPeering)).
		Watches(&vpcapi.RemotePeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByFabricWide)).
		Watches(&vpcapi.IPv4Namespace{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByFabricWide)).
		Watches(&wiringapi.Fabric{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByFabric)).
		Watches(&wiringapi.VLANNamespace{}, handler.EnqueueRequestsFromMapFunc(r.enqueueByVLANNamespace)).
		Complete(r), "failed to setup agent controller")
}

func (r *AgentReconciler) enqueueBySwitchListLabelsAndSpines(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	labelSwitches := switchesOfConnection(obj)
	res := switchRequests(obj.GetNamespace(), labelSwitches)

	// static externals outside of a VPC are configured on all spines
	if obj.GetLabels()[wiringapi.LabelConnectionType] != wiringapi.ConnectionTypeStaticExternal {
		return res
	}

	// also enqueue all spines
	sws := &wiringapi.SwitchList{}
	err := r.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()))
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing switches to reconcile spine switches")

		return res
	}

	for _, sw := range sws.Items {
		if !sw.Spec.Role.IsSpine() {
			continue
		}
		if _, ok := labelSwitches[sw.Name]; ok {
			// already enqueued by label
			continue
		}
		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
			Namespace: sw.Namespace,
			Name:      sw.Name,
		}})
	}

	return res
}

// switchesOfConnection returns the switches a connection is on, by its switch list labels, which are also what a
// switch's connections are listed by in the reconcile
func switchesOfConnection(conn kclient.Object) map[string]bool {
	prefix := wiringapi.ListLabelPrefix(wiringapi.ConnectionLabelTypeSwitch)

	switches := map[string]bool{}
	for label, val := range conn.GetLabels() {
		if val != wiringapi.ListLabelValue {
			continue
		}
		if name, ok := strings.CutPrefix(label, prefix); ok {
			switches[name] = true
		}
	}

	return switches
}

// switchRequests turns switch names into requests, sorted for a stable order
func switchRequests(namespace string, switches map[string]bool) []reconcile.Request {
	res := make([]reconcile.Request, 0, len(switches))
	for _, name := range slices.Sorted(maps.Keys(switches)) {
		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{Namespace: namespace, Name: name}})
	}

	return res
}

// enqueueByAttachment reconciles the switches of the connection a VPC or external attachment is on, they're the only
// ones getting it
func (r *AgentReconciler) enqueueByAttachment(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	var connName string
	switch attach := obj.(type) {
	case *vpcapi.VPCAttachment:
		connName = attach.Spec.Connection
	case *vpcapi.ExternalAttachment:
		connName = attach.Spec.Connection
	default:
		kctrllog.FromContext(ctx).Error(fmt.Errorf("unexpected type %T", obj), "error mapping attachment to switches") //nolint:err113

		return r.enqueueAllSwitches(ctx, obj)
	}

	conn := &wiringapi.Connection{}
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: obj.GetNamespace(), Name: connName}, conn); err != nil {
		// without its connection the attachment isn't on any switch, and deleting the connection reconciles its
		// switches by itself
		if kapierrors.IsNotFound(err) {
			return nil
		}

		kctrllog.FromContext(ctx).Error(err, "error getting attachment connection, reconciling all switches", "connection", connName)

		return r.enqueueAllSwitches(ctx, obj)
	}

	return switchRequests(obj.GetNamespace(), switchesOfConnection(conn))
}

// neighborChanged admits the switch changes that reach the other switches' Agents: its spec and its labels, as they
// take the highest bench touch label of their neighbors too
var neighborChanged = predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{})

// enqueueNeighbors reconciles the switches sharing a connection with the switch, as their Agents carry a copy of its
// spec, and the switches of its redundancy group, as their Agents list it as a peer
func (r *AgentReconciler) enqueueNeighbors(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	sw, ok := obj.(*wiringapi.Switch)
	if !ok {
		kctrllog.FromContext(ctx).Error(fmt.Errorf("unexpected type %T", obj), "error mapping to neighbor switches, reconciling all switches") //nolint:err113

		return r.enqueueAllSwitches(ctx, obj)
	}

	conns := &wiringapi.ConnectionList{}
	if err := r.List(ctx, conns, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(sw.Name)); err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing switch connections, reconciling all switches")

		return r.enqueueAllSwitches(ctx, obj)
	}

	switches := map[string]bool{}
	for _, conn := range conns.Items {
		maps.Copy(switches, switchesOfConnection(&conn))
	}

	if group := sw.Spec.Redundancy.Group; group != "" {
		peers := &wiringapi.SwitchList{}
		if err := r.List(ctx, peers, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForSwitchGroup(group)); err != nil {
			kctrllog.FromContext(ctx).Error(err, "error listing redundancy group switches, reconciling all switches")

			return r.enqueueAllSwitches(ctx, obj)
		}
		for _, peer := range peers.Items {
			// the label is on every switch listing the group, not only the ones having it as their redundancy group
			if peer.Spec.Redundancy.Group == group {
				switches[peer.Name] = true
			}
		}
	}

	// the switch itself is reconciled as the controller's own object
	delete(switches, sw.Name)

	return switchRequests(sw.Namespace, switches)
}

// enqueueByVLANNamespace reconciles the switches in the VLANNamespace, by their VLANNamespace label
func (r *AgentReconciler) enqueueByVLANNamespace(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	sws := &wiringapi.SwitchList{}
	if err := r.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()), kclient.MatchingLabels{
		wiringapi.ListLabelVLANNamespace(obj.GetName()): wiringapi.ListLabelValue,
	}); err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing switches of vlan namespace, reconciling all switches")

		return r.enqueueAllSwitches(ctx, obj)
	}

	switches := make(map[string]bool, len(sws.Items))
	for _, sw := range sws.Items {
		switches[sw.Name] = true
	}

	return switchRequests(obj.GetNamespace(), switches)
}

// switchesInFabric returns the switches of the fabric in any of the domains, or all of its switches if no domains are
// given, by their fabric and domain labels
func (r *AgentReconciler) switchesInFabric(ctx context.Context, namespace, fabric string, domains ...string) (map[string]bool, error) {
	switches := map[string]bool{}
	list := func(selector kclient.MatchingLabels) error {
		sws := &wiringapi.SwitchList{}
		if err := r.List(ctx, sws, kclient.InNamespace(namespace), selector); err != nil {
			return err //nolint:wrapcheck
		}
		for _, sw := range sws.Items {
			switches[sw.Name] = true
		}

		return nil
	}

	if len(domains) == 0 {
		if err := list(kclient.MatchingLabels{wiringapi.ListLabelFabric(fabric): wiringapi.ListLabelValue}); err != nil {
			return nil, fmt.Errorf("listing switches of fabric %s: %w", fabric, err)
		}

		return switches, nil
	}

	// a switch can be in multiple domains, so there is a list per domain
	for _, domain := range domains {
		if err := list(kclient.MatchingLabels{
			wiringapi.ListLabelFabric(fabric): wiringapi.ListLabelValue,
			wiringapi.ListLabelDomain(domain): wiringapi.ListLabelValue,
		}); err != nil {
			return nil, fmt.Errorf("listing switches of fabric %s domain %s: %w", fabric, domain, err)
		}
	}

	return switches, nil
}

// enqueueByFabricWide reconciles all switches of the fabric of an External, RemotePeering or IPv4Namespace, every switch
// gets all of them in its fabric, and of a VPC or VPCPeering, as VPCs never leave their fabric
func (r *AgentReconciler) enqueueByFabricWide(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	var fabric string
	switch o := obj.(type) {
	case *vpcapi.External:
		fabric = o.Spec.Topology.Fabric
	case *vpcapi.RemotePeering:
		fabric = o.Spec.Topology.Fabric
	case *vpcapi.IPv4Namespace:
		fabric = o.Spec.Topology.Fabric
	case *vpcapi.VPC:
		fabric = o.Spec.Topology.Fabric
	case *vpcapi.VPCPeering:
		fabric = o.Spec.Topology.Fabric
	default:
		kctrllog.FromContext(ctx).Error(fmt.Errorf("unexpected type %T", obj), "error mapping to switches of the fabric") //nolint:err113

		return r.enqueueAllSwitches(ctx, obj)
	}
	if fabric == "" {
		kctrllog.FromContext(ctx).Error(fmt.Errorf("no fabric"), "error mapping to switches of the fabric, reconciling all switches", "kind", fmt.Sprintf("%T", obj), "name", obj.GetName()) //nolint:err113

		return r.enqueueAllSwitches(ctx, obj)
	}

	switches, err := r.switchesInFabric(ctx, obj.GetNamespace(), fabric)
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error mapping to switches of the fabric, reconciling all switches")

		return r.enqueueAllSwitches(ctx, obj)
	}

	return switchRequests(obj.GetNamespace(), switches)
}

// enqueueByExternalPeering reconciles the switches an ExternalPeering could be configured on: only the ones attached
// to its External get it and an ExternalAttachment's switch has to be in the External's domain
func (r *AgentReconciler) enqueueByExternalPeering(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	peering, ok := obj.(*vpcapi.ExternalPeering)
	if !ok {
		kctrllog.FromContext(ctx).Error(fmt.Errorf("unexpected type %T", obj), "error mapping to switches of the external, reconciling all switches") //nolint:err113

		return r.enqueueAllSwitches(ctx, obj)
	}

	// without the External its domain isn't known, but it has to be in the peering's fabric
	fabric, domains := peering.Spec.Topology.Fabric, []string(nil)
	ext := &vpcapi.External{}
	if err := r.Get(ctx, kclient.ObjectKey{Namespace: peering.Namespace, Name: peering.Spec.Permit.External.Name}, ext); err == nil {
		fabric, domains = ext.Spec.Topology.Fabric, []string{ext.Spec.Topology.Domain}
	} else if !kapierrors.IsNotFound(err) {
		kctrllog.FromContext(ctx).Error(err, "error getting external, reconciling all switches", "external", peering.Spec.Permit.External.Name)

		return r.enqueueAllSwitches(ctx, obj)
	}
	if fabric == "" {
		kctrllog.FromContext(ctx).Error(fmt.Errorf("no fabric"), "error mapping to switches of the external, reconciling all switches", "name", peering.Name) //nolint:err113

		return r.enqueueAllSwitches(ctx, obj)
	}

	switches, err := r.switchesInFabric(ctx, peering.Namespace, fabric, domains...)
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error mapping to switches of the external, reconciling all switches")

		return r.enqueueAllSwitches(ctx, obj)
	}

	return switchRequests(peering.Namespace, switches)
}

func (r *AgentReconciler) enqueueBySwitchProfileLabel(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	res := []reconcile.Request{}

	sws := &wiringapi.SwitchList{}
	err := r.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()))
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing switches to reconcile by profile")

		return res
	}

	for _, sw := range sws.Items {
		if sw.Spec.Profile != obj.GetName() {
			continue
		}

		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
			Namespace: sw.Namespace,
			Name:      sw.Name,
		}})
	}

	return res
}

func (r *AgentReconciler) enqueueByFabric(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	res := []reconcile.Request{}

	sws := &wiringapi.SwitchList{}
	err := r.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()))
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing switches to reconcile by fabric")

		return res
	}

	for _, sw := range sws.Items {
		if sw.Spec.Topology.Fabric != obj.GetName() {
			continue
		}

		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
			Namespace: sw.Namespace,
			Name:      sw.Name,
		}})
	}

	return res
}

func (r *AgentReconciler) enqueueAllSwitches(ctx context.Context, obj kclient.Object) []reconcile.Request {
	// all switches are queued and reconciled once unlocked, see lockedRequeueAfter; the refresh alone would fan out
	// every write of a refreshed object to every switch otherwise
	if r.lock.Locked() {
		return nil
	}

	res := []reconcile.Request{}

	sws := &wiringapi.SwitchList{}
	err := r.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()))
	if err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing switches to reconcile all")

		return res
	}

	for _, sw := range sws.Items {
		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
			Namespace: sw.Namespace,
			Name:      sw.Name,
		}})
	}

	return res
}

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents/status,verbs=get;get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=agents/finalizers,verbs=update

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switches/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchprofiles/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchgroups,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchgroups/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=servers/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=vlannamespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=vlannamespaces/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcs/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcattachments/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcpeerings,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=vpcpeerings/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=ipv4namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=ipv4namespaces/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externals/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externalattachments,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externalattachments/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externalpeerings,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=externalpeerings/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=remotepeerings,verbs=get;list;watch
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=remotepeerings/status,verbs=get;update;patch

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	sw := &wiringapi.Switch{}
	err := r.Get(ctx, req.NamespacedName, sw)
	if err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, errors.Wrapf(err, "error getting switch")
	}

	// everything produced for the switch is garbage collected with it
	if sw.DeletionTimestamp != nil {
		return kctrl.Result{}, nil
	}

	fabric, err := wiringapi.GetFabricSpec(ctx, r, sw.Namespace, sw.Spec.Topology.Fabric)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("error getting switch fabric: %w", err)
	}
	// a switch the refresh had to leave alone may still have none
	if len(sw.Spec.Topology.Domains) == 0 {
		return kctrl.Result{}, fmt.Errorf("switch has no domains") //nolint:err113
	}
	// only for the deprecated scalar ASNs in the agent config, which agents from before domains read
	domainName := slices.Min(sw.Spec.Topology.Domains)
	domain, exists := fabric.Domains[domainName]
	if !exists {
		return kctrl.Result{}, fmt.Errorf("switch domain %s not found in fabric", domainName) //nolint:err113
	}

	// TODO impl
	statusUpdates := appendUpdate(nil, sw)

	switchNsName := kmetav1.ObjectMeta{Name: sw.Name, Namespace: sw.Namespace}
	res, err := r.prepareAgentInfra(ctx, sw)
	if err != nil {
		return kctrl.Result{}, err
	}
	if res != nil {
		return *res, nil
	}

	connList := &wiringapi.ConnectionList{}
	err = r.List(ctx, connList, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(sw.Name))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error getting switch connections")
	}

	benchTouch := maxBenchTouch(0, sw)

	conns := map[string]wiringapi.ConnectionSpec{}
	for _, conn := range connList.Items {
		if !r.cfg.LoopbackWorkaround && conn.Spec.VPCLoopback != nil {
			continue
		}

		conns[conn.Name] = conn.Spec
		benchTouch = maxBenchTouch(benchTouch, &conn)
	}

	// for spines, also add static external connections that are not within VPC
	if sw.Spec.Role.IsSpine() {
		staticConnList := &wiringapi.ConnectionList{}
		err = r.List(ctx, staticConnList, kclient.InNamespace(sw.Namespace), kclient.MatchingLabels{wiringapi.LabelConnectionType: wiringapi.ConnectionTypeStaticExternal})
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting static external connections for spine %s", sw.Name)
		}
		for _, conn := range staticConnList.Items {
			if conn.Spec.StaticExternal.WithinVPC != "" {
				continue
			}
			conns[conn.Name] = conn.Spec
			benchTouch = maxBenchTouch(benchTouch, &conn)
		}
	}

	neighborSwitches := map[string]bool{}
	for _, conn := range connList.Items {
		sws, _, _, _, err := conn.Spec.Endpoints()
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting endpoints for connection %s", conn.Name)
		}
		for _, sw := range sws {
			neighborSwitches[sw] = true
		}
	}

	neighbors, err := r.getSwitches(ctx, sw.Namespace, neighborSwitches)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting neighbor switches: %w", err)
	}
	switches := map[string]wiringapi.SwitchSpec{}
	for name, neighbor := range neighbors {
		switches[name] = neighbor.Spec
		benchTouch = maxBenchTouch(benchTouch, neighbor)
	}

	attaches := map[string]agentapi.VPCAttachmentSpecAnn{}
	configuredSubnets := map[string]bool{} // TODO probably it's not really needed
	attachedVPCs := map[string]bool{}
	attachList := &vpcapi.VPCAttachmentList{}
	if err := r.listByConnections(ctx, sw.Namespace, attachList, conns); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing vpc attachments: %w", err)
	}
	for _, attach := range attachList.Items {
		_, conn := conns[attach.Spec.Connection]

		if conn {
			anns := map[string]string{}
			for k, v := range attach.Annotations {
				if !strings.HasPrefix(k, "fabric.githedgehog.com/") {
					continue
				}

				anns[k] = v
			}

			attaches[attach.Name] = agentapi.VPCAttachmentSpecAnn{
				VPCAttachmentSpec: attach.Spec,
				Annotations:       anns,
			}
			benchTouch = maxBenchTouch(benchTouch, &attach)

			attachedVPCs[attach.Spec.VPCName()] = true
			configuredSubnets[attach.Spec.Subnet] = true
		}
	}

	staticExtVPCs := map[string]bool{}
	for _, conn := range conns {
		if conn.StaticExternal == nil {
			continue
		}
		if conn.StaticExternal.WithinVPC == "" {
			continue
		}

		staticExtVPCs[conn.StaticExternal.WithinVPC] = true
	}

	vpcs := map[string]vpcapi.VPCSpec{}
	vpcList := &vpcapi.VPCList{}
	vpcRelays := map[string]bool{}
	err = r.List(ctx, vpcList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing vpcs")
	}
	for _, vpc := range vpcList.Items {
		ok := attachedVPCs[vpc.Name] || staticExtVPCs[vpc.Name]
		for subnetName, subnetSpec := range vpc.Spec.Subnets {
			if configuredSubnets[fmt.Sprintf("%s/%s", vpc.Name, subnetName)] {
				ok = true
				// if an attached VPC subnet refers to another VPC as DHCP relay, we need to add the VPC too
				if subnetSpec.DHCP.RelayVPC != "" {
					vpcRelays[subnetSpec.DHCP.RelayVPC] = true
				}
			}
		}
		if ok {
			vpcs[vpc.Name] = vpc.Spec
		}
	}

	for _, vpc := range vpcList.Items {
		if vpcRelays[vpc.Name] {
			vpcs[vpc.Name] = vpc.Spec
		}
	}

	// TODO only query for related peerings
	peerings := map[string]vpcapi.VPCPeeringSpec{}
	peeringsList := &vpcapi.VPCPeeringList{}
	peeredVPCs := map[string]bool{}
	err = r.List(ctx, peeringsList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing vpc peerings")
	}
	for _, peer := range peeringsList.Items {
		vpc1, vpc2, err := peer.Spec.VPCs()
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting vpcs for peering %s", peer.Name)
		}

		_, exists1 := vpcs[vpc1]
		_, exists2 := vpcs[vpc2]

		if exists1 || exists2 {
			peerings[peer.Name] = peer.Spec
			benchTouch = maxBenchTouch(benchTouch, &peer)
			peeredVPCs[vpc1] = true
			peeredVPCs[vpc2] = true
		}
	}

	attachedExternals := map[string]bool{}
	proxyStaticExtAttachments := map[string]bool{}
	externalAttaches := map[string]vpcapi.ExternalAttachmentSpec{}
	externalAttachList := &vpcapi.ExternalAttachmentList{}
	if err := r.listByConnections(ctx, sw.Namespace, externalAttachList, conns); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing external attachments: %w", err)
	}
	for _, attach := range externalAttachList.Items {
		if _, exists := conns[attach.Spec.Connection]; !exists {
			continue
		}

		attachedExternals[attach.Spec.External] = true
		externalAttaches[attach.Name] = attach.Spec
		benchTouch = maxBenchTouch(benchTouch, &attach)

		if attach.Spec.Static != nil && attach.Spec.Static.Proxy {
			proxyStaticExtAttachments[attach.Name] = true
		}
	}

	externals := map[string]vpcapi.ExternalSpec{}
	externalsToConfig := map[string]vpcapi.ExternalSpec{}
	externalList := &vpcapi.ExternalList{}
	externalsReq := map[string]bool{}
	err = r.List(ctx, externalList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing externals")
	}
	for _, ext := range externalList.Items {
		if ext.Spec.Topology.Fabric != sw.Spec.Topology.Fabric {
			continue
		}

		externals[ext.Name] = ext.Spec
		benchTouch = maxBenchTouch(benchTouch, &ext)
		if attachedExternals[ext.Name] {
			externalsToConfig[ext.Name] = ext.Spec
			externalsReq[ext.Name] = true
		}
	}

	externalPeerings := map[string]vpcapi.ExternalPeeringSpec{}
	externalPeeringList := &vpcapi.ExternalPeeringList{}
	err = r.List(ctx, externalPeeringList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing external peerings")
	}
	for _, peering := range externalPeeringList.Items {
		if _, exists := externalsToConfig[peering.Spec.Permit.External.Name]; !exists {
			continue
		}

		// TODO is it ok?
		peeredVPCs[peering.Spec.Permit.VPC.Name] = true

		externalPeerings[peering.Name] = peering.Spec
		benchTouch = maxBenchTouch(benchTouch, &peering)
	}

	// every switch of the fabric gets all remote peerings, for their communities, but only configures the ones with a
	// link on it
	remotePeerings := map[string]vpcapi.RemotePeeringSpec{}
	remotePeeringsReq := map[string]bool{}
	attachedRemotePeerings := map[string]bool{}
	rpList := &vpcapi.RemotePeeringList{}
	if err := r.List(ctx, rpList, kclient.InNamespace(sw.Namespace)); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing remote peerings: %w", err)
	}
	for _, rp := range rpList.Items {
		if rp.Spec.Topology.Fabric != sw.Spec.Topology.Fabric {
			continue
		}

		remotePeerings[rp.Name] = rp.Spec
		remotePeeringsReq[rp.Name] = true
		benchTouch = maxBenchTouch(benchTouch, &rp)
		if !slices.ContainsFunc(rp.Spec.Links, func(link vpcapi.RemotePeeringLink) bool {
			_, exists := conns[link.Connection]

			return exists
		}) {
			continue
		}

		attachedRemotePeerings[rp.Name] = true
		for vpcName := range rp.Spec.Local {
			peeredVPCs[vpcName] = true
		}
	}

	for _, vpc := range vpcList.Items {
		if peeredVPCs[vpc.Name] {
			vpcs[vpc.Name] = vpc.Spec
		}
	}

	for _, vpc := range vpcList.Items {
		if _, exists := vpcs[vpc.Name]; exists {
			benchTouch = maxBenchTouch(benchTouch, &vpc)
		}
	}

	for name, vpc := range vpcs {
		if !slices.Contains(sw.Spec.VLANNamespaces, vpc.VLANNamespace) {
			return kctrl.Result{}, errors.Errorf("switch %s doesn't have vlan namespace %s while gets vpc %s", sw.Name, vpc.VLANNamespace, name)
		}
	}

	ipv4NamespaceList := &vpcapi.IPv4NamespaceList{}
	err = r.List(ctx, ipv4NamespaceList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing ipv4 namespaces")
	}

	ipv4Namespaces := map[string]vpcapi.IPv4NamespaceSpec{}
	for _, ns := range ipv4NamespaceList.Items {
		if ns.Spec.Topology.Fabric != sw.Spec.Topology.Fabric {
			continue
		}

		ipv4Namespaces[ns.Name] = ns.Spec
		benchTouch = maxBenchTouch(benchTouch, &ns)
	}

	vlanNamespaceList := &wiringapi.VLANNamespaceList{}
	err = r.List(ctx, vlanNamespaceList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing vlan namespaces")
	}

	vlanNamespaces := map[string]wiringapi.VLANNamespaceSpec{}
	for _, ns := range vlanNamespaceList.Items {
		if !slices.Contains(sw.Spec.VLANNamespaces, ns.Name) {
			continue
		}

		vlanNamespaces[ns.Name] = ns.Spec
		benchTouch = maxBenchTouch(benchTouch, &ns)
	}

	usedVPCs := map[string]bool{}
	for name := range vpcs {
		usedVPCs[name] = true
	}

	portChanConns := map[string]bool{}
	for name, conn := range conns {
		if conn.Bundled == nil && conn.ESLAG == nil {
			continue
		}

		portChanConns[name] = true
	}

	rgPeers, err := r.redundancyGroupPeers(ctx, sw)
	if err != nil {
		return kctrl.Result{}, fmt.Errorf("getting redundancy group peers: %w", err)
	}

	for _, rgPeerName := range rgPeers {
		connList := &wiringapi.ConnectionList{}
		err = r.List(ctx, connList, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(rgPeerName))
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting rg peer switch %s connections", rgPeerName)
		}

		for _, conn := range connList.Items {
			if conn.Spec.Bundled == nil && conn.Spec.ESLAG == nil {
				continue
			}

			portChanConns[conn.Name] = true
		}
	}

	idConns := map[string]bool{}
	for name, conn := range conns {
		if conn.ESLAG == nil {
			continue
		}

		idConns[name] = true
	}

	if _, err := r.libr.EnsureVNIs(ctx, r.Client, vpcs, externalsReq, remotePeeringsReq); err != nil {
		return kctrl.Result{}, fmt.Errorf("updating VNIs catalog: %w", err)
	}

	cat := &agentapi.CatalogSpec{}

	err = r.libr.CatalogForRedundancyGroup(ctx, r.Client, cat, sw, usedVPCs, portChanConns, idConns, externalsReq, remotePeeringsReq, attachedRemotePeerings)
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error getting redundancy group catalog")
	}

	loWorkaroundLinks := []string{}
	for name, conn := range conns {
		if conn.VPCLoopback == nil {
			continue
		}

		for linkID, link := range conn.VPCLoopback.Links {
			ports := []string{link.Switch1.LocalPortName(), link.Switch2.LocalPortName()}
			sort.Strings(ports)

			if len(ports) != 2 {
				return kctrl.Result{}, errors.Errorf("invalid vpc loopback %s link %d", name, linkID)
			}

			loRef := fmt.Sprintf("%s--%s", ports[0], ports[1])
			loWorkaroundLinks = append(loWorkaroundLinks, loRef)
		}
	}

	loWorkaroundReqs := map[string]bool{}
	if r.cfg.LoopbackWorkaround {
		for name, peering := range peerings {
			vpc1, vpc2, err := peering.VPCs()
			if err != nil {
				return kctrl.Result{}, errors.Wrapf(err, "error getting vpcs for peering %s", name)
			}

			if !attachedVPCs[vpc1] || !attachedVPCs[vpc2] {
				continue
			}

			loWorkaroundReqs[librarian.LoWReqForVPC(name)] = true
		}
		for name, peering := range externalPeerings {
			if !attachedVPCs[peering.Permit.VPC.Name] {
				continue
			}

			loWorkaroundReqs[librarian.ReqForExt(name)] = true
		}
	}

	subnetsReq := map[string]bool{}
	for _, vpc := range vpcs {
		for _, subnet := range vpc.Subnets {
			subnetsReq[subnet.Subnet] = true
		}
	}
	for _, peering := range externalPeerings {
		for _, prefix := range peering.Permit.External.Prefixes {
			subnetsReq[prefix.Prefix] = true
		}
	}
	for name := range attachedRemotePeerings {
		for _, prefix := range remotePeerings[name].Remote.Prefixes {
			subnetsReq[prefix] = true
		}
	}
	for connName, conn := range conns {
		if conn.StaticExternal == nil {
			continue
		}

		_, ipNet, err := net.ParseCIDR(conn.StaticExternal.Link.Switch.IP)
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error parsing static external conn %s ip %s", connName, conn.StaticExternal.Link.Switch.IP)
		}

		subnetsReq[ipNet.String()] = true

		for _, subnet := range conn.StaticExternal.Link.Switch.Subnets {
			subnetsReq[subnet] = true
		}
	}

	th5WorkaroundReqs := map[string]bool{}
	var spSpec *wiringapi.SwitchProfileSpec

	if sw.Spec.Profile != "" {
		sp := &wiringapi.SwitchProfile{}
		err = r.Get(ctx, ktypes.NamespacedName{Namespace: sw.Namespace, Name: sw.Spec.Profile}, sp)
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting switch profile")
		}

		spSpec = &sp.Spec
		// TODO validate using current switch profile
	}
	if spSpec != nil && spSpec.SwitchSilicon == switchprofile.SiliconBroadcomTH5 {
		// TODO: Verify whether we need to do this also for fabric links
		for _, conn := range conns {
			if conn.Mesh != nil {
				for _, link := range conn.Mesh.Links {
					if link.Leaf1.DeviceName() == sw.Name {
						th5WorkaroundReqs[link.Leaf1.LocalPortName()] = true
					} else {
						th5WorkaroundReqs[link.Leaf2.LocalPortName()] = true
					}
				}
			} else if conn.Gateway != nil {
				for _, link := range conn.Gateway.Links {
					th5WorkaroundReqs[link.Switch.LocalPortName()] = true
				}
			}
		}
	}

	err = r.libr.CatalogForSwitch(ctx, r.Client, cat, sw, loWorkaroundLinks, loWorkaroundReqs, externalsReq, attachedRemotePeerings, proxyStaticExtAttachments, subnetsReq, th5WorkaroundReqs)
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error getting switch catalog")
	}

	userCreds := []agentapi.UserCreds{}
	for _, user := range r.cfg.Users {
		userCreds = append(userCreds, agentapi.UserCreds{
			Name:     user.Name,
			Password: user.Password,
			Role:     user.Role,
			SSHKeys:  user.SSHKeys,
		})
	}

	alloyCfg := alloy.Config{
		Hostname: sw.Name,
		ProxyURL: r.cfg.ControlProxyURL,
		Targets:  r.cfg.AlloyTargets,
		Scrapes:  map[string]alloy.Scrape{
			// TODO make it configurable
			// "alloy": {
			// 	Self: alloy.ScrapeSelf{
			// 		Enable: true,
			// 	},
			// 	IntervalSeconds: 120,
			// },
		},
		LogFiles: map[string]alloy.LogFile{},
	}
	if r.cfg.Observability.Agent.Metrics {
		alloyCfg.Scrapes["agent"] = alloy.Scrape{
			Address:         net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", fmeta.AgentExporterPort)),
			Relabel:         r.cfg.Observability.Agent.MetricsRelabel,
			IntervalSeconds: r.cfg.Observability.Agent.MetricsInterval,
		}
	}
	if r.cfg.Observability.Agent.Logs {
		alloyCfg.LogFiles["agent"] = alloy.LogFile{
			PathTargets: []alloy.LogFilePathTarget{
				{
					Path: "/var/log/agent.log",
				},
			},
		}
	}
	if r.cfg.Observability.Unix.Metrics {
		alloyCfg.Scrapes["node"] = alloy.Scrape{
			Unix: alloy.ScrapeUnix{
				Enable:     true,
				Collectors: r.cfg.Observability.Unix.MetricsCollectors,
			},
			Relabel:         r.cfg.Observability.Unix.MetricsRelabel,
			IntervalSeconds: r.cfg.Observability.Unix.MetricsInterval,
		}
	}
	if r.cfg.Observability.Unix.Syslog {
		alloyCfg.LogFiles["syslog"] = alloy.LogFile{
			PathTargets: []alloy.LogFilePathTarget{
				{
					Path: "/var/log/syslog",
				},
			},
		}
	}

	swAnns := map[string]string{}
	for k, v := range sw.Annotations {
		if !strings.Contains(k, "githedgehog.com/") {
			continue
		}

		swAnns[k] = v
	}

	agent := &agentapi.Agent{ObjectMeta: switchNsName}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, agent, func() error {
		if err := setOwner(sw, agent, r.Scheme()); err != nil {
			return err
		}

		agent.Annotations = swAnns
		agent.Labels = sw.Labels
		agent.Spec.Role = sw.Spec.Role
		agent.Spec.Description = sw.Spec.Description

		agent.Spec.Switch = sw.Spec
		agent.Spec.SwitchProfile = spSpec
		agent.Spec.Switches = switches
		agent.Spec.RedundancyGroupPeers = rgPeers
		agent.Spec.Connections = conns
		agent.Spec.VPCs = vpcs
		agent.Spec.VPCAttachments = attaches
		agent.Spec.VPCPeerings = peerings
		agent.Spec.IPv4Namespaces = ipv4Namespaces
		agent.Spec.VLANNamespaces = vlanNamespaces
		agent.Spec.Externals = externals
		agent.Spec.ExternalAttachments = externalAttaches
		agent.Spec.ExternalPeerings = externalPeerings
		agent.Spec.RemotePeerings = remotePeerings
		agent.Spec.ConfiguredVPCSubnets = configuredSubnets
		agent.Spec.AttachedVPCs = attachedVPCs
		agent.Spec.Users = userCreds

		agent.Spec.Version.CA = r.regCA
		agent.Spec.Version.Username = r.regUsername
		agent.Spec.Version.Password = r.regPassword

		agent.Spec.Version.Default = version.Version
		agent.Spec.Version.Repo = r.cfg.AgentRepo

		agent.Spec.Version.AlloyRepo = r.cfg.AlloyRepo
		agent.Spec.Version.AlloyVersion = r.cfg.AlloyVersion

		agent.Spec.Catalog = *cat

		agent.Spec.BenchTouch = benchTouch

		agent.Spec.StatusUpdates = statusUpdates

		agent.Spec.Config = agentapi.AgentSpecConfig{
			DeploymentID:          r.cfg.DeploymentID,
			ControlVIP:            r.cfg.ControlVIP,
			BaseVPCCommunity:      r.cfg.BaseVPCCommunity,
			VPCLoopbackSubnet:     r.cfg.VPCLoopbackSubnet,
			FabricMTU:             r.cfg.FabricMTU,
			ServerFacingMTUOffset: r.cfg.ServerFacingMTUOffset,
			ESLAGMACBase:          r.cfg.ESLAGMACBase,
			ESLAGESIPrefix:        r.cfg.ESLAGESIPrefix,
			DefaultMaxPathsEBGP:   r.cfg.DefaultMaxPathsEBGP,
			GatewayASN:            domain.GatewayASN,
			SpineASN:              domain.SpineASN,
			Domains:               fabric.Domains,
			LoopbackWorkaround:    r.cfg.LoopbackWorkaround,
			ProtocolSubnet:        r.cfg.ProtocolSubnet,
			VTEPSubnet:            r.cfg.VTEPSubnet,
			FabricSubnet:          r.cfg.FabricSubnet,
			ProxyExternalSubnet:   r.cfg.L2ProxyExternalSubnet,
			DisableBFD:            fabric.DisableBFD,
			GatewayBFD:            !fabric.DisableBFD,
			Alloy:                 alloyCfg,
			GatewayCommunities:    map[string]string{},
		}
		if r.cfg.FabricMode == fmeta.FabricModeSpineLeaf {
			agent.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		}
		for idx, val := range r.cfg.GatewayCommunities {
			agent.Spec.Config.GatewayCommunities[fmt.Sprintf("%d", idx)] = val
		}

		return nil
	})
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error creating agent")
	}

	l.Info("agent reconciled")

	return kctrl.Result{}, nil
}

// prepareAgentInfra creates the agent credentials, all of them garbage collected with the switch
func (r *AgentReconciler) prepareAgentInfra(ctx context.Context, sw *wiringapi.Switch) (*kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	saName := AgentServiceAccount(sw.Name)
	sa := &corev1.ServiceAccount{ObjectMeta: kmetav1.ObjectMeta{Namespace: sw.Namespace, Name: saName}}
	_, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		return setOwner(sw, sa, r.Scheme())
	})
	if err != nil {
		return nil, fmt.Errorf("creating service account: %w", err)
	}

	role := &rbacv1.Role{ObjectMeta: kmetav1.ObjectMeta{Namespace: sw.Namespace, Name: saName}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		if err := setOwner(sw, role, r.Scheme()); err != nil {
			return err
		}

		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents"},
				ResourceNames: []string{sw.Name},
				Verbs:         []string{"get", "watch"},
			},
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents/status"},
				ResourceNames: []string{sw.Name},
				Verbs:         []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating role: %w", err)
	}

	roleBinding := &rbacv1.RoleBinding{ObjectMeta: kmetav1.ObjectMeta{Namespace: sw.Namespace, Name: saName}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
		if err := setOwner(sw, roleBinding, r.Scheme()); err != nil {
			return err
		}

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
		return nil, fmt.Errorf("creating role binding: %w", err)
	}

	tokenSecret := &corev1.Secret{ObjectMeta: kmetav1.ObjectMeta{Namespace: sw.Namespace, Name: saName + "-satoken"}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, tokenSecret, func() error {
		if err := setOwner(sw, tokenSecret, r.Scheme()); err != nil {
			return err
		}

		if tokenSecret.Annotations == nil {
			tokenSecret.Annotations = map[string]string{}
		}

		tokenSecret.Annotations[corev1.ServiceAccountNameKey] = saName
		tokenSecret.Type = corev1.SecretTypeServiceAccountToken

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating token secret: %w", err)
	}

	// we don't yet have service account token for the agent
	if len(tokenSecret.Data) < 3 {
		// TODO is it the best we can do? or should we do few in-place retries?
		l.Info("requeue to wait for service account token")

		return &kctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	kubeconfig, err := r.genKubeconfig(tokenSecret)
	if err != nil {
		return nil, fmt.Errorf("generating kubeconfig: %w", err)
	}

	secretName := AgentKubeconfigSecret(sw.Name)
	kubeconfigSecret := &corev1.Secret{ObjectMeta: kmetav1.ObjectMeta{Namespace: sw.Namespace, Name: secretName}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, kubeconfigSecret, func() error {
		if err := setOwner(sw, kubeconfigSecret, r.Scheme()); err != nil {
			return err
		}

		// not StringData: it's write-only, so it never matches what's stored and the secret would be rewritten on
		// every reconcile
		kubeconfigSecret.Data = map[string][]byte{
			AgentKubeconfigKey: []byte(kubeconfig),
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating kubeconfig secret: %w", err)
	}

	return nil, nil //nolint: nilnil
}

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
		Server: r.cfg.APIServer,
		CA:     base64.StdEncoding.EncodeToString(secret.Data[corev1.ServiceAccountRootCAKey]),
		Token:  string(secret.Data[corev1.ServiceAccountTokenKey]),
	})
	if err != nil {
		return "", errors.Wrapf(err, "error executing kubeconfig template")
	}

	return buf.String(), nil
}

// getSwitches gets the named switches one by one rather than copying all of them out of the cache. They're the
// neighbors of a switch, so they must exist: connections refuse missing switches and connected ones can't be deleted.
func (r *AgentReconciler) getSwitches(ctx context.Context, namespace string, names map[string]bool) (map[string]*wiringapi.Switch, error) {
	switches := make(map[string]*wiringapi.Switch, len(names))
	for name := range names {
		sw := &wiringapi.Switch{}
		if err := r.Get(ctx, ktypes.NamespacedName{Namespace: namespace, Name: name}, sw); err != nil {
			return nil, fmt.Errorf("getting switch %s: %w", name, err)
		}

		switches[name] = sw
	}

	return switches, nil
}

// redundancyGroupPeers returns the other switches of the switch's redundancy group, sorted so that the agent spec
// doesn't change with the order they're listed in. They're found by the label of the group, which the redundancy
// group is always one of.
func (r *AgentReconciler) redundancyGroupPeers(ctx context.Context, sw *wiringapi.Switch) ([]string, error) {
	peers := []string{}

	group := sw.Spec.Redundancy.Group
	if group == "" {
		return peers, nil
	}

	sws := &wiringapi.SwitchList{}
	if err := r.List(ctx, sws, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForSwitchGroup(group)); err != nil {
		return nil, fmt.Errorf("listing switches of group %s: %w", group, err)
	}

	for _, other := range sws.Items {
		if other.Name == sw.Name || other.Spec.Redundancy.Group != group {
			continue
		}
		if other.Spec.Redundancy.Type != sw.Spec.Redundancy.Type {
			return nil, fmt.Errorf("switch %s and %s have different redundancy types but are in the same redundancy group", sw.Name, other.Name) //nolint:err113
		}

		peers = append(peers, other.Name)
	}
	slices.Sort(peers)

	return peers, nil
}

// listByConnections lists the objects labeled with one of the connections, as VPC and external attachments are, so
// that a switch only gets the attachments of its own connections and not a copy of all of them. The list is left
// empty without any connections.
func (r *AgentReconciler) listByConnections(ctx context.Context, namespace string, list kclient.ObjectList, conns map[string]wiringapi.ConnectionSpec) error {
	if len(conns) == 0 {
		return nil
	}

	names := make([]string, 0, len(conns))
	for name := range conns {
		if errs := kvalidation.IsValidLabelValue(name); len(errs) > 0 {
			return fmt.Errorf("connection name %s isn't a valid label value, which attachments refer to it by: %s", name, strings.Join(errs, ", ")) //nolint:err113
		}

		names = append(names, name)
	}

	req, err := labels.NewRequirement(wiringapi.LabelConnection, selection.In, names)
	if err != nil {
		return fmt.Errorf("selecting by connections: %w", err)
	}

	if err := r.List(ctx, list, kclient.InNamespace(namespace), kclient.MatchingLabelsSelector{Selector: labels.NewSelector().Add(*req)}); err != nil {
		return fmt.Errorf("listing by connections: %w", err)
	}

	return nil
}

// maxBenchTouch returns the max of cur and the obj's bench touch label value, ignoring missing or
// unparseable labels. Max (not last-seen) keeps the value stable regardless of reconcile order.
func maxBenchTouch(cur int64, obj kmetav1.Object) int64 {
	val, ok := obj.GetLabels()[fmeta.BenchTouchLabel]
	if !ok {
		return cur
	}

	ts, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return cur
	}

	return max(cur, ts)
}

func appendUpdate(statusUpdates []agentapi.ApplyStatusUpdate, obj kclient.Object) []agentapi.ApplyStatusUpdate {
	return append(statusUpdates, agentapi.ApplyStatusUpdate{
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Generation: obj.GetGeneration(),
	})
}
