// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"sync/atomic"

	authnv1 "k8s.io/api/authentication/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// Lock keeps the fabric controller read-only until the running version is initialized: reconcilers wait and only
// the fabric controller itself may write the objects it serves webhooks for. It starts locked and is set by the
// FabricController controller, which runs on every replica as webhooks are served by all of them.
type Lock struct {
	unlocked atomic.Bool
	self     string
}

func NewLock(self string) *Lock {
	return &Lock{self: self}
}

// Locked is true until the FabricController records the running version as initialized
func (l *Lock) Locked() bool {
	return !l.unlocked.Load()
}

// IsSelf is true for the username the fabric controller itself runs as
func (l *Lock) IsSelf(username string) bool {
	return username == l.self
}

func (l *Lock) set(ctx context.Context, locked bool, reason string, keysAndValues ...any) {
	if l.unlocked.Swap(!locked) == !locked {
		return
	}

	msg := "Fabric controller unlocked"
	if locked {
		msg = "Fabric controller locked"
	}
	kctrllog.FromContext(ctx).Info(msg, append([]any{"reason", reason}, keysAndValues...)...)
}

//+kubebuilder:rbac:groups=authentication.k8s.io,resources=selfsubjectreviews,verbs=create

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
