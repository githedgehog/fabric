// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"sync"
	"time"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/equality"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// refreshPassInterval gives the cache time to catch up with the writes of a pass before the next one checks it
	refreshPassInterval = 5 * time.Second
	refreshMaxPasses    = 20
	refreshWorkers      = 4
	// refreshMaxAttempts is how many times an object may come back without the current defaults, e.g. defaulted by
	// another version's webhook, before it's left stale
	refreshMaxAttempts = 5
	refreshReportEvery = 5 * time.Second
)

// refreshKinds are all kinds with a defaulting webhook. Their Default() must only depend on the object itself: a kind
// whose pass finds nothing to change isn't checked again, which only holds while refreshing other kinds can't change
// what its defaults are. The webhooks' Default only calls the object's Default(), so it's called directly.
var refreshKinds = []refreshKind{
	{"Fabric", func() kclient.ObjectList { return &wiringapi.FabricList{} }},
	{"SwitchProfile", func() kclient.ObjectList { return &wiringapi.SwitchProfileList{} }},
	{"VLANNamespace", func() kclient.ObjectList { return &wiringapi.VLANNamespaceList{} }},
	{"IPv4Namespace", func() kclient.ObjectList { return &vpcapi.IPv4NamespaceList{} }},
	{"SwitchGroup", func() kclient.ObjectList { return &wiringapi.SwitchGroupList{} }},
	{"Switch", func() kclient.ObjectList { return &wiringapi.SwitchList{} }},
	{"Server", func() kclient.ObjectList { return &wiringapi.ServerList{} }},
	{"Connection", func() kclient.ObjectList { return &wiringapi.ConnectionList{} }},
	{"VPC", func() kclient.ObjectList { return &vpcapi.VPCList{} }},
	{"VPCAttachment", func() kclient.ObjectList { return &vpcapi.VPCAttachmentList{} }},
	{"VPCPeering", func() kclient.ObjectList { return &vpcapi.VPCPeeringList{} }},
	{"External", func() kclient.ObjectList { return &vpcapi.ExternalList{} }},
	{"ExternalAttachment", func() kclient.ObjectList { return &vpcapi.ExternalAttachmentList{} }},
	{"ExternalPeering", func() kclient.ObjectList { return &vpcapi.ExternalPeeringList{} }},
	{"GatewayGroup", func() kclient.ObjectList { return &gwapi.GatewayGroupList{} }},
	{"Gateway", func() kclient.ObjectList { return &gwapi.GatewayList{} }},
	{"VPCInfo", func() kclient.ObjectList { return &gwapi.VPCInfoList{} }},
	{"GatewayPeering", func() kclient.ObjectList { return &gwapi.GatewayPeeringList{} }},
}

//+kubebuilder:rbac:groups=wiring.githedgehog.com,resources=fabrics;switchprofiles;vlannamespaces;switchgroups;switches;servers;connections,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=vpc.githedgehog.com,resources=ipv4namespaces;vpcs;vpcattachments;vpcpeerings;externals;externalattachments;externalpeerings,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gatewaygroups;gateways;vpcinfos;gatewaypeerings,verbs=get;list;watch;update

type refreshKind struct {
	kind    string
	newList func() kclient.ObjectList
}

type defaultable interface {
	kclient.Object
	Default()
}

// refreshState is the progress of a refresh, shared by the workers of a pass. Objects are keyed by kind/ns/name.
type refreshState struct {
	mu    sync.Mutex
	kinds map[string]*fcintapi.FabricControllerRefreshKind
	// writes counts the successful writes per object. A write made from an outdated cached copy fails with a
	// conflict and isn't counted, so an object only gets a second successful write if the cache already showed
	// the first one stored and it still didn't carry the current defaults, e.g. because another version's webhook
	// defaulted it. After refreshMaxAttempts writes the object is left alone and counted as stale.
	writes   map[string]int
	excluded map[string]bool
	// validated are the objects already validated by this refresh, once is enough
	validated map[string]bool
}

func (st *refreshState) kind(kind string) *fcintapi.FabricControllerRefreshKind {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, exists := st.kinds[kind]; !exists {
		st.kinds[kind] = &fcintapi.FabricControllerRefreshKind{Kind: kind}
	}

	return st.kinds[kind]
}

func (st *refreshState) isExcluded(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	return st.excluded[key]
}

// firstValidation records the object as validated, it's true the first time only
func (st *refreshState) firstValidation(key string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.validated[key] {
		return false
	}
	st.validated[key] = true

	return true
}

func (st *refreshState) update(f func()) {
	st.mu.Lock()
	defer st.mu.Unlock()

	f()
}

