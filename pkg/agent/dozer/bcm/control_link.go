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
	"net"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
)

const (
	managementPort = "M1"
)

type route struct {
	dst []*net.IPNet
	gw  net.IP
}

func (p *BroadcomProcessor) EnsureControlLink(_ context.Context, agent *agentapi.Agent) error {
	if agent == nil {
		return errors.New("no agent config")
	}

	_, controlVIP, err := net.ParseCIDR(agent.Spec.Config.ControlVIP) // it's ok as we're using /32
	if err != nil {
		return errors.Wrapf(err, "failed to parse control VIP %s", agent.Spec.Config.ControlVIP)
	}
	if controlVIP.Mask.String() != net.IPv4Mask(255, 255, 255, 255).String() {
		return errors.Errorf("control VIP %s is not a /32", agent.Spec.Config.ControlVIP)
	}

	exists := false
	dev := ""
	switchIP := ""
	controlIP := ""
	for _, spec := range agent.Spec.Connections {
		if spec.Management != nil {
			dev = spec.Management.Link.Switch.LocalPortName()
			if dev != managementPort {
				continue
			}

			switchIP = spec.Management.Link.Switch.IP
			controlIP = spec.Management.Link.Server.IP
			exists = true

			break
		}
	}

	// it's not a directly connected switch or front panel used
	if !exists || dev != managementPort {
		return nil
	}
	if dev == "" {
		return errors.New("no management interface found")
	}
	if switchIP == "" {
		return errors.New("no management IP found")
	}
	if controlIP == "" {
		return errors.New("no control IP found")
	}

	// it's temp till we can properly configure it through GNMI
	if dev == managementPort {
		dev = "eth0"
	}
	if dev != "eth0" {
		return errors.Errorf("unsupported management interface %s (only eth0 currently supported)", dev)
	}

	link, err := netlink.LinkByName(dev)
	if err != nil {
		return errors.Wrapf(err, "failed to get link %s", dev)
	}

	addr, err := netlink.ParseAddr(switchIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse ip %s", switchIP)
	}

	addrs, err := netlink.AddrList(link, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to get addresses for link %s", dev)
	}

	exists = false
	for _, a := range addrs {
		if a.Equal(*addr) {
			exists = true

			break
		}
	}

	if !exists {
		if err := netlink.AddrAdd(link, addr); err != nil {
			return errors.Wrapf(err, "failed to add address %s to link %s", switchIP, dev)
		}
	}

	existingRoutes, err := netlink.RouteList(link, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to get routes for link %s", dev)
	}

	routes := []route{
		{
			dst: []*net.IPNet{
				controlVIP,
			},
			gw: net.ParseIP(controlIP),
		},
	}

	for _, route := range routes {
		for _, dst := range route.dst {
			exists = false
			for _, existingRoute := range existingRoutes {
				if existingRoute.Dst == nil {
					continue
				}
				if existingRoute.Dst.IP.Equal(dst.IP) && existingRoute.Dst.Mask.String() == dst.Mask.String() {
					exists = true

					break
				}
			}
			if !exists {
				if err := netlink.RouteAdd(&netlink.Route{
					LinkIndex: link.Attrs().Index,
					Dst:       dst,
					Gw:        route.gw,
				}); err != nil {
					return errors.Wrapf(err, "failed to add route %s via %s to link %s", dst, route.gw, dev)
				}
			}
		}
	}

	return nil
}
