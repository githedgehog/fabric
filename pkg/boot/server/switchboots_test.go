// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/pkg/boot"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	"oras.land/oras-go/v2/registry/remote/auth"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
		name     string
		serial   string
		mac      string
		want     string
		conflict bool
	}{
		{name: "serial", serial: "sn-0002", want: "leaf-1"},
		{name: "mac", mac: "0c:20:12:fe:03:00", want: "leaf-3"},
		{name: "serial and mac", serial: "sn-0002", mac: "0c:20:12:fe:01:00", want: "leaf-1"},
		{name: "unknown mac with known serial", serial: "sn-0002", mac: "0c:20:12:fe:ff:00", want: "leaf-1"},
		{name: "duplicate serial", serial: "sn-0001", conflict: true},
		{name: "serial of one and mac of another", serial: "sn-0002", mac: "0c:20:12:fe:03:00", conflict: true},
		{name: "unknown", serial: "sn-ffff", mac: "0c:20:12:fe:ff:00"},
		{name: "empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findSwitchBoot(t.Context(), boots, tt.serial, tt.mac)
			if tt.conflict {
				require.ErrorContains(t, err, "match serial")
				require.NotErrorIs(t, err, ErrNotFound)

				return
			}
			if tt.want == "" {
				require.ErrorIs(t, err, ErrNotFound)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got.Name)
		})
	}
}

func TestWriteBootAttempt(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))

	key := kclient.ObjectKey{Namespace: kmetav1.NamespaceDefault, Name: "leaf-1"}
	conflicts := 1
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&fcintapi.SwitchBoot{}).
		WithObjects(&fcintapi.SwitchBoot{ObjectMeta: kmetav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}).
		WithInterceptorFuncs(interceptor.Funcs{
			// as if the controller updated the spec in the meantime
			SubResourceUpdate: func(ctx context.Context, c kclient.Client, sub string, obj kclient.Object, opts ...kclient.SubResourceUpdateOption) error {
				if conflicts > 0 {
					conflicts--

					return kapierrors.NewConflict(schema.GroupResource{Group: fcintapi.GroupVersion.Group, Resource: "switchboots"}, obj.GetName(), errors.New("modified")) //nolint:err113
				}

				return c.SubResource(sub).Update(ctx, obj, opts...)
			},
		}).
		Build()
	svc := &service{kube: kube}

	get := func() *fcintapi.SwitchBoot {
		sb := &fcintapi.SwitchBoot{}
		require.NoError(t, kube.Get(t.Context(), key, sb))

		return sb
	}
	// stored with microseconds only
	now := time.Now().Truncate(time.Microsecond)
	attempt := func(at time.Duration, result fcintapi.SwitchBootAttemptResult) fcintapi.SwitchBootAttempt {
		return fcintapi.SwitchBootAttempt{
			Time:   kmetav1.NewMicroTime(now.Add(at)),
			Type:   fcintapi.SwitchBootAttemptTypeCumulusZTP,
			Result: result,
			Remote: "192.0.2.1",
		}
	}
	requireLast := func(want fcintapi.SwitchBootAttempt) {
		t.Helper()

		last := get().Status.LastAttempt
		require.NotNil(t, last)
		require.True(t, want.Time.Equal(&last.Time), "time %s, want %s", last.Time, want.Time)
		require.Equal(t, want.Result, last.Result)
	}

	// the first one is recorded despite a conflict with nothing recorded yet
	first := attempt(0, fcintapi.SwitchBootAttemptResultAgentNotCached)
	require.NoError(t, svc.writeBootAttempt(t.Context(), get(), first))
	require.Zero(t, conflicts)
	requireLast(first)

	// every attempt is recorded, the same outcome too
	again := attempt(10*time.Second, fcintapi.SwitchBootAttemptResultAgentNotCached)
	require.NoError(t, svc.writeBootAttempt(t.Context(), get(), again))
	requireLast(again)

	// overlapping requests, both read before either is recorded: the newer one is recorded even if it's the second
	// one to finish
	before := get()
	failed := attempt(20*time.Second, fcintapi.SwitchBootAttemptResultError)
	require.NoError(t, svc.writeBootAttempt(t.Context(), before.DeepCopy(), failed))
	requireLast(failed)
	served := attempt(21*time.Second, fcintapi.SwitchBootAttemptResultServed)
	require.NoError(t, svc.writeBootAttempt(t.Context(), before.DeepCopy(), served))
	requireLast(served)

	// and the older one isn't if it's the second one to finish
	require.NoError(t, svc.writeBootAttempt(t.Context(), before.DeepCopy(), attempt(20*time.Second+time.Millisecond, fcintapi.SwitchBootAttemptResultError)))
	requireLast(served)

	// concurrent attempts of a switch, as many as there are writers: the newest one wins regardless of the order
	before = get()
	wg := sync.WaitGroup{}
	for idx := range bootAttemptWriters {
		wg.Go(func() {
			assert.NoError(t, svc.writeBootAttempt(t.Context(), before.DeepCopy(), attempt(time.Minute+time.Duration(idx)*time.Second, fcintapi.SwitchBootAttemptResultServed)))
		})
	}
	wg.Wait()
	requireLast(attempt(time.Minute+(bootAttemptWriters-1)*time.Second, fcintapi.SwitchBootAttemptResultServed))

	// gone already
	require.NoError(t, kube.Delete(t.Context(), get()))
	require.True(t, kapierrors.IsNotFound(svc.writeBootAttempt(t.Context(), before.DeepCopy(), attempt(time.Hour, fcintapi.SwitchBootAttemptResultServed))))
}

