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
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	configVolumeName       = "config"
	dataplaneRunVolumeName = "dataplane-run"
	frrRunVolumeName       = "frr-run"
	frrTmpVolumeName       = "frr-tmp"
	frrRootRunVolumeName   = "frr-root-run"

	dataplaneRunHostPath = "/run/hedgehog/dataplane"
	frrRunHostPath       = "/run/hedgehog/frr"

	dataplaneRunMountPath = "/var/run/dataplane"
	frrRunMountPath       = "/var/run/frr"
	frrRootRunMountPath   = "/run/frr"
	cpiSocket             = "hh/dataplane.sock"
	frrAgentSocket        = "frr-agent.sock"
)

// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaygroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaypeerings,verbs=get;list;watch

// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.cattle.io,resources=helmcharts,verbs=get;list;watch;create;update;patch;delete

type GatewayReconciler struct {
	kclient.Client
	cfg *meta.FabricConfig
}

func SetupGatewayReconcilerWith(mgr kctrl.Manager, cfg *meta.FabricConfig) error {
	if cfg == nil {
		return fmt.Errorf("gateway controller config is nil") //nolint:goerr113
	}

	r := &GatewayReconciler{
		Client: mgr.GetClient(),
		cfg:    cfg,
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("Gateway").
		For(&gwapi.Gateway{}).
		Watches(&gwapi.Gateway{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllGateways)).
		Watches(&gwapi.GatewayPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllGateways)).
		Watches(&gwapi.VPCInfo{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllGateways)).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *GatewayReconciler) enqueueAllGateways(ctx context.Context, obj kclient.Object) []reconcile.Request {
	res := []reconcile.Request{}

	gws := &gwapi.GatewayList{}
	if err := r.List(ctx, gws); err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing gateways to reconcile all")

		return nil
	}

	for _, gw := range gws.Items {
		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}})
	}

	return res
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

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
		l.Info("Gateway is being deleted, skipping")

		return kctrl.Result{}, nil
	}

	{
		defGwGr := &gwapi.GatewayGroup{
			ObjectMeta: kmetav1.ObjectMeta{
				Name:      gwapi.DefaultGatewayGroup,
				Namespace: kmetav1.NamespaceDefault,
			},
		}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, defGwGr, func() error {
			return nil
		}); err != nil {
			return kctrl.Result{}, fmt.Errorf("creating/updating default gateway group: %w", err)
		}

		orig := gw.DeepCopy()
		gw.Default()
		if !reflect.DeepEqual(orig, gw) {
			l.Info("Applying defaults to Gateway")

			if err := r.Update(ctx, gw); err != nil {
				return kctrl.Result{}, fmt.Errorf("updating gateway: %w", err)
			}
		}
	}

	l.Info("Reconciling Gateway")

	newGwAg, err := BuildGatewayAgent(ctx, r.Client, r.cfg, gw)
	if err != nil {
		if errors.Is(err, ErrRetryLater) {
			return kctrl.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
		}

		return kctrl.Result{}, fmt.Errorf("building gateway agent: %w", err)
	}

	// we intentionally manage gateway agent in the default namespace
	gwAg := &gwintapi.GatewayAgent{ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: gw.Name}}
	if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, gwAg, func() error {
		// TODO consider blocking owner deletion, would require foregroundDeletion finalizer on the owner
		gwAg.Spec = newGwAg.Spec

		return nil
	}); err != nil {
		return kctrl.Result{}, fmt.Errorf("creating or updating gateway agent: %w", err)
	}

	if err := r.deployGateway(ctx, gw); err != nil {
		return kctrl.Result{}, fmt.Errorf("deploying gateway: %w", err)
	}

	return kctrl.Result{}, nil
}

var ErrRetryLater = fmt.Errorf("retry later")

