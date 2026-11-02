// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	gwapi "go.githedgehog.com/fabric/api/gateway/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	"go.githedgehog.com/fabric/pkg/version"
	"k8s.io/apimachinery/pkg/api/equality"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// develVersion is the version of builds without a version set, they never count as initialized
const develVersion = "(devel)"

const (
	initRetryDelay    = 1 * time.Second
	initRetryMaxDelay = 30 * time.Second
	// initCheckInterval re-checks an initialized fabric controller, e.g. for a status changed by hand, which doesn't
	// change the generation and so doesn't trigger a reconcile
	initCheckInterval = 10 * time.Minute
)

// FabricControllerInitializer enforces the built-in switch profiles once per process and initializes the running
// fabric controller version for the current FabricController spec: the default fabric for a deployment that predates
// fabrics and the refresh of stored objects. Recording it as Ready then unlocks the fabric controller on every
// replica. While it runs the fabric controller is locked, so its own writes are the only ones. It's a leader-elected
// reconciler of the FabricController: errors are retried with backoff, a spec change (e.g. leaving maintenance)
// initializes again and a deleted FabricController is recreated.
type FabricControllerInitializer struct {
	kclient.Client
	apiReader    kclient.Reader
	key          ktypes.NamespacedName
	version      string
	cfg          *meta.FabricConfig
	profiles     *switchprofile.Default
	passInterval time.Duration
	// reconciles are sequential, so they're plain fields
	enforced    bool // the built-in switch profiles were enforced by this process
	initialized bool // this process completed initialization, devel builds initialize once per process
}

func SetupFabricControllerInitializerWith(mgr kctrl.Manager, namespace string, cfg *meta.FabricConfig, profiles *switchprofile.Default) error {
	if namespace == "" {
		return fmt.Errorf("fabric controller namespace is empty") //nolint:err113
	}
	if cfg == nil {
		return fmt.Errorf("fabric config is nil") //nolint:err113
	}
	if profiles == nil {
		return fmt.Errorf("switch profiles are nil") //nolint:err113
	}

	i := &FabricControllerInitializer{
		Client:       mgr.GetClient(),
		apiReader:    mgr.GetAPIReader(),
		key:          ktypes.NamespacedName{Namespace: namespace, Name: fcintapi.FabricControllerName},
		version:      version.Version,
		cfg:          cfg,
		profiles:     profiles,
		passInterval: refreshPassInterval,
	}

	onlyOwn := predicate.NewPredicateFuncs(func(obj kclient.Object) bool {
		return obj.GetNamespace() == i.key.Namespace && obj.GetName() == i.key.Name
	})

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("FabricControllerInit").
		// own status writes don't change the generation, so they don't trigger another initialization
		For(&fcintapi.FabricController{}, builder.WithPredicates(onlyOwn, predicate.GenerationChangedPredicate{})).
		// a new deployment has no FabricController to get an event for until the first reconcile creates it
		WatchesRawSource(source.Func(func(_ context.Context, queue workqueue.TypedRateLimitingInterface[kctrl.Request]) error {
			queue.Add(kctrl.Request{NamespacedName: i.key})

			return nil
		})).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[kctrl.Request](initRetryDelay, initRetryMaxDelay),
		}).
		Complete(i); err != nil {
		return fmt.Errorf("setting up fabric controller initializer: %w", err)
	}

	return nil
}

// +kubebuilder:rbac:groups=fcint.githedgehog.com,resources=fabriccontrollers,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=fcint.githedgehog.com,resources=fabriccontrollers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=wiring.githedgehog.com,resources=fabrics,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=wiring.githedgehog.com,resources=connections,verbs=delete

// Reconcile returns an error for anything that keeps the running version from being initialized but objects that need
// a fix, so it's retried with backoff: giving up would leave the fabric controller locked
func (i *FabricControllerInitializer) Reconcile(ctx context.Context, _ kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx).WithValues("version", i.version)
	ctx = kctrllog.IntoContext(ctx, l)

	fc, err := i.getOrCreate(ctx)
	if err != nil {
		return kctrl.Result{}, err
	}

	before := fc.Status.DeepCopy()
	fc.Status.Version = i.version

	if fc.Spec.Maintenance {
		// nothing is written meanwhile, the built-in switch profiles included: they may be what's being fixed
		fc.Status.Error = ""
		setReady(fc, fc.Generation, kmetav1.ConditionFalse, fcintapi.ReasonMaintenance, "nothing is deployed")

		return kctrl.Result{}, i.writeStatusIfChanged(ctx, fc, before)
	}

	if fc.IsReady(i.version) && (i.version != develVersion || i.initialized) {
		// once per process: it's what marks this process's catalog of built-in profiles as initialized, and the
		// profiles may have been changed by hand since
		if !i.enforced {
			if err := i.profiles.Enforce(ctx, i.Client, i.cfg, true); err != nil {
				return kctrl.Result{}, fmt.Errorf("enforcing switch profiles: %w", err)
			}
			i.enforced = true
		}

		if err := i.writeStatusIfChanged(ctx, fc, before); err != nil {
			return kctrl.Result{}, err
		}

		return kctrl.Result{RequeueAfter: initCheckInterval}, nil
	}

	// the initialization is for the spec it started with, a spec change meanwhile needs another one
	gen := fc.Generation
	if err := i.initialize(ctx, fc, gen); err != nil {
		fc.Status.Error = err.Error()
		setReady(fc, gen, kmetav1.ConditionFalse, fcintapi.ReasonInitializing, "retrying, see status.error")
		if err := i.writeStatus(ctx, fc); err != nil {
			l.Info("Failed to record initialization error", "error", err.Error())
		}

		return kctrl.Result{}, err
	}

	// failed on objects the refresh couldn't fix: it stays locked until a spec change, e.g. leaving maintenance, or a
	// restart initializes again, instead of repeating the refresh and its writes
	if !fc.IsReady(i.version) {
		return kctrl.Result{}, nil
	}

	return kctrl.Result{RequeueAfter: initCheckInterval}, nil
}

