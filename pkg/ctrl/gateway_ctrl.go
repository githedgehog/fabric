// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/fabric/api/gwint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	configVolumeName       = "config"
	frrRunVolumeName       = "frr-run"
	dataplaneTmpVolumeName = "dataplane-tmp"
	frrTmpVolumeName       = "frr-tmp"

	// /var/run is a symlink to /run in the dataplane image, nothing outside of the pod uses the sockets, so the dataplane
	// ones stay in its container and only the FRR ones are shared with the exporter
	dataplaneRunMountPath = "/run/dataplane"
	frrRunMountPath       = "/run/frr"
	cpiSocket             = "hh/dataplane.sock"
	frrAgentSocket        = "frr-agent.sock"
)

// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaypeerings,verbs=get;list;watch
// +kubebuilder:rbac:groups=wiring.githedgehog.com,resources=fabrics,verbs=get;list;watch

// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.cattle.io,resources=helmcharts,verbs=get;list;watch;create;update;patch;delete

type GatewayReconciler struct {
	kclient.Client
	cfg  *meta.FabricConfig
	lock *Lock
}

func SetupGatewayReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig, lock *Lock) error {
	if cfg == nil {
		return fmt.Errorf("gateway controller config is nil") //nolint:err113
	}
	if lock == nil {
		return fmt.Errorf("lock is nil") //nolint:err113
	}

	r := &GatewayReconciler{
		Client: mgr.GetClient(),
		cfg:    cfg,
		lock:   lock,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("Gateway").
		For(&gwapi.Gateway{}).
		// recreated if deleted, garbage collected with the gateway otherwise
		Owns(&gwintapi.GatewayAgent{}, builder.WithPredicates(onlyDeletes)).
		Owns(&rbacv1.Role{}, builder.WithPredicates(onlyDeletes)).
		Owns(&rbacv1.RoleBinding{}, builder.WithPredicates(onlyDeletes)).
		// in the gateway namespace, out of reach of owner references, so they're found by label
		Watches(&appv1.DaemonSet{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForDeployed), builder.WithPredicates(onlyDeletes)).
		Watches(&corev1.ServiceAccount{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForDeployed), builder.WithPredicates(onlyDeletes)).
		Watches(&gwapi.Gateway{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForGateway)).
		Watches(&gwapi.GatewayPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForPeering)).
		Watches(&gwapi.VPCInfo{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForVPCInfo)).
		Watches(&wiringapi.Fabric{}, handler.EnqueueRequestsFromMapFunc(r.enqueueForFabric)).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

// enqueueGateways enqueues the gateways of the fabric, only the ones in the given domains if any. No fabric means
// all gateways.
func (r *GatewayReconciler) enqueueGateways(ctx context.Context, fabric string, domains []string) []reconcile.Request {
	// all gateways are queued and reconciled once unlocked, see lockedRequeueAfter
	if r.lock.Locked() {
		return nil
	}

	opts := []kclient.ListOption{}
	if fabric != "" {
		opts = append(opts, kclient.MatchingLabels{wiringapi.ListLabelFabric(fabric): gwapi.ListLabelValue})
	}

	gws := &gwapi.GatewayList{}
	if err := r.List(ctx, gws, opts...); err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing gateways to reconcile", "fabric", fabric, "domains", domains)

		return nil
	}

	res := []reconcile.Request{}
	for _, gw := range gws.Items {
		if len(domains) > 0 && !slices.Contains(domains, gw.Spec.Topology.Domain) {
			continue
		}

		res = append(res, reconcile.Request{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		})
	}

	return res
}

// a gateway is part of the groups of the other gateways in its fabric and domain
func (r *GatewayReconciler) enqueueForGateway(ctx context.Context, obj kclient.Object) []reconcile.Request {
	gw, ok := obj.(*gwapi.Gateway)
	if !ok {
		return nil
	}

	return r.enqueueGateways(ctx, gw.Spec.Topology.Fabric, []string{gw.Spec.Topology.Domain})
}

// peerings have no domain of their own, they go to all gateways of the fabric that have the peered VPCs
func (r *GatewayReconciler) enqueueForPeering(ctx context.Context, obj kclient.Object) []reconcile.Request {
	peering, ok := obj.(*gwapi.GatewayPeering)
	if !ok {
		return nil
	}

	return r.enqueueGateways(ctx, peering.Spec.Topology.Fabric, nil)
}

