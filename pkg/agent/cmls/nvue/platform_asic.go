// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"maps"
	"slices"
)

// ASICResources are the hardware table usages of an ASIC
// TODO report them as the critical resources of the switch, see https://github.com/githedgehog/fabric/issues/1842
type ASICResources struct {
	// Global is by resource, such as "IPv4-Routes" or "ECMP-nexthops"
	Global map[string]ASICResource `json:"global"`
	// ACL is by table, such as "Ingress-ACL-ipv4-filter-table"
	ACL map[string]ASICACLTable `json:"acl"`
}

// ASICResource is the usage of a table, some such as the neighbors only have the used count
type ASICResource struct {
	Used          Number `json:"used"`
	Free          Number `json:"free"`
	Max           Number `json:"max"`
	HighWatermark Number `json:"high-watermark"`
}

type ASICACLTable struct {
	RuleCount Number `json:"rule-count"`
}

// ASICs returns the names of the ASICs of the switch, such as "ASIC1"
func (c *Client) ASICs(ctx context.Context) ([]string, error) {
	asics := map[string]struct{}{}
	if err := c.get(ctx, pathOf("platform", "asic"), nil, &asics); err != nil {
		return nil, err
	}

	return slices.Sorted(maps.Keys(asics)), nil
}

func (c *Client) ASICResources(ctx context.Context, asic string) (*ASICResources, error) {
	resources := &ASICResources{}
	if err := c.get(ctx, pathOf("platform", "asic", asic, "resource"), nil, resources); err != nil {
		return nil, err
	}

	return resources, nil
}
