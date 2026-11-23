// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func errOf[T any](_ T, err error) error {
	return err
}

func TestEndpointsReturnErrors(t *testing.T) {
	client, err := New(WithSocket(socketPath(t)))
	require.NoError(t, err)

	for name, call := range map[string]func(ctx context.Context) error{
		"Interfaces":        func(ctx context.Context) error { return errOf(client.Interfaces(ctx)) },
		"InterfaceCounters": func(ctx context.Context) error { return errOf(client.InterfaceCounters(ctx, "swp1")) },
		"InterfaceRates":    func(ctx context.Context) error { return errOf(client.InterfaceRates(ctx)) },
		"Breakouts":         func(ctx context.Context) error { return errOf(client.Breakouts(ctx)) },
		"Transceivers":      func(ctx context.Context) error { return errOf(client.Transceivers(ctx)) },
		"System":            func(ctx context.Context) error { return errOf(client.System(ctx)) },
		"Platform":          func(ctx context.Context) error { return errOf(client.Platform(ctx)) },
		"Fans":              func(ctx context.Context) error { return errOf(client.Fans(ctx)) },
		"PSUs":              func(ctx context.Context) error { return errOf(client.PSUs(ctx)) },
		"Temperatures":      func(ctx context.Context) error { return errOf(client.Temperatures(ctx)) },
		"Firmware":          func(ctx context.Context) error { return errOf(client.Firmware(ctx)) },
		"ASICs":             func(ctx context.Context) error { return errOf(client.ASICs(ctx)) },
		"ASICResources":     func(ctx context.Context) error { return errOf(client.ASICResources(ctx, "ASIC1")) },
		"RoCE":              func(ctx context.Context) error { return errOf(client.RoCE(ctx)) },
		"VRFs":              func(ctx context.Context) error { return errOf(client.VRFs(ctx)) },
		"BGPNeighbors":      func(ctx context.Context) error { return errOf(client.BGPNeighbors(ctx, VRFDefault)) },
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, call(t.Context()), fs.ErrNotExist)
		})
	}
}
