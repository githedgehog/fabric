// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	ni "go.githedgehog.com/fabric/pkg/boot/nosinstall"
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
			ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault},
			Spec:       fcintapi.SwitchBootSpec{Serial: "sn-0001"},
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
