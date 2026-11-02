// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// lockedRequeueAfter is how long a reconciler waits before checking again while fabric-ctrl is locked. Every object
// it got an event for stays queued meanwhile, so all of them are reconciled within that time once it's unlocked,
// without the events dropped by the fan-out mappers while locked.
const lockedRequeueAfter = 15 * time.Second

type lockState int32

const (
	lockLocked      lockState = iota // reconcilers wait, only the fabric controller itself writes, unvalidated
	lockMaintenance                  // reconcilers wait, everyone writes, validation errors are only warnings
	lockUnlocked                     // reconcilers run, every write is validated
)

// Lock keeps the fabric controller read-only until the running version is initialized: reconcilers wait and only
// the fabric controller itself may write the objects it serves webhooks for. It starts locked and is set by the
// FabricController controller, which runs on every replica as webhooks are served by all of them.
type Lock struct {
	state atomic.Int32
	self  string
}

func NewLock(self string) *Lock {
	return &Lock{self: self}
}

// Locked is true until the FabricController records the running version as initialized, and in maintenance
func (l *Lock) Locked() bool {
	return l.get() != lockUnlocked
}

// IsSelf is true for the username the fabric controller itself runs as
func (l *Lock) IsSelf(username string) bool {
	return username == l.self
}

func (l *Lock) get() lockState {
	return lockState(l.state.Load())
}

func (l *Lock) set(ctx context.Context, state lockState, reason string, keysAndValues ...any) {
	if lockState(l.state.Swap(int32(state))) == state {
		return
	}

	msg := "Fabric controller unlocked"
	switch state {
	case lockLocked:
		msg = "Fabric controller locked"
	case lockMaintenance:
		msg = "Fabric controller in maintenance"
	case lockUnlocked:
	}
	kctrllog.FromContext(ctx).Info(msg, append([]any{"reason", reason}, keysAndValues...)...)
}

// +kubebuilder:rbac:groups=authentication.k8s.io,resources=selfsubjectreviews,verbs=create

// SelfUsername returns the username the API server sees for the fabric controller, the same one it puts into
// admission requests, so own writes can be recognized without assembling a service account username
func SelfUsername(ctx context.Context, kube kclient.Client) (string, error) {
	review := &authnv1.SelfSubjectReview{}
	if err := kube.Create(ctx, review); err != nil {
		return "", fmt.Errorf("creating self subject review: %w", err)
	}

	if review.Status.UserInfo.Username == "" {
		return "", fmt.Errorf("self subject review returned no username") //nolint:err113
	}

	return review.Status.UserInfo.Username, nil
}
