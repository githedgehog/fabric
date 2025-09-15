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

package librarian

import (
	"context"
	"maps"
	"math"
	"sync"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Namespace                = kmetav1.NamespaceDefault
	CatConns                 = "connections"
	CatVNIs                  = "vpcs" // contains both VPC and External VNIs
	CatSwitchPrefix          = "switch."
	CatRedGroupPrefix        = "redundancy."
	VPCVNIOffset             = 100
	VPCVNIMax                = (16_777_215 - VPCVNIOffset) / VPCVNIOffset * VPCVNIOffset
	PortChannelMin           = 1
	PortChannelMax           = 249
	LoWorkaroundReqPrefixVPC = "vpc@"
	LoWorkaroundReqPrefixExt = "ext@"
)

type Manager struct {
	cfg   *meta.FabricConfig
	mutex sync.RWMutex
}

func NewManager(cfg *meta.FabricConfig) *Manager {
	return &Manager{
		cfg: cfg,
	}
}

func (m *Manager) getCatalog(ctx context.Context, kube kclient.Client, key string) (*agentapi.Catalog, error) {
	cat := &agentapi.Catalog{}
	err := kube.Get(ctx, ktypes.NamespacedName{Name: key, Namespace: Namespace}, cat)
	if kclient.IgnoreNotFound(err) != nil {
		return nil, errors.Wrapf(err, "failed to get catalog %s", key)
	}

	cat.Name = key
	cat.Namespace = Namespace

	if cat.Spec.ConnectionIDs == nil {
		cat.Spec.ConnectionIDs = map[string]uint32{}
	}
	if cat.Spec.VPCVNIs == nil {
		cat.Spec.VPCVNIs = map[string]uint32{}
	}
	if cat.Spec.VPCSubnetVNIs == nil {
		cat.Spec.VPCSubnetVNIs = map[string]map[string]uint32{}
	}
	if cat.Spec.ExternalVNIs == nil {
		cat.Spec.ExternalVNIs = map[string]uint32{}
	}
	if cat.Spec.IRBVLANs == nil {
		cat.Spec.IRBVLANs = map[string]uint16{}
	}
	if cat.Spec.PortChannelIDs == nil {
		cat.Spec.PortChannelIDs = map[string]uint16{}
	}

	return cat, nil
}

func (m *Manager) saveCatalog(ctx context.Context, kube kclient.Client, key string, cat *agentapi.Catalog) error {
	if err := kube.Update(ctx, cat); err != nil {
		if kapierrors.IsNotFound(err) {
			if err := kube.Create(ctx, cat); err != nil {
				return errors.Wrapf(err, "failed to create catalog %s", key)
			}
		} else {
			return errors.Wrapf(err, "failed to update catalog %s", key)
		}
	}

	return nil
}

func (m *Manager) UpdateConnections(ctx context.Context, kube kclient.Client) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cat, err := m.getCatalog(ctx, kube, CatConns)
	if err != nil {
		return err
	}

	connList := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, connList, kclient.MatchingLabels{
		wiringapi.LabelConnectionType: wiringapi.ConnectionTypeESLAG,
	}); err != nil {
		return errors.Wrapf(err, "error listing ESLAG connections")
	}

	conns := map[string]bool{}
	for _, conn := range connList.Items {
		conns[conn.Name] = true
	}

	a := &Allocator[uint32]{
		Values: NewNextFreeValueFromRanges([][2]uint32{{1, math.MaxUint32}}, 1), // TODO replace with some kind of range from config
	}
	cat.Spec.ConnectionIDs, err = a.Allocate(cat.Spec.ConnectionIDs, conns)
	if err != nil {
		return errors.Wrapf(err, "failed to allocate connection IDs")
	}

	return m.saveCatalog(ctx, kube, CatConns, cat)
}

func (m *Manager) UpdateVNIs(ctx context.Context, kube kclient.Client) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cat, err := m.getCatalog(ctx, kube, CatVNIs)
	if err != nil {
		return err
	}

	vpcList := &vpcapi.VPCList{}
	if err := kube.List(ctx, vpcList); err != nil {
		return errors.Wrapf(err, "error listing VPCs")
	}

	externalList := &vpcapi.ExternalList{}
	if err := kube.List(ctx, externalList); err != nil {
		return errors.Wrapf(err, "error listing externals")
	}

	vpcs := map[string]bool{}
	for _, vpc := range vpcList.Items {
		vpcs[vpc.Name] = true
	}

	exts := map[string]bool{}
	for _, ext := range externalList.Items {
		exts[ext.Name] = true
	}

	a := &Allocator[uint32]{
		Values: NewNextFreeValueFromRanges([][2]uint32{{VPCVNIOffset, VPCVNIMax}}, VPCVNIOffset),
	}

	cat.Spec.ExternalVNIs, err = a.Allocate(cat.Spec.ExternalVNIs, exts)
	if err != nil {
		return errors.Wrapf(err, "failed to allocate external VNIs")
	}

	cat.Spec.VPCVNIs, err = a.Allocate(cat.Spec.VPCVNIs, vpcs)
	if err != nil {
		return errors.Wrapf(err, "failed to allocate VPC VNIs")
	}

	for _, vpc := range vpcList.Items {
		subnets := map[string]bool{}
		for subnetName := range vpc.Spec.Subnets {
			subnets[subnetName] = true
		}

		vpcVNI := cat.Spec.VPCVNIs[vpc.Name]
		a := &Allocator[uint32]{
			Values: NewNextFreeValueFromRanges([][2]uint32{{vpcVNI + 1, vpcVNI + VPCVNIOffset - 1}}, 1),
		}
		cat.Spec.VPCSubnetVNIs[vpc.Name], err = a.Allocate(cat.Spec.VPCSubnetVNIs[vpc.Name], subnets)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate VPC subnet VNIs for %s", vpc.Name)
		}
	}

	return m.saveCatalog(ctx, kube, CatVNIs, cat)
}

