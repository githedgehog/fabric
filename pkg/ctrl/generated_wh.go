// Copyright 2026 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ctrl

import (
	"context"
	"fmt"

	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// generatedValidator keeps the objects generated for a VPCInterconnect to the fabric controller. Anyone else may only
// update or delete one once its VPCInterconnect is gone or being deleted, as the garbage collector does when it
// deletes them. Orphaning deletes are refused by the VPCInterconnect webhook.
type generatedValidator[T kclient.Object] struct {
	inner admission.Validator[T]
	kube  kclient.Reader
	lock  *Lock
}

func newGeneratedValidator[T kclient.Object](inner admission.Validator[T], kube kclient.Reader, lock *Lock) admission.Validator[T] {
	return &generatedValidator[T]{inner: inner, kube: kube, lock: lock}
}

// check refuses a write to an object generated for the owner by anyone but the fabric controller, unless ownerGone
// allows it and the owner is gone or being deleted
func (v *generatedValidator[T]) check(ctx context.Context, obj T, owner string, ownerGone bool) error {
	if owner == "" {
		return nil
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting admission request: %w", err)
	}
	if v.lock.IsSelf(req.UserInfo.Username) {
		return nil
	}

	if ownerGone {
		ic := &vpcapi.VPCInterconnect{}
		err := v.kube.Get(ctx, kclient.ObjectKey{Namespace: obj.GetNamespace(), Name: owner}, ic)
		if kapierrors.IsNotFound(err) || err == nil && ic.DeletionTimestamp != nil {
			return nil
		}
		if err != nil {
			return fmt.Errorf("getting VPC interconnect %s: %w", owner, err) // TODO hide internal error
		}
	}

	return fmt.Errorf("generated for VPC interconnect %s, change or delete that instead", owner) //nolint:err113
}

func (v *generatedValidator[T]) ValidateCreate(ctx context.Context, obj T) (admission.Warnings, error) {
	if err := v.check(ctx, obj, vpcapi.VPCInterconnectOwner(obj), false); err != nil {
		return nil, err
	}

	return v.inner.ValidateCreate(ctx, obj) //nolint:wrapcheck
}

func (v *generatedValidator[T]) ValidateUpdate(ctx context.Context, oldObj, newObj T) (admission.Warnings, error) {
	// an owner added by hand to a user object has to exist, or it would be garbage collected
	if owner := vpcapi.VPCInterconnectOwner(oldObj); owner != "" {
		if err := v.check(ctx, oldObj, owner, true); err != nil {
			return nil, err
		}
	} else if err := v.check(ctx, newObj, vpcapi.VPCInterconnectOwner(newObj), false); err != nil {
		return nil, err
	}

	return v.inner.ValidateUpdate(ctx, oldObj, newObj) //nolint:wrapcheck
}

func (v *generatedValidator[T]) ValidateDelete(ctx context.Context, obj T) (admission.Warnings, error) {
	if err := v.check(ctx, obj, vpcapi.VPCInterconnectOwner(obj), true); err != nil {
		return nil, err
	}

	return v.inner.ValidateDelete(ctx, obj) //nolint:wrapcheck
}
