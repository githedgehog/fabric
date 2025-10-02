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
	"net"
	"slices"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fmeta "go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/manager/librarian"
	"go.githedgehog.com/fabric/pkg/version"
	"go.githedgehog.com/libmeta/pkg/alloy"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
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
}

func SetupAgentReconsilerWith(mgr kctrl.Manager, cfg *fmeta.FabricConfig, libMngr *librarian.Manager, ca, username, password string) error {
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

	r := &AgentReconciler{
		Client:      mgr.GetClient(),
		cfg:         cfg,
		libr:        libMngr,
		regCA:       ca,
		regUsername: username,
		regPassword: password,
	}

	// TODO only enqueue switches when related VPC/VPCAttach/VPCPeering changes
	return errors.Wrapf(kctrl.NewControllerManagedBy(mgr).
		Named("Agent").
		For(&wiringapi.Switch{}).
		Watches(&wiringapi.Connection{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchListLabelsAndSpines)).
		Watches(&wiringapi.SwitchProfile{}, handler.EnqueueRequestsFromMapFunc(r.enqueueBySwitchProfileLabel)).
		Watches(&vpcapi.VPC{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.VPCAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.VPCPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.External{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.ExternalAttachment{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Watches(&vpcapi.ExternalPeering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllSwitches)).
		Complete(r), "failed to setup agent controller")
}

func (r *AgentReconciler) enqueueBySwitchListLabelsAndSpines(ctx context.Context, obj kclient.Object) []reconcile.Request {
	res := []reconcile.Request{}

	labels := obj.GetLabels()

	// TODO extract to lib
	switchConnPrefix := wiringapi.ListLabelPrefix(wiringapi.ConnectionLabelTypeSwitch)

	labelSwitches := map[string]bool{}
	needSpines := false
	for label, val := range labels {
		if label == wiringapi.LabelConnectionType && val == wiringapi.ConnectionTypeStaticExternal {
			needSpines = true

			continue
		}
		if val != wiringapi.ListLabelValue {
			continue
		}

		if strings.HasPrefix(label, switchConnPrefix) {
			switchName := strings.TrimPrefix(label, switchConnPrefix)
			res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      switchName,
			}})
			labelSwitches[switchName] = true
		}
	}

	if !needSpines {
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

func (r *AgentReconciler) enqueueBySwitchProfileLabel(ctx context.Context, obj kclient.Object) []reconcile.Request {
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

func (r *AgentReconciler) enqueueAllSwitches(ctx context.Context, obj kclient.Object) []reconcile.Request {
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

//+kubebuilder:rbac:groups=agent.githedgehog.com,resources=catalogs,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	sw := &wiringapi.Switch{}
	err := r.Get(ctx, req.NamespacedName, sw)
	if err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, errors.Wrapf(err, "error getting switch")
	}

	// TODO impl
	statusUpdates := appendUpdate(nil, sw)

	switchNsName := kmetav1.ObjectMeta{Name: sw.Name, Namespace: sw.Namespace}
	res, err := r.prepareAgentInfra(ctx, switchNsName)
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

	conns := map[string]wiringapi.ConnectionSpec{}
	for _, conn := range connList.Items {
		if !r.cfg.LoopbackWorkaround && conn.Spec.VPCLoopback != nil {
			continue
		}

		conns[conn.Name] = conn.Spec
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
		}
	}

	neighborSwitches := map[string]bool{}
	mclagPeerName := ""
	for _, conn := range connList.Items {
		sws, _, _, _, err := conn.Spec.Endpoints()
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting endpoints for connection %s", conn.Name)
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
	err = r.List(ctx, switchList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error getting switches")
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
		err = r.Get(ctx, ktypes.NamespacedName{Namespace: sw.Namespace, Name: mclagPeerName}, mclagPeer)
		if err != nil && !kapierrors.IsNotFound(err) {
			return kctrl.Result{}, errors.Wrapf(err, "error getting peer agent")
		}

		connList := &wiringapi.ConnectionList{}
		err = r.List(ctx, connList, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(mclagPeerName))
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting mclag peer switch connections")
		}

		for _, conn := range connList.Items {
			mclagConns[conn.Name] = conn.Spec
		}
	}

	// TODO optimize by only getting related VPC attachments
	attaches := map[string]vpcapi.VPCAttachmentSpec{}
	configuredSubnets := map[string]bool{} // TODO probably it's not really needed
	attachedVPCs := map[string]bool{}
	attachList := &vpcapi.VPCAttachmentList{}
	err = r.List(ctx, attachList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing vpc attachments")
	}
	for _, attach := range attachList.Items {
		_, conn := conns[attach.Spec.Connection]
		_, mclagConn := mclagConns[attach.Spec.Connection]

		if conn {
			attaches[attach.Name] = attach.Spec
		}

		// whatever vpc subnet that got configured on our mclag peer should be configured on us too
		if conn || mclagConn {
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
	err = r.List(ctx, vpcList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing vpcs")
	}
	for _, vpc := range vpcList.Items {
		ok := attachedVPCs[vpc.Name] || staticExtVPCs[vpc.Name]
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

		if exists1 || exists2 || peer.Spec.Remote != "" && slices.Contains(sw.Spec.Groups, peer.Spec.Remote) {
			peerings[peer.Name] = peer.Spec
			peeredVPCs[vpc1] = true
			peeredVPCs[vpc2] = true
		}
	}

	attachedExternals := map[string]bool{}
	externalAttaches := map[string]vpcapi.ExternalAttachmentSpec{}
	externalAttachList := &vpcapi.ExternalAttachmentList{}
	err = r.List(ctx, externalAttachList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing external attachments")
	}
	for _, attach := range externalAttachList.Items {
		if _, exists := conns[attach.Spec.Connection]; !exists {
			continue
		}

		attachedExternals[attach.Spec.External] = true
		externalAttaches[attach.Name] = attach.Spec
	}

	externals := map[string]vpcapi.ExternalSpec{}
	externalsToConfig := map[string]vpcapi.ExternalSpec{}
	externalList := &vpcapi.ExternalList{}
	err = r.List(ctx, externalList, kclient.InNamespace(sw.Namespace))
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error listing externals")
	}
	for _, ext := range externalList.Items {
		externals[ext.Name] = ext.Spec
		if attachedExternals[ext.Name] {
			externalsToConfig[ext.Name] = ext.Spec
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
	}

	for _, vpc := range vpcList.Items {
		if peeredVPCs[vpc.Name] {
			vpcs[vpc.Name] = vpc.Spec
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
		ipv4Namespaces[ns.Name] = ns.Spec
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
	}

	usedVPCs := map[string]bool{}
	for name := range vpcs {
		usedVPCs[name] = true
	}

	portChanConns := map[string]bool{}
	for name, conn := range conns {
		if conn.Bundled == nil && conn.MCLAG == nil && conn.ESLAG == nil {
			continue
		}

		portChanConns[name] = true
	}

	rgPeers := []string{}
	if sw.Spec.Redundancy.Group != string(fmeta.RedundancyTypeNone) {
		for _, other := range switchList.Items {
			if sw.Spec.Redundancy.Group == other.Spec.Redundancy.Group && sw.Name != other.Name {
				if sw.Spec.Redundancy.Type != other.Spec.Redundancy.Type {
					return kctrl.Result{}, errors.Errorf("switch %s and %s have different redundancy types but in the redundancy same group", sw.Name, other.Name)
				}

				rgPeers = append(rgPeers, other.Name)
			}
		}
	}

	for _, rgPeerName := range rgPeers {
		connList := &wiringapi.ConnectionList{}
		err = r.List(ctx, connList, kclient.InNamespace(sw.Namespace), wiringapi.MatchingLabelsForListLabelSwitch(rgPeerName))
		if err != nil {
			return kctrl.Result{}, errors.Wrapf(err, "error getting rg peer switch %s connections", rgPeerName)
		}

		for _, conn := range connList.Items {
			if conn.Spec.Bundled == nil && conn.Spec.MCLAG == nil && conn.Spec.ESLAG == nil {
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

	cat := &agentapi.CatalogSpec{}

	err = r.libr.CatalogForRedundancyGroup(ctx, r.Client, cat, sw.Name, sw.Spec.Redundancy, usedVPCs, portChanConns, idConns)
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
			if peering.Remote != "" {
				continue
			}

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

			loWorkaroundReqs[librarian.LoWReqForExt(name)] = true
		}
	}

	externalsReq := map[string]bool{}
	for name := range externalsToConfig {
		externalsReq[name] = true
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

	err = r.libr.CatalogForSwitch(ctx, r.Client, cat, sw.Name, loWorkaroundLinks, loWorkaroundReqs, externalsReq, subnetsReq)
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

	alloyCfg := alloy.Config{
		Hostname: sw.Name,
		ProxyURL: r.cfg.ControlProxyURL,
		Targets:  r.cfg.AlloyTargets,
		Scrapes: map[string]alloy.Scrape{
			"alloy": {
				Self: alloy.ScrapeSelf{
					Enable: true,
				},
				IntervalSeconds: 120,
			},
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

	agent := &agentapi.Agent{ObjectMeta: switchNsName}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, agent, func() error {
		agent.Annotations = sw.Annotations
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
			MCLAGSessionSubnet:    r.cfg.MCLAGSessionSubnet,
			GatewayASN:            r.cfg.GatewayASN,
			LoopbackWorkaround:    r.cfg.LoopbackWorkaround,
			ProtocolSubnet:        r.cfg.ProtocolSubnet,
			VTEPSubnet:            r.cfg.VTEPSubnet,
			FabricSubnet:          r.cfg.FabricSubnet,
			DisableBFD:            r.cfg.DisableBFD,
			Alloy:                 alloyCfg,
		}
		if r.cfg.FabricMode == fmeta.FabricModeSpineLeaf {
			agent.Spec.Config.SpineLeaf = &agentapi.AgentSpecConfigSpineLeaf{}
		}

		return nil
	})
	if err != nil {
		return kctrl.Result{}, errors.Wrapf(err, "error creating agent")
	}

	l.Info("agent reconciled")

	return kctrl.Result{}, nil
}

func (r *AgentReconciler) prepareAgentInfra(ctx context.Context, ag kmetav1.ObjectMeta) (*kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	saName := AgentServiceAccount(ag.Name)
	sa := &corev1.ServiceAccount{ObjectMeta: kmetav1.ObjectMeta{Namespace: ag.Namespace, Name: saName}}
	_, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error { return nil })
	if err != nil {
		return nil, errors.Wrapf(err, "error creating service account")
	}

	role := &rbacv1.Role{ObjectMeta: kmetav1.ObjectMeta{Namespace: ag.Namespace, Name: saName}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents"},
				ResourceNames: []string{ag.Name},
				Verbs:         []string{"get", "watch"},
			},
			{
				APIGroups:     []string{agentapi.GroupVersion.Group},
				Resources:     []string{"agents/status"},
				ResourceNames: []string{ag.Name},
				Verbs:         []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating role")
	}

	roleBinding := &rbacv1.RoleBinding{ObjectMeta: kmetav1.ObjectMeta{Namespace: ag.Namespace, Name: saName}}
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

	tokenSecret := &corev1.Secret{ObjectMeta: kmetav1.ObjectMeta{Namespace: ag.Namespace, Name: saName + "-satoken"}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, tokenSecret, func() error {
		if tokenSecret.Annotations == nil {
			tokenSecret.Annotations = map[string]string{}
		}

		tokenSecret.Annotations[corev1.ServiceAccountNameKey] = saName
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

		return &kctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	kubeconfig, err := r.genKubeconfig(tokenSecret)
	if err != nil {
		return nil, errors.Wrapf(err, "error generating kubeconfig")
	}

	secretName := AgentKubeconfigSecret(ag.Name)
	kubeconfigSecret := &corev1.Secret{ObjectMeta: kmetav1.ObjectMeta{Namespace: ag.Namespace, Name: secretName}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, kubeconfigSecret, func() error {
		kubeconfigSecret.StringData = map[string]string{
			AgentKubeconfigKey: kubeconfig,
		}

		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error creating kubeconfig secret")
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

func appendUpdate(statusUpdates []agentapi.ApplyStatusUpdate, obj kclient.Object) []agentapi.ApplyStatusUpdate {
	return append(statusUpdates, agentapi.ApplyStatusUpdate{
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Generation: obj.GetGeneration(),
	})
}
