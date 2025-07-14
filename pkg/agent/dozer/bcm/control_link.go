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

package bcm

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/vishvananda/netlink"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
)

const (
	mgmtPort = "eth0"
)

func (p *BroadcomProcessor) EnsureControlLink(_ context.Context, agent *agentapi.Agent) error {
	if agent == nil {
		return fmt.Errorf("no agent config") //nolint:err113
	}

	controlVIP, err := netip.ParsePrefix(agent.Spec.Config.ControlVIP)
	if err != nil {
		return fmt.Errorf("parsing control VIP %s: %w", agent.Spec.Config.ControlVIP, err)
	}
	if controlVIP.Bits() != 32 {
		return fmt.Errorf("control VIP %s is not a /32", agent.Spec.Config.ControlVIP) //nolint:err113
	}

	switchIP, err := netip.ParsePrefix(agent.Spec.Switch.IP)
	if err != nil {
		return fmt.Errorf("parsing switch IP %s: %w", agent.Spec.Switch.IP, err)
	}

	if !switchIP.Contains(controlVIP.Addr()) {
		return fmt.Errorf("control VIP %s is not in switch IP subnet %s", controlVIP, switchIP) //nolint:err113
	}

	link, err := netlink.LinkByName(mgmtPort)
	if err != nil {
		return fmt.Errorf("getting link %s: %w", mgmtPort, err)
	}

	addr, err := netlink.ParseAddr(agent.Spec.Switch.IP)
	if err != nil {
		return fmt.Errorf("parsing switch IP %s: %w", agent.Spec.Switch.IP, err)
	}

	addrs, err := netlink.AddrList(link, 0)
	if err != nil {
		return fmt.Errorf("getting addresses for link %s: %w", mgmtPort, err)
	}

	exists := false
	for _, a := range addrs {
		if a.Equal(*addr) {
			exists = true

			break
		}
	}

	if !exists {
		if err := netlink.AddrAdd(link, addr); err != nil {
			return fmt.Errorf("adding address %s to link %s: %w", switchIP, mgmtPort, err)
		}
	}

	return nil
}
