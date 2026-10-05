// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// CleanupFinalizer holds an object until the controller manager cleaned up what the garbage collection can't, e.g.
// objects in other namespaces, which owner references can't reach
const CleanupFinalizer = "fabric.githedgehog.com/cleanup"

// onlyDeletes is for watching owned objects (Owns) so that a deleted one gets recreated, without reconciling the owner
// on every other change of it, e.g. the agent status heartbeats
var onlyDeletes = predicate.Funcs{
	CreateFunc:  func(event.CreateEvent) bool { return false },
	UpdateFunc:  func(event.UpdateEvent) bool { return false },
	GenericFunc: func(event.GenericEvent) bool { return false },
}

// setOwner makes owner the controller of obj, so that obj is garbage collected with the owner even if it's deleted
// while the controller isn't running. Only works within a namespace. The owner deletion isn't blocked on obj, that
// would need the finalizers permission on the owner.
func setOwner(owner, obj kclient.Object, scheme *runtime.Scheme) error {
	if err := ctrlutil.SetControllerReference(owner, obj, scheme, ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
		return fmt.Errorf("setting owner of %s: %w", obj.GetName(), err)
	}

	return nil
}
