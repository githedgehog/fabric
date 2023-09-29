package hhfctl

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/manager/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type VPCCreateOptions struct {
	Name   string
	Subnet string
	DHCP   vpcapi.VPCDHCP
}

func VPCCreate(ctx context.Context, printYaml bool, options *VPCCreateOptions) error {
	vpc := &vpcapi.VPC{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: "default", // TODO ns
		},
		Spec: vpcapi.VPCSpec{
			Subnet: options.Subnet,
			DHCP:   options.DHCP,
		},
	}

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	vpc.Default()
	warnings, err := vpc.Validate(ctx, validation.WithCtrlRuntime(kube))
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
	VPC        string
	Connection string
}

func VPCAttach(ctx context.Context, printYaml bool, options *VPCAttachOptions) error {
	name := options.Name
	if name == "" {
		name = fmt.Sprintf("%s--%s", options.VPC, options.Connection)
	}

	attach := &vpcapi.VPCAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default", // TODO ns
		},
		Spec: vpcapi.VPCAttachmentSpec{
			VPC:        options.VPC,
			Connection: options.Connection,
		},
	}

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	attach.Default()
	warnings, err := attach.Validate(ctx, validation.WithCtrlRuntime(kube))
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
	Name string
	VPCs []string
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
			VPCs: options.VPCs,
		},
	}

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	peering.Default()
	warnings, err := peering.Validate(ctx, validation.WithCtrlRuntime(kube))
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
