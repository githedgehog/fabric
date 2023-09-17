package agent

import (
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	agentapi "go.githedgehog.com/fabric/api/agent/v1alpha2"
	"sigs.k8s.io/yaml"
)

const (
	DEFAULT_BASEDIR = "/etc/sonic/hedgehog/"
	CONF_FILE       = "agent-config.yaml"
)

type Service struct {
	Basedir string

	Config *agentapi.Agent
}

func (s *Service) Run() error {
	if s.Basedir == "" {
		s.Basedir = DEFAULT_BASEDIR
	}

	// load config file
	// ensure control link
	// apply config from file

	// loop
	// check we can access k8s api
	// get config from k8s api
	// apply config from k8s api
	// save it to the file
	// update last applied status
	// TODO save last known good config for rollbacks

	if err := s.loadConfig(); err != nil {
		return errors.Wrap(err, "failed to load config")
	}

	if err := s.ensureControlLink(); err != nil {
		return errors.Wrap(err, "failed to ensure control link")
	}

	return nil
}

func (s *Service) configPath() string {
	return filepath.Join(s.Basedir, CONF_FILE)
}

func (s *Service) loadConfig() error {
	data, err := os.ReadFile(s.configPath())
	if err != nil {
		return errors.Wrapf(err, "failed to read config file %s", s.configPath())
	}

	config := &agentapi.Agent{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal config file %s", s.configPath())
	}

	s.Config = config

	return nil
}

func (s *Service) saveConfig() error {
	if s.Config == nil {
		return errors.New("no config found")
	}

	data, err := yaml.Marshal(s.Config)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal config")
	}

	err = os.WriteFile(s.configPath(), data, 0o640)
	if err != nil {
		return errors.Wrapf(err, "failed to write config file %s", s.configPath())
	}

	return nil
}

type route struct {
	Dst []*net.IPNet
	Gw  net.IP
}

func (s *Service) ensureControlLink() error {
	if s.Config == nil {
		return errors.New("no config found")
	}

	_, controlVIP, err := net.ParseCIDR(s.Config.Spec.ControlVIP)
	if err != nil {
		return errors.Wrapf(err, "failed to parse control VIP %s", s.Config.Spec.ControlVIP)
	}
	if controlVIP.Mask.String() != net.IPv4Mask(255, 255, 255, 255).String() {
		return errors.Errorf("control VIP %s is not a /32", s.Config.Spec.ControlVIP)
	}

	exists := false
	dev := ""
	switchIP := ""
	controlIP := ""
	for _, conn := range s.Config.Spec.Connections {
		if conn.Management != nil {
			dev = conn.Management.Link.Switch.LocalPortName()
			switchIP = conn.Management.Link.Switch.IP
			controlIP = conn.Management.Link.Server.IP
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

	// temp till we can properly configure it through GNMI
	if dev == "Management0" {
		dev = "eth0"
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
