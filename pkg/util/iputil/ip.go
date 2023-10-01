package iputil

import (
	"net"

	cidrlib "github.com/apparentlymart/go-cidr/cidr"
	"github.com/pkg/errors"
)

type ParsedCIDR struct {
	IP         net.IP
	Subnet     net.IPNet
	Gateway    net.IP
	RangeStart net.IP
}

func ParseCIDR(cidr string) (*ParsedCIDR, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	return &ParsedCIDR{
		IP:         ip,
		Subnet:     *ipNet,
		Gateway:    cidrlib.Inc(ipNet.IP),
		RangeStart: cidrlib.Inc(cidrlib.Inc(ipNet.IP)),
	}, nil
}

func Range(cidr string) (string, string, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to parse cidr %s", cidr)
	}

	first, last := cidrlib.AddressRange(subnet)

	return first.String(), last.String(), nil
}
