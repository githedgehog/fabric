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
	"log/slog"
	"maps"
	"math"
	"slices"
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

	// TODO only enqueue switches when related VPC/VPCAttach/VPCPeering changes
	return ctrl.NewControllerManagedBy(mgr).
		For(&wiringapi.Switch{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabels)).
		Watches(&vpcapi.VPC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.VPCAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.VPCPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.External{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.ExternalAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.ExternalPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
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

func (r *AgentReconciler) enqueueAllSwitches(ctx context.Context, obj client.Object) []reconcile.Request {
	res := []reconcile.Request{}

	sws := &wiringapi.SwitchList{}
	err := r.List(ctx, sws, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		log.FromContext(ctx).Error(err, "error listing switches to reconcile all")
		return res
	}

	for _, sw := range sws.Items {
		res = append(res, reconcile.Request{NamespacedName: types.NamespacedName{
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

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchgroups,verbs=get;list;watch
//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=switchgroups/status,verbs=get;update;patch

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

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	sw := &wiringapi.Switch{}
	err := r.Get(ctx, req.NamespacedName, sw)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrapf(err, "error getting switch")
	}

	// TODO impl
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

	conns := map[string]wiringapi.ConnectionSpec{}
	for _, conn := range connList.Items {
		conns[conn.Name] = conn.Spec
	}

	neighborSwitches := map[string]bool{}
	mclagPeerName := ""
	for _, conn := range connList.Items {
		sws, _, _, _, err := conn.Spec.Endpoints()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting endpoints for connection %s", conn.Name)
		}
		for _, sw := range sws {
			neighborSwitches[sw] = true
		}

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

	switchList := &wiringapi.SwitchList{}
	err = r.List(ctx, switchList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error getting switches")
	}

	switches := map[string]wiringapi.SwitchSpec{}
	for _, sw := range switchList.Items {
		if !neighborSwitches[sw.Name] {
			continue
		}
		switches[sw.Name] = sw.Spec
	}

	// handle MCLAG things if we see a peer switch
	// We only support MCLAG switch pairs for now
	// It means that 2 switches would have the same MCLAG connections and same set of PortChannels
	var mclagPeer *agentapi.Agent
	mclagConns := map[string]wiringapi.ConnectionSpec{}
	if mclagPeerName != "" {
		mclagPeer = &agentapi.Agent{}
		err = r.Get(ctx, types.NamespacedName{Namespace: sw.Namespace, Name: mclagPeerName}, mclagPeer)
		if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrapf(err, "error getting peer agent")
		}

		connList := &wiringapi.ConnectionList{}
		err = r.List(ctx, connList, client.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(mclagPeerName))
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting mclag peer switch connections")
		}

		for _, conn := range connList.Items {
			mclagConns[conn.Name] = conn.Spec
		}
	}

	// TODO optimize by only getting related VPC attachments
	attaches := map[string]vpcapi.VPCAttachmentSpec{}
	configuredSubnets := map[string]bool{} // TODO probably it's not really needed
	attachedVPCs := map[string]bool{}
	mclagAttachedVPCs := map[string]bool{} // TODO remove?
	attachList := &vpcapi.VPCAttachmentList{}
	err = r.List(ctx, attachList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing vpc attachments")
	}
	for _, attach := range attachList.Items {
		_, conn := conns[attach.Spec.Connection]
		_, mclagConn := mclagConns[attach.Spec.Connection]

		if conn {
			attaches[attach.Name] = attach.Spec
			attachedVPCs[attach.Spec.VPCName()] = true
		}

		// whatever vpc subnet that got configured on our mclag peer should be configured on us too
		if conn || mclagConn {
			configuredSubnets[attach.Spec.Subnet] = true
			mclagAttachedVPCs[attach.Spec.VPCName()] = true
		}
	}

	// we handle VPCs attached to our MCLAG peer like our attached VPCs in most cases
	maps.Copy(attachedVPCs, mclagAttachedVPCs)

	vpcs := map[string]vpcapi.VPCSpec{}
	vpcList := &vpcapi.VPCList{}
	err = r.List(ctx, vpcList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing vpcs")
	}
	for _, vpc := range vpcList.Items {
		ok := attachedVPCs[vpc.Name] || mclagAttachedVPCs[vpc.Name]
		for subnetName := range vpc.Spec.Subnets {
			if configuredSubnets[fmt.Sprintf("%s/%s", vpc.Name, subnetName)] {
				ok = true
				break
			}
		}
		if ok {
			vpcs[vpc.Name] = vpc.Spec
		}
	}

	// TODO only query for related peerings
	peerings := map[string]vpcapi.VPCPeeringSpec{}
	peeringsList := &vpcapi.VPCPeeringList{}
	peeredVPCs := map[string]bool{}
	err = r.List(ctx, peeringsList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing vpc peerings")
	}
	for _, peer := range peeringsList.Items {
		vpc1, vpc2, err := peer.Spec.VPCs()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "error getting vpcs for peering %s", peer.Name)
		}

		_, exists1 := vpcs[vpc1]
		_, exists2 := vpcs[vpc2]

		if exists1 || exists2 || peer.Spec.Remote != "" && slices.Contains(sw.Spec.Groups, peer.Spec.Remote) {
			peerings[peer.Name] = peer.Spec
			peeredVPCs[vpc1] = true
			peeredVPCs[vpc2] = true
		}
	}

	attachedExternals := map[string]bool{}
	externalAttaches := map[string]vpcapi.ExternalAttachmentSpec{}
	externalAttachList := &vpcapi.ExternalAttachmentList{}
	err = r.List(ctx, externalAttachList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing external attachments")
	}
	for _, attach := range externalAttachList.Items {
		if _, exists := conns[attach.Spec.Connection]; !exists {
			continue
		}

		attachedExternals[attach.Spec.External] = true
		externalAttaches[attach.Name] = attach.Spec
	}

	externals := map[string]vpcapi.ExternalSpec{}
	externalList := &vpcapi.ExternalList{}
	err = r.List(ctx, externalList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing externals")
	}
	for _, ext := range externalList.Items {
		if !attachedExternals[ext.Name] {
			continue
		}

		externals[ext.Name] = ext.Spec
	}

	externalPeerings := map[string]vpcapi.ExternalPeeringSpec{}
	externalPeeringList := &vpcapi.ExternalPeeringList{}
	err = r.List(ctx, externalPeeringList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing external peerings")
	}
	for _, peering := range externalPeeringList.Items {
		if _, exists := externals[peering.Spec.Permit.External.Name]; !exists {
			continue
		}

		// TODO is it ok?
		peeredVPCs[peering.Spec.Permit.VPC.Name] = true

		externalPeerings[peering.Name] = peering.Spec
	}

	for _, vpc := range vpcList.Items {
		if peeredVPCs[vpc.Name] {
			vpcs[vpc.Name] = vpc.Spec
		}
	}

	for name, vpc := range vpcs {
		if !slices.Contains(sw.Spec.VLANNamespaces, vpc.VLANNamespace) {
			return ctrl.Result{}, errors.Errorf("switch %s doesn't have vlan namespace %s while gets vpc %s", sw.Name, vpc.VLANNamespace, name)
		}
	}

	vnis := map[string]uint32{}

	for _, vpc := range vpcList.Items {
		if _, exists := vpcs[vpc.Name]; !exists {
			continue
		}

		vnis[vpc.Name] = vpc.Status.VNI

		for subnetName := range vpc.Spec.Subnets {
			vnis[fmt.Sprintf("%s/%s", vpc.Name, subnetName)] = vpc.Status.SubnetVNIs[subnetName]
		}
	}

	ipv4NamespaceList := &vpcapi.IPv4NamespaceList{}
	err = r.List(ctx, ipv4NamespaceList, client.InNamespace(sw.Namespace))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "error listing ipv4 namespaces")
	}

	ipv4Namespaces := map[string]vpcapi.IPv4NamespaceSpec{}
	for _, ns := range ipv4NamespaceList.Items {
		ipv4Namespaces[ns.Name] = ns.Spec
	}

	agent := &agentapi.Agent{ObjectMeta: switchNsName}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, agent, func() error {
		agent.Labels = sw.Labels
		agent.Spec.Role = sw.Spec.Role
		agent.Spec.Description = sw.Spec.Description

		agent.Spec.Switch = sw.Spec
		agent.Spec.Switches = switches
		agent.Spec.Connections = conns
		agent.Spec.VPCs = vpcs
		agent.Spec.VPCAttachments = attaches
		agent.Spec.VPCPeerings = peerings
		agent.Spec.IPv4Namespaces = ipv4Namespaces
		agent.Spec.Externals = externals
		agent.Spec.ExternalAttachments = externalAttaches
		agent.Spec.ExternalPeerings = externalPeerings
		agent.Spec.ConfiguredVPCSubnets = configuredSubnets
		agent.Spec.MCLAGAttachedVPCs = mclagAttachedVPCs
		agent.Spec.VNIs = vnis
		agent.Spec.Users = r.Cfg.Users

		agent.Spec.Version.Default = r.Version
		agent.Spec.Version.Repo = r.Cfg.AgentRepo
		agent.Spec.Version.CA = r.Cfg.AgentRepoCA

		agent.Spec.StatusUpdates = statusUpdates

		agent.Spec.Config = agentapi.AgentSpecConfig{
			ControlVIP:            r.Cfg.ControlVIP,
			BaseVPCCommunity:      r.Cfg.BaseVPCCommunity,
			VPCLoopbackSubnet:     r.Cfg.VPCLoopbackSubnet,
			FabricMTU:             r.Cfg.FabricMTU,
			ServerFacingMTUOffset: r.Cfg.ServerFacingMTUOffset,
		}
		if r.Cfg.FabricMode == config.FabricModeCollapsedCore {
			agent.Spec.Config.CollapsedCore = &agentapi.AgentSpecConfigCollapsedCore{}
		} else if r.Cfg.FabricMode == config.FabricModeSpineLeaf {
			agent.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		}

		agent.Spec.PortChannels, err = r.calculatePortChannels(ctx, agent, mclagPeer, conns)
		if err != nil {
			return errors.Wrapf(err, "error calculating port channels")
		}

		agent.Spec.IRBVLANs, err = r.calculateIRBVLANs(agent, vpcs)
		if err != nil {
			return errors.Wrapf(err, "error calculating IRB VLANs")
		}

		agent.Spec.VPCLoopbackLinks, err = r.calculateVPCLoopbackLinkAllocation(agent, conns, peerings, externalPeerings, attachedVPCs)
		if err != nil {
			return errors.Wrapf(err, "error calculating vpc loopback allocation")
		}

		agent.Spec.VPCLoopbackVLANs, err = r.calculateVPCLoopbackVLANAllocation(agent, peerings, externalPeerings, attachedVPCs)
		if err != nil {
			return errors.Wrapf(err, "error calculating vpc loopback vlan allocation")
		}

		externalSeqs := map[string]uint16{}
		takenSeqs := map[uint16]bool{}
		for name := range externals {
			if agent.Spec.ExternalSeqs[name] == 0 {
				continue
			}

			externalSeqs[name] = agent.Spec.ExternalSeqs[name]
			takenSeqs[externalSeqs[name]] = true
		}
		for name := range externals {
			if externalSeqs[name] != 0 {
				continue
			}

			for idx := 10; idx <= 65535; idx++ {
				if !takenSeqs[uint16(idx)] {
					externalSeqs[name] = uint16(idx)
					takenSeqs[uint16(idx)] = true
					break
				}
			}

			if externalSeqs[name] == 0 {
				return errors.Errorf("error calculating external seqs for %s", name)
			}
		}
		agent.Spec.ExternalSeqs = externalSeqs

		externalPeeringPrefixIDs := map[string]uint32{}
		taken := map[uint32]bool{}
		for _, peering := range externalPeerings {
			for _, prefix := range peering.Permit.External.Prefixes {
				val := agent.Spec.ExternalPeeringPrefixIDs[prefix.Prefix]
				if val == 0 {
					continue
				}

				externalPeeringPrefixIDs[prefix.Prefix] = val
				taken[val] = true
			}
		}
		for _, peering := range externalPeerings {
			for _, prefix := range peering.Permit.External.Prefixes {
				if externalPeeringPrefixIDs[prefix.Prefix] != 0 {
					continue
				}

				for idx := uint32(10); idx <= 4294967295; idx++ {
					if !taken[idx] {
						externalPeeringPrefixIDs[prefix.Prefix] = idx
						taken[idx] = true
						break
					}
				}

				if externalPeeringPrefixIDs[prefix.Prefix] == 0 {
					return errors.Errorf("error calculating external peering prefix ids for %s", prefix.Prefix)
				}
			}
		}
		agent.Spec.ExternalPeeringPrefixIDs = externalPeeringPrefixIDs

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

func (r *AgentReconciler) calculatePortChannels(ctx context.Context, agent, peer *agentapi.Agent, conns map[string]wiringapi.ConnectionSpec) (map[string]uint16, error) {
	portChannels := map[string]uint16{}

	taken := make([]bool, PORT_CHAN_MAX-PORT_CHAN_MIN+1)
	for connName, connSpec := range conns {
		if connSpec.MCLAG != nil {
			if peer == nil {
				slog.Warn("MCLAG connection has no peer", "conn", connName, "switch", agent.Name)
				continue
			}

			pc1 := agent.Spec.PortChannels[connName]
			pc2 := peer.Spec.PortChannels[connName]

			if pc1 == 0 && pc2 == 0 {
				continue
			}

			if pc1 != 0 && pc2 != 0 && pc1 != pc2 {
				return nil, errors.Errorf("port channel mismatch for conn %s on %s and %s", connName, agent.Name, peer.Name)
			}

			if pc1 != 0 {
				if pc1 < PORT_CHAN_MIN || pc1 > PORT_CHAN_MAX {
					return nil, errors.Errorf("port channel %d for conn %s on %s is out of range %d..%d", portChannels[connName], connName, agent.Name, PORT_CHAN_MIN, PORT_CHAN_MAX)
				}
				if taken[pc1-PORT_CHAN_MIN] {
					return nil, errors.Errorf("port channel %d for conn %s assigned on %s is already taken", pc2, connName, agent.Name)
				}

				portChannels[connName] = pc1
			}
			if pc2 != 0 {
				if pc2 < PORT_CHAN_MIN || pc2 > PORT_CHAN_MAX {
					return nil, errors.Errorf("port channel %d for conn %s on peer %s is out of range %d..%d", portChannels[connName], connName, peer.Name, PORT_CHAN_MIN, PORT_CHAN_MAX)
				}
				if taken[pc2-PORT_CHAN_MIN] {
					return nil, errors.Errorf("port channel %d for conn %s assigned on peer %s is already taken", pc2, connName, peer.Name)
				}

				portChannels[connName] = pc2
			}

			taken[portChannels[connName]-PORT_CHAN_MIN] = true
		} else if connSpec.Bundled != nil {
			pc := agent.Spec.PortChannels[connName]
			if pc == 0 {
				continue
			}

			if taken[pc-PORT_CHAN_MIN] {
				return nil, errors.Errorf("port channel %d for conn %s on %s is already taken", portChannels[connName], connName, agent.Name)
			}
			portChannels[connName] = pc

			taken[pc-PORT_CHAN_MIN] = true
		}
	}

	if peer != nil {
		// mark all port channels on the peer as taken so we don't assign them to other connections
		for _, pc := range peer.Spec.PortChannels {
			if pc == 0 {
				continue
			}

			taken[pc-PORT_CHAN_MIN] = true
		}
	}

	for connName, connSpec := range conns {
		if connSpec.MCLAG != nil || connSpec.Bundled != nil {
			if portChannels[connName] != 0 {
				continue
			}

			// TODO optimize by storing last taken port channel
			for i := PORT_CHAN_MIN; i <= PORT_CHAN_MAX; i++ {
				if !taken[i-PORT_CHAN_MIN] {
					portChannels[connName] = uint16(i)
					taken[i-PORT_CHAN_MIN] = true
					break
				}
			}

			if portChannels[connName] == 0 {
				return nil, errors.Errorf("no port channel available for conn %s on %s", connName, agent.Name)
			}
		}
	}

	return portChannels, nil
}

func (r *AgentReconciler) calculateIRBVLANs(agent *agentapi.Agent, vpcs map[string]vpcapi.VPCSpec) (map[string]uint16, error) {
	irbVLANs := map[string]uint16{}
	taken := map[uint16]bool{}

	for vpc, vlan := range agent.Spec.IRBVLANs {
		if vlan < 1 {
			continue
		}

		// TODO check it's still in the reserved ranges

		if _, exist := vpcs[vpc]; !exist {
			continue
		}

		irbVLANs[vpc] = vlan
		taken[vlan] = true
	}

	for vpcName := range vpcs {
		if irbVLANs[vpcName] > 0 {
			continue
		}

		// TODO optimize by storing last taken vlan
	loop:
		for _, vlanRange := range r.Cfg.VPCIRBVLANRanges {
			for vlan := vlanRange.From; vlan <= vlanRange.To; vlan++ {
				if !taken[vlan] {
					irbVLANs[vpcName] = vlan
					taken[vlan] = true
					break loop
				}
			}
		}

		if irbVLANs[vpcName] == 0 {
			return nil, errors.Errorf("no IRB VLAN available for vpc %s", vpcName)
		}
	}

	return irbVLANs, nil
}

func (r *AgentReconciler) calculateVPCLoopbackLinkAllocation(agent *agentapi.Agent, conns map[string]wiringapi.ConnectionSpec, peerings map[string]vpcapi.VPCPeeringSpec, externalPeerings map[string]vpcapi.ExternalPeeringSpec, attachedVPCs map[string]bool) (map[string]string, error) {
	loopbackMapping := map[string]string{}

	vpcLoopbacks := map[string]bool{}
	loopbackUsage := map[string]int{}
	for connName, conn := range conns {
		if conn.VPCLoopback == nil {
			continue
		}

		for linkID, link := range conn.VPCLoopback.Links {
			ports := []string{link.Switch1.LocalPortName(), link.Switch2.LocalPortName()}
			sort.Strings(ports)

			if len(ports) != 2 {
				return nil, errors.Errorf("invalid vpc loopback link %s %d", connName, linkID)
			}

			loRef := fmt.Sprintf("%s--%s", ports[0], ports[1])
			vpcLoopbacks[loRef] = true
			loopbackUsage[loRef] = 0
		}
	}

	for peeringHack, loopback := range agent.Spec.VPCLoopbackLinks {
		if !vpcLoopbacks[loopback] {
			continue
		}

		if strings.HasPrefix(peeringHack, "vpc@") {
			if peeringSpec, exists := peerings[strings.TrimPrefix(peeringHack, "vpc@")]; !exists {
				continue
			} else {
				if peeringSpec.Remote != "" {
					continue
				}

				vpc1, vpc2, err := peeringSpec.VPCs()
				if err != nil {
					return nil, errors.Wrapf(err, "error getting vpcs for peering %s", peeringHack)
				}

				if !attachedVPCs[vpc1] || !attachedVPCs[vpc2] {
					continue
				}
			}
		} else if strings.HasPrefix(peeringHack, "ext@") {
			if peeringSpec, exists := externalPeerings[strings.TrimPrefix(peeringHack, "ext@")]; !exists {
				continue
			} else {
				if _, exists := attachedVPCs[peeringSpec.Permit.VPC.Name]; !exists {
					continue
				}
			}
		}

		loopbackMapping[peeringHack] = loopback
		loopbackUsage[loopback] += 1
	}

	for peeringName, peering := range peerings {
		if peering.Remote != "" {
			continue
		}

		if _, exists := loopbackMapping["vpc@"+peeringName]; exists {
			continue
		}

		vpc1, vpc2, err := peering.VPCs()
		if err != nil {
			return nil, errors.Wrapf(err, "error getting vpcs for peering %s", peering)
		}

		if !attachedVPCs[vpc1] || !attachedVPCs[vpc2] {
			continue
		}

		minLoUsage := math.MaxInt
		minLo := ""

		for loopback, usage := range loopbackUsage {
			if usage < minLoUsage {
				minLoUsage = usage
				minLo = loopback
			}
		}

		if minLo == "" {
			return nil, errors.Errorf("no vpc loopback available for vpc peering %s", peeringName)
		}

		loopbackMapping["vpc@"+peeringName] = minLo
		loopbackUsage[minLo] += 1
	}

	for peeringName, peering := range externalPeerings {
		if _, exists := loopbackMapping["ext@"+peeringName]; exists {
			continue
		}

		if _, exists := attachedVPCs[peering.Permit.VPC.Name]; !exists {
			continue
		}

		minLoUsage := math.MaxInt
		minLo := ""

		for loopback, usage := range loopbackUsage {
			if usage < minLoUsage {
				minLoUsage = usage
				minLo = loopback
			}
		}

		if minLo == "" {
			return nil, errors.Errorf("no vpc loopback available for external peering %s", peeringName)
		}

		loopbackMapping["ext@"+peeringName] = minLo
		loopbackUsage[minLo] += 1
	}

	return loopbackMapping, nil
}

// TODO merge with IRB vlan allocation
func (r *AgentReconciler) calculateVPCLoopbackVLANAllocation(agent *agentapi.Agent, peerings map[string]vpcapi.VPCPeeringSpec, externalPeerings map[string]vpcapi.ExternalPeeringSpec, attachedVPCs map[string]bool) (map[string]uint16, error) {
	vlans := map[string]uint16{}
	taken := map[uint16]bool{}

	for peeringHack, vlan := range agent.Spec.VPCLoopbackVLANs {
		if vlan < 1 {
			continue
		}

		// TODO check that it still belongs to reserved ranges

		if strings.HasPrefix(peeringHack, "vpc@") {
			if peerSpec, exist := peerings[peeringHack]; !exist {
				continue
			} else {
				if peerSpec.Remote != "" {
					continue
				}

				vpc1, vpc2, err := peerSpec.VPCs()
				if err != nil {
					return nil, errors.Wrapf(err, "error getting vpcs for peering %s", peeringHack)
				}

				if !attachedVPCs[vpc1] || !attachedVPCs[vpc2] {
					continue
				}
			}
		} else if strings.HasPrefix(peeringHack, "ext@") {
			if peeringSpec, exists := externalPeerings[strings.TrimPrefix(peeringHack, "ext@")]; !exists {
				continue
			} else {
				if _, exists := attachedVPCs[peeringSpec.Permit.VPC.Name]; !exists {
					continue
				}
			}
		}

		vlans[peeringHack] = vlan
		taken[vlan] = true
	}

	for peeringName, peering := range peerings {
		if peering.Remote != "" {
			continue
		}

		if vlans["vpc@"+peeringName] > 0 {
			continue
		}

		vpc1, vpc2, err := peering.VPCs()
		if err != nil {
			return nil, errors.Wrapf(err, "error getting vpcs for peering %s", peeringName)
		}

		if !attachedVPCs[vpc1] || !attachedVPCs[vpc2] {
			continue
		}

		// TODO optimize by storing last taken vlan
	vpcLoop:
		for _, vlanRange := range r.Cfg.VPCPeeringVLANRanges {
			for vlan := vlanRange.From; vlan <= vlanRange.To; vlan++ {
				if !taken[vlan] {
					vlans["vpc@"+peeringName] = vlan
					taken[vlan] = true
					break vpcLoop
				}
			}
		}

		if vlans["vpc@"+peeringName] == 0 {
			return nil, errors.Errorf("no peering VLAN available for peer %s", peeringName)
		}
	}

	for peeringName, peering := range externalPeerings {
		if vlans["ext@"+peeringName] > 0 {
			continue
		}

		if _, exists := attachedVPCs[peering.Permit.VPC.Name]; !exists {
			continue
		}

		// TODO optimize by storing last taken vlan
	extLoop:
		for _, vlanRange := range r.Cfg.VPCPeeringVLANRanges {
			for vlan := vlanRange.From; vlan <= vlanRange.To; vlan++ {
				if !taken[vlan] {
					vlans["ext@"+peeringName] = vlan
					taken[vlan] = true
					break extLoop
				}
			}
		}

		if vlans["ext@"+peeringName] == 0 {
			return nil, errors.Errorf("no peering VLAN available for peer %s", peeringName)
		}
	}

	return vlans, nil
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
