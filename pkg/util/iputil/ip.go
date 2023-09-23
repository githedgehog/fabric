package iputil

import "net"

type ParsedCIDR struct {
	IP         net.IP
	Subnet     net.IP
	Mask       net.IPMask
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
		Subnet:     ipNet.IP,
		Mask:       ipNet.Mask,
		Gateway:    inc(ipNet.IP),
		RangeStart: inc(inc(ipNet.IP)),
	}, nil
}

// Taken from https://github.com/apparentlymart/go-cidr/blob/master/cidr/cidr.go (MIT license)
func inc(IP net.IP) net.IP {
	IP = checkIPv4(IP)
	incIP := make([]byte, len(IP))
	copy(incIP, IP)
	for j := len(incIP) - 1; j >= 0; j-- {
		incIP[j]++
		if incIP[j] > 0 {
			break
		}
	}
	return incIP
}

// Taken from https://github.com/apparentlymart/go-cidr/blob/master/cidr/cidr.go (MIT license)
func checkIPv4(ip net.IP) net.IP {
	// Go for some reason allocs IPv6len for IPv4 so we have to correct it
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}
