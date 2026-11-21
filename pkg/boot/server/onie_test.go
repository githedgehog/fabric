// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/boot"
	ni "go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/ctrl"
	corev1 "k8s.io/api/core/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHandleONIEInstallError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, agentapi.AddToScheme(scheme))
	require.NoError(t, fcintapi.AddToScheme(scheme))

	// the agent isn't there yet
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&fcintapi.SwitchBoot{}).
		WithIndex(&fcintapi.SwitchBoot{}, switchBootSerialField, indexSwitchBootSerial).
		WithIndex(&fcintapi.SwitchBoot{}, switchBootMACField, indexSwitchBootMAC).
		WithObjects(&fcintapi.SwitchBoot{
			Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
			Spec: fcintapi.SwitchBootSpec{Serial: "sn-0001"},
		}).Build()

	svc := &service{kube: kube, boots: kube, bootAttempts: make(chan func(), bootAttemptQueue)}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, ni.OnieURLSuffix, nil)
	req.Header.Set("Onie-Arch", "x86_64")
	req.Header.Set("Onie-Operation", "os-install")
	req.Header.Set("Onie-Serial-Number", "SN-0001")
	rec := httptest.NewRecorder()

	middleware.ClientIPFromRemoteAddr(http.HandlerFunc(svc.handleONIE)).ServeHTTP(rec, req)
	processBootAttempts(svc)

	require.Equal(t, http.StatusNotFound, rec.Code)
	requireLastBootAttempt(t, kube, fcintapi.SwitchBootAttemptTypeONIE, fcintapi.SwitchBootAttemptResultError)
}

func TestHandleONIEInstallCumulus(t *testing.T) {
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
				Name: "leaf-1", Namespace: kmetav1.NamespaceDefault,
				Spec: fcintapi.SwitchBootSpec{Serial: "sn-0001", NOSType: meta.NOSTypeCumulusMlx, AgentVersion: "v1.2.3"},
			},
			&agentapi.Agent{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
			&corev1.Secret{
				Name: ctrl.AgentKubeconfigSecret("leaf-1"), Namespace: kmetav1.NamespaceDefault,
				Data: map[string][]byte{ctrl.AgentKubeconfigKey: []byte("kubeconfig")},
			},
		).Build()

	const nosRepo = "127.0.0.1:1/fabric/cumulus"
	svc := &service{
		cfg: &boot.ServerConfig{
			NOSRepos:    map[meta.NOSType]string{meta.NOSTypeCumulusMlx: nosRepo},  //nolint:exhaustive
			NOSVersions: map[meta.NOSType]string{meta.NOSTypeCumulusMlx: "5.18.0"}, //nolint:exhaustive
		},
		kube:         kube,
		boots:        kube,
		cacheDir:     t.TempDir(),
		bootAttempts: make(chan func(), bootAttemptQueue),
	}
	cached := filepath.Join(svc.cacheDir, getCacheName(nosRepo, "5.18.0"))
	require.NoError(t, os.MkdirAll(cached, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cached, "cumulus-linux.bin"), []byte("signed installer"), 0o600))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, ni.OnieURLSuffix, nil)
	req.Header.Set("Onie-Arch", "x86_64")
	req.Header.Set("Onie-Operation", "os-install")
	req.Header.Set("Onie-Serial-Number", "SN-0001")
	rec := httptest.NewRecorder()

	middleware.ClientIPFromRemoteAddr(http.HandlerFunc(svc.handleONIE)).ServeHTTP(rec, req)
	processBootAttempts(svc)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "signed installer", rec.Body.String())
	requireLastBootAttempt(t, kube, fcintapi.SwitchBootAttemptTypeONIE, fcintapi.SwitchBootAttemptResultServed)
}
