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

package apiabbr

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	VPCParamDefaultIsolated   = []string{"defaultIsolated", "defI", "i"}
	VPCParamDefaultRestricted = []string{"defaultRestricted", "defR", "r"}
	VPCParamIPNS              = []string{"ipns"}
	VPCParamVLANNS            = []string{"vlanns"}
	VPCParamSubnets           = []string{"subnet", "s"}
	VPCParamPermits           = []string{"permit", "p"}

	VPCParams = [][]string{
		VPCParamDefaultIsolated,
		VPCParamDefaultRestricted,
		VPCParamIPNS,
		VPCParamVLANNS,
		VPCParamSubnets,
		VPCParamPermits,
	}
)

func newVPCHandler(ignoreNotDefined bool) (*ObjectAbbrHandler[*vpcapi.VPC, *vpcapi.VPCList], error) {
	return (&ObjectAbbrHandler[*vpcapi.VPC, *vpcapi.VPCList]{
		AbbrType:          AbbrTypeVPC,
		CleanupNotDefined: !ignoreNotDefined,
		AcceptedParams:    VPCParams,
		AcceptNoTypeFn: func(abbr string) bool {
			return strings.HasPrefix(abbr, "vpc-") &&
				!strings.Contains(abbr, VPCAttachmentAbbrSeparator) &&
				!strings.Contains(abbr, VPCPeeringAbbrSeparator) &&
				!strings.Contains(abbr, ExternalPeeringSeparator)
		},
		ParseObjectFn: func(name, _ string, params AbbrParams) (*vpcapi.VPC, error) {
			spec := vpcapi.VPCSpec{
				Subnets: map[string]*vpcapi.VPCSubnet{},
			}
			var err error

			if spec.DefaultIsolated, err = params.GetBool(VPCParamDefaultIsolated); err != nil {
				return nil, err
			}
			if spec.DefaultRestricted, err = params.GetBool(VPCParamDefaultRestricted); err != nil {
				return nil, err
			}
			if spec.IPv4Namespace, err = params.GetString(VPCParamIPNS); err != nil {
				return nil, err
			}
			if spec.VLANNamespace, err = params.GetString(VPCParamVLANNS); err != nil {
				return nil, err
			}

			for _, subnetRaw := range params.GetStringSlice(VPCParamSubnets) {
				name, subnet, err := ParseVPCSubnet(subnetRaw)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse subnet: %s", subnetRaw)
				}

				spec.Subnets[name] = subnet
			}

			for _, permitRaw := range params.GetStringSlice(VPCParamPermits) {
				permit, err := ParseVPCPermits(permitRaw)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse permit entry: %s", permitRaw)
				}

				spec.Permit = append(spec.Permit, permit)
			}

			return &vpcapi.VPC{
				TypeMeta:   kmetav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPC},
				ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
				Spec:       spec,
			}, nil
		},
		ObjectListFn: func(ctx context.Context, kube kclient.Client) (*vpcapi.VPCList, error) {
			list := &vpcapi.VPCList{}

			return list, kube.List(ctx, list)
		},
		CreateOrUpdateFn: func(ctx context.Context, kube kclient.Client, newObj *vpcapi.VPC) (ctrlutil.OperationResult, error) {
			// TODO if no subnets assigned to the VPC, consider auto allocate some from the used IP namespace

			vpc := &vpcapi.VPC{ObjectMeta: newObj.ObjectMeta}

			return ctrlutil.CreateOrUpdate(ctx, kube, vpc, func() error {
				vpc.Spec = newObj.Spec

				return nil
			})
		},
	}).Init()
}

const (
	VPCAttachmentAbbrSeparator = "@"
)

var (
	VPCAttachmentParamNativeVLAN = []string{"native-vlan", "native", "nv"}

	VPCAttachmentParams = [][]string{
		VPCAttachmentParamNativeVLAN,
	}
)

