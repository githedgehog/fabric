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

	"github.com/pkg/errors"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
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

	kube, err := kubeutil.NewClient("", wiringapi.SchemeBuilder)
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