// a VPCInfo has the topology of its VPC or External
func (r *GatewayReconciler) enqueueForVPCInfo(ctx context.Context, obj kclient.Object) []reconcile.Request {
	vpcInfo, ok := obj.(*gwapi.VPCInfo)
	if !ok {
		return nil
	}

	return r.enqueueGateways(ctx, vpcInfo.Spec.Topology.Fabric, vpcInfo.Spec.Topology.Domains)
}

func (r *GatewayReconciler) enqueueForFabric(ctx context.Context, obj kclient.Object) []reconcile.Request {
	return r.enqueueGateways(ctx, obj.GetName(), nil)
}

// LabelGateway marks what's deployed for a gateway in the gateway namespace, which owner references can't reach
var LabelGateway = wiringapi.LabelName("gateway")

func setGatewayLabel(obj kclient.Object, gw *gwapi.Gateway) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[LabelGateway] = gw.Name
	obj.SetLabels(labels)
}

// enqueueForDeployed reconciles the gateway something in the gateway namespace was deployed for. Not silenced while
// locked like the other mappers: it's a single request without any fan-out and the refresh never triggers it.
func (r *GatewayReconciler) enqueueForDeployed(_ context.Context, obj kclient.Object) []reconcile.Request {
	name := obj.GetLabels()[LabelGateway]
	if name == "" {
		return nil
	}

	return []reconcile.Request{{Namespace: kmetav1.NamespaceDefault, Name: name}}
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	if r.lock.Locked() {
		return kctrl.Result{RequeueAfter: lockedRequeueAfter}, nil
	}

	if req.Namespace != kmetav1.NamespaceDefault {
		l.Info("Skipping Gateway in non-default namespace")

		return kctrl.Result{}, nil
	}

	gw := &gwapi.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gw); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting gateway: %w", err)
	}

	if gw.DeletionTimestamp != nil {
		if !ctrlutil.ContainsFinalizer(gw, CleanupFinalizer) {
			return kctrl.Result{}, nil
		}

		l.Info("Gateway is being deleted, cleaning up")

		if err := r.cleanupGateway(ctx, gw); err != nil {
			return kctrl.Result{}, fmt.Errorf("cleaning up gateway: %w", err)
		}

		ctrlutil.RemoveFinalizer(gw, CleanupFinalizer)
		if err := r.Update(ctx, gw); err != nil {
			return kctrl.Result{}, fmt.Errorf("removing gateway finalizer: %w", err)
		}

		return kctrl.Result{}, nil
	}

	{
		orig := gw.DeepCopy()
		gw.Default()
		// the gateway namespace isn't reachable by owner references
		ctrlutil.AddFinalizer(gw, CleanupFinalizer)
		if !reflect.DeepEqual(orig, gw) {
			l.Info("Applying defaults and finalizer to Gateway")

			if err := r.Update(ctx, gw); err != nil {
				return kctrl.Result{}, fmt.Errorf("updating gateway: %w", err)
			}
		}
	}

	l.Info("Reconciling Gateway")

	newGwAg, err := BuildGatewayAgent(ctx, r.Client, r.cfg, gw)
	if err != nil {
		if errors.Is(err, ErrRetryLater) {
			return kctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}

		return kctrl.Result{}, fmt.Errorf("building gateway agent: %w", err)
	}

	// we intentionally manage gateway agent in the default namespace
	gwAg := &gwintapi.GatewayAgent{Namespace: kmetav1.NamespaceDefault, Name: gw.Name}
	if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, gwAg, func() error {
		if err := setOwner(gw, gwAg, r.Scheme()); err != nil {
			return err
		}

		gwAg.Spec = newGwAg.Spec

		return nil
	}); err != nil {
		return kctrl.Result{}, fmt.Errorf("creating or updating gateway agent: %w", err)
	}

	if err := r.deployGateway(ctx, gw); err != nil {
		if errors.Is(err, ErrRetryLater) {
			l.Info("Deploying gateway later", "reason", err.Error())

			return kctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}

		return kctrl.Result{}, fmt.Errorf("deploying gateway: %w", err)
	}

	return kctrl.Result{}, nil
}

var ErrRetryLater = fmt.Errorf("retry later")