func (m *Manager) getRedundancyGroupKey(swName string, redundancy wiringapi.SwitchRedundancy) string {
	if redundancy.Type == meta.RedundancyTypeNone || redundancy.Group == "" {
		return m.getSwitchKey(swName)
	}

	return CatRedGroupPrefix + redundancy.Group
}

func (m *Manager) getSwitchKey(swName string) string {
	return CatSwitchPrefix + swName
}

func (m *Manager) CatalogForRedundancyGroup(ctx context.Context, kube kclient.Client, ret *agentapi.CatalogSpec, swName string, redundancy wiringapi.SwitchRedundancy, vpcs, portChanConns, idConns map[string]bool, externals map[string]bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := m.getRedundancyGroupKey(swName, redundancy)

	cat, err := m.getCatalog(ctx, kube, key)
	if err != nil {
		return errors.Errorf("failed to get switch/redundancy catalog %s", key)
	}

	{
		a := &Allocator[uint16]{
			Values: NewNextFreeValueFromVLANRanges(m.cfg.VPCIRBVLANRanges),
		}
		extVlans, err := a.Allocate(cat.Spec.IRBVLANs, externals)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate IRB VLANs for externals %s", key)
		}

		cat.Spec.IRBVLANs, err = a.Allocate(cat.Spec.IRBVLANs, vpcs)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate IRB VLANs for %s", key)
		}
		maps.Copy(cat.Spec.IRBVLANs, extVlans)
	}

	{
		a := &Allocator[uint16]{
			Values: NewNextFreeValueFromRanges([][2]uint16{{PortChannelMin, PortChannelMax}}, 1),
		}
		cat.Spec.PortChannelIDs, err = a.Allocate(cat.Spec.PortChannelIDs, portChanConns)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate PortChannel IDs for %s", key)
		}
	}

	if err := m.saveCatalog(ctx, kube, key, cat); err != nil {
		return errors.Errorf("failed to save catalog %s", key)
	}

	connsCat, err := m.getCatalog(ctx, kube, CatConns)
	if err != nil {
		return errors.Errorf("failed to get connections catalog %s", CatConns)
	}

	vnisCat, err := m.getCatalog(ctx, kube, CatVNIs)
	if err != nil {
		return errors.Errorf("failed to get VNIs catalog %s", CatVNIs)
	}

	ret.ConnectionIDs = map[string]uint32{}
	for name := range idConns {
		if id, exists := connsCat.Spec.ConnectionIDs[name]; exists {
			ret.ConnectionIDs[name] = id
		} else {
			return errors.Errorf("failed to find ID for connection %s", name)
		}
	}

	ret.VPCVNIs = map[string]uint32{}
	ret.VPCSubnetVNIs = map[string]map[string]uint32{}
	for name := range vpcs {
		if vni, exists := vnisCat.Spec.VPCVNIs[name]; exists {
			ret.VPCVNIs[name] = vni
			ret.VPCSubnetVNIs[name] = vnisCat.Spec.VPCSubnetVNIs[name] // TODO pass configured subnets and check if they exist or even pass only configured ones
		} else {
			return errors.Errorf("failed to find VPC VNI for vpc %s", name)
		}
	}

	ret.IRBVLANs = map[string]uint16{}
	for name := range vpcs {
		if vlan, exists := cat.Spec.IRBVLANs[name]; exists {
			ret.IRBVLANs[name] = vlan
		} else {
			return errors.Errorf("failed to find IRB VLAN for vpc %s", name)
		}
	}
	for name := range externals {
		if vlan, exists := cat.Spec.IRBVLANs[name]; exists {
			ret.IRBVLANs[name] = vlan
		} else {
			return errors.Errorf("failed to find IRB VLAN for external %s", name)
		}
	}

	ret.PortChannelIDs = map[string]uint16{}
	for name := range portChanConns {
		if id, exists := cat.Spec.PortChannelIDs[name]; exists {
			ret.PortChannelIDs[name] = id
		} else {
			return errors.Errorf("failed to find PortChannel ID for connection %s", name)
		}
	}

	return nil
}

