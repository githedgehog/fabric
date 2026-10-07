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
	"testing"

	"github.com/stretchr/testify/require"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	admissionv1 "k8s.io/api/admission/v1"
	authnv1 "k8s.io/api/authentication/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type nopPeeringValidator struct{}

func (nopPeeringValidator) ValidateCreate(_ context.Context, _ *vpcapi.ExternalPeering) (admission.Warnings, error) {
	return nil, nil
}

func (nopPeeringValidator) ValidateUpdate(_ context.Context, _, _ *vpcapi.ExternalPeering) (admission.Warnings, error) {
	return nil, nil
}

func (nopPeeringValidator) ValidateDelete(_ context.Context, _ *vpcapi.ExternalPeering) (admission.Warnings, error) {
	return nil, nil
}

func TestGeneratedValidator(t *testing.T) {
	const self = "system:serviceaccount:fab:fabric-ctrl"

	ic := &vpcapi.VPCInterconnect{ObjectMeta: kmetav1.ObjectMeta{Name: "ic-01", Namespace: kmetav1.NamespaceDefault}}
	deleting := &vpcapi.VPCInterconnect{ObjectMeta: kmetav1.ObjectMeta{
		Name: "ic-02", Namespace: kmetav1.NamespaceDefault, Finalizers: []string{testFinalizer},
	}}
	scheme := runtime.NewScheme()
	require.NoError(t, vpcapi.AddToScheme(scheme))
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ic, deleting).Build()
	require.NoError(t, kube.Delete(t.Context(), deleting))

	peering := func(owner string) *vpcapi.ExternalPeering {
		p := &vpcapi.ExternalPeering{ObjectMeta: kmetav1.ObjectMeta{Name: "p", Namespace: kmetav1.NamespaceDefault}}
		if owner != "" {
			p.OwnerReferences = []kmetav1.OwnerReference{{
				APIVersion: vpcapi.GroupVersion.String(), Kind: vpcapi.KindVPCInterconnect, Name: owner, Controller: pointer.To(true),
			}}
		}

		return p
	}
	v := newGeneratedValidator[*vpcapi.ExternalPeering](nopPeeringValidator{}, kube, unlockedLock())
	ctxFor := func(username string) context.Context {
		return admission.NewContextWithRequest(t.Context(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			UserInfo: authnv1.UserInfo{Username: username},
		}})
	}
	user, ctrl := ctxFor("system:admin"), ctxFor(self)

	for _, tt := range []struct {
		name     string
		op       string
		ctx      context.Context //nolint:containedctx
		old, obj *vpcapi.ExternalPeering
		err      bool
	}{
		{name: "user creates a user object", op: "create", ctx: user, obj: peering("")},
		{name: "user creates a generated object", op: "create", ctx: user, obj: peering("ic-01"), err: true},
		{name: "controller creates a generated object", op: "create", ctx: ctrl, obj: peering("ic-01")},
		{name: "user updates a generated object", op: "update", ctx: user, old: peering("ic-01"), obj: peering("ic-01"), err: true},
		{name: "user takes a generated object over", op: "update", ctx: user, old: peering("ic-01"), obj: peering(""), err: true},
		{name: "user adds an owner to a user object", op: "update", ctx: user, old: peering(""), obj: peering("ic-01"), err: true},
		{name: "controller updates a generated object", op: "update", ctx: ctrl, old: peering("ic-01"), obj: peering("ic-01")},
		{name: "user deletes a generated object", op: "delete", ctx: user, obj: peering("ic-01"), err: true},
		{name: "controller deletes a generated object", op: "delete", ctx: ctrl, obj: peering("ic-01")},
		{name: "garbage collector deletes one of a deleted VPC interconnect", op: "delete", ctx: user, obj: peering("ic-gone")},
		{name: "garbage collector deletes one of a VPC interconnect being deleted", op: "delete", ctx: user, obj: peering("ic-02")},
		{name: "garbage collector orphans one of a VPC interconnect being deleted", op: "update", ctx: user, old: peering("ic-02"), obj: peering("")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			switch tt.op {
			case "create":
				_, err = v.ValidateCreate(tt.ctx, tt.obj)
			case "update":
				_, err = v.ValidateUpdate(tt.ctx, tt.old, tt.obj)
			case "delete":
				_, err = v.ValidateDelete(tt.ctx, tt.obj)
			}
			if tt.err {
				require.ErrorContains(t, err, "generated for VPC interconnect")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
