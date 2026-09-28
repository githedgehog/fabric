// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"context"
	"fmt"
	"maps"
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

// VPCConnections returns the connections that put the VPC on a switch: those of its attachments,
// and static external connections within it. skipAttach and skipConn leave out the attachment or
// connection being admitted, whose stored version may differ.
func VPCConnections(ctx context.Context, kube kclient.Reader, namespace, vpcName, skipAttach, skipConn string) ([]string, error) {
	connNames := []string{}

	attaches := &VPCAttachmentList{}
	if err := kube.List(ctx, attaches, kclient.InNamespace(namespace), kclient.MatchingLabels{LabelVPC: vpcName}); err != nil {
		return nil, fmt.Errorf("failed to list vpc attachments: %w", err) // TODO replace with some internal error to not expose to the user
	}
	for _, attach := range attaches.Items {
		if attach.Name != skipAttach && attach.Spec.VPCName() == vpcName {
			connNames = append(connNames, attach.Spec.Connection)
		}
	}

	conns := &wiringapi.ConnectionList{}
	if err := kube.List(ctx, conns, kclient.InNamespace(namespace), kclient.MatchingLabels{wiringapi.LabelVPC: vpcName}); err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err) // TODO replace with some internal error to not expose to the user
	}
	for _, conn := range conns.Items {
		if conn.Name != skipConn && conn.Spec.StaticExternal != nil && conn.Spec.StaticExternal.WithinVPC == vpcName {
			connNames = append(connNames, conn.Name)
		}
	}

	return connNames, nil
}

// CheckCommonDomain checks the switches have a domain in common, and that pin is one of them if
// set. The set only shrinks as switches are added, so the check does not depend on the order
// attachments are admitted in.
func CheckCommonDomain(pin string, switches map[string]*wiringapi.Switch) error {
	var common []string
	if pin != "" {
		common = []string{pin}
	}

	for _, name := range slices.Sorted(maps.Keys(switches)) {
		domains := wiringapi.DomainsOrDefault(switches[name].Spec.Topology.Domains)
		if common == nil {
			common = slices.Clone(domains)

			continue
		}

		narrowed := slices.DeleteFunc(slices.Clone(common), func(domain string) bool {
			return !slices.Contains(domains, domain)
		})
		if len(narrowed) == 0 {
			if pin != "" {
				return fmt.Errorf("switch %s is in domains %v, not in pinned domain %s", name, domains, pin) //nolint:err113
			}

			return fmt.Errorf("switch %s is in domains %v, sharing none with the other attachments in %v", name, domains, common) //nolint:err113
		}
		common = narrowed
	}

	return nil
}