func BuildGatewayAgent(ctx context.Context, kube kclient.Reader, cfg *meta.FabricConfig, gw *gwapi.Gateway) (*gwintapi.GatewayAgent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg is nil") //nolint:err113
	}
	if gw == nil {
		return nil, fmt.Errorf("gw is nil") //nolint:err113
	}

	inGwGroups := map[string]bool{}
	for _, gr := range gw.Spec.Groups {
		inGwGroups[gr.Name] = true
	}
	gwGroups := map[string]gwintapi.GatewayGroupInfo{}
	gws := &gwapi.GatewayList{}
	if err := kube.List(ctx, gws); err != nil {
		return nil, fmt.Errorf("listing gateways: %w", err)
	}
	for _, gw := range gws.Items {
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

	vpcList := &gwapi.VPCInfoList{}
	if err := kube.List(ctx, vpcList); err != nil {
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

	peeringList := &gwapi.GatewayPeeringList{}
	if err := kube.List(ctx, peeringList); err != nil {
		return nil, fmt.Errorf("listing peerings: %w", err)
	}
	peerings := map[string]gwapi.PeeringSpec{}
	for _, peering := range peeringList.Items {
		missingVPC := false

		for peerVPC := range peering.Spec.Peering {
			if _, exists := vpcs[peerVPC]; !exists {
				slog.Info("Peered VPC not found while building gateway agent, skipping", "gateway", gw.Name, "peering", peering.Name, "vpc", peerVPC, "ns", peering.Namespace)

				missingVPC = true

				break
			}
		}

		if missingVPC {
			continue
		}

		peerings[peering.Name] = peering.Spec
	}

	comms := map[string]string{}
	for id, comm := range cfg.GatewayCommunities {
		comms[strconv.FormatUint(uint64(id), 10)] = comm
	}

	gwAg := &gwintapi.GatewayAgent{
		ObjectMeta: kmetav1.ObjectMeta{Namespace: kmetav1.NamespaceDefault, Name: gw.Name},
		Spec: gwintapi.GatewayAgentSpec{
			AgentVersion: "",
			Gateway:      gw.Spec,
			VPCs:         vpcs,
			Peerings:     peerings,
			Groups:       gwGroups,
			Communities:  comms,
			Config: gwintapi.GatewayAgentSpecConfig{
				FabricBFD: !cfg.DisableBFD,
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

	gws := &gwapi.GatewayList{}
	if err := kube.List(ctx, gws); err != nil {
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
				ag.Spec.Peerings[peering.Name] = peering.Spec

				return ag, nil
			}
		}
	}

	return nil, fmt.Errorf("gateway not found for gateway group: %s", peering.Spec.GatewayGroup) //nolint:err113
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
		sa := &corev1.ServiceAccount{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: r.cfg.GatewayNamespace,
			Name:      saName,
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error { return nil }); err != nil {
			return fmt.Errorf("creating service account: %w", err)
		}

		role := &rbacv1.Role{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
			Name:      saName,
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
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

		roleBinding := &rbacv1.RoleBinding{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: kmetav1.NamespaceDefault,
			Name:      saName,
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
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
			MaxUnavailable: ptr.To(intstr.FromInt(1)),
			MaxSurge:       ptr.To(intstr.FromInt(0)),
		},
	}

	dataplaneSocketVolume := corev1.Volume{
		Name: dataplaneRunVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: dataplaneRunHostPath,
				Type: ptr.To(corev1.HostPathDirectoryOrCreate),
			},
		},
	}

	frrSocketVolume := corev1.Volume{
		Name: frrRunVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: frrRunHostPath,
				Type: ptr.To(corev1.HostPathDirectoryOrCreate),
			},
		},
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
		}
		if gw.Spec.Profiling.Enabled {
			// args = append(args, "--pyroscope-url", "http://alloy-gw.fab.svc.cluster.local:4040")
			args = append(args, "--pyroscope-url", "http://localhost:4040")
		}

		pcis, kernels := 0, 0
		for _, ifaceName := range slices.Sorted(maps.Keys(gw.Spec.Interfaces)) {
			iface := gw.Spec.Interfaces[ifaceName]
			val := ifaceName
			switch {
			case iface.PCI != "":
				pcis++
				val += "=pci@" + iface.PCI
			case iface.Kernel != "":
				kernels++
				val += "=kernel@" + iface.Kernel
				// TODO enable after migrating dataplane to a new interface format
				// default:
				// return nil
			}
			args = append(args, "--interface", val)
		}

		driver := "kernel"
		if pcis > 0 {
			driver = "dpdk"
		}
		if pcis > 0 && kernels > 0 {
			return fmt.Errorf("cannot use mixed PCI address and kernel name interfaces") //nolint:err113
		}
		args = append(args, "--driver", driver)

		// The kernel netdev beside a dataplane-driven NIC is a hazard rather than a spare: it
		// answers ARP, accepts connections and routes, all without the dataplane knowing.
		// dataplane-init moves it into a network namespace of its own making, out of reach, and
		// the dataplane puts a tap carrying the same name where it was -- which is what FRR and
		// the interface manager find in its place.
		//
		// Both drivers. Under DPDK it is forced (on vfio-pci there is no netdev at all); under the
		// kernel driver it is a choice, and it is the one that makes the netfilter rules keeping
		// VXLAN away from the host stack unnecessary. The dataplane's own AF_PACKET sockets follow
		// the interfaces, because its workers enter that namespace before opening anything.
		args = append(args, "--datapath-netns")

		// FRR is this process's to start, not a container of its own. That buys three things
		// nothing outside the pod can express: a startup order (zebra's dplane module connects to
		// the dataplane's control-plane socket as it loads, and a zebra that starts first finds
		// nothing there), shared fate (when one of them dies they all do, rather than the
		// dataplane forwarding on a FIB whose author has gone), and a control network namespace
		// that FRR and the dataplane share while the outward-facing work -- the k8s client, the
		// metrics endpoint -- stays in the host's.
		//
		// It also retires the three init containers that used to precede FRR. The nexthop sweep
		// and the VTEP address flush were both cleanup after a *previous* FRR in a namespace that
		// outlived it; the namespace is created per start now and dies with the process tree, so
		// there is nothing left to meet. The chown-and-sweep of the state directory is
		// dataplane-init's, which is the only thing here that knows when FRR is about to start.
		args = append(args, "--supervise-frr")

		// tmp hack to make dp work
		var initContainers []corev1.Container
		if driver == "kernel" {
			iArgs := "set -ex && "
			for _, ifaceName := range slices.Sorted(maps.Keys(gw.Spec.Interfaces)) {
				iface := gw.Spec.Interfaces[ifaceName]
				iArgs += fmt.Sprintf("(ethtool -K %s gro off || echo 'gro off failed') && ", ifaceName)
				iArgs += fmt.Sprintf("ip l set mtu %d dev %s && ", iface.MTU, ifaceName)
				iArgs += fmt.Sprintf("([[ $(basename $(readlink -f \"/sys/class/net/%[1]s/device/driver\")) == e1000 ]] && tee /sys/class/net/%[1]s/queues/rx-0/rps_cpus <<< ff || echo 'not e1000') && ", ifaceName)
				iArgs += fmt.Sprintf("ip l set dev %s up && ", ifaceName)
			}
			iArgs += "date && echo done"

			initContainers = []corev1.Container{
				{
					Name:    "init-ifaces",
					Image:   r.cfg.ToolboxRef,
					Command: []string{"/bin/bash", "-c", "--"},
					Args:    []string{iArgs},
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr.To(true),
						RunAsUser:  ptr.To(int64(0)),
					},
				},
			}
		}

		dpDS := &appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: r.cfg.GatewayNamespace,
			Name:      entityName(gw.Name, "dataplane"),
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, dpDS, func() error {
			labels := map[string]string{
				"app.kubernetes.io/name": dpDS.Name, // TODO
			}

			dpDS.Spec = appv1.DaemonSetSpec{
				Selector: &kmetav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: kmetav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						NodeSelector:                  map[string]string{"kubernetes.io/hostname": gw.Name},
						HostNetwork:                   true,
						DNSPolicy:                     corev1.DNSClusterFirstWithHostNet,
						TerminationGracePeriodSeconds: ptr.To(int64(10)),
						Tolerations:                   r.cfg.GatewayTolerations,
						ServiceAccountName:            saName,
						InitContainers:                initContainers,
						Containers: []corev1.Container{
							{
								Name:  "dataplane",
								Image: r.cfg.DataplaneRef,
								// Command, not just Args. The image's entrypoint is /bin/dataplane,
								// so dataplane-init has shipped in it and never run: everything it
								// does -- mounting hugetlbfs, binding NICs to vfio-pci or leaving a
								// bifurcated one alone, moving the netdev into the datapath
								// namespace -- was either done by an init container in shell or not
								// done at all. Naming it here is what puts it in the path.
								//
								// It takes the same arguments as the dataplane and execs it, so
								// under the kernel driver this is a no-op beyond one extra fork.
								Command: []string{"/bin/dataplane-init"},
								Args:    args,
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								Env: []corev1.EnvVar{
									{
										Name:  "RUST_BACKTRACE",
										Value: "FULL",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      dataplaneRunVolumeName,
										MountPath: dataplaneRunMountPath,
									},
									{
										Name:      frrRunVolumeName,
										MountPath: frrRunMountPath,
									},
									{
										Name:      "dataplane-tmp",
										MountPath: "/tmp",
									},
									{
										Name:      frrTmpVolumeName,
										MountPath: "/var/tmp/frr",
									},
								},
							},
							{
								// Not under the supervisor, deliberately. It reads FRR's vty
								// sockets, which are files, so it does not care which network
								// namespace FRR ends up in -- and a metrics endpoint should not
								// be able to take the gateway down with it, which is exactly what
								// being supervised would mean.
								//
								// From the dataplane image, which now carries FRR and everything
								// beside it.
								Name:    "frr-exporter",
								Image:   r.cfg.DataplaneRef,
								Command: []string{"/bin/frr_exporter"},
								Args: []string{
									"--web.listen-address", fmt.Sprintf("127.0.0.1:%d", r.cfg.FRRMetricsPort),
									"--frr.socket.dir-path", frrRunMountPath,
									"--no-collector.ospf",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      frrRunVolumeName,
										MountPath: frrRunMountPath,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							dataplaneSocketVolume,
							frrSocketVolume,
							{
								Name: frrTmpVolumeName,
								VolumeSource: corev1.VolumeSource{
									// TODO consider memory medium
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},

							{
								Name: "dataplane-tmp",
								VolumeSource: corev1.VolumeSource{
									// TODO consider memory medium
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
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

	// The FRR DaemonSet is gone: FRR runs under `dataplane-init` in the dataplane pod, which is
	// what lets the two share a network namespace, a startup order and a fate. `CreateOrUpdate`
	// cannot express a removal, so the old object has to be deleted by name -- left behind it
	// would keep running a second FRR against the same sockets, which is a worse failure than
	// either arrangement alone.
	frrDS := &appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{
		Namespace: r.cfg.GatewayNamespace,
		Name:      entityName(gw.Name, "frr"),
	}}
	if err := r.Client.Delete(ctx, frrDS); err != nil && !kapierrors.IsNotFound(err) {
		return fmt.Errorf("deleting the standalone gateway frr daemonset: %w", err)
	}

	return nil
}
