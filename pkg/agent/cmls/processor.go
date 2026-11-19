// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"context"
	"fmt"

	"go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm"
)

type CumulusProcessor struct{}

var _ dozer.Processor = &CumulusProcessor{}

func Processor() *CumulusProcessor {
	return &CumulusProcessor{}
}

// TODO
func (c *CumulusProcessor) Reboot(ctx context.Context, force bool) error {
	return bcm.Processor().Reboot(ctx, force) //nolint:wrapcheck
}

// TODO: use onie-select -i (it asks for y/N user input...)
func (c *CumulusProcessor) Reinstall(ctx context.Context) error {
	return fmt.Errorf("not implemented") //nolint:err113
}

// TODO
func (c *CumulusProcessor) FactoryReset(ctx context.Context) error {
	return fmt.Errorf("not implemented") //nolint:err113
}

// TODO
func (c *CumulusProcessor) EnsureControlLink(ctx context.Context, agent *v1beta1.Agent) error {
	return nil
}

// TODO
func (c *CumulusProcessor) GetRoCE(ctx context.Context) (bool, error) {
	return false, nil
}

// TODO
func (c *CumulusProcessor) SetRoCE(ctx context.Context, enable bool) error {
	return fmt.Errorf("not supported") //nolint:err113
}

// TODO
func (c *CumulusProcessor) WaitReady(ctx context.Context) error {
	return nil
}

// Invalid "type"

func (c *CumulusProcessor) LoadActualState(ctx context.Context, agent *v1beta1.Agent) (*dozer.Spec, error) {
	return nil, fmt.Errorf("unsupported operation") //nolint:err113
}

func (c *CumulusProcessor) PlanDesiredState(ctx context.Context, agent *v1beta1.Agent) (*dozer.Spec, error) {
	return nil, fmt.Errorf("unsupported operation") //nolint:err113
}

func (c *CumulusProcessor) ApplyActions(ctx context.Context, actions []dozer.Action) ([]string, error) {
	return nil, fmt.Errorf("unsupported operation") //nolint:err113
}

func (c *CumulusProcessor) CalculateActions(ctx context.Context, actual *dozer.Spec, desired *dozer.Spec) ([]dozer.Action, error) {
	return nil, fmt.Errorf("unsupported operation") //nolint:err113
}