// initialize returns an error for anything that's retried with backoff, objects the refresh couldn't fix are only
// recorded as Failed
func (i *FabricControllerInitializer) initialize(ctx context.Context, fc *fcintapi.FabricController, gen int64) error {
	l := kctrllog.FromContext(ctx)

	l.Info("Initializing", "initialized", fc.Status.InitializedVersion)

	fc.Status.Error = ""
	fc.Status.Problems = nil
	setReady(fc, gen, kmetav1.ConditionFalse, fcintapi.ReasonInitializing, "starting")
	if err := i.writeStatus(ctx, fc); err != nil {
		return fmt.Errorf("recording initialization start: %w", err)
	}

	if err := i.profiles.Enforce(ctx, i.Client, i.cfg, true); err != nil {
		return fmt.Errorf("enforcing switch profiles: %w", err)
	}
	i.enforced = true

	if err := i.ensureFabricForUpgrade(ctx); err != nil {
		return err
	}

	if err := i.deleteVPCLoopbackConnections(ctx); err != nil {
		return err
	}

	if err := i.refresh(ctx, fc); err != nil {
		return fmt.Errorf("refreshing stored objects: %w", err)
	}
	fc.Status.Refresh.FinishedAt = kmetav1.Now()

	if failure := refreshFailure(fc.Status.Refresh.Kinds); failure != "" {
		setReady(fc, gen, kmetav1.ConditionFalse, fcintapi.ReasonFailed, failure)
		if err := i.writeStatus(ctx, fc); err != nil {
			return fmt.Errorf("recording failed initialization: %w", err)
		}

		l.Info("Initialization failed, objects need a fix in maintenance", "failure", failure)

		return nil
	}

	fc.Status.InitializedVersion = i.version
	setReady(fc, gen, kmetav1.ConditionTrue, fcintapi.ReasonInitialized, i.version)
	// this write unlocks fabric-ctrl on every replica; retried by writeStatus so a temporary error or a concurrent
	// spec change doesn't make the whole initialization, incl. a pass over all stored objects, start over
	if err := i.writeStatus(ctx, fc); err != nil {
		return fmt.Errorf("recording initialized version: %w", err)
	}
	i.initialized = true

	l.Info("Initialized")

	return nil
}

// refreshFailure summarizes the objects the refresh couldn't fix, it's empty if there are none
func refreshFailure(kinds []fcintapi.FabricControllerRefreshKind) string {
	invalid, rejected, stale := 0, 0, 0
	for _, k := range kinds {
		invalid += k.Invalid
		rejected += k.Rejected
		stale += k.Stale
	}

	parts := []string{}
	if invalid > 0 {
		parts = append(parts, fmt.Sprintf("%d invalid", invalid))
	}
	if rejected > 0 {
		parts = append(parts, fmt.Sprintf("%d rejected", rejected))
	}
	if stale > 0 {
		parts = append(parts, fmt.Sprintf("%d stale", stale))
	}

	return strings.Join(parts, ", ")
}

// setReady sets the only condition, ones left by earlier versions are dropped
func setReady(fc *fcintapi.FabricController, gen int64, status kmetav1.ConditionStatus, reason, msg string) {
	fc.Status.Conditions = slices.DeleteFunc(fc.Status.Conditions, func(c kmetav1.Condition) bool {
		return c.Type != fcintapi.ConditionReady
	})
	kmeta.SetStatusCondition(&fc.Status.Conditions, kmetav1.Condition{
		Type:               fcintapi.ConditionReady,
		Status:             status,
		ObservedGeneration: gen,
		Reason:             reason,
		Message:            msg,
	})
}

// setReadyMessage updates the summary of the current state, e.g. the progress of the initialization
func setReadyMessage(fc *fcintapi.FabricController, msg string) {
	if ready := kmeta.FindStatusCondition(fc.Status.Conditions, fcintapi.ConditionReady); ready != nil {
		ready.Message = msg
	}
}

