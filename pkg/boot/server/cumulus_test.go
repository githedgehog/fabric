// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/pkg/boot"
)

func TestHandleAgent(t *testing.T) {
	agentRef := "127.0.0.1:1/fabric/agent"

	svc := &service{
		cfg:      &boot.ServerConfig{AgentRef: agentRef},
		cacheDir: t.TempDir(),
	}

	cached := filepath.Join(svc.cacheDir, getCacheName(agentRef, "v1.2.3"))
	require.NoError(t, os.MkdirAll(cached, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cached, "agent"), []byte("bin"), 0o600))

	for _, tt := range []struct {
		name    string
		version string
		code    int
	}{
		{name: "cached", version: "v1.2.3", code: http.StatusOK},
		{name: "not cached", version: "v1.2.4", code: http.StatusInternalServerError},
		{name: "missing", code: http.StatusBadRequest},
		{name: "dot dot", version: "..", code: http.StatusBadRequest},
		{name: "traversal", version: "../../etc/passwd", code: http.StatusBadRequest},
		{name: "slash", version: "v1/agent", code: http.StatusBadRequest},
		{name: "backslash", version: `v1\agent`, code: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, AgentURLSuffix+"?version="+url.QueryEscape(tt.version), nil)
			rec := httptest.NewRecorder()

			svc.handleAgent(rec, req)

			require.Equal(t, tt.code, rec.Code)
			if tt.code == http.StatusOK {
				require.Equal(t, "bin", rec.Body.String())
			}
		})
	}
}
