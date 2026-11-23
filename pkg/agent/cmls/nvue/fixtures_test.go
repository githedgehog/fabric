// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// envLive runs the tests against the local nvued instead of the fixtures, see testdata/README.md
	envLive = "NVUE_LIVE"
	// the switches the fixtures are captured from, see testdata/README.md
	modelSN5610 = "sn5610"
	modelSN4700 = "sn4700"
	modelSN4600 = "sn4600"
	modelSN2700 = "sn2700"
	modelSN2201 = "sn2201"

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

// fixtureName is the file name of the fixture for a request URI relative to the API prefix, such as
// "/interface?view=rates" -> "interface@view=rates", the status is added for the error responses
func fixtureName(uri string) string {
	path, query, _ := strings.Cut(strings.TrimPrefix(uri, "/"), "?")
	name := strings.ReplaceAll(path, "/", "__")
	if query != "" {
		name += "@" + query
	}

	return name
}

// fixtureFile finds the fixture of a request, returning its status from the name: "<name>.json" is a 200 and
// "<name>.<status>.json" an error response
func fixtureFile(dir, uri string) (string, int, error) {
	name := fixtureName(uri)
	if path := filepath.Join(dir, name+".json"); fileExists(path) {
		return path, http.StatusOK, nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, globEscape(name)+".[0-9][0-9][0-9].json"))
	if err != nil {
		return "", 0, err //nolint:wrapcheck
	}
	if len(matches) != 1 {
		return "", 0, fs.ErrNotExist
	}

	status, err := strconv.Atoi(filepath.Ext(strings.TrimSuffix(matches[0], ".json"))[1:])
	if err != nil {
		return "", 0, err //nolint:wrapcheck
	}

	return matches[0], status, nil
}

func globEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`).Replace(s)
}

func fileExists(path string) bool {
	_, err := os.Stat(path) //nolint:gosec // fixtures of the requests the tests send

	return err == nil
}

// socketPath is a path for a test socket, t.TempDir() can be too long for sun_path
func socketPath(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "nvue") //nolint:usetesting // see above
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return filepath.Join(dir, "nvue.sock")
}

func listen(t *testing.T) (net.Listener, string) {
	t.Helper()

	sock := socketPath(t)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err)

	return listener, sock
}

// silentSocket accepts connections and never answers, for the tests of a client giving up
func silentSocket(t *testing.T) string {
	t.Helper()

	listener, sock := listen(t)

	conns := make(chan net.Conn, 16)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				close(conns)

				return
			}
			conns <- conn
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		for conn := range conns {
			_ = conn.Close()
		}
	})

	return sock
}

// fakeNVUE serves the fixtures in dir on a unix socket and fails the test on a request without one
func fakeNVUE(t *testing.T, dir string) string {
	t.Helper()

	listener, sock := listen(t)

	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uri, ok := strings.CutPrefix(r.RequestURI, apiPrefix)
			if !ok || r.Method != http.MethodGet {
				t.Errorf("unexpected request %s %s", r.Method, r.RequestURI)
				w.WriteHeader(http.StatusTeapot)

				return
			}

			path, status, err := fixtureFile(dir, uri)
			if err != nil {
				t.Errorf("no fixture for %s in %s: %v", uri, dir, err)
				w.WriteHeader(http.StatusTeapot)

				return
			}

			body, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("reading fixture %s: %v", path, err)
				w.WriteHeader(http.StatusTeapot)

				return
			}

			if status == http.StatusOK {
				w.Header().Set("Content-Type", "application/json")
			} else {
				w.Header().Set("Content-Type", "application/problem+json")
			}
			w.WriteHeader(status)
			_, _ = w.Write(body)
		}),
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("serving fake nvued: %v", err)
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	return sock
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
// regenerated by running the tests with UPDATE=true (just test-update), running live has nothing to compare with
func requireGolden(t *testing.T, model, name string, actual any) {
	t.Helper()

	if model == "" {
		return
	}

	// JSON as YAML doesn't keep the order of the keys stable
	data, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err)
	data = append(data, '\n')

	dir := filepath.Join("testdata", goldenDir, model)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	expectedFile := filepath.Join(dir, name+".expected.json")
	actualFile := filepath.Join(dir, name+".actual.json")

	require.NoError(t, os.WriteFile(actualFile, data, 0o600))
	if envEnabled("UPDATE") {
		require.NoError(t, os.WriteFile(expectedFile, data, 0o600))
	}

	expected, err := os.ReadFile(expectedFile)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(data),
		"%s of %s mismatch, compare %s with %s or regenerate it with just test-update", name, model, actualFile, expectedFile)
}

// testClient talks to the local nvued when running live and to a fake one serving the fixtures in the given
// subdirectory of testdata otherwise
func testClient(t *testing.T, fixtures string, opts ...Option) *Client {
	t.Helper()

	if !isLive() {
		opts = append([]Option{WithSocket(fakeNVUE(t, filepath.Join("testdata", fixtures)))}, opts...)
	}

	client, err := New(opts...)
	require.NoError(t, err)

	return client
}
