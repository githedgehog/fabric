package funcs

import (
	"context"
	"math/big"
	stdnet "net"
	"net/netip"

	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/hairyhenderson/gomplate/v3/conv"
	"github.com/hairyhenderson/gomplate/v3/net"
	"github.com/pkg/errors"
	"inet.af/netaddr"
)

// NetNS - the net namespace
// Deprecated: don't use
func NetNS() *NetFuncs {
	return &NetFuncs{}
}

// AddNetFuncs -
// Deprecated: use CreateNetFuncs instead
func AddNetFuncs(f map[string]interface{}) {
	for k, v := range CreateNetFuncs(context.Background()) {
		f[k] = v
	}
}

// CreateNetFuncs -
func CreateNetFuncs(ctx context.Context) map[string]interface{} {
	ns := &NetFuncs{ctx}
	return map[string]interface{}{
		"net": func() interface{} { return ns },
	}
}

// NetFuncs -
type NetFuncs struct {
	ctx context.Context
}

// LookupIP -
func (f NetFuncs) LookupIP(name interface{}) (string, error) {
	return net.LookupIP(conv.ToString(name))
}

// LookupIPs -
func (f NetFuncs) LookupIPs(name interface{}) ([]string, error) {
	return net.LookupIPs(conv.ToString(name))
}

// LookupCNAME -
func (f NetFuncs) LookupCNAME(name interface{}) (string, error) {
	return net.LookupCNAME(conv.ToString(name))
}

// LookupSRV -
func (f NetFuncs) LookupSRV(name interface{}) (*stdnet.SRV, error) {
	return net.LookupSRV(conv.ToString(name))
}

// LookupSRVs -
func (f NetFuncs) LookupSRVs(name interface{}) ([]*stdnet.SRV, error) {
	return net.LookupSRVs(conv.ToString(name))
}

// LookupTXT -
func (f NetFuncs) LookupTXT(name interface{}) ([]string, error) {
	return net.LookupTXT(conv.ToString(name))
}

// ParseIP -
func (f NetFuncs) ParseIP(ip interface{}) (netaddr.IP, error) {
	return netaddr.ParseIP(conv.ToString(ip))
}

// ParseIPPrefix -
func (f NetFuncs) ParseIPPrefix(ipprefix interface{}) (netaddr.IPPrefix, error) {
	return netaddr.ParseIPPrefix(conv.ToString(ipprefix))
}

// ParseIPRange -
func (f NetFuncs) ParseIPRange(iprange interface{}) (netaddr.IPRange, error) {
	return netaddr.ParseIPRange(conv.ToString(iprange))
}

func (f NetFuncs) parseStdnetIPNet(prefix interface{}) (*stdnet.IPNet, error) {
	switch p := prefix.(type) {
	case *stdnet.IPNet:
		return p, nil
	case netaddr.IPPrefix:
		return p.Masked().IPNet(), nil
	case netip.Prefix:
		net := &stdnet.IPNet{
			IP:   p.Masked().Addr().AsSlice(),
			Mask: stdnet.CIDRMask(p.Bits(), p.Addr().BitLen()),
		}
		return net, nil
	default:
		_, network, err := stdnet.ParseCIDR(conv.ToString(prefix))
		return network, err
	}
}

// TODO: look at using this instead of parseStdnetIPNet
//
//nolint:unused
func (f NetFuncs) parseNetipPrefix(prefix interface{}) (netip.Prefix, error) {
	switch p := prefix.(type) {
	case *stdnet.IPNet:
		return f.ipPrefixFromIPNet(p), nil
	case netaddr.IPPrefix:
		return f.ipPrefixFromIPNet(p.Masked().IPNet()), nil
	case netip.Prefix:
		return p, nil
	default:
		return netip.ParsePrefix(conv.ToString(prefix))
	}
}

func (f NetFuncs) ipFromNetIP(n stdnet.IP) netip.Addr {
	ip, _ := netip.AddrFromSlice(n)
	return ip
}

func (f NetFuncs) ipPrefixFromIPNet(n *stdnet.IPNet) netip.Prefix {
	ip, _ := netip.AddrFromSlice(n.IP)
	ones, _ := n.Mask.Size()
	return netip.PrefixFrom(ip, ones)
}

