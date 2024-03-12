// Copyright 2023 Hedgehog
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

//go:build e2e

package e2e

import (
	"context"

	gg "github.com/onsi/ginkgo/v2"
	g "github.com/onsi/gomega"
)

var _ = gg.Describe("Collapsed Core", func() {
	gg.Describe("VPCs", func() {
		gg.It("VPC should get VLAN assigned", func(ctx context.Context) {
			// TODO
			// could be a small benchmark creating 100 VPCs and expecting them to be ready fast
		})
		gg.It("With enabled DHCP server should get an IP", func(ctx context.Context) {
			// TODO
		})
		gg.It("Two servers in a single VPC should be able to access each other", func(ctx context.Context) {
			// TODO
		})
		gg.It("With enabled peering VPCs should be able to access each other", func(ctx context.Context) {
			testCollapsedCoreVPCPeering(ctx)
		})
		gg.It("WIth enabled SNAT server should be able to reach external network", func(ctx context.Context) {
			// TODO
		})
		gg.It("With enabled DNAT server should be reachable from external network", func(ctx context.Context) {
			// TODO
		})
		gg.It("With enabled DHCP/SNAT/DNAT everything should still work", gg.Label("aio"), func(ctx context.Context) {
			// TODO
		})
		gg.It("With peering removed VPCs shouldn't be able to access each other", func(ctx context.Context) {
			// TODO
		})
	})
})

func testCollapsedCoreVPCPeering(ctx context.Context) {
	c := h.CollapsedCore()

	//
	gg.By("Creating 2 VPCs with DHCP enabled and peering them")
	//

	// vpc1, err := h.Kube.VPCCreate(ctx, "vpc-1", vpcapi.VPCSpec{
	// 	Subnet: "10.90.0.1/24",
	// 	DHCP: vpcapi.VPCDHCP{
	// 		Enable: true,
	// 	},
	// })
	// g.Expect(err).ToNot(g.HaveOccurred(), "vpc-1 should be created")
	// g.Expect(vpc1).ToNot(g.BeNil(), "vpc-1 should not be nil")

	// vpc1attach, err := h.Kube.VPCAttach(ctx, vpc1, c.DualHomedServer1)
	// g.Expect(err).ToNot(g.HaveOccurred(), "vpc-1 should be attached to server-1")
	// g.Expect(vpc1attach).ToNot(g.BeNil(), "vpc-1 attach should not be nil")

	// vpc2, err := h.Kube.VPCCreate(context.TODO(), "vpc-2", vpcapi.VPCSpec{
	// 	Subnet: "10.90.0.2/24",
	// 	DHCP: vpcapi.VPCDHCP{
	// 		Enable: true,
	// 	},
	// })
	// g.Expect(err).ToNot(g.HaveOccurred(), "vpc-2 should be created")
	// g.Expect(vpc2).ToNot(g.BeNil(), "vpc-2 should not be nil")

	// vpc2attach, err := h.Kube.VPCAttach(ctx, vpc2, c.DualHomedServer2)
	// g.Expect(err).ToNot(g.HaveOccurred(), "vpc-2 should be attached to server-2")
	// g.Expect(vpc2attach).ToNot(g.BeNil(), "vpc-2 attach should not be nil")

	// peerVpc1Vpc2, err := h.Kube.VPCPeer(ctx, vpc1, vpc2)
	// g.Expect(err).ToNot(g.HaveOccurred(), "vpc-1 and vpc-2 should be peered")
	// g.Expect(peerVpc1Vpc2).ToNot(g.BeNil(), "vpc-1 and vpc-2 peer should not be nil")

	//
	gg.By("Waiting for VPCs to be ready")
	//

	// g.Expect(h.Kube.Wait(ctx, vpc1, vpc2, vpc1attach, vpc2attach, peerVpc1Vpc2)).
	// 	To(g.Succeed(), "vpc-1 and vpc-2 should be ready")

	//
	gg.By("Checking network connectivity")
	//

	server1ip, err := h.Server.NetworkSetup(ctx, c.DualHomedServer1)
	g.Expect(err).ToNot(g.HaveOccurred(), "server-1 network should be setup")
	g.Expect(server1ip).ToNot(g.BeEmpty(), "server-1 should have an ip")

	server2ip, err := h.Server.NetworkSetup(ctx, c.DualHomedServer2)
	g.Expect(err).ToNot(g.HaveOccurred(), "server-2 network should be setup")
	g.Expect(server2ip).ToNot(g.BeEmpty(), "server-2 should have an ip")

	g.Expect(h.Server.NetworkCheck(ctx, c.DualHomedServer1, server2ip)).
		To(g.Succeed(), "server-1 should be able to access server-2")

	g.Expect(h.Server.NetworkCheck(ctx, c.DualHomedServer2, server1ip)).
		To(g.Succeed(), "server-2 should be able to access server-1")
}
