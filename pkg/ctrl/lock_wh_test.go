// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	authnv1 "k8s.io/api/authentication/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type countingValidator struct {
	calls int
}

func (v *countingValidator) ValidateCreate(_ context.Context, _ *wiringapi.Server) (admission.Warnings, error) {
	v.calls++

	return nil, nil
}

func (v *countingValidator) ValidateUpdate(_ context.Context, _, _ *wiringapi.Server) (admission.Warnings, error) {
	v.calls++

	return nil, nil
}

func (v *countingValidator) ValidateDelete(_ context.Context, _ *wiringapi.Server) (admission.Warnings, error) {
	v.calls++

	return nil, nil
}

func TestLockedValidator(t *testing.T) {
	const self = "system:serviceaccount:fab:fabric-ctrl"

	for _, tt := range []struct {
		name        string
		unlocked    bool
		username    string
		noRequest   bool
		validated   bool
		unavailable bool
		err         bool
	}{
		{name: "unlocked user", unlocked: true, username: "system:admin", validated: true},
		{name: "unlocked self", unlocked: true, username: self, validated: true},
		{name: "locked user", username: "system:admin", unavailable: true},
		{name: "locked self", username: self},
		{name: "locked without request", noRequest: true, err: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lock := NewLock(self)
			lock.unlocked.Store(tt.unlocked)

			inner := &countingValidator{}
			v := newLockedValidator[*wiringapi.Server](inner, lock)

			ctx := t.Context()
			if !tt.noRequest {
				ctx = admission.NewContextWithRequest(ctx, admission.Request{
					UserInfo: authnv1.UserInfo{Username: tt.username},
				})
			}

			srv := &wiringapi.Server{}
			_, createErr := v.ValidateCreate(ctx, srv)
			_, updateErr := v.ValidateUpdate(ctx, srv, srv)
			_, deleteErr := v.ValidateDelete(ctx, srv)

			for _, err := range []error{createErr, updateErr, deleteErr} {
				switch {
				case tt.unavailable:
					require.True(t, kapierrors.IsServiceUnavailable(err), "expected service unavailable, got %v", err)
				case tt.err:
					require.Error(t, err)
				default:
					require.NoError(t, err)
				}
			}

			if tt.validated {
				require.Equal(t, 3, inner.calls)
			} else {
				require.Zero(t, inner.calls)
			}
		})
	}
}

type failingValidator struct {
	err error
}

func (v *failingValidator) ValidateCreate(_ context.Context, _ *wiringapi.Server) (admission.Warnings, error) {
	return nil, v.err
}

func (v *failingValidator) ValidateUpdate(_ context.Context, _, _ *wiringapi.Server) (admission.Warnings, error) {
	return nil, v.err
}

func (v *failingValidator) ValidateDelete(_ context.Context, _ *wiringapi.Server) (admission.Warnings, error) {
	return nil, v.err
}

func TestLockedValidatorStatusErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		err      error
		field    string
		message  string
		status   bool
		internal bool
	}{
		{
			name:     "plain error",
			err:      errors.New("listing failed"), //nolint:err113
			internal: true,
			message:  "Internal error occurred: listing failed",
		},
		{
			name:    "wrapped field error",
			err:     fmt.Errorf("failed to validate server: %w", field.Invalid(field.NewPath("spec", "profile"), "foo", "not found")),
			field:   "spec.profile",
			message: `Server.wiring.githedgehog.com "server-1" is invalid: spec.profile: Invalid value: "foo": not found`,
		},
		{
			name:   "api status error",
			err:    kapierrors.NewServiceUnavailable("try later"),
			status: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lock := NewLock("self")
			lock.unlocked.Store(true)
			v := newLockedValidator[*wiringapi.Server](&failingValidator{err: tt.err}, lock)

			ctx := admission.NewContextWithRequest(t.Context(), admission.Request{
				Name: "server-1",
				Kind: kmetav1.GroupVersionKind{Group: "wiring.githedgehog.com", Version: "v1beta1", Kind: "Server"},
			})

			srv := &wiringapi.Server{}
			_, createErr := v.ValidateCreate(ctx, srv)
			_, updateErr := v.ValidateUpdate(ctx, srv, srv)
			_, deleteErr := v.ValidateDelete(ctx, srv)

			require.Equal(t, tt.err, deleteErr, "delete errors are returned as is")

			for _, err := range []error{createErr, updateErr} {
				if tt.status {
					require.Equal(t, tt.err, err)

					continue
				}

				if tt.internal {
					require.True(t, kapierrors.IsInternalError(err), "expected internal error, got %v", err)
					require.EqualError(t, err, tt.message)

					continue
				}

				require.True(t, kapierrors.IsInvalid(err), "expected invalid, got %v", err)
				require.EqualError(t, err, tt.message)

				var statusErr *kapierrors.StatusError
				require.ErrorAs(t, err, &statusErr)
				require.Len(t, statusErr.ErrStatus.Details.Causes, 1)
				require.Equal(t, tt.field, statusErr.ErrStatus.Details.Causes[0].Field)
			}
		})
	}
}
