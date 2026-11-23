// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
	"net/url"
)

// InterfaceRates are averaged over LoadInterval seconds, utilization is a percentage of the link speed
type InterfaceRates struct {
	InBitsRate     Number `json:"in-bits-rate"`
	InPktsRate     Number `json:"in-pkts-rate"`
	InUtilization  Number `json:"in-utilization"`
	OutBitsRate    Number `json:"out-bits-rate"`
	OutPktsRate    Number `json:"out-pkts-rate"`
	OutUtilization Number `json:"out-utilization"`
	LoadInterval   Number `json:"load-interval"`
}

// InterfaceRates returns the rates of every interface that has them by name, all in one request
func (c *Client) InterfaceRates(ctx context.Context) (map[string]InterfaceRates, error) {
	ifaces := map[string]struct {
		Rates *InterfaceRates `json:"rates"`
	}{}
	if err := c.get(ctx, pathOf("interface"), url.Values{"view": {"rates"}}, &ifaces); err != nil {
		return nil, err
	}

	rates := map[string]InterfaceRates{}
	for name, iface := range ifaces {
		if iface.Rates != nil {
			rates[name] = *iface.Rates
		}
	}

	return rates, nil
}
