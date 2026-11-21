// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

type CumulusProcessor struct{}

var _ dozer.Processor = &CumulusProcessor{}

func Processor() *CumulusProcessor {
	return &CumulusProcessor{}
}

// SelectONIEInstall makes the next boot only go to ONIE in install mode, on both legacy BIOS and UEFI switches
func SelectONIEInstall(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	return run(ctx, "onie-select", "/usr/cumulus/bin/onie-select", "-i", "-f")
}

// TODO
func (c *CumulusProcessor) EnsureControlLink(ctx context.Context, agent *v1beta1.Agent) error {
	return nil
}

func (c *CumulusProcessor) GetRoCE(ctx context.Context) (bool, error) {
	out, err := runOutput(ctx, "nv show qos roce", "nv", "show", "qos", "roce", "-o", "json")
	if err != nil {
		return false, err
	}
	roce := struct {
		State string `json:"state"`
		Mode  string `json:"mode"`
	}{}
	if err := json.Unmarshal(out, &roce); err != nil {
		return false, fmt.Errorf("parsing RoCE state: %w", err)
	}

	return roce.State == "enabled" && roce.Mode == "lossless", nil
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