func (m *Manager) CatalogForSwitch(ctx context.Context, kube kclient.Client, ret *agentapi.CatalogSpec, swName string, loWorkaroundLinks []string, loWorkaroundReqs, externals, subnets map[string]bool) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := m.getSwitchKey(swName)

	cat, err := m.getCatalog(ctx, kube, key)
	if err != nil {
		return errors.Errorf("failed to get switch catalog %s", key)
	}

	{
		a := Allocator[string]{
			Values: NewBalancedValues(loWorkaroundLinks),
		}
		cat.Spec.LooopbackWorkaroundLinks, err = a.Allocate(cat.Spec.LooopbackWorkaroundLinks, loWorkaroundReqs)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate loopback workaround links for %s", key)
		}
	}

	{
		a := Allocator[uint16]{
			Values: NewNextFreeValueFromVLANRanges(m.cfg.VPCPeeringVLANRanges),
		}
		cat.Spec.LoopbackWorkaroundVLANs, err = a.Allocate(cat.Spec.LoopbackWorkaroundVLANs, loWorkaroundReqs)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate loopback workaround VLANs for %s", key)
		}
	}

	{
		a := Allocator[uint16]{
			Values: NewNextFreeValueFromRanges([][2]uint16{{10, math.MaxUint16}}, 1),
		}
		cat.Spec.ExternalIDs, err = a.Allocate(cat.Spec.ExternalIDs, externals)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate external IDs for %s", key)
		}
	}

	{
		a := Allocator[uint32]{
			Values: NewNextFreeValueFromRanges([][2]uint32{{100, 64999}}, 1),
		}
		cat.Spec.SubnetIDs, err = a.Allocate(cat.Spec.SubnetIDs, subnets)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate subnet IDs for %s", key)
		}
	}

	vnisCat, err := m.getCatalog(ctx, kube, CatVNIs)
	if err != nil {
		return errors.Errorf("failed to get VNIs catalog %s", CatVNIs)
	}
	ret.ExternalVNIs = map[string]uint32{}
	for ext := range externals {
		if vni, exists := vnisCat.Spec.ExternalVNIs[ext]; exists {
			ret.ExternalVNIs[ext] = vni
		} else {
			return errors.Errorf("failed to find external VNI for %s", ext)
		}
	}

	if err := m.saveCatalog(ctx, kube, key, cat); err != nil {
		return errors.Errorf("failed to save switch catalog %s", key)
	}

	ret.LooopbackWorkaroundLinks = cat.Spec.LooopbackWorkaroundLinks
	ret.LoopbackWorkaroundVLANs = cat.Spec.LoopbackWorkaroundVLANs
	for req := range loWorkaroundReqs {
		if _, exists := ret.LooopbackWorkaroundLinks[req]; !exists {
			return errors.Errorf("failed to find loopback workaround link for %s", req)
		}
	}

	ret.ExternalIDs = cat.Spec.ExternalIDs
	for ext := range externals {
		if _, exists := ret.ExternalIDs[ext]; !exists {
			return errors.Errorf("failed to find external ID for %s", ext)
		}
	}

	ret.SubnetIDs = cat.Spec.SubnetIDs
	for prefix := range subnets {
		if _, exists := ret.SubnetIDs[prefix]; !exists {
			return errors.Errorf("failed to find external peering prefix ID for %s", prefix)
		}
	}

	return nil
}

func LoWReqForVPC(vpcPeeringName string) string {
	return LoWorkaroundReqPrefixVPC + vpcPeeringName
}

func LoWReqForExt(extPeeringName string) string {
	return LoWorkaroundReqPrefixExt + extPeeringName
}

func (m *Manager) GetVPCVNI(ctx context.Context, kube kclient.Client, vpc string) (uint32, error) {
	vnisCat, err := m.getCatalog(ctx, kube, CatVNIs)
	if err != nil {
		return 0, errors.Errorf("failed to get VNIs catalog %s", CatVNIs)
	}

	if vni, exists := vnisCat.Spec.VPCVNIs[vpc]; exists {
		return vni, nil
	}

	return 0, errors.Errorf("failed to find VPC VNI for vpc %s", vpc)
}

func (m *Manager) GetExternalVNI(ctx context.Context, kube kclient.Client, external string) (uint32, error) {
	vnisCat, err := m.getCatalog(ctx, kube, CatVNIs)
	if err != nil {
		return 0, errors.Errorf("failed to get VNIs catalog %s", CatVNIs)
	}

	if vni, exists := vnisCat.Spec.ExternalVNIs[external]; exists {
		return vni, nil
	}

	return 0, errors.Errorf("failed to find VNI for external %s", external)
}