func newVPCAttachmentHandler(ignoreNotDefined bool) (*ObjectAbbrHandler[*vpcapi.VPCAttachment, *vpcapi.VPCAttachmentList], error) {
	return (&ObjectAbbrHandler[*vpcapi.VPCAttachment, *vpcapi.VPCAttachmentList]{
		AbbrType:          AbbrTypeVPCAttachment,
		CleanupNotDefined: !ignoreNotDefined,
		AcceptedParams:    VPCAttachmentParams,
		AcceptNoTypeFn:    func(abbr string) bool { return strings.Contains(abbr, "@") },
		NameFn: func(abbr string) string {
			return strings.ReplaceAll(strings.ReplaceAll(abbr, "/", "--"), VPCAttachmentAbbrSeparator, "--")
		},
		ParseObjectFn: func(name, abbr string, params AbbrParams) (*vpcapi.VPCAttachment, error) {
			spec := vpcapi.VPCAttachmentSpec{}

			parts := strings.Split(abbr, VPCAttachmentAbbrSeparator)
			if len(parts) != 2 {
				return nil, errors.New("VPCAttachment abbr should contain exactly vpc/subnet and connection (or server) name separated by " + VPCAttachmentAbbrSeparator)
			}

			if len(strings.Split(abbr, "/")) != 2 {
				return nil, errors.New("VPCAttachment abbr should contain full vpc/subnet name")
			}

			spec.Subnet = parts[0]
			spec.Connection = parts[1]

			var err error
			if spec.NativeVLAN, err = params.GetBool(VPCAttachmentParamNativeVLAN); err != nil {
				return nil, err
			}

			return &vpcapi.VPCAttachment{
				TypeMeta:   kmetav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPCAttachment},
				ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
				Spec:       spec,
			}, nil
		},
		ObjectListFn: func(ctx context.Context, kube kclient.Client) (*vpcapi.VPCAttachmentList, error) {
			list := &vpcapi.VPCAttachmentList{}

			return list, kube.List(ctx, list)
		},
		CreateOrUpdateFn: func(ctx context.Context, kube kclient.Client, newObj *vpcapi.VPCAttachment) (ctrlutil.OperationResult, error) {
			conn := &wiringapi.Connection{}
			if err := kube.Get(ctx, kclient.ObjectKey{Name: newObj.Spec.Connection, Namespace: kmetav1.NamespaceDefault}, conn); err != nil {
				if kclient.IgnoreNotFound(err) != nil {
					return ctrlutil.OperationResultNone, errors.Wrapf(err, "cannot get connection")
				}

				serverName := newObj.Spec.Connection
				srv := &wiringapi.Server{}
				if err := kube.Get(ctx, kclient.ObjectKey{Name: serverName, Namespace: kmetav1.NamespaceDefault}, srv); err != nil {
					return ctrlutil.OperationResultNone, errors.Wrapf(err, "cannot get server %s", serverName)
				}

				connList := &wiringapi.ConnectionList{}
				if err := kube.List(ctx, connList, wiringapi.MatchingLabelsForListLabelServer(serverName)); err != nil {
					return ctrlutil.OperationResultNone, errors.Wrapf(err, "cannot list connections for server %s", serverName)
				}

				if len(connList.Items) == 0 {
					return ctrlutil.OperationResultNone, errors.Errorf("no connections found for the server %s", serverName)
				} else if len(connList.Items) > 1 {
					return ctrlutil.OperationResultNone, errors.Errorf("multiple connections found for the server %s", serverName)
				}

				newObj.Spec.Connection = connList.Items[0].Name
			}

			attachment := &vpcapi.VPCAttachment{ObjectMeta: newObj.ObjectMeta}

			return ctrlutil.CreateOrUpdate(ctx, kube, attachment, func() error {
				attachment.Spec = newObj.Spec

				return nil
			})
		},
	}).Init()
}

const (
	VPCPeeringAbbrSeparator = "+"
)

var (
	VPCPeeringParamRemote = []string{"remote", "r"}
	VPCPeeringParamPermit = []string{"permit", "p"}

	VPCPeeringParams = [][]string{
		VPCPeeringParamRemote,
		VPCPeeringParamPermit,
	}
)

func newVPCPeeringHandler(ignoreNotDefined bool) (*ObjectAbbrHandler[*vpcapi.VPCPeering, *vpcapi.VPCPeeringList], error) {
	return (&ObjectAbbrHandler[*vpcapi.VPCPeering, *vpcapi.VPCPeeringList]{
		AbbrType:          AbbrTypeVPCPeering,
		CleanupNotDefined: !ignoreNotDefined,
		AcceptedParams:    VPCPeeringParams,
		AcceptNoTypeFn:    func(abbr string) bool { return strings.Contains(abbr, "+") },
		NameFn: func(abbr string) string {
			return strings.ReplaceAll(abbr, VPCPeeringAbbrSeparator, "--")
		},
		ParseObjectFn: func(name, abbr string, params AbbrParams) (*vpcapi.VPCPeering, error) {
			spec := vpcapi.VPCPeeringSpec{
				Permit: []map[string]vpcapi.VPCPeer{},
			}
			var err error

			vpcNames := strings.Split(abbr, VPCPeeringAbbrSeparator)
			if len(vpcNames) != 2 {
				return nil, errors.New("VPCPeering abbr should contain exactly two VPC names separated by " + VPCPeeringAbbrSeparator)
			}

			if spec.Remote, err = params.GetString(VPCPeeringParamRemote); err != nil {
				return nil, err
			}

			for _, permitRaw := range params.GetStringSlice(VPCPeeringParamPermit) {
				permit, err := ParseVPCPeeringPermits(vpcNames[0], vpcNames[1], name)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to parse permit entry: %s", permitRaw)
				}

				spec.Permit = append(spec.Permit, permit)
			}

			if len(spec.Permit) == 0 {
				spec.Permit = []map[string]vpcapi.VPCPeer{
					{
						vpcNames[0]: {},
						vpcNames[1]: {},
					},
				}
			}

			return &vpcapi.VPCPeering{
				TypeMeta:   kmetav1.TypeMeta{APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPCPeering},
				ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
				Spec:       spec,
			}, nil
		},
		ObjectListFn: func(ctx context.Context, kube kclient.Client) (*vpcapi.VPCPeeringList, error) {
			list := &vpcapi.VPCPeeringList{}

			return list, kube.List(ctx, list)
		},
		CreateOrUpdateFn: func(ctx context.Context, kube kclient.Client, newObj *vpcapi.VPCPeering) (ctrlutil.OperationResult, error) {
			peering := &vpcapi.VPCPeering{ObjectMeta: newObj.ObjectMeta}

			return ctrlutil.CreateOrUpdate(ctx, kube, peering, func() error {
				peering.Spec = newObj.Spec

				return nil
			})
		},
	}).Init()
}