// refresh writes the current defaults to every stored object that doesn't carry them. It repeats passes over the
// cache until one finds nothing left to change: informers aren't ordered against each other, so the FabricController
// update that unlocks reconcilers could otherwise reach them before the last refreshed objects do. Objects whose
// write is rejected or that keep coming back without the current defaults are left out and recorded in the status,
// as are the objects that fail the current validation, which are still updated with the current defaults.
func (i *FabricControllerInitializer) refresh(ctx context.Context, fc *fcintapi.FabricController) error {
	l := kctrllog.FromContext(ctx)

	fc.Status.Refresh = fcintapi.FabricControllerRefresh{Version: i.version, StartedAt: kmetav1.Now()}
	kmeta.SetStatusCondition(&fc.Status.Conditions, kmetav1.Condition{
		Type:               fcintapi.ConditionRefreshing,
		Status:             kmetav1.ConditionTrue,
		ObservedGeneration: fc.Generation,
		Reason:             "Refreshing",
		Message:            "Refreshing stored objects with the defaults of version " + i.version,
	})
	if err := i.writeStatus(ctx, fc); err != nil {
		return fmt.Errorf("recording refresh start: %w", err)
	}

	st := &refreshState{
		kinds:     map[string]*fcintapi.FabricControllerRefreshKind{},
		writes:    map[string]int{},
		excluded:  map[string]bool{},
		validated: map[string]bool{},
	}
	lastReport := time.Now()

	pending := refreshKinds
	for pass := 1; ; pass++ {
		if pass > refreshMaxPasses {
			names := []string{}
			for _, k := range pending {
				names = append(names, k.kind)
			}

			return fmt.Errorf("refresh didn't converge after %d passes, pending %v", refreshMaxPasses, names) //nolint:err113
		}

		next := []refreshKind{}
		found := map[string]int{}
		for _, k := range pending {
			diffs, err := i.refreshKind(ctx, k, st)
			if err != nil {
				return err
			}
			if diffs > 0 {
				next = append(next, k)
				found[k.kind] = diffs
			}

			// progress goes to the status at most every refreshReportEvery, a large kind alone can take a while
			if time.Since(lastReport) >= refreshReportEvery {
				i.reportRefresh(ctx, fc, st, pass)
				lastReport = time.Now()
			}
		}

		l.Info("Refresh pass done", "pass", pass, "found", found)

		if len(next) == 0 {
			// the final counts, the caller records the initialized version right after
			i.reportRefresh(ctx, fc, st, pass)

			return nil
		}
		pending = next

		select {
		case <-ctx.Done():
			return fmt.Errorf("refresh cancelled: %w", ctx.Err())
		case <-time.After(i.passInterval):
		}
	}
}

// refreshKind writes the current defaults to every object of the kind that doesn't carry them in the cache and
// returns how many it found
func (i *FabricControllerInitializer) refreshKind(ctx context.Context, k refreshKind, st *refreshState) (int, error) {
	list := k.newList()
	if err := i.List(ctx, list, kclient.UnsafeDisableDeepCopy); err != nil {
		if kmeta.IsNoMatchError(err) {
			return 0, nil
		}

		return 0, fmt.Errorf("listing %s: %w", k.kind, err)
	}

	// the total is re-counted every pass the kind is checked in
	progress := st.kind(k.kind)
	st.update(func() { progress.Total = kmeta.LenList(list) })

	diffs := 0
	g := &errgroup.Group{}
	g.SetLimit(refreshWorkers)
	if err := kmeta.EachListItem(list, func(item runtime.Object) error {
		obj, ok := item.(defaultable)
		if !ok {
			return fmt.Errorf("%s %T can't be defaulted", k.kind, item) //nolint:err113
		}

		key := k.kind + "/" + obj.GetNamespace() + "/" + obj.GetName()
		if st.isExcluded(key) {
			return nil
		}

		// the list shares the cache's objects, so defaulting needs a copy: the only one per object and pass
		want, _ := obj.DeepCopyObject().(defaultable)
		want.Default()
		if st.firstValidation(key) {
			i.validateStored(ctx, k.kind, want, st)
		}
		if defaultedEqual(obj, want) {
			return nil
		}

		diffs++
		g.Go(func() error {
			i.refreshObject(ctx, k.kind, key, want, st)

			return nil
		})

		return nil
	}); err != nil {
		return 0, fmt.Errorf("refreshing %s: %w", k.kind, err)
	}
	_ = g.Wait()

	return diffs, nil
}