// CIDRHost -
// Experimental!
func (f NetFuncs) CIDRHost(hostnum interface{}, prefix interface{}) (netip.Addr, error) {
	if err := checkExperimental(f.ctx); err != nil {
		return netip.Addr{}, err
	}

	network, err := f.parseStdnetIPNet(prefix)
	if err != nil {
		return netip.Addr{}, err
	}

	ip, err := cidr.HostBig(network, big.NewInt(conv.ToInt64(hostnum)))

	return f.ipFromNetIP(ip), err
}

// CIDRNetmask -
// Experimental!
func (f NetFuncs) CIDRNetmask(prefix interface{}) (netip.Addr, error) {
	if err := checkExperimental(f.ctx); err != nil {
		return netip.Addr{}, err
	}

	network, err := f.parseStdnetIPNet(prefix)
	if err != nil {
		return netip.Addr{}, err
	}

	netmask := stdnet.IP(network.Mask)
	return f.ipFromNetIP(netmask), nil
}

// CIDRSubnets -
// Experimental!
func (f NetFuncs) CIDRSubnets(newbits interface{}, prefix interface{}) ([]netip.Prefix, error) {
	if err := checkExperimental(f.ctx); err != nil {
		return nil, err
	}

	network, err := f.parseStdnetIPNet(prefix)
	if err != nil {
		return nil, err
	}

	nBits := conv.ToInt(newbits)
	if nBits < 1 {
		return nil, errors.Errorf("must extend prefix by at least one bit")
	}

	maxNetNum := int64(1 << uint64(nBits))
	retValues := make([]netip.Prefix, maxNetNum)
	for i := int64(0); i < maxNetNum; i++ {
		subnet, err := cidr.SubnetBig(network, nBits, big.NewInt(i))
		if err != nil {
			return nil, err
		}
		retValues[i] = f.ipPrefixFromIPNet(subnet)
	}

	return retValues, nil
}

// CIDRSubnetSizes -
// Experimental!
func (f NetFuncs) CIDRSubnetSizes(args ...interface{}) ([]netip.Prefix, error) {
	if err := checkExperimental(f.ctx); err != nil {
		return nil, err
	}

	if len(args) < 2 {
		return nil, errors.Errorf("wrong number of args: want 2 or more, got %d", len(args))
	}

	network, err := f.parseStdnetIPNet(args[len(args)-1])
	if err != nil {
		return nil, err
	}
	newbits := conv.ToInts(args[:len(args)-1]...)

	startPrefixLen, _ := network.Mask.Size()
	firstLength := newbits[0]

	firstLength += startPrefixLen
	retValues := make([]netip.Prefix, len(newbits))

	current, _ := cidr.PreviousSubnet(network, firstLength)

	for i, length := range newbits {
		if length < 1 {
			return nil, errors.Errorf("must extend prefix by at least one bit")
		}
		// For portability with 32-bit systems where the subnet number
		// will be a 32-bit int, we only allow extension of 32 bits in
		// one call even if we're running on a 64-bit machine.
		// (Of course, this is significant only for IPv6.)
		if length > 32 {
			return nil, errors.Errorf("may not extend prefix by more than 32 bits")
		}

		length += startPrefixLen
		if length > (len(network.IP) * 8) {
			protocol := "IP"
			switch len(network.IP) {
			case stdnet.IPv4len:
				protocol = "IPv4"
			case stdnet.IPv6len:
				protocol = "IPv6"
			}
			return nil, errors.Errorf("would extend prefix to %d bits, which is too long for an %s address", length, protocol)
		}

		next, rollover := cidr.NextSubnet(current, length)
		if rollover || !network.Contains(next.IP) {
			// If we run out of suffix bits in the base CIDR prefix then
			// NextSubnet will start incrementing the prefix bits, which
			// we don't allow because it would then allocate addresses
			// outside of the caller's given prefix.
			return nil, errors.Errorf("not enough remaining address space for a subnet with a prefix of %d bits after %s", length, current.String())
		}
		current = next
		retValues[i] = f.ipPrefixFromIPNet(current)
	}

	return retValues, nil
}
