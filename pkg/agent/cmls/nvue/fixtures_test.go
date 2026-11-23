// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/agent/cmls/nvue/nvuetest"
)

const (
	// envLive runs the tests against the local nvued instead of the fixtures, see testdata/README.md
	envLive = "NVUE_LIVE"
	// the switches the fixtures are captured from, see testdata/README.md
	modelSN5610 = "sn5610"
	modelSN4700 = "sn4700"
	modelSN4600 = "sn4600"
	// the spine only has the fixtures of the routing tests
	modelSN3700C = "sn3700c"
	modelSN2700  = "sn2700"
	modelSN2201  = "sn2201"

	// fixtureModel is the switch the fixtures shared by all tests are captured from
	fixtureModel = modelSN5610

	// goldenDir in testdata has the values the tests expect from the fixtures of each model
	goldenDir = "golden"
)

var allModels = []string{modelSN5610, modelSN4700, modelSN4600, modelSN2700, modelSN2201}

func isLive() bool {
	return envEnabled(envLive)
}

func envEnabled(name string) bool {
	return os.Getenv(name) == "true"
}

// forEachModel runs test against the fixtures of each model or, when running live, once against the local nvued with
// an empty model: the checks that hold for any switch run either way, the ones of exact values only for their model
func forEachModel(t *testing.T, models []string, test func(t *testing.T, client *Client, model string)) {
	t.Helper()

	if isLive() {
		test(t, testClient(t, ""), "")

		return
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			test(t, testClient(t, model), model)
		})
	}
}

// requireGolden compares what the client got from the fixtures of a model with the expected values in testdata/golden,
// running live has nothing to compare with
func requireGolden(t *testing.T, model, name string, actual any) {
	t.Helper()

	if model == "" {
		return
	}

	nvuetest.RequireGolden(t, filepath.Join("testdata", goldenDir, model), name, actual)
}

// testClient talks to the local nvued when running live and to a fake one serving the fixtures in the given
// subdirectory of testdata otherwise
func testClient(t *testing.T, fixtures string, opts ...Option) *Client {
	t.Helper()

	if !isLive() {
		opts = append([]Option{WithSocket(nvuetest.Serve(t, filepath.Join("testdata", fixtures)))}, opts...)
	}

	client, err := New(opts...)
	require.NoError(t, err)

	return client
}
