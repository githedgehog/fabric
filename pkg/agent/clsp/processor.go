// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package clsp

import (
	"context"
	"fmt"
	"os"

	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	kyaml "sigs.k8s.io/yaml"
)

type CelesticaPlusProcessor struct{}

var _ dozer.Processor = &CelesticaPlusProcessor{}

func Processor() *CelesticaPlusProcessor {
	return &CelesticaPlusProcessor{}
}

// TODO
func (c *CelesticaPlusProcessor) Reboot(ctx context.Context, force bool) error {
	return bcm.Processor().Reboot(ctx, force) //nolint:wrapcheck
}

// TODO: use onie-select -i (it asks for y/N user input...)
func (c *CelesticaPlusProcessor) Reinstall(ctx context.Context) error {
	return bcm.Processor().Reinstall(ctx) //nolint:wrapcheck
}

// TODO
func (c *CelesticaPlusProcessor) FactoryReset(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

// TODO
func (c *CelesticaPlusProcessor) EnsureControlLink(ctx context.Context, agent *agentapi.Agent) error {
	return bcm.Processor().EnsureControlLink(ctx, agent) //nolint:wrapcheck
}

// TODO
func (c *CelesticaPlusProcessor) GetRoCE(ctx context.Context) (bool, error) {
	return false, nil
}

// TODO
func (c *CelesticaPlusProcessor) SetRoCE(ctx context.Context, enable bool) error {
	return fmt.Errorf("not supported")
}

// TODO
func (c *CelesticaPlusProcessor) WaitReady(ctx context.Context) error {
	return nil
}

// TODO
func (c *CelesticaPlusProcessor) UpdateSwitchState(ctx context.Context, agent *agentapi.Agent, reg *switchstate.Registry) error {
	swState := &agentapi.SwitchState{
		Interfaces:   map[string]agentapi.SwitchStateInterface{},
		Breakouts:    map[string]agentapi.SwitchStateBreakout{},
		Transceivers: map[string]agentapi.SwitchStateTransceiver{},
		BGPNeighbors: map[string]map[string]agentapi.SwitchStateBGPNeighbor{},
		Platform: agentapi.SwitchStatePlatform{
			Fans:         map[string]agentapi.SwitchStatePlatformFan{},
			PSUs:         map[string]agentapi.SwitchStatePlatformPSU{},
			Temperatures: map[string]agentapi.SwitchStatePlatformTemperature{},
		},
		Firmware: map[string]string{},
	}

	versionData, err := os.ReadFile("/etc/sonic/sonic_version.yml")
	if err != nil {
		return fmt.Errorf("reading sonic version: %w", err)
	}
	parsedVersion := map[string]string{}
	if err := kyaml.Unmarshal(versionData, &parsedVersion); err != nil {
		return fmt.Errorf("parsing sonic version: %w", err)
	}

	swState.NOS = agentapi.SwitchStateNOS{
		HwskuVersion:    "Celestica DS1000", // TODO read it from somewhere
		SoftwareVersion: parsedVersion["build_version"],
	}

	reg.SaveSwitchState(swState)

	return nil
}

// Invalid "type"

func (c *CelesticaPlusProcessor) LoadActualState(ctx context.Context, agent *agentapi.Agent) (*dozer.Spec, error) {
	return nil, fmt.Errorf("unsupported operation")
}

func (c *CelesticaPlusProcessor) PlanDesiredState(ctx context.Context, agent *agentapi.Agent) (*dozer.Spec, error) {
	return nil, fmt.Errorf("unsupported operation")
}

func (c *CelesticaPlusProcessor) ApplyActions(ctx context.Context, actions []dozer.Action) ([]string, error) {
	return nil, fmt.Errorf("unsupported operation")
}

func (c *CelesticaPlusProcessor) CalculateActions(ctx context.Context, actual *dozer.Spec, desired *dozer.Spec) ([]dozer.Action, error) {
	return nil, fmt.Errorf("unsupported operation")
}