func ParseVPCSubnet(in string) (string, *vpcapi.VPCSubnet, error) {
	parts := strings.Split(in, ",")

	name := ""
	subnet := &vpcapi.VPCSubnet{
		DHCP: vpcapi.VPCDHCP{
			Range:   &vpcapi.VPCDHCPRange{},
			Options: &vpcapi.VPCDHCPOptions{},
		},
	}

	for idx, part := range parts {
		part := strings.TrimSpace(part)

		kv := strings.Split(part, "=")
		if len(kv) == 0 || len(kv) > 2 {
			return "", nil, errors.Errorf("invalid key-value pair: %s (expected k=v or k)", part)
		}

		key := strings.TrimSpace(kv[0])
		value := ""
		if len(kv) == 2 {
			value = strings.TrimSpace(kv[1])
		}

		trueVal := slices.Contains(TrueValsDefault, value)

		if idx == 0 {
			name = key
			subnet.Subnet = value
		} else if key == "vlan" {
			vlan, err := strconv.ParseUint(value, 10, 16)
			if err != nil {
				return "", nil, errors.Wrapf(err, "failed to parse VLAN: %s", value)
			}

			subnet.VLAN = uint16(vlan)
		} else if key == "isolated" || key == "i" {
			if trueVal {
				subnet.Isolated = pointer.To(true)
			}
		} else if key == "restricted" || key == "r" {
			if trueVal {
				subnet.Restricted = pointer.To(true)
			}
		} else if key == "dhcp" {
			if trueVal {
				subnet.DHCP.Enable = true
			}
		} else if key == "dhcp-start" {
			subnet.DHCP.Range.Start = value
		} else if key == "dhcp-end" {
			subnet.DHCP.Range.End = value
		} else if key == "dhcp-relay" {
			subnet.DHCP.Relay = value
		} else if key == "dhcp-pxe-url" {
			subnet.DHCP.Options.PXEURL = value
		} else {
			return "", nil, errors.Errorf("unknown key: %s", key)
		}
	}

	if !subnet.DHCP.Enable {
		subnet.DHCP.Range = nil
		subnet.DHCP.Options = nil
	}

	if name == "" {
		return "", nil, errors.New("subnet name is required")
	}

	return name, subnet, nil
}

func ParseVPCPermits(in string) ([]string, error) {
	parts := strings.Split(in, ",")

	if len(parts) < 2 {
		return nil, errors.New("permit entry should contain at least two subnets")
	}

	permits := []string{}

	for _, part := range parts {
		permits = append(permits, strings.TrimSpace(part))
	}

	return permits, nil
}

func ParseVPCPeeringPermits(vpc1, vpc2, in string) (map[string]vpcapi.VPCPeer, error) {
	vpcParts := strings.Split(in, "~")

	if len(vpcParts) != 2 {
		return nil, errors.New("permit entry should contain exactly two VPCs")
	}

	vpc1Subnets := strings.Split(vpcParts[0], ",")
	if len(vpc1Subnets) == 1 && vpc1Subnets[0] == "" {
		vpc1Subnets = nil
	}

	vpc2Subnets := strings.Split(vpcParts[1], ",")
	if len(vpc2Subnets) == 1 && vpc2Subnets[0] == "" {
		vpc2Subnets = nil
	}

	return map[string]vpcapi.VPCPeer{
		vpc1: {
			Subnets: vpc1Subnets,
		},
		vpc2: {
			Subnets: vpc2Subnets,
		},
	}, nil
}
