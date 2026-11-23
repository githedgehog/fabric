// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

import (
	"context"
)

// InterfaceCounters are the hardware counters of a port, not served by the interface listing
type InterfaceCounters struct {
	InBytes         Counter `json:"in-bytes"`
	InPkts          Counter `json:"in-pkts"`
	InUnicastPkts   Counter `json:"in-unicast-pkts"`
	InMulticastPkts Counter `json:"in-multicast-pkts"`
	InBroadcastPkts Counter `json:"in-broadcast-pkts"`
	InDrops         Counter `json:"in-drops"`
	InErrors        Counter `json:"in-errors"`

	OutBytes         Counter `json:"out-bytes"`
	OutPkts          Counter `json:"out-pkts"`
	OutUnicastPkts   Counter `json:"out-unicast-pkts"`
	OutMulticastPkts Counter `json:"out-multicast-pkts"`
	OutBroadcastPkts Counter `json:"out-broadcast-pkts"`
	OutDrops         Counter `json:"out-drops"`
	OutErrors        Counter `json:"out-errors"`

	QoS *InterfaceQoSCounters `json:"qos"`
}

type InterfaceQoSCounters struct {
	// EgressQueueStats is per traffic class
	EgressQueueStats map[string]EgressQueueStats `json:"egress-queue-stats"`
}

type EgressQueueStats struct {
	TxBytes            Counter `json:"tx-bytes"`
	TxFrames           Counter `json:"tx-frames"`
	TxUCBufferDiscards Counter `json:"tx-uc-buffer-discards"`
	WREDDiscards       Counter `json:"wred-discards"`
	ECNMarkedFrames    Counter `json:"ecn-marked-frames"`
}

// InterfaceCounters returns the hardware counters of a port, they're ErrNotFound for the other interfaces such as
// the management port or bonds
func (c *Client) InterfaceCounters(ctx context.Context, name string) (*InterfaceCounters, error) {
	counters := &InterfaceCounters{}
	if err := c.get(ctx, pathOf("interface", name, "counters"), nil, counters); err != nil {
		return nil, err
	}

	return counters, nil
}
