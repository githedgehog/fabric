package hhfctl

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type ExternalCreateOptions struct {
	Name              string
	IPv4Namespace     string
	InboundCommunity  string
	OutboundCommunity string
}

func ExternalCreate(ctx context.Context, printYaml bool, options *ExternalCreateOptions) error {
	ext := &vpcapi.External{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: "default", // TODO ns
		},
		Spec: vpcapi.ExternalSpec{
			IPv4Namespace:     options.IPv4Namespace,
			InboundCommunity:  options.InboundCommunity,
			OutboundCommunity: options.OutboundCommunity,
		},
	}

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	ext.Default()
	warnings, err := ext.Validate(ctx /* validation.WithCtrlRuntime(kube) */, nil)
	if err != nil {
		slog.Warn("Validation", "error", err)
		return errors.Errorf("validation failed")
	}
	if warnings != nil {
		slog.Warn("Validation", "warnings", warnings)
	}

	err = kube.Create(ctx, ext)
	if err != nil {
		return errors.Wrap(err, "cannot create external")
	}

	slog.Info("External created", "name", ext.Name)

	if printYaml {
		ext.ObjectMeta.ManagedFields = nil
		ext.ObjectMeta.Generation = 0
		ext.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(ext)
		if err != nil {
			return errors.Wrap(err, "cannot marshal ext")
		}

		fmt.Println(string(out))
	}

	return nil
}

type ExternalAttachCreateOptions struct{}