// validateStored reports a stored object that fails the validation of the current version. The object is still
// updated with the current defaults: the refresh skips validation, as an object left without the current defaults
// and labels is missed by whatever looks it up by them. It's validated without a client, so without looking up other
// objects, and on its own copy, as it's only checked. obj is the defaulted object, as the webhooks validate it.
func (i *FabricControllerInitializer) validateStored(ctx context.Context, kind string, obj defaultable, st *refreshState) {
	var err error
	switch v := obj.DeepCopyObject().(type) {
	case interface {
		Validate(ctx context.Context, kube kclient.Reader, cfg *meta.FabricConfig) (admission.Warnings, error)
	}:
		_, err = v.Validate(ctx, nil, i.cfg)
	case interface {
		Validate(ctx context.Context, kube kclient.Reader, cfg *meta.FabricConfig) error
	}:
		err = v.Validate(ctx, nil, i.cfg)
	default:
		err = fmt.Errorf("%T can't be validated", obj) //nolint:err113
	}
	if err == nil {
		return
	}

	kctrllog.FromContext(ctx).Error(err, "Stored object fails validation, any change to it is rejected until it's fixed",
		"kind", kind, "ns", obj.GetNamespace(), "name", obj.GetName())
	progress := st.kind(kind)
	st.update(func() { progress.Invalid++ })
}

// refreshObject writes an object with the current defaults and records the outcome. obj is the refresh's own copy.
func (i *FabricControllerInitializer) refreshObject(ctx context.Context, kind, key string, obj defaultable, st *refreshState) {
	l := kctrllog.FromContext(ctx).WithValues("kind", kind, "ns", obj.GetNamespace(), "name", obj.GetName())

	alreadyStored := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := i.Update(ctx, obj)
		if !kapierrors.IsConflict(err) {
			return err //nolint:wrapcheck
		}

		// The write was made from an outdated copy: the cache hasn't caught up with an earlier write yet, or the
		// object changed. Re-read the stored object from the API server, bypassing the cache, into obj (it's our
		// own copy, nothing else refers to it). It may already carry the current defaults, e.g. written by the
		// previous pass, then there's nothing to write and it's not counted as a write.
		if err := i.apiReader.Get(ctx, kclient.ObjectKeyFromObject(obj), obj); err != nil {
			return fmt.Errorf("getting stored object: %w", err)
		}

		want, _ := obj.DeepCopyObject().(defaultable)
		want.Default()
		if defaultedEqual(obj, want) {
			alreadyStored = true

			return nil
		}

		// retry with the defaulted stored object, it carries the current resource version
		obj = want

		return err //nolint:wrapcheck
	})

	progress := st.kind(kind)
	switch {
	case kapierrors.IsNotFound(err):
		// deleted since the cache listed it, nothing left to refresh; it's gone from the cache by a later pass too
		return
	case kapierrors.IsInvalid(err) || kapierrors.IsForbidden(err) || kapierrors.IsBadRequest(err):
		l.Error(err, "Refresh rejected, leaving the object as is")
		st.update(func() {
			st.excluded[key] = true
			progress.Rejected++
		})

		return
	case err != nil:
		// retried by the next pass
		l.Info("Refresh failed", "error", err.Error())

		return
	case alreadyStored:
		return
	}

	// whether the write stuck is checked by the next pass against the cache, see refreshState.writes
	st.update(func() {
		st.writes[key]++
		switch st.writes[key] {
		case 1:
			progress.Updated++
		case refreshMaxAttempts:
			st.excluded[key] = true
			progress.Updated--
			progress.Stale++
			l.Info("Refreshed object keeps coming back without the current defaults, leaving it stale")
		}
	})
}

// reportRefresh records the refresh progress, failing to do so doesn't stop the refresh: the next report writes the
// whole status again
func (i *FabricControllerInitializer) reportRefresh(ctx context.Context, fc *fcintapi.FabricController, st *refreshState, passes int) {
	// the full per-kind list in the refreshKinds order
	fc.Status.Refresh.Passes = passes
	fc.Status.Refresh.Kinds = nil
	st.update(func() {
		for _, k := range refreshKinds {
			if progress, exists := st.kinds[k.kind]; exists {
				fc.Status.Refresh.Kinds = append(fc.Status.Refresh.Kinds, *progress)
			}
		}
	})

	if err := i.writeStatus(ctx, fc); err != nil {
		kctrllog.FromContext(ctx).Info("Failed to record refresh progress", "error", err.Error())
	}
}

// defaultedEqual compares what defaulting may change: labels, annotations and spec. The rest of the metadata differs
// between the cached and the written object anyway, e.g. resource version and managed fields.
func defaultedEqual(a, b kclient.Object) bool {
	return maps.Equal(a.GetLabels(), b.GetLabels()) &&
		maps.Equal(a.GetAnnotations(), b.GetAnnotations()) &&
		equality.Semantic.DeepEqual(specOf(a), specOf(b))
}

// specOf returns the Spec field of an API object, compared typed as it's much cheaper than through unstructured
func specOf(obj kclient.Object) any {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	spec := v.FieldByName("Spec")
	if !spec.IsValid() {
		return nil
	}

	return spec.Interface()
}
