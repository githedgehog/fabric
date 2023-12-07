package hhfctl

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type SwitchGroupCreateOptions struct {
	Name string
}

func SwitchGroupCreate(ctx context.Context, printYaml bool, options *SwitchGroupCreateOptions) error {
	sg := &wiringapi.SwitchGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: "default", // TODO ns
		},
		Spec: wiringapi.SwitchGroupSpec{},
	}

	kube, err := kubeClient()
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	err = kube.Create(ctx, sg)
	if err != nil {
		return errors.Wrap(err, "cannot create switch group")
	}

	slog.Info("SwitchGroup created", "name", sg.Name)

	if printYaml {
		sg.ObjectMeta.ManagedFields = nil
		sg.ObjectMeta.Generation = 0
		sg.ObjectMeta.ResourceVersion = ""

		out, err := yaml.Marshal(sg)
		if err != nil {
			return errors.Wrap(err, "cannot marshal sg")
		}

		fmt.Println(string(out))
	}

	return nil
}
