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

package hhfctl

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type VPCCreateOptions struct {
	Name   string
	Subnet string
	VLAN   uint16
	DHCP   vpcapi.VPCDHCP
}

func VPCCreate(ctx context.Context, printYaml bool, options *VPCCreateOptions) error {
	vpc := &vpcapi.VPC{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: vpcapi.VPCSpec{
			Subnets: map[string]*vpcapi.VPCSubnet{
				"default": {
					Subnet: options.Subnet,
					VLAN:   options.VLAN,
					DHCP:   options.DHCP,
				},
			},
		},
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	vpc.Default()
	warnings, err := vpc.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil, nil)
	if err != nil {
		slog.Warn("Validation", "error", err)

		return errors.Errorf("validation failed")
	}
	if warnings != nil {
		slog.Warn("Validation", "warnings", warnings)
	}

	err = kube.Create(ctx, vpc)
	if err != nil {
		return errors.Wrap(err, "cannot create vpc")
	}

	slog.Info("VPC created", "name", vpc.Name)

	if printYaml {
		vpc.ObjectMeta.ManagedFields = nil
		vpc.ObjectMeta.Generation = 0
		vpc.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(vpc)
		if err != nil {
			return errors.Wrap(err, "cannot marshal vpc")
		}

		fmt.Println(string(out))
	}

	return nil
}

type VPCAttachOptions struct {
	Name       string
	VPCSubnet  string
	Connection string
}

func VPCAttach(ctx context.Context, printYaml bool, options *VPCAttachOptions) error {
	name := options.Name
	if name == "" {
		name = fmt.Sprintf("%s--%s", strings.ReplaceAll(options.VPCSubnet, "/", "--"), options.Connection)
	}

	attach := &vpcapi.VPCAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: vpcapi.VPCAttachmentSpec{
			Subnet:     options.VPCSubnet,
			Connection: options.Connection,
		},
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	attach.Default()
	warnings, err := attach.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil, nil)
	if err != nil {
		slog.Warn("Validation", "error", err)

		return errors.Errorf("validation failed")
	}
	if warnings != nil {
		slog.Warn("Validation", "warnings", warnings)
	}

	err = kube.Create(ctx, attach)
	if err != nil {
		return errors.Wrap(err, "cannot create vpc attachment")
	}

	slog.Info("VPCAttachment created", "name", attach.Name)

	if printYaml {
		attach.ObjectMeta.ManagedFields = nil
		attach.ObjectMeta.Generation = 0
		attach.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(attach)
		if err != nil {
			return errors.Wrap(err, "cannot marshal vpc attachment")
		}

		fmt.Println(string(out))
	}

	return nil
}

type VPCPeerOptions struct {
	Name   string
	VPCs   []string
	Remote string
}

func VPCPeer(ctx context.Context, printYaml bool, options *VPCPeerOptions) error {
	name := options.Name
	if name == "" {
		name = strings.Join(options.VPCs, "--")
	}

	peering := &vpcapi.VPCPeering{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: vpcapi.VPCPeeringSpec{
			Remote: options.Remote,
			Permit: []map[string]vpcapi.VPCPeer{
				{
					options.VPCs[0]: {},
					options.VPCs[1]: {},
				},
			},
		},
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	peering.Default()
	warnings, err := peering.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil, nil)
	if err != nil {
		slog.Warn("Validation", "error", err)

		return errors.Errorf("validation failed")
	}
	if warnings != nil {
		slog.Warn("Validation", "warnings", warnings)
	}

	err = kube.Create(ctx, peering)
	if err != nil {
		return errors.Wrap(err, "cannot create vpc peering")
	}

	slog.Info("VPCPeering created", "name", peering.Name)

	if printYaml {
		peering.ObjectMeta.ManagedFields = nil
		peering.ObjectMeta.Generation = 0
		peering.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(peering)
		if err != nil {
			return errors.Wrap(err, "cannot marshal vpc peering")
		}

		fmt.Println(string(out))
	}

	return nil
}

type VPCSNATOptions struct {
	VPC    string
	Enable bool
}

