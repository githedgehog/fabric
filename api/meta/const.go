// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package meta

const (
	AgentExporterPort = 7042
	AlloyUser         = "alloy"
	AgentUser         = "hhagent"
)

// BenchTouchLabel is set by benchmarks on objects to simulate user updates. Its value is a unix
// timestamp in nanoseconds as a decimal string. The Agent controller propagates the max value
// across the objects making up the Agent spec into AgentSpec.BenchTouch.
const BenchTouchLabel = "bench.githedgehog.com/touch"