// cleanupGateway deletes what's deployed for the gateway in the gateway namespace, the rest in the gateway's own
// namespace is garbage collected with it
func (r *GatewayReconciler) cleanupGateway(ctx context.Context, gw *gwapi.Gateway) error {
	dsNames := append([]string{gwapi.DaemonSetName(gw.Name)}, oldGatewayDSNames(gw.Name)...)
	objs := []kclient.Object{} //nolint:prealloc
	for _, name := range dsNames {
		objs = append(objs, &appv1.DaemonSet{Namespace: r.cfg.GatewayNamespace, Name: name})
	}
	objs = append(objs, &corev1.ServiceAccount{Namespace: r.cfg.GatewayNamespace, Name: entityName(gw.Name)})
	for _, obj := range objs {
		if err := r.Delete(ctx, obj); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("deleting %T %s: %w", obj, obj.GetName(), err)
		}
	}

	return nil
}

func BuildGatewayAgent(ctx context.Context, kube kclient.Reader, cfg *meta.FabricConfig, gw *gwapi.Gateway) (*gwintapi.GatewayAgent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is nil") //nolint:err113
	}
	if gw == nil {
		return nil, fmt.Errorf("gw is nil") //nolint:err113
	}

	fabric, err := wiringapi.GetFabricSpec(ctx, kube, gw.Namespace, gw.Spec.Topology.Fabric)
	if err != nil {
		return nil, fmt.Errorf("getting gateway fabric: %w", err)
	}
	// selecting by an empty domain's label matches nothing and would leave the gateway with an empty config
	if gw.Spec.Topology.Domain == "" {
		return nil, fmt.Errorf("gateway %s has no domain", gw.Name) //nolint:err113
	}

	// the gateway only gets what's in its own fabric and domain
	gwFabric := gw.Spec.Topology.Fabric
	inFabric := kclient.MatchingLabels{
		wiringapi.ListLabelFabric(gwFabric): gwapi.ListLabelValue,
	}
	inDomain := kclient.MatchingLabels{
		wiringapi.ListLabelFabric(gwFabric):                gwapi.ListLabelValue,
		wiringapi.ListLabelDomain(gw.Spec.Topology.Domain): gwapi.ListLabelValue,
	}

	inGwGroups := map[string]bool{}
	for _, gr := range gw.Spec.Groups {
		inGwGroups[gr.Name] = true
	}
	gwGroups := map[string]gwintapi.GatewayGroupInfo{}
	// groups are per fabric and domain
	gws := &gwapi.GatewayList{}
	if err := kube.List(ctx, gws, inDomain); err != nil {
		return nil, fmt.Errorf("listing gateways: %w", err)
	}
	for _, gw := range gws.Items {
		// a gateway being deleted is no longer a member
		if gw.DeletionTimestamp != nil {
			continue
		}

		for _, gr := range gw.Spec.Groups {
			if !inGwGroups[gr.Name] {
				continue
			}

			info := gwGroups[gr.Name]
			info.Members = append(info.Members, gwintapi.GatewayGroupMember{
				Name:     gw.Name,
				Priority: gr.Priority,
				VTEPIP:   gw.Spec.VTEPIP,
			})
			gwGroups[gr.Name] = info
		}
	}
	for _, info := range gwGroups {
		slices.SortFunc(info.Members, func(a, b gwintapi.GatewayGroupMember) int {
			if a.Priority == b.Priority {
				return strings.Compare(a.Name, b.Name)
			}

			return cmp.Compare(a.Priority, b.Priority)
		})
	}

	// a VPC in more than one domain has the labels of all of them
	vpcList := &gwapi.VPCInfoList{}
	if err := kube.List(ctx, vpcList, inDomain); err != nil {
		return nil, fmt.Errorf("listing vpcinfos: %w", err)
	}
	vpcs := map[string]gwintapi.VPCInfoData{}
	for _, vpc := range vpcList.Items {
		if !vpc.IsReady() {
			// TODO consider ignoring non-ready VPCs
			slog.Info("VPC not ready while building gateway agent, retrying", "gateway", gw.Name, "vpc", vpc.Name, "ns", vpc.Namespace)

			return nil, fmt.Errorf("vpcinfo not ready: %s: %w", vpc.Name, ErrRetryLater)
		}
		vpcs[vpc.Name] = gwintapi.VPCInfoData{
			VPCInfoSpec:   vpc.Spec,
			VPCInfoStatus: vpc.Status,
		}
	}

	// peerings have no domain of their own, only the ones of the VPCs in the gateway's domain are kept below
	peeringList := &gwapi.GatewayPeeringList{}
	if err := kube.List(ctx, peeringList, inFabric); err != nil {
		return nil, fmt.Errorf("listing peerings: %w", err)
	}
	peerings := map[string]gwintapi.PeeringSpecAnn{}
	for _, peering := range peeringList.Items {
		missingVPC := false

		for peerVPC := range peering.Spec.Peering {
			if _, exists := vpcs[peerVPC]; !exists {
				// expected for the peerings of the VPCs in the other domains of the fabric
				slog.Debug("Peered VPC not found while building gateway agent, skipping", "gateway", gw.Name, "peering", peering.Name, "vpc", peerVPC, "ns", peering.Namespace)

				missingVPC = true

				break
			}
		}

		if missingVPC {
			continue
		}

		peerings[peering.Name] = peeringSpecAnn(&peering)
	}

	comms := map[string]string{}
	for id, comm := range cfg.GatewayCommunities {
		comms[strconv.FormatUint(uint64(id), 10)] = comm
	}

	gwAg := &gwintapi.GatewayAgent{
		Namespace: kmetav1.NamespaceDefault, Name: gw.Name,
		Spec: gwintapi.GatewayAgentSpec{
			AgentVersion: "",
			Gateway:      gw.Spec,
			VPCs:         vpcs,
			Peerings:     peerings,
			Groups:       gwGroups,
			Communities:  comms,
			Config: gwintapi.GatewayAgentSpecConfig{
				FabricBFD: !fabric.DisableBFD,
			},
		},
	}

	return gwAg, nil
}

