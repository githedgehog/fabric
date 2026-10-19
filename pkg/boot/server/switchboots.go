// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	switchBootSerialField = "spec.serial"
	switchBootMACField    = "spec.mac"

	// fabric-boot is restarted by the liveness probe soon after anyway if it's not serving yet
	cacheSyncTimeout = 30 * time.Second

	agentPreCacheAttempts       = 3
	agentPreCacheAttemptTimeout = 5 * time.Minute
	agentPreCacheRetryDelay     = 10 * time.Second

	bootAttemptWriters      = 4
	bootAttemptQueue        = 2048
	bootAttemptWriteTimeout = 10 * time.Second
)

func indexSwitchBootSerial(obj kclient.Object) []string {
	return indexSwitchBoot(obj, func(sb *fcintapi.SwitchBoot) string { return sb.Spec.Serial })
}

func indexSwitchBootMAC(obj kclient.Object) []string {
	return indexSwitchBoot(obj, func(sb *fcintapi.SwitchBoot) string { return sb.Spec.MAC })
}

// indexSwitchBoot indexes a field as stored, it's normalized by the controller already
func indexSwitchBoot(obj kclient.Object, field func(*fcintapi.SwitchBoot) string) []string {
	sb, ok := obj.(*fcintapi.SwitchBoot)
	if !ok {
		return nil
	}

	if value := field(sb); value != "" {
		return []string{value}
	}

	return nil
}

func (svc *service) startSwitchBootCache(ctx context.Context, cfg *rest.Config, scheme *runtime.Scheme) (cache.Cache, error) {
	c, err := cache.New(cfg, cache.Options{
		Scheme:                      scheme,
		DefaultNamespaces:           map[string]cache.Config{kmetav1.NamespaceDefault: {}},
		ByObject:                    map[kclient.Object]cache.ByObject{&fcintapi.SwitchBoot{}: {}},
		ReaderFailOnMissingInformer: true,
		DefaultTransform:            cache.TransformStripManagedFields(),
		DefaultWatchErrorHandler:    switchBootWatchErrorHandler,
	})
	if err != nil {
		return nil, fmt.Errorf("creating cache: %w", err)
	}

	if err := c.IndexField(ctx, &fcintapi.SwitchBoot{}, switchBootSerialField, indexSwitchBootSerial); err != nil {
		return nil, fmt.Errorf("indexing switch boots by serial: %w", err)
	}
	if err := c.IndexField(ctx, &fcintapi.SwitchBoot{}, switchBootMACField, indexSwitchBootMAC); err != nil {
		return nil, fmt.Errorf("indexing switch boots by MAC: %w", err)
	}

	inf, err := c.GetInformer(ctx, &fcintapi.SwitchBoot{})
	if err != nil {
		return nil, fmt.Errorf("getting switch boot informer: %w", err)
	}
	if _, err := inf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) { svc.preCacheAgentFor(ctx, obj) },
		UpdateFunc: func(oldObj, obj any) {
			if switchBootSpecChanged(oldObj, obj) {
				svc.preCacheAgentFor(ctx, obj)
			}
		},
	}); err != nil {
		return nil, fmt.Errorf("adding switch boot event handler: %w", err)
	}

	go func() {
		if err := c.Start(ctx); err != nil {
			slog.Error("Failed to run switch boot cache", "error", err.Error())
			os.Exit(1)
		}
	}()

	syncCtx, cancel := context.WithTimeout(ctx, cacheSyncTimeout)
	defer cancel()

	if !c.WaitForCacheSync(syncCtx) {
		return nil, fmt.Errorf("switch boot cache not synced in %s", cacheSyncTimeout) //nolint:err113
	}

	return c, nil
}

func switchBootWatchErrorHandler(ctx context.Context, r *toolscache.Reflector, err error) {
	if fatalWatchError(err) {
		slog.Error("Failed to watch switch boots", "error", err.Error())
		os.Exit(1)
	}

	toolscache.DefaultWatchErrorHandler(ctx, r, err)
}

func fatalWatchError(err error) bool {
	return kapierrors.IsForbidden(err) || kapierrors.IsUnauthorized(err) || kapierrors.IsNotFound(err)
}

// normalizeBootIDs normalizes the serial and MAC of a boot request the same way the SwitchBoots are. An invalid MAC
// is left out, so the switch is still found by serial.
func normalizeBootIDs(l *slog.Logger, serial, mac string) (string, string) {
	normMAC, err := wiringapi.NormalizeBootMAC(mac)
	if err != nil {
		l.Info("Ignoring invalid MAC", "error", err.Error())
	}

	return wiringapi.NormalizeBootSerial(serial), normMAC
}

