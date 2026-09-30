// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"context"
	"fmt"
	"slices"

	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ConnectionSwitches returns the switches of the named connections by name. Connections and
// switches that don't exist are skipped, as they are refused on their own admission
func ConnectionSwitches(ctx context.Context, kube kclient.Reader, namespace string, connNames []string) (map[string]*wiringapi.Switch, error) {
	switches := map[string]*wiringapi.Switch{}
	for _, connName := range slices.Compact(slices.Sorted(slices.Values(connNames))) {
		conn := &wiringapi.Connection{}
		if err := kube.Get(ctx, ktypes.NamespacedName{Name: connName, Namespace: namespace}, conn); err != nil {
			if kapierrors.IsNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("failed to get connection %s: %w", connName, err) // TODO replace with some internal error to not expose to the user
		}
		switchNames, _, _, _, err := conn.Spec.Endpoints()
		if err != nil {
			return nil, fmt.Errorf("failed to get endpoints for connection %s: %w", connName, err)
		}

		for _, switchName := range switchNames {
			if _, exists := switches[switchName]; exists {
				continue
			}
			sw := &wiringapi.Switch{}
			if err := kube.Get(ctx, ktypes.NamespacedName{Name: switchName, Namespace: namespace}, sw); err != nil {
				if kapierrors.IsNotFound(err) {
					continue
				}

				return nil, fmt.Errorf("failed to get switch %s: %w", switchName, err) // TODO replace with some internal error to not expose to the user
			}
			switches[switchName] = sw
		}
	}

	return switches, nil
}