// BuildGatewayAgentForPeering builds a GatewayAgent for the first gateway that matches the peering gateway group.
func BuildGatewayAgentForPeering(ctx context.Context, kube kclient.Reader, cfg *meta.FabricConfig, peering *gwapi.GatewayPeering) (*gwintapi.GatewayAgent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is nil") //nolint:err113
	}
	if peering == nil {
		return nil, fmt.Errorf("peering is nil") //nolint:err113
	}
	// selecting by an empty fabric's label matches no gateway
	if peering.Spec.Topology.Fabric == "" {
		return nil, fmt.Errorf("peering %s has no fabric", peering.Name) //nolint:err113
	}

	gws := &gwapi.GatewayList{}
	if err := kube.List(ctx, gws, kclient.MatchingLabels{
		wiringapi.ListLabelFabric(peering.Spec.Topology.Fabric): gwapi.ListLabelValue,
	}); err != nil {
		return nil, fmt.Errorf("listing gateways: %w", err)
	}
	slices.SortFunc(gws.Items, func(a, b gwapi.Gateway) int {
		return strings.Compare(a.Name, b.Name)
	})

	for _, gw := range gws.Items {
		for _, group := range gw.Spec.Groups {
			if group.Name == peering.Spec.GatewayGroup {
				ag, err := BuildGatewayAgent(ctx, kube, cfg, &gw)
				if err != nil {
					return nil, err
				}

				// make sure the peering that is currently being processed is added to the agent
				ag.Spec.Peerings[peering.Name] = peeringSpecAnn(peering)

				return ag, nil
			}
		}
	}

	return nil, fmt.Errorf("gateway not found for gateway group: %s", peering.Spec.GatewayGroup) //nolint:err113
}

func peeringSpecAnn(peering *gwapi.GatewayPeering) gwintapi.PeeringSpecAnn {
	res := gwintapi.PeeringSpecAnn{PeeringSpec: peering.Spec}
	for k, v := range peering.Annotations {
		if !strings.Contains(k, "githedgehog.com/") {
			continue
		}
		// set by the peering defaulting from its spec
		if k == gwapi.LabelVPCs || k == gwapi.LabelNAT {
			continue
		}

		if res.Annotations == nil {
			res.Annotations = map[string]string{}
		}
		res.Annotations[k] = v
	}

	return res
}

func dataplaneIfaceArgs(gw *gwapi.Gateway) []string {
	args := []string{}
	for _, ifaceName := range slices.Sorted(maps.Keys(gw.Spec.Interfaces)) {
		iface := gw.Spec.Interfaces[ifaceName]
		val := ifaceName
		switch {
		case gw.Spec.Driver == gwapi.GatewayDriverDPDK:
			// kernel name is ignored with the dpdk driver
			val += "=pci@" + iface.PCI
		case iface.Kernel != "" && iface.Kernel != ifaceName:
			val += "=kernel@" + iface.Kernel
		}
		if iface.MTU > 0 {
			val += fmt.Sprintf("/mtu=%d", iface.MTU)
		}
		args = append(args, "--interface", val)
	}

	return append(args, "--driver", string(gw.Spec.Driver))
}

