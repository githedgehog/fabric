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

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getAgent(ctx context.Context, kube client.WithWatch, name string) (*agentapi.Agent, error) {
	agent := &agentapi.Agent{}
	err := kube.Get(ctx, client.ObjectKey{Name: name, Namespace: metav1.NamespaceDefault}, agent)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get agent")
	}

	return agent, nil
}

func SwitchReboot(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient("", agentapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.RunID == "" {
		return errors.Errorf("agent is not running (missing .status.runID)")
	}

	agent.Spec.Reboot = agent.Status.RunID
	err = kube.Update(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "cannot update agent")
	}

	return nil
}

func SwitchPowerReset(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient("", agentapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.RunID == "" {
		return errors.Errorf("agent is not running (missing .status.runID)")
	}

	agent.Spec.PowerReset = agent.Status.RunID
	err = kube.Update(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "cannot update agent")
	}

	return nil
}

func SwitchReinstall(ctx context.Context, name string) error {
	kube, err := kubeutil.NewClient("", agentapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.InstallID == "" {
		return errors.Errorf("agent is not installed (missing .status.installID)")
	}

	agent.Spec.Reinstall = agent.Status.InstallID
	err = kube.Update(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "cannot update agent")
	}

	return nil
}

func SwitchForceAgentVersion(ctx context.Context, name string, version string) error {
	kube, err := kubeutil.NewClient("", agentapi.SchemeBuilder)
	if err != nil {
		return errors.Wrap(err, "cannot create kube client")
	}

	agent, err := getAgent(ctx, kube, name)
	if err != nil {
		return err
	}

	if agent.Status.RunID == "" {
		return errors.Errorf("agent is not running")
	}

	agent.Spec.Version.Override = version
	agent.Spec.Reboot = agent.Status.RunID

	err = kube.Update(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "cannot update agent")
	}

	return nil
}
