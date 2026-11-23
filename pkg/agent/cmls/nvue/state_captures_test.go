// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package nvue

// stateCaptures complete the fixtures of the SN4600 to everything the Cumulus agent reads for the switch state, its
// tests in pkg/agent/cmls collect the whole state from them: the counters of every port that is up administratively
// and the routing of every VRF but the management one
var stateCaptures = map[string][]string{
	modelSN4600: {
		"/qos/roce",
		"/interface?view=rates",
		"/interface/swp1/counters",
		"/interface/swp2/counters",
		"/interface/swp3/counters",
		"/interface/swp17s1/counters",
		"/interface/swp17s2/counters",
		"/interface/swp17s3/counters",
		"/vrf/vpc-vpc-05/router/bgp/neighbor?view=detail",
		"/vrf/vpc-vpc-06/router/bgp/neighbor?view=detail",
		"/vrf/vpc-vpc-07/router/bgp/neighbor?view=detail",
		"/vrf/vpc-vpc-05/router/bfd/peers",
		"/vrf/vpc-vpc-06/router/bfd/peers",
		"/vrf/vpc-vpc-07/router/bfd/peers",
	},
}
