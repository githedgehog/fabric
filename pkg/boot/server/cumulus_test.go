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
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/boot"
	"go.githedgehog.com/fabric/pkg/ctrl"
	corev1 "k8s.io/api/core/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testAgentRef = "127.0.0.1:1/fabric/agent"

// cacheTestAgent puts the agent version into the cache as if it was pre-cached
func cacheTestAgent(t *testing.T, svc *service, version string) {
	t.Helper()

	cached := filepath.Join(svc.cacheDir, getCacheName(testAgentRef, version))
	require.NoError(t, os.MkdirAll(cached, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cached, "agent"), []byte("bin"), 0o600))
}

func TestHandleAgent(t *testing.T) {
	svc := &service{
		cfg:      &boot.ServerConfig{AgentRef: testAgentRef},
		cacheDir: t.TempDir(),
	}
	cacheTestAgent(t, svc, "v1.2.3")

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

func TestHandleCumulusZTP(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, fcintapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&fcintapi.SwitchBoot{}).
		WithIndex(&fcintapi.SwitchBoot{}, switchBootSerialField, indexSwitchBootSerial).
		WithIndex(&fcintapi.SwitchBoot{}, switchBootMACField, indexSwitchBootMAC).
		WithObjects(
			&fcintapi.SwitchBoot{
				ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
				Spec:       fcintapi.SwitchBootSpec{Serial: "sn-0001", MAC: "0c:20:12:fe:01:00", NOSType: meta.NOSTypeCumulusVX, AgentVersion: "v1.2.3"},
			},
			&agentapi.Agent{
				ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
				Spec: agentapi.AgentSpec{
					Config: agentapi.AgentSpecConfig{ControlVIP: "172.30.0.1/32"},
					Switch: wiringapi.SwitchSpec{ProtocolIP: "172.30.8.1/32"},
				},
			},
			&corev1.Secret{
				ObjectMeta: kmetav1.ObjectMeta{Name: ctrl.AgentKubeconfigSecret("leaf-1"), Namespace: kmetav1.NamespaceDefault},
				Data:       map[string][]byte{ctrl.AgentKubeconfigKey: []byte("kubeconfig")},
			},
		).Build()

	svc := &service{
		cfg:      &boot.ServerConfig{AgentRef: testAgentRef},
		kube:     kube,
		boots:    kube,
		cacheDir: t.TempDir(),
	}

	ztp := func(serial, mac string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, CumulusZTPURLSuffix, nil)
		req.Header.Set("CUMULUS-ARCH", "x86_64")
		req.Header.Set("CUMULUS-SERIAL", serial)
		req.Header.Set("CUMULUS-MGMT-MAC", mac)
		rec := httptest.NewRecorder()

		middleware.ClientIPFromRemoteAddr(http.HandlerFunc(svc.handleCumulusZTP)).ServeHTTP(rec, req)
		svc.bootAttempts.Wait()

		return rec
	}

	// the script would fail to download the agent
	rec := ztp(" SN-0001 ", "")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "10", rec.Header().Get("Retry-After"))
	require.Empty(t, rec.Body.String())
	requireLastBootAttempt(t, kube, fcintapi.SwitchBootAttemptTypeCumulusZTP, fcintapi.SwitchBootAttemptResultAgentNotCached)

	cacheTestAgent(t, svc, "v1.2.3")

	// a different outcome is recorded right away
	rec = ztp(" SN-0001 ", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `/agent?version=v1.2.3"`)
	sb := requireLastBootAttempt(t, kube, fcintapi.SwitchBootAttemptTypeCumulusZTP, fcintapi.SwitchBootAttemptResultServed)

	// the MAC matches in any format
	require.Equal(t, http.StatusOK, ztp("", "0C-20-12-FE-01-00").Code)

	// the same outcome isn't recorded again that soon
	require.Equal(t, sb.ResourceVersion, requireLastBootAttempt(t, kube, fcintapi.SwitchBootAttemptTypeCumulusZTP, fcintapi.SwitchBootAttemptResultServed).ResourceVersion)

	// an invalid MAC is ignored, it's still found by serial
	require.Equal(t, http.StatusOK, ztp("sn-0001", "not-a-mac").Code)
	require.Equal(t, http.StatusBadRequest, ztp("", "not-a-mac").Code)
	require.Equal(t, http.StatusBadRequest, ztp("", "").Code)
	require.Equal(t, http.StatusNotFound, ztp("sn-ffff", "").Code)
}

// requireLastBootAttempt checks the last boot attempt recorded for the test switch leaf-1, made from the test
// request's address
func requireLastBootAttempt(t *testing.T, kube kclient.Reader, typ fcintapi.SwitchBootAttemptType, result fcintapi.SwitchBootAttemptResult) *fcintapi.SwitchBoot {
	t.Helper()

	sb := &fcintapi.SwitchBoot{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKey{Namespace: kmetav1.NamespaceDefault, Name: "leaf-1"}, sb))
	require.NotNil(t, sb.Status.LastAttempt)
	require.Equal(t, typ, sb.Status.LastAttempt.Type)
	require.Equal(t, result, sb.Status.LastAttempt.Result)
	require.Equal(t, "192.0.2.1", sb.Status.LastAttempt.Remote)
	require.WithinDuration(t, time.Now(), sb.Status.LastAttempt.Time.Time, time.Minute)

	return sb
}
