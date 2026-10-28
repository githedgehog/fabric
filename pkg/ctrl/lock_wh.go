// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"errors"
	"fmt"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// lockedValidator keeps the objects behind a webhook read-only for everyone but the fabric controller itself while
// it's locked. The fabric controller's own writes skip validation then: reconcilers wait while locked, so they're
// all from the initialization (refreshing stored objects with the current defaults, the default fabric, built-in
// switch profiles), which has to go through no matter how expensive or how much stricter validation has become.
type lockedValidator[T runtime.Object] struct {
	inner admission.Validator[T]
	lock  *Lock
}

func newLockedValidator[T runtime.Object](inner admission.Validator[T], lock *Lock) admission.Validator[T] {
	return &lockedValidator[T]{inner: inner, lock: lock}
}

// admitLocked returns true if the request is admitted without validation and an error if it's refused
func (v *lockedValidator[T]) admitLocked(ctx context.Context) (bool, error) {
	if !v.lock.Locked() {
		return false, nil
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return false, fmt.Errorf("getting admission request: %w", err)
	}

	if v.lock.IsSelf(req.UserInfo.Username) {
		return true, nil
	}

	return false, kapierrors.NewServiceUnavailable("fabric controller is initializing, retry shortly") //nolint:wrapcheck // API status error returned to the client as is
}

func (v *lockedValidator[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	if admitted, err := v.admitLocked(ctx); admitted || err != nil {
		return nil, err
	}

	warns, err := v.inner.ValidateCreate(ctx, obj)

	return warns, invalidError(ctx, err)
}

func (v *lockedValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	if admitted, err := v.admitLocked(ctx); admitted || err != nil {
		return nil, err
	}

	warns, err := v.inner.ValidateUpdate(ctx, oldObj, newObj)

	return warns, invalidError(ctx, err)
}

func (v *lockedValidator[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	if admitted, err := v.admitLocked(ctx); admitted || err != nil {
		return nil, err
	}

	return v.inner.ValidateDelete(ctx, obj) //nolint:wrapcheck
}

// invalidError turns a create or update validation error into a 422 Invalid, pointing at spec unless it's already a
// field error. controller-runtime returns a 403 for plain errors, on which kubectl edit (and so k9s) gives up instead
// of reopening the editor with the error.
func invalidError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	req, reqErr := admission.RequestFromContext(ctx)
	if reqErr != nil {
		return err
	}
	gk := schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind}

	if fieldErr, ok := errors.AsType[*field.Error](err); ok {
		return kapierrors.NewInvalid(gk, req.Name, field.ErrorList{fieldErr}) //nolint:wrapcheck // API status error returned to the client as is
	}

	var apiStatus kapierrors.APIStatus
	if errors.As(err, &apiStatus) {
		return err
	}

	return kapierrors.NewInvalid(gk, req.Name, field.ErrorList{field.Invalid(field.NewPath("spec"), field.OmitValueType{}, err.Error())}) //nolint:wrapcheck // API status error returned to the client as is
}
