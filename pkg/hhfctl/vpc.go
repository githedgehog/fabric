package hhfctl

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

type VPCCreateOptions struct {
	Name   string
	Subnet string
	VLAN   string
	DHCP   vpcapi.VPCDHCP
}

func VPCCreate(ctx context.Context, printYaml bool, options *VPCCreateOptions) error {
	vpc := &vpcapi.VPC{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: "default", // TODO ns
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

	kube, err := kubeClient()
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
			Namespace: "default", // TODO ns
		},
		Spec: vpcapi.VPCAttachmentSpec{
			Subnet:     options.VPCSubnet,
			Connection: options.Connection,
		},
	}

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	attach.Default()
	warnings, err := attach.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil)
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
			Namespace: "default", // TODO ns
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

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	peering.Default()
	warnings, err := peering.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil, false)
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

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	vpc := &vpcapi.VPC{}
	err = kube.Get(ctx, types.NamespacedName{Name: options.VPC, Namespace: "default"}, vpc) // TODO ns
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

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	vpc := &vpcapi.VPC{}
	err = kube.Get(ctx, types.NamespacedName{Name: options.VPC, Namespace: "default"}, vpc) // TODO ns
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
