package hhfctl

import (
	"context"

	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getAgent(ctx context.Context, kube client.WithWatch, name string) (*agentapi.Agent, error) {
	agent := &agentapi.Agent{}
	err := kube.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, agent) // TODO ns
	if err != nil {
		return nil, errors.Wrap(err, "cannot get agent")
	}

	return agent, nil
}

func SwitchReboot(ctx context.Context, yes bool, name string) error {
	kube, err := kubeClient()
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

func SwitchPowerReset(ctx context.Context, yes bool, name string) error {
	kube, err := kubeClient()
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

func SwitchReinstall(ctx context.Context, yes bool, name string) error {
	kube, err := kubeClient()
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

func SwitchForceAgentVersion(ctx context.Context, yes bool, name string, version string) error {
	kube, err := kubeClient()
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

	agent.Spec.Reboot = agent.Status.RunID
	err = kube.Update(ctx, agent)
	if err != nil {
		return errors.Wrap(err, "cannot update agent")
	}
	return nil
}