// findSwitchBoot expects the serial and MAC normalized, see normalizeBootIDs. The validation rejects switches sharing
// a serial or MAC, but it can't fully prevent them, e.g. created at the same time, so more than one switch matching
// is an error instead of booting one of them as the other.
func findSwitchBoot(ctx context.Context, boots kclient.Reader, serial, mac string) (*fcintapi.SwitchBoot, error) {
	var found *fcintapi.SwitchBoot
	for _, by := range []struct{ field, value string }{
		{switchBootSerialField, serial},
		{switchBootMACField, mac},
	} {
		if by.value == "" {
			continue
		}

		sbs := &fcintapi.SwitchBootList{}
		if err := boots.List(ctx, sbs, kclient.InNamespace(kmetav1.NamespaceDefault), kclient.MatchingFields{by.field: by.value}); err != nil {
			return nil, fmt.Errorf("listing switch boots by %s: %w", by.field, err)
		}

		for idx := range sbs.Items {
			sb := &sbs.Items[idx]
			if found != nil && found.Name != sb.Name {
				return nil, fmt.Errorf("both switches %s and %s match serial %q or MAC %q", found.Name, sb.Name, serial, mac) //nolint:err113
			}
			found = sb
		}
	}

	if found == nil {
		return nil, fmt.Errorf("switch boot: %w", ErrNotFound)
	}

	return found, nil
}

func switchBootSpecChanged(oldObj, obj any) bool {
	oldSB, oldOk := oldObj.(*fcintapi.SwitchBoot)
	sb, ok := obj.(*fcintapi.SwitchBoot)

	return !oldOk || !ok || oldSB.Generation != sb.Generation
}

func (svc *service) recordBootAttempt(ctx context.Context, l *slog.Logger, sb *fcintapi.SwitchBoot, attempt fcintapi.SwitchBootAttempt) {
	ctx = context.WithoutCancel(ctx)
	write := func() {
		if err := svc.writeBootAttempt(ctx, sb, attempt); err != nil {
			l.Warn("Failed to record boot attempt", "error", err.Error())
		}
	}

	select {
	case svc.bootAttempts <- write:
	default:
		l.Warn("Too many boot attempts queued, skipping", "result", attempt.Result)
	}
}

func (svc *service) writeBootAttempt(ctx context.Context, sb *fcintapi.SwitchBoot, attempt fcintapi.SwitchBootAttempt) error {
	ctx, cancel := context.WithTimeout(ctx, bootAttemptWriteTimeout)
	defer cancel()

	key := kclient.ObjectKeyFromObject(sb)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error { //nolint:wrapcheck
		if last := sb.Status.LastAttempt; last != nil && last.Time.After(attempt.Time.Time) {
			return nil
		}

		sb.Status = fcintapi.SwitchBootStatus{LastAttempt: attempt.DeepCopy()}
		err := svc.kube.Status().Update(ctx, sb)
		if kapierrors.IsConflict(err) {
			sb = &fcintapi.SwitchBoot{}
			if err := svc.kube.Get(ctx, key, sb); err != nil {
				return fmt.Errorf("getting switch boot: %w", err)
			}
		}

		return err //nolint:wrapcheck
	})
}

// preCacheAgentFor runs in the informer's goroutine for this handler, which buffers the events in the meantime, so a
// slow download only delays the next events of this handler and not the cache itself
func (svc *service) preCacheAgentFor(ctx context.Context, obj any) {
	sb, ok := obj.(*fcintapi.SwitchBoot)
	if !ok {
		slog.Warn("Unexpected object in switch boot informer", "type", fmt.Sprintf("%T", obj))

		return
	}

	if sb.Spec.AgentVersion == "" {
		return
	}

	if err := svc.preCacheAgent(ctx, sb.Spec.AgentVersion); err != nil {
		if ctx.Err() != nil {
			return
		}

		slog.Error("Failed to pre-cache agent", "error", err.Error())
		os.Exit(1)
	}
}

// preCacheAgent is a no-op for an already cached version
func (svc *service) preCacheAgent(ctx context.Context, version string) error {
	var err error
	for attempt := 1; attempt <= agentPreCacheAttempts; attempt++ {
		if err = svc.preCacheAgentAttempt(ctx, version); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("context done: %w", ctx.Err())
		}

		if attempt < agentPreCacheAttempts {
			slog.Warn("Failed to pre-cache agent, retrying", "version", version, "attempt", attempt, "error", err.Error())

			select {
			case <-ctx.Done():
				return fmt.Errorf("context done: %w", ctx.Err())
			case <-time.After(agentPreCacheRetryDelay):
			}
		}
	}

	return fmt.Errorf("pre-caching agent %s %s: %w", svc.cfg.AgentRef, version, err)
}

func (svc *service) preCacheAgentAttempt(ctx context.Context, version string) error {
	ctx, cancel := context.WithTimeout(ctx, agentPreCacheAttemptTimeout)
	defer cancel()

	if _, err := svc.getCachedOrDownload(ctx, svc.cfg.AgentRef, version, true); err != nil {
		return err
	}

	return nil
}
