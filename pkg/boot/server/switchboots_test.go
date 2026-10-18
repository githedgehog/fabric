// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/pkg/boot"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"oras.land/oras-go/v2/registry/remote/auth"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindSwitchBoot(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))

	sb := func(name, serial, mac string) *fcintapi.SwitchBoot {
		return &fcintapi.SwitchBoot{
			ObjectMeta: kmetav1.ObjectMeta{Name: name, Namespace: kmetav1.NamespaceDefault},
			Spec:       fcintapi.SwitchBootSpec{Serial: serial, MAC: mac},
		}
	}

	boots := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&fcintapi.SwitchBoot{}, switchBootSerialField, indexSwitchBootSerial).
		WithIndex(&fcintapi.SwitchBoot{}, switchBootMACField, indexSwitchBootMAC).
		WithObjects(
			sb("leaf-1", "sn-0002", "0c:20:12:fe:01:00"),
			sb("leaf-2", "sn-0001", ""),
			sb("leaf-3", "sn-0001", "0c:20:12:fe:03:00"),
			sb("leaf-4", "", ""),
		).Build()

	for _, tt := range []struct {
		name   string
		serial string
		mac    string
		want   string
	}{
		{name: "serial", serial: "sn-0002", want: "leaf-1"},
		{name: "mac", mac: "0c:20:12:fe:03:00", want: "leaf-3"},
		{name: "duplicate serial, first by name", serial: "sn-0001", want: "leaf-2"},
		{name: "serial or mac, first by name", serial: "sn-0001", mac: "0c:20:12:fe:01:00", want: "leaf-1"},
		{name: "unknown mac with known serial", serial: "sn-0002", mac: "0c:20:12:fe:ff:00", want: "leaf-1"},
		{name: "unknown", serial: "sn-ffff", mac: "0c:20:12:fe:ff:00"},
		{name: "empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findSwitchBoot(t.Context(), boots, tt.serial, tt.mac)
			if tt.want == "" {
				require.ErrorIs(t, err, ErrNotFound)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got.Name)
		})
	}
}

func TestFatalWatchError(t *testing.T) {
	gr := schema.GroupResource{Group: fcintapi.GroupVersion.Group, Resource: "switchboots"}

	for _, tt := range []struct {
		name  string
		err   error
		fatal bool
	}{
		{name: "forbidden", err: kapierrors.NewForbidden(gr, "", errors.New("no access")), fatal: true}, //nolint:err113
		{name: "unauthorized", err: kapierrors.NewUnauthorized("expired token"), fatal: true},
		{name: "no crd", err: kapierrors.NewNotFound(gr, ""), fatal: true},
		{name: "expired", err: kapierrors.NewResourceExpired("too old")},
		{name: "unavailable", err: kapierrors.NewServiceUnavailable("restarting")},
		{name: "internal", err: kapierrors.NewInternalError(errors.New("oops"))}, //nolint:err113
		{name: "other", err: errors.New("connection refused")},                   //nolint:err113
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.fatal, fatalWatchError(tt.err))
		})
	}
}

func TestPreCacheAgentFor(t *testing.T) {
	// unreachable, any download attempt fails
	agentRef := "127.0.0.1:1/fabric/agent"

	svc := &service{
		cfg:          &boot.ServerConfig{AgentRef: agentRef},
		cacheDir:     t.TempDir(),
		orasClient:   &auth.Client{},
		downloadLock: &sync.Mutex{},
	}

	cached := filepath.Join(svc.cacheDir, getCacheName(agentRef, "v1"))
	require.NoError(t, os.MkdirAll(cached, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cached, "agent"), []byte("bin"), 0o600))

	sb := func(version string) *fcintapi.SwitchBoot {
		return &fcintapi.SwitchBoot{Spec: fcintapi.SwitchBootSpec{AgentVersion: version}}
	}

	// it would exit if anything was downloaded or failed
	svc.preCacheAgentFor(t.Context(), sb("v1"))
	svc.preCacheAgentFor(t.Context(), sb(""))
	svc.preCacheAgentFor(t.Context(), &agentapi.Agent{})

	require.NoError(t, svc.preCacheAgent(t.Context(), "v1"))

	// shutting down while not cached yet, no exit and no retries
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, svc.preCacheAgent(ctx, "v2"), context.Canceled)
	svc.preCacheAgentFor(ctx, sb("v2"))
}
