// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// lockedValidator keeps the objects behind a webhook read-only for everyone but the fabric controller itself while
// it's locked. The fabric controller's own writes skip validation then: reconcilers wait while locked, so they're
// all from the initialization (refreshing stored objects with the current defaults, the default fabric, built-in
// switch profiles), which has to go through no matter how expensive or how much stricter validation has become.
// In maintenance everyone's writes are admitted, with validation errors as warnings, to fix what keeps it locked.
type lockedValidator[T runtime.Object] struct {
	inner admission.Validator[T]
	lock  *Lock
}

func newLockedValidator[T runtime.Object](inner admission.Validator[T], lock *Lock) admission.Validator[T] {
	return &lockedValidator[T]{inner: inner, lock: lock}
}

func (v *lockedValidator[T]) validate(ctx context.Context, inner func() (admission.Warnings, error)) (admission.Warnings, error) {
	state := v.lock.get()
	if state == lockUnlocked {
		return inner()
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting admission request: %w", err)
	}

	if v.lock.IsSelf(req.UserInfo.Username) {
		return nil, nil
	}

	if state == lockMaintenance {
		warnings, err := inner()
		if err != nil {
			warnings = append(warnings, "Not enforced in maintenance: "+err.Error())
		}

		return warnings, nil
	}

	return nil, kapierrors.NewServiceUnavailable("fabric controller is locked, see kubectl get fabriccontrollers -A") //nolint:wrapcheck // API status error returned to the client as is
}

func (v *lockedValidator[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	return v.validate(ctx, func() (admission.Warnings, error) {
		return v.inner.ValidateCreate(ctx, obj)
	})
}

func (v *lockedValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	return v.validate(ctx, func() (admission.Warnings, error) {
		return v.inner.ValidateUpdate(ctx, oldObj, newObj)
	})
}

func (v *lockedValidator[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	return v.validate(ctx, func() (admission.Warnings, error) {
		return v.inner.ValidateDelete(ctx, obj)
	})
}
