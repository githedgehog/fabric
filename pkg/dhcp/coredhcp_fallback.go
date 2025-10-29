// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package dhcp

import (
	"context"
	"fmt"
)

func (s *Server) startCoreDHCP(ctx context.Context) error {
	_ = s.setupDHCP4Plugin(ctx)

	return fmt.Errorf("only supported on linux") //nolint:err113
}
