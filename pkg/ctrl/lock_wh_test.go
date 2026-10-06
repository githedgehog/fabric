// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	admissionv1 "k8s.io/api/admission/v1"
	authnv1 "k8s.io/api/authentication/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
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
				ctx = admission.NewContextWithRequest(ctx, admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					UserInfo: authnv1.UserInfo{Username: tt.username},
				}})
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
