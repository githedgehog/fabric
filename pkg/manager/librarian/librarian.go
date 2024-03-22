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
	"math"
	"sync"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	NS                = "default" // think about more than default namespace support
	CAT_CONNS         = "connections"
	CAT_VPCs          = "vpcs"
	CAT_SW_PREFIX     = "switch."
	CAT_RG_PREFIX     = "redundancy."
	VPC_VNI_OFFSET    = 100
	VPC_VNI_MAX       = (16_777_215 - VPC_VNI_OFFSET) / VPC_VNI_OFFSET * VPC_VNI_OFFSET
	PORT_CHAN_MIN     = 1
	PORT_CHAN_MAX     = 249
	LO_REQ_PREFIX_VPC = "vpc@"
	LO_REQ_PREFIX_EXT = "ext@"
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

func (m *Manager) getCatalog(ctx context.Context, kube client.Client, key string) (*agentapi.Catalog, error) {
	cat := &agentapi.Catalog{}
	if err := kube.Get(ctx, types.NamespacedName{Name: key, Namespace: NS}, cat); client.IgnoreNotFound(err) != nil {
		return nil, errors.Wrapf(err, "failed to get catalog %s", key)
	} else {
		cat.Name = key
		cat.Namespace = NS
	}

	if cat.Spec.ConnectionIDs == nil {
		cat.Spec.ConnectionIDs = map[string]uint32{}
	}
	if cat.Spec.VPCVNIs == nil {
		cat.Spec.VPCVNIs = map[string]uint32{}
	}
	if cat.Spec.VPCSubnetVNIs == nil {
		cat.Spec.VPCSubnetVNIs = map[string]map[string]uint32{}
	}
	if cat.Spec.IRBVLANs == nil {
		cat.Spec.IRBVLANs = map[string]uint16{}
	}
	if cat.Spec.PortChannelIDs == nil {
		cat.Spec.PortChannelIDs = map[string]uint16{}
	}

	return cat, nil
}

func (m *Manager) saveCatalog(ctx context.Context, kube client.Client, key string, cat *agentapi.Catalog) error {
	if err := kube.Update(ctx, cat); err != nil {
		if apierrors.IsNotFound(err) {
			if err := kube.Create(ctx, cat); err != nil {
				return errors.Wrapf(err, "failed to create catalog %s", key)
			}
		} else {
			return errors.Wrapf(err, "failed to update catalog %s", key)
		}
	}

	return nil
}

func (m *Manager) UpdateConnections(ctx context.Context, kube client.Client) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cat, err := m.getCatalog(ctx, kube, CAT_CONNS)
	if err != nil {
		return err
	}

	connList := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, connList, client.MatchingLabels{
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

	return m.saveCatalog(ctx, kube, CAT_CONNS, cat)
}

func (m *Manager) UpdateVPCs(ctx context.Context, kube client.Client) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cat, err := m.getCatalog(ctx, kube, CAT_VPCs)
	if err != nil {
		return err
	}

	vpcList := &vpcapi.VPCList{}
	if err := kube.List(ctx, vpcList); err != nil {
		return errors.Wrapf(err, "error listing VPCs")
	}

	vpcs := map[string]bool{}
	for _, vpc := range vpcList.Items {
		vpcs[vpc.Name] = true
	}

	a := &Allocator[uint32]{
		Values: NewNextFreeValueFromRanges([][2]uint32{{VPC_VNI_OFFSET, VPC_VNI_MAX}}, VPC_VNI_OFFSET),
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
			Values: NewNextFreeValueFromRanges([][2]uint32{{vpcVNI + 1, vpcVNI + VPC_VNI_OFFSET - 1}}, 1),
		}
		cat.Spec.VPCSubnetVNIs[vpc.Name], err = a.Allocate(cat.Spec.VPCSubnetVNIs[vpc.Name], subnets)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate VPC subnet VNIs for %s", vpc.Name)
		}
	}

	return m.saveCatalog(ctx, kube, CAT_VPCs, cat)
}

func (m *Manager) getRedundancyGroupKey(swName string, redundancy wiringapi.SwitchRedundancy) string {
	if redundancy.Type == meta.RedundancyTypeNone || redundancy.Group == "" {
		return m.getSwitchKey(swName)
	}

	return CAT_RG_PREFIX + redundancy.Group
}

func (m *Manager) getSwitchKey(swName string) string {
	return CAT_SW_PREFIX + swName
}

func (m *Manager) CatalogForRedundancyGroup(ctx context.Context, kube client.Client, ret *agentapi.CatalogSpec, swName string, redundancy wiringapi.SwitchRedundancy, vpcs, portChanConns, idConns map[string]bool) error {
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
		cat.Spec.IRBVLANs, err = a.Allocate(cat.Spec.IRBVLANs, vpcs)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate IRB VLANs for %s", key)
		}
	}

	{
		a := &Allocator[uint16]{
			Values: NewNextFreeValueFromRanges([][2]uint16{{PORT_CHAN_MIN, PORT_CHAN_MAX}}, 1),
		}
		cat.Spec.PortChannelIDs, err = a.Allocate(cat.Spec.PortChannelIDs, portChanConns)
		if err != nil {
			return errors.Wrapf(err, "failed to allocate PortChannel IDs for %s", key)
		}
	}

	if err := m.saveCatalog(ctx, kube, key, cat); err != nil {
		return errors.Errorf("failed to save catalog %s", key)
	}

	connsCat, err := m.getCatalog(ctx, kube, CAT_CONNS)
	if err != nil {
		return errors.Errorf("failed to get connections catalog %s", CAT_CONNS)
	}

	vpcsCat, err := m.getCatalog(ctx, kube, CAT_VPCs)
	if err != nil {
		return errors.Errorf("failed to get VPCs catalog %s", CAT_VPCs)
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
		if vni, exists := vpcsCat.Spec.VPCVNIs[name]; exists {
			ret.VPCVNIs[name] = vni
			ret.VPCSubnetVNIs[name] = vpcsCat.Spec.VPCSubnetVNIs[name] // TODO pass configured subnets and check if they exist or even pass only configured ones
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

func (m *Manager) CatalogForSwitch(ctx context.Context, kube client.Client, ret *agentapi.CatalogSpec, swName string, loWorkaroundLinks []string, loWorkaroundReqs, externals, subnets map[string]bool) error {
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
	return LO_REQ_PREFIX_VPC + vpcPeeringName
}

func LoWReqForExt(extPeeringName string) string {
	return LO_REQ_PREFIX_EXT + extPeeringName
}