// writeStatusIfChanged skips the write if nothing changed, it's called on every periodic check
func (i *FabricControllerInitializer) writeStatusIfChanged(ctx context.Context, fc *fcintapi.FabricController, before *fcintapi.FabricControllerStatus) error {
	if equality.Semantic.DeepEqual(before, &fc.Status) {
		return nil
	}

	return i.writeStatus(ctx, fc)
}

// writeStatus stores fc's status as a whole. fabric-ctrl's leader is the only status writer, so a conflict can only
// come from a spec change (e.g. maintenance): it re-reads the object from the API server, bypassing the cache, puts
// the same status on top and writes again. Temporary API errors are retried a few times as well.
func (i *FabricControllerInitializer) writeStatus(ctx context.Context, fc *fcintapi.FabricController) error {
	status := fc.Status.DeepCopy()

	if err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return !kapierrors.IsNotFound(err) && !kapierrors.IsInvalid(err) && !kapierrors.IsForbidden(err)
	}, func() error {
		err := i.Status().Update(ctx, fc)
		if kapierrors.IsConflict(err) {
			if err := i.apiReader.Get(ctx, i.key, fc); err != nil {
				return fmt.Errorf("getting fabric controller: %w", err)
			}
			fc.Status = *status.DeepCopy()
		}

		return err //nolint:wrapcheck
	}); err != nil {
		return fmt.Errorf("writing fabric controller status: %w", err)
	}

	return nil
}

func (i *FabricControllerInitializer) getOrCreate(ctx context.Context) (*fcintapi.FabricController, error) {
	fc := &fcintapi.FabricController{}
	err := i.Get(ctx, i.key, fc)
	if err == nil {
		return fc, nil
	}
	if !kapierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting fabric controller: %w", err)
	}

	fc = &fcintapi.FabricController{Namespace: i.key.Namespace, Name: i.key.Name}
	if err := i.Create(ctx, fc); err != nil {
		if !kapierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("creating fabric controller: %w", err)
		}

		// the cache hasn't caught up with an earlier create yet
		if err := i.apiReader.Get(ctx, i.key, fc); err != nil {
			return nil, fmt.Errorf("getting created fabric controller: %w", err)
		}
	}

	return fc, nil
}

// ensureFabricForUpgrade creates the default fabric for a deployment that predates fabrics: no fabric at all but
// objects that refer to one. Refreshing them then puts them into the default fabric and domain. A new deployment
// has nothing yet when it's initialized and gets its fabrics from the user
func (i *FabricControllerInitializer) ensureFabricForUpgrade(ctx context.Context) error {
	fabrics := &wiringapi.FabricList{}
	if err := i.List(ctx, fabrics, kclient.Limit(1)); err != nil {
		return fmt.Errorf("listing fabrics: %w", err)
	}
	if len(fabrics.Items) > 0 {
		return nil
	}

	predates := false
	for _, objs := range []kclient.ObjectList{
		&wiringapi.SwitchList{},
		&wiringapi.SwitchGroupList{},
		&wiringapi.ConnectionList{},
		&vpcapi.IPv4NamespaceList{},
		&vpcapi.VPCList{},
		&vpcapi.ExternalList{},
		&gwapi.GatewayList{},
	} {
		if err := i.List(ctx, objs, kclient.Limit(1)); err != nil {
			return fmt.Errorf("listing %T: %w", objs, err)
		}
		if kmeta.LenList(objs) > 0 {
			predates = true

			break
		}
	}
	if !predates {
		return nil
	}

	fabric := &wiringapi.Fabric{
		Namespace: kmetav1.NamespaceDefault, Name: wiringapi.DefaultFabric,
		Spec: wiringapi.DefaultFabricSpec(i.cfg),
	}
	if err := i.Create(ctx, fabric); err != nil {
		return fmt.Errorf("creating default fabric: %w", err)
	}

	kctrllog.FromContext(ctx).Info("Created the default fabric for a deployment that predates fabrics")

	return nil
}

// deleteVPCLoopbackConnections deletes the Connections of the VPC loopback workaround, whose type is gone from the
// API: what's stored of them has no spec left, only the type label their webhook set
func (i *FabricControllerInitializer) deleteVPCLoopbackConnections(ctx context.Context) error {
	conns := &wiringapi.ConnectionList{}
	if err := i.List(ctx, conns, kclient.MatchingLabels{wiringapi.LabelConnectionType: "vpc-loopback"}); err != nil {
		return fmt.Errorf("listing vpc-loopback connections: %w", err)
	}

	for idx := range conns.Items {
		conn := &conns.Items[idx]
		if err := i.Delete(ctx, conn); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("deleting vpc-loopback connection %s: %w", conn.Name, err)
		}

		kctrllog.FromContext(ctx).Info("Deleted vpc-loopback connection", "name", conn.Name)
	}

	return nil
}
