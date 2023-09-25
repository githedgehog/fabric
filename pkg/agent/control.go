package agent

import (
	"net"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
)

type route struct {
	Dst []*net.IPNet
	Gw  net.IP
}

func (svc *Service) ensureControlLink(agent *agentapi.Agent) error {
	if agent == nil {
		return errors.New("no agent config")
	}

	_, controlVIP, err := net.ParseCIDR(agent.Spec.ControlVIP) // it's ok as we're using /32
	if err != nil {
		return errors.Wrapf(err, "failed to parse control VIP %s", agent.Spec.ControlVIP)
	}
	if controlVIP.Mask.String() != net.IPv4Mask(255, 255, 255, 255).String() {
		return errors.Errorf("control VIP %s is not a /32", agent.Spec.ControlVIP)
	}

	exists := false
	dev := ""
	switchIP := ""
	controlIP := ""
	for _, conn := range agent.Spec.Connections {
		if conn.Spec.Management != nil {
			dev = conn.Spec.Management.Link.Switch.LocalPortName()
			switchIP = conn.Spec.Management.Link.Switch.IP
			controlIP = conn.Spec.Management.Link.Server.IP
			exists = true
			break
		}
	}

	if !exists {
		return errors.New("no management connection found")
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
	if dev == "Management0" {
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
			Dst: []*net.IPNet{
				controlVIP,
			},
			Gw: net.ParseIP(controlIP),
		},
	}

	for _, route := range routes {
		for _, dst := range route.Dst {
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
					Gw:        route.Gw,
				}); err != nil {
					return errors.Wrapf(err, "failed to add route %s via %s to link %s", dst, route.Gw, dev)
				}
			}
		}
	}

	return nil
}
