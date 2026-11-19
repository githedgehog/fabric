// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	authnv1 "k8s.io/api/authentication/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type countingValidator struct {
	calls int
	err   error
}

func (v *countingValidator) ValidateCreate(_ context.Context, _ *wiringapi.Server) (admission.Warnings, error) {
	v.calls++

	return admission.Warnings{"inner"}, v.err
}

func (v *countingValidator) ValidateUpdate(_ context.Context, _, _ *wiringapi.Server) (admission.Warnings, error) {
	v.calls++

	return admission.Warnings{"inner"}, v.err
}

func (v *countingValidator) ValidateDelete(_ context.Context, _ *wiringapi.Server) (admission.Warnings, error) {
	v.calls++

	return admission.Warnings{"inner"}, v.err
}

func TestLockedValidator(t *testing.T) {
	const self = "system:serviceaccount:fab:fabric-ctrl"

	invalid := errors.New("invalid server") //nolint:err113

	for _, tt := range []struct {
		name        string
		state       lockState
		username    string
		noRequest   bool
		innerErr    error
		validated   bool
		unavailable bool
		err         bool
		warnings    admission.Warnings
	}{
		{name: "unlocked user", state: lockUnlocked, username: "system:admin", validated: true, warnings: admission.Warnings{"inner"}},
		{name: "unlocked self", state: lockUnlocked, username: self, validated: true, warnings: admission.Warnings{"inner"}},
		{name: "unlocked invalid", state: lockUnlocked, username: "system:admin", innerErr: invalid, validated: true, err: true, warnings: admission.Warnings{"inner"}},
		{name: "locked user", state: lockLocked, username: "system:admin", unavailable: true},
		{name: "locked self", state: lockLocked, username: self},
		{name: "locked without request", state: lockLocked, noRequest: true, err: true},
		{
			name: "maintenance user", state: lockMaintenance, username: "system:admin", validated: true,
			warnings: admission.Warnings{"inner", maintenanceWarning},
		},
		{
			name: "maintenance invalid", state: lockMaintenance, username: "system:admin", innerErr: invalid, validated: true,
			warnings: admission.Warnings{"inner", maintenanceWarning, "Not enforced in maintenance: invalid server"},
		},
		{name: "maintenance self", state: lockMaintenance, username: self},
		{name: "maintenance without request", state: lockMaintenance, noRequest: true, err: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lock := NewLock(self)
			lock.state.Store(int32(tt.state))

			inner := &countingValidator{err: tt.innerErr}
			v := newLockedValidator[*wiringapi.Server](inner, lock)

			ctx := t.Context()
			if !tt.noRequest {
				ctx = admission.NewContextWithRequest(ctx, admission.Request{
					UserInfo: authnv1.UserInfo{Username: tt.username},
				})
			}

			srv := &wiringapi.Server{}
			createWarnings, createErr := v.ValidateCreate(ctx, srv)
			updateWarnings, updateErr := v.ValidateUpdate(ctx, srv, srv)
			deleteWarnings, deleteErr := v.ValidateDelete(ctx, srv)

			for _, warnings := range []admission.Warnings{createWarnings, updateWarnings, deleteWarnings} {
				require.Equal(t, tt.warnings, warnings)
			}

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
