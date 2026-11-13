// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	fcintapi "go.githedgehog.com/fabric/api/fcint/v1alpha1"
	"go.githedgehog.com/fabric/api/meta"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckUpgrade runs the initialization of this version against kube, without a FabricController, and returns the
// stored objects that would fail it. It writes to kube as the initialization does, so it's meant for a fake client
// loaded with the objects, e.g. from a support dump or listed from a cluster before upgrading it. Only invalid objects
// are found, rejected and stale ones need the API server.
func CheckUpgrade(ctx context.Context, kube kclient.Client, cfg *meta.FabricConfig) ([]fcintapi.FabricControllerProblem, error) {
	if cfg == nil {
		return nil, fmt.Errorf("fabric config is nil") //nolint:err113
	}

	profiles := switchprofile.NewDefaultSwitchProfiles()
	if err := profiles.RegisterAll(ctx, kube, cfg); err != nil {
		return nil, fmt.Errorf("registering default switch profiles: %w", err)
	}
	if err := profiles.Enforce(ctx, kube, cfg, false); err != nil {
		return nil, fmt.Errorf("enforcing switch profiles: %w", err)
	}

	i := &FabricControllerInitializer{Client: kube, apiReader: kube, cfg: cfg, profiles: profiles}
	if err := i.ensureFabricForUpgrade(ctx); err != nil {
		return nil, err
	}
	if err := i.deleteVPCLoopbackConnections(ctx); err != nil {
		return nil, err
	}

	// without a cache to catch up with, a single pass finds what the refresh passes do
	problems := []fcintapi.FabricControllerProblem{}
	for _, k := range refreshKinds {
		list := k.newList()
		if err := kube.List(ctx, list); err != nil {
			return nil, fmt.Errorf("listing %s: %w", k.kind, err)
		}

		if err := kmeta.EachListItem(list, func(item runtime.Object) error {
			obj, ok := item.(defaultable)
			if !ok {
				return fmt.Errorf("%s %T can't be defaulted", k.kind, item) //nolint:err113
			}
			if obj.GetDeletionTimestamp() != nil {
				return nil
			}

			obj.Default()
			if err := validateLocal(ctx, obj, cfg); err != nil {
				problems = append(problems, fcintapi.FabricControllerProblem{
					Kind: k.kind, Namespace: obj.GetNamespace(), Name: obj.GetName(),
					Reason: fcintapi.ProblemInvalid, Message: err.Error(),
				})
			}

			return nil
		}); err != nil {
			return nil, fmt.Errorf("checking %s: %w", k.kind, err)
		}
	}
	sortProblems(problems)

	return problems, nil
}