func TestWriteBootAttemptRecreated(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))

	key := kclient.ObjectKey{Namespace: kmetav1.NamespaceDefault, Name: "leaf-1"}
	newSB := func(uid ktypes.UID) *fcintapi.SwitchBoot {
		return &fcintapi.SwitchBoot{ObjectMeta: kmetav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace, UID: uid}}
	}
	recreated := false
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&fcintapi.SwitchBoot{}).
		WithObjects(newSB("uid-1")).
		WithInterceptorFuncs(interceptor.Funcs{
			// the switch is recreated while the attempt is being written
			SubResourceUpdate: func(ctx context.Context, c kclient.Client, sub string, obj kclient.Object, opts ...kclient.SubResourceUpdateOption) error {
				if !recreated {
					recreated = true
					if err := c.Delete(ctx, newSB("")); err != nil {
						return err //nolint:wrapcheck
					}
					if err := c.Create(ctx, newSB("uid-2")); err != nil {
						return err //nolint:wrapcheck
					}

					return kapierrors.NewConflict(schema.GroupResource{Group: fcintapi.GroupVersion.Group, Resource: "switchboots"}, obj.GetName(), errors.New("modified")) //nolint:err113
				}

				return c.SubResource(sub).Update(ctx, obj, opts...)
			},
		}).
		Build()
	svc := &service{kube: kube}

	before := &fcintapi.SwitchBoot{}
	require.NoError(t, kube.Get(t.Context(), key, before))
	require.Equal(t, ktypes.UID("uid-1"), before.UID)

	// the attempt was for the switch that's gone, it's not recorded on the new one
	require.NoError(t, svc.writeBootAttempt(t.Context(), before, fcintapi.SwitchBootAttempt{
		Time:   kmetav1.NowMicro(),
		Type:   fcintapi.SwitchBootAttemptTypeONIE,
		Result: fcintapi.SwitchBootAttemptResultServed,
	}))
	require.True(t, recreated)

	after := &fcintapi.SwitchBoot{}
	require.NoError(t, kube.Get(t.Context(), key, after))
	require.Equal(t, ktypes.UID("uid-2"), after.UID)
	require.Nil(t, after.Status.LastAttempt)
}

func TestRecordBootAttemptQueueFull(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, fcintapi.AddToScheme(scheme))

	sb := &fcintapi.SwitchBoot{ObjectMeta: kmetav1.ObjectMeta{Name: "leaf-1", Namespace: kmetav1.NamespaceDefault}}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(sb).WithObjects(sb).Build()
	svc := &service{kube: kube, bootAttempts: make(chan func(), 1)}

	attempt := func(result fcintapi.SwitchBootAttemptResult) fcintapi.SwitchBootAttempt {
		return fcintapi.SwitchBootAttempt{Time: kmetav1.NowMicro(), Type: fcintapi.SwitchBootAttemptTypeONIE, Result: result}
	}

	// the second one doesn't fit and is dropped instead of blocking
	svc.recordBootAttempt(t.Context(), slog.Default(), sb.DeepCopy(), attempt(fcintapi.SwitchBootAttemptResultServed))
	svc.recordBootAttempt(t.Context(), slog.Default(), sb.DeepCopy(), attempt(fcintapi.SwitchBootAttemptResultError))
	require.Len(t, svc.bootAttempts, 1)

	processBootAttempts(svc)
	stored := &fcintapi.SwitchBoot{}
	require.NoError(t, kube.Get(t.Context(), kclient.ObjectKeyFromObject(sb), stored))
	require.NotNil(t, stored.Status.LastAttempt)
	require.Equal(t, fcintapi.SwitchBootAttemptResultServed, stored.Status.LastAttempt.Result)
}

// processBootAttempts writes the queued boot attempts as the writers would
func processBootAttempts(svc *service) {
	for len(svc.bootAttempts) > 0 {
		(<-svc.bootAttempts)()
	}
}

func TestSwitchBootSpecChanged(t *testing.T) {
	sb := func(generation int64) *fcintapi.SwitchBoot {
		return &fcintapi.SwitchBoot{ObjectMeta: kmetav1.ObjectMeta{Generation: generation}}
	}

	require.False(t, switchBootSpecChanged(sb(1), sb(1)))
	require.True(t, switchBootSpecChanged(sb(1), sb(2)))
	require.True(t, switchBootSpecChanged(nil, sb(1)))
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
