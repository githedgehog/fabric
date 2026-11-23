// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package cmls

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue/nvuetest"
)

// nvueFixtures are the NVUE responses captured from the switches of env-7, see nvue/testdata/README.md
var nvueFixtures = filepath.Join("nvue", "testdata")

func fixtureProcessor(t *testing.T, model string) *CumulusProcessor {
	t.Helper()

	client, err := nvue.New(nvue.WithSocket(nvuetest.Serve(t, filepath.Join(nvueFixtures, model))))
	require.NoError(t, err)

	return Processor(client)
}

func TestGetRoCE(t *testing.T) {
	// env-7 has RoCE only on the SN2201
	for model, want := range map[string]bool{"sn5610": false, "sn2201": true} {
		t.Run(model, func(t *testing.T) {
			roce, err := fixtureProcessor(t, model).GetRoCE(t.Context())
			require.NoError(t, err)
			require.Equal(t, want, roce)
		})
	}
}

func TestGetRoCEError(t *testing.T) {
	client, err := nvue.New(nvue.WithSocket(nvuetest.SocketPath(t)))
	require.NoError(t, err)

	_, err = Processor(client).GetRoCE(t.Context())
	require.ErrorContains(t, err, "getting RoCE")
}
