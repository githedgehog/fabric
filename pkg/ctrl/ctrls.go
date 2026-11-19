// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"maps"
	"slices"

	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// switchRequests turns switch names into requests, sorted for a stable order
func switchRequests(namespace string, switches map[string]bool) []reconcile.Request {
	res := make([]reconcile.Request, 0, len(switches))
	for _, name := range slices.Sorted(maps.Keys(switches)) {
		res = append(res, reconcile.Request{Namespace: namespace, Name: name})
	}

	return res
}

func onlyObject(key ktypes.NamespacedName) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj kclient.Object) bool {
		return obj.GetNamespace() == key.Namespace && obj.GetName() == key.Name
	})
}

// enqueueAllSwitches reconciles all switches in the object's namespace, all of them for an object without one
func enqueueAllSwitches(kube kclient.Reader, lock *Lock) handler.MapFunc {
	return func(ctx context.Context, obj kclient.Object) []reconcile.Request {
		// all switches are queued and reconciled once unlocked, see lockedRequeueAfter; the refresh alone would fan out
		// every write of a refreshed object to every switch otherwise
		if lock.Locked() {
			return nil
		}

		res := []reconcile.Request{}

		sws := &wiringapi.SwitchList{}
		err := kube.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()))
		if err != nil {
			kctrllog.FromContext(ctx).Error(err, "error listing switches to reconcile all")

			return res
		}

		for _, sw := range sws.Items {
			res = append(res, reconcile.Request{
				Namespace: sw.Namespace,
				Name:      sw.Name,
			})
		}

		return res
	}
}

// enqueueSwitchesByProfile reconciles the switches using the switch profile
func enqueueSwitchesByProfile(kube kclient.Reader, lock *Lock) handler.MapFunc {
	return func(ctx context.Context, obj kclient.Object) []reconcile.Request {
		// all switches are queued and reconciled once unlocked, see lockedRequeueAfter
		if lock.Locked() {
			return nil
		}

		res := []reconcile.Request{}

		sws := &wiringapi.SwitchList{}
		err := kube.List(ctx, sws, kclient.InNamespace(obj.GetNamespace()))
		if err != nil {
			kctrllog.FromContext(ctx).Error(err, "error listing switches to reconcile by profile")

			return res
		}

		for _, sw := range sws.Items {
			if sw.Spec.Profile != obj.GetName() {
				continue
			}

			res = append(res, reconcile.Request{
				Namespace: sw.Namespace,
				Name:      sw.Name,
			})
		}

		return res
	}
}