// dataplaneHugePages returns the dataplane container resources and args for the configured hugepages, the dataplane
// currently only uses 1G pages and only with the dpdk driver
func dataplaneHugePages(cfg *meta.FabricConfig, gw *gwapi.Gateway) (corev1.ResourceRequirements, []string) {
	limits := corev1.ResourceList{} //nolint:exhaustive
	if cfg.GatewayHugePages1G > 0 {
		limits[corev1.ResourceHugePagesPrefix+"1Gi"] = quantity(cfg.GatewayHugePages1G << 30) // N × 1Gi
	}
	if cfg.GatewayHugePages2M > 0 {
		limits[corev1.ResourceHugePagesPrefix+"2Mi"] = quantity(cfg.GatewayHugePages2M << 21) // N × 2Mi
	}
	if len(limits) == 0 {
		return corev1.ResourceRequirements{}, nil
	}

	requests := maps.Clone(limits)
	// hugepages require a cpu or memory request, there is no memory limit on purpose
	requests[corev1.ResourceMemory] = quantity(1 << 30) // 1Gi

	var args []string
	if gw.Spec.Driver == gwapi.GatewayDriverDPDK && cfg.GatewayHugePages1G > 0 {
		args = []string{"--hugepages-gib", strconv.FormatInt(cfg.GatewayHugePages1G, 10)}
	}

	return corev1.ResourceRequirements{Limits: limits, Requests: requests}, args
}

func quantity(bytes int64) resource.Quantity {
	return *resource.NewQuantity(bytes, resource.BinarySI)
}

// oldGatewayDSNames are the daemonsets deployed for a gateway before FRR moved into the dataplane pod
func oldGatewayDSNames(gwName string) []string {
	return []string{entityName(gwName, "dataplane"), entityName(gwName, "frr")}
}

func entityName(gwName string, t ...string) string {
	if len(t) == 0 {
		return fmt.Sprintf("gw-%s", gwName)
	}

	return fmt.Sprintf("gw--%s--%s", gwName, strings.Join(t, "-"))
}

