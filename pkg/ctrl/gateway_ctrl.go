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
								Args:  args,
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
								},
							},
						},
						Volumes: []corev1.Volume{
							dataplaneSocketVolume,
							frrSocketVolume,

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

	frrVolumeMounts := []corev1.VolumeMount{
		{
			Name:      frrRunVolumeName,
			MountPath: frrRunMountPath,
		},
		{
			Name:      frrTmpVolumeName,
			MountPath: "/var/tmp/frr",
		},
		{
			Name:      frrRootRunVolumeName,
			MountPath: frrRootRunMountPath,
		},
	}

	{
		frrDS := &appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: r.cfg.GatewayNamespace,
			Name:      entityName(gw.Name, "frr"),
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, frrDS, func() error {
			labels := map[string]string{
				"app.kubernetes.io/name": frrDS.Name, // TODO
			}

			frrDS.Spec = appv1.DaemonSetSpec{
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
						InitContainers: []corev1.Container{
							// TODO remove it after frr container will take care of this
							{
								Name:    "init-frr",
								Image:   r.cfg.FRRRef,
								Command: []string{"/bin/bash", "-c", "--"},
								Args: []string{
									"set -ex && " +
										"chown -R frr:frr /run/frr/ && chmod -R 760 /run/frr && " +
										"mkdir -p /var/run/frr/hh && chown -R frr:frr /var/run/frr/ && chmod -R 766 /var/run/frr &&" +
										"rm -f /var/run/frr/*.pid /var/run/frr/*.sock /var/run/frr/*.vty /var/run/frr/*.api /var/run/frr/*.started",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: frrVolumeMounts,
							},
							// it's needed to avoid issues with leftover routes in the kernel being loaded by FRR on startup
							{
								Name:    "flush-zebra-nexthops",
								Image:   r.cfg.FRRRef,
								Command: []string{"/bin/bash", "-c", "--"},
								Args: []string{
									"set -ex && " +
										"ip -j -d nexthop show | jq '.[]|select(.protocol=\"zebra\")|.id' | while read -r id ; do ip nexthop del id $id ; done",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
							},
							// it's needed to avoid issues with leftover routes on the physical interface learned from BGP
							{
								Name:    "flush-vtepip",
								Image:   r.cfg.FRRRef,
								Command: []string{"/bin/bash", "-c", "--"},
								Args: []string{
									"set -ex && " +
										fmt.Sprintf("ip addr del %s dev lo || true", gw.Spec.VTEPIP),
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:    "frr",
								Image:   r.cfg.FRRRef,
								Command: []string{"/bin/tini", "--"},
								Args: []string{
									"/libexec/frr/docker-start",
									"--sock-path", filepath.Join(frrRunMountPath, frrAgentSocket),
									"--reloader", "/libexec/frr/frr-reload.py",
									"--bindir", "/bin",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: frrVolumeMounts,
							},
							{
								Name:    "frr-exporter",
								Image:   r.cfg.FRRRef,
								Command: []string{"/bin/frr_exporter"},
								Args: []string{
									"--web.listen-address", fmt.Sprintf("127.0.0.1:%d", r.cfg.FRRMetricsPort),
									"--frr.socket.dir-path", frrRootRunMountPath,
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      frrRootRunVolumeName,
										MountPath: frrRootRunMountPath,
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							frrSocketVolume,
							{
								Name: frrTmpVolumeName,
								VolumeSource: corev1.VolumeSource{
									// TODO consider memory medium
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
							{
								Name: frrRootRunVolumeName,
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/run/hedgehog/frr-root",
										Type: ptr.To(corev1.HostPathDirectoryOrCreate),
									},
								},
							},
						},
					},
				},
				UpdateStrategy: replaceUpdateStrategy,
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating or updating gateway frr daemonset: %w", err)
		}
	}

	return nil
}