func VPCSNAT(ctx context.Context, printYaml bool, options *VPCSNATOptions) error {
	if options.VPC == "" {
		return errors.Errorf("vpc is required")
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	vpc := &vpcapi.VPC{}
	err = kube.Get(ctx, types.NamespacedName{Name: options.VPC, Namespace: metav1.NamespaceDefault}, vpc)
	if err != nil {
		return errors.Wrapf(err, "cannot get vpc %s", options.VPC)
	}

	// TODO fix
	// vpc.Spec.SNAT = options.Enable

	err = kube.Update(ctx, vpc)
	if err != nil {
		return errors.Wrapf(err, "cannot update vpc %s", options.VPC)
	}

	// TODO fix
	// slog.Info("VPC SNAT set", "vpc", vpc.Name, "snat", vpc.Spec.SNAT)

	if printYaml {
		vpc.ObjectMeta.ManagedFields = nil
		vpc.ObjectMeta.Generation = 0
		vpc.ObjectMeta.ResourceVersion = ""
		vpc.Status = vpcapi.VPCStatus{}

		out, err := yaml.Marshal(vpc)
		if err != nil {
			return errors.Wrap(err, "cannot marshal vpc")
		}

		fmt.Println(string(out))
	}

	return nil
}

type VPCDNATOptions struct {
	VPC      string
	Requests []string
}

func VPCDNATRequest(ctx context.Context, printYaml bool, options *VPCDNATOptions) error {
	if options.VPC == "" {
		return errors.Errorf("vpc is required")
	}
	if len(options.Requests) == 0 {
		return errors.Errorf("at least one request is required")
	}

	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	vpc := &vpcapi.VPC{}
	err = kube.Get(ctx, types.NamespacedName{Name: options.VPC, Namespace: metav1.NamespaceDefault}, vpc)
	if err != nil {
		return errors.Wrapf(err, "cannot get vpc %s", options.VPC)
	}

	// TODO fix
	// if vpc.Spec.DNATRequests == nil {
	// 	vpc.Spec.DNATRequests = map[string]string{}
	// }

	// for _, req := range options.Requests {
	// 	parts := strings.Split(req, "=")
	// 	if len(parts) == 1 {
	// 		vpc.Spec.DNATRequests[parts[0]] = ""
	// 	} else if len(parts) == 2 {
	// 		vpc.Spec.DNATRequests[parts[0]] = parts[1]
	// 	} else {
	// 		return errors.Errorf("request should be privateIP=externalIP or privateIP, found: %s", req)
	// 	}
	// }

	err = kube.Update(ctx, vpc)
	if err != nil {
		return errors.Wrapf(err, "cannot update vpc %s", options.VPC)
	}

	slog.Info("VPC DNAT requests", "vpc", vpc.Name, "requests", strings.Join(options.Requests, ", "))

	if printYaml {
		vpc.ObjectMeta.ManagedFields = nil
		vpc.ObjectMeta.Generation = 0
		vpc.ObjectMeta.ResourceVersion = ""
		vpc.Status = vpcapi.VPCStatus{}

		out, err := yaml.Marshal(vpc)
		if err != nil {
			return errors.Wrap(err, "cannot marshal vpc")
		}

		fmt.Println(string(out))
	}

	return nil
}

// Defined as a separate function so it can be used in release tests
func VPCWipeWithClient(ctx context.Context, kube client.Client) error {
	delAllOpts := client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			Namespace: metav1.NamespaceDefault,
		},
	}
	// delete all external peerings
	if err := kube.DeleteAllOf(ctx, &vpcapi.ExternalPeering{}, &delAllOpts); err != nil {
		return errors.Wrap(err, "cannot delete external peerings")
	}

	// delete all regular peerings
	if err := kube.DeleteAllOf(ctx, &vpcapi.VPCPeering{}, &delAllOpts); err != nil {
		return errors.Wrap(err, "cannot delete vpc peerings")
	}

	// delete all attachments
	if err := kube.DeleteAllOf(ctx, &vpcapi.VPCAttachment{}, &delAllOpts); err != nil {
		return errors.Wrap(err, "cannot delete vpc attachments")
	}

	// delete all vpcs
	if err := kube.DeleteAllOf(ctx, &vpcapi.VPC{}, &delAllOpts); err != nil {
		return errors.Wrap(err, "cannot delete vpcs")
	}

	return nil
}

func VPCWipe(ctx context.Context) error {
	kube, err := kubeutil.NewClient(ctx, "", vpcapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	if err := VPCWipeWithClient(ctx, kube); err != nil {
		return errors.Wrap(err, "cannot wipe vpcs")
	}

	slog.Info("All VPCs, attachments and peerings wiped")

	return nil
}