func (r *GatewayReconciler) deployGateway(ctx context.Context, gw *gwapi.Gateway) error {
	saName := entityName(gw.Name)

	{
		sa := &corev1.ServiceAccount{
			Namespace: r.cfg.GatewayNamespace,
			Name:      saName,
		}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
			setGatewayLabel(sa, gw)

			return nil
		}); err != nil {
			return fmt.Errorf("creating service account: %w", err)
		}

		role := &rbacv1.Role{
			Namespace: kmetav1.NamespaceDefault,
			Name:      saName,
		}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
			if err := setOwner(gw, role, r.Scheme()); err != nil {
				return err
			}

			role.Rules = []rbacv1.PolicyRule{
				{
					APIGroups:     []string{gwintapi.GroupVersion.Group},
					Resources:     []string{"gatewayagents"},
					ResourceNames: []string{gw.Name},
					Verbs:         []string{"get", "watch"},
				},
				{
					APIGroups:     []string{gwintapi.GroupVersion.Group},
					Resources:     []string{"gatewayagents/status"},
					ResourceNames: []string{gw.Name},
					Verbs:         []string{"get", "update", "patch"},
				},
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating role: %w", err)
		}

		roleBinding := &rbacv1.RoleBinding{
			Namespace: kmetav1.NamespaceDefault,
			Name:      saName,
		}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
			if err := setOwner(gw, roleBinding, r.Scheme()); err != nil {
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
		}); err != nil {
			return fmt.Errorf("creating role binding: %w", err)
		}
	}

	replaceUpdateStrategy := appv1.DaemonSetUpdateStrategy{
		Type: appv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appv1.RollingUpdateDaemonSet{
			MaxUnavailable: new(intstr.FromInt(1)),
			MaxSurge:       new(intstr.FromInt(0)),
		},
	}

	// the old dataplane and frr daemonsets have to be gone before the new one starts, both dataplanes would take the same
	// interfaces otherwise
	oldLeft := false
	for _, name := range oldGatewayDSNames(gw.Name) {
		ds := &appv1.DaemonSet{}
		if err := r.Get(ctx, kclient.ObjectKey{Namespace: r.cfg.GatewayNamespace, Name: name}, ds); err != nil {
			if kapierrors.IsNotFound(err) {
				continue
			}

			return fmt.Errorf("getting old gateway daemonset %s: %w", name, err)
		}

		oldLeft = true
		if ds.DeletionTimestamp != nil {
			continue
		}
		if err := r.Delete(ctx, ds, kclient.PropagationPolicy(kmetav1.DeletePropagationForeground)); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("deleting old gateway daemonset %s: %w", name, err)
		}
	}
	if oldLeft {
		return fmt.Errorf("waiting for old gateway daemonsets to be deleted: %w", ErrRetryLater)
	}

	{
		args := []string{
			"--num-workers", fmt.Sprintf("%d", gw.Spec.Workers),
			"--cli-sock-path", filepath.Join(dataplaneRunMountPath, "cli.sock"),
			"--cpi-sock-path", filepath.Join(frrRunMountPath, cpiSocket),
			"--frr-agent-path", filepath.Join(frrRunMountPath, frrAgentSocket),
			"--metrics-address", fmt.Sprintf("127.0.0.1:%d", r.cfg.DataplaneMetricsPort),
			"--bmp-enable",
			"--bmp-address", "127.0.0.1:5000", // TODO: make it available via config
			"--bmp-interval", "10000",
			"--supervise-frr",
		}
		if gw.Spec.Profiling.Enabled {
			// TODO: should it become http://alloy-gw.fab.svc.cluster.local:4040
			args = append(args, "--pyroscope-url", "http://localhost:4040")
		}

		args = append(args, dataplaneIfaceArgs(gw)...)

		resources, hugePagesArgs := dataplaneHugePages(r.cfg, gw)
		args = append(args, hugePagesArgs...)

		frrRunMount := corev1.VolumeMount{
			Name:      frrRunVolumeName,
			MountPath: frrRunMountPath,
		}

		dpDS := &appv1.DaemonSet{
			Namespace: r.cfg.GatewayNamespace,
			Name:      gwapi.DaemonSetName(gw.Name),
		}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, dpDS, func() error {
			setGatewayLabel(dpDS, gw)

			labels := map[string]string{
				"app.kubernetes.io/name": dpDS.Name,
			}

			dpDS.Spec = appv1.DaemonSetSpec{
				Selector: &kmetav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					Labels: labels,
					Spec: corev1.PodSpec{
						NodeSelector:                  map[string]string{"kubernetes.io/hostname": gw.Name},
						HostNetwork:                   true,
						DNSPolicy:                     corev1.DNSClusterFirstWithHostNet,
						TerminationGracePeriodSeconds: new(int64(10)),
						Tolerations:                   r.cfg.GatewayTolerations,
						ServiceAccountName:            saName,
						InitContainers: []corev1.Container{
							// FRR used to run in the host network namespace and could leave the VTEP IP on lo
							{
								Name:    "flush-vtepip",
								Image:   r.cfg.DataplaneRef,
								Command: []string{"/bin/bash", "-c", "--"},
								Args: []string{
									fmt.Sprintf("set -ex; addr=$(ip -o addr show dev lo to %[1]s); if [ -n \"$addr\" ]; then ip addr del %[1]s dev lo; fi", gw.Spec.VTEPIP),
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: new(true),
									RunAsUser:  new(int64(0)),
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:      "dataplane",
								Image:     r.cfg.DataplaneRef,
								Args:      args,
								Resources: resources,
								SecurityContext: &corev1.SecurityContext{
									Privileged: new(true),
									RunAsUser:  new(int64(0)),
								},
								Env: []corev1.EnvVar{
									{
										Name:  "RUST_BACKTRACE",
										Value: "FULL",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									frrRunMount,
									{
										Name:      dataplaneTmpVolumeName,
										MountPath: "/tmp",
									},
									{
										Name:      frrTmpVolumeName,
										MountPath: "/var/tmp/frr",
									},
								},
							},
							{
								Name:    "frr-exporter",
								Image:   r.cfg.DataplaneRef,
								Command: []string{"/bin/frr_exporter"},
								Args: []string{
									"--web.listen-address", fmt.Sprintf("127.0.0.1:%d", r.cfg.FRRMetricsPort),
									"--frr.socket.dir-path", frrRunMountPath,
									"--no-collector.ospf",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: new(true),
									RunAsUser:  new(int64(0)),
								},
								VolumeMounts: []corev1.VolumeMount{frrRunMount},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name:     frrRunVolumeName,
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
							{
								Name: dataplaneTmpVolumeName,
								// TODO consider memory medium
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
							{
								Name: frrTmpVolumeName,
								// TODO consider memory medium
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
				UpdateStrategy: replaceUpdateStrategy,
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating or updating gateway dataplane daemonset: %w", err)
		}
	}

	return nil
}
