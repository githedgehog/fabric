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

package iputil

import (
	"net"

	"github.com/apparentlymart/go-cidr/cidr"
	cidrlib "github.com/apparentlymart/go-cidr/cidr"
	"github.com/pkg/errors"
)

type ParsedCIDR struct {
	IP             net.IP
	Subnet         net.IPNet
	Gateway        net.IP
	DHCPRangeStart net.IP
	DHCPRangeEnd   net.IP
}

func ParseCIDR(cidr string) (*ParsedCIDR, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	_, last, err := Range(cidr)
	if err != nil {
		return nil, err
	}

	return &ParsedCIDR{
		IP:             ip,
		Subnet:         *ipNet,
		Gateway:        cidrlib.Inc(ipNet.IP),
		DHCPRangeStart: cidrlib.Inc(cidrlib.Inc(ipNet.IP)),
		DHCPRangeEnd:   net.ParseIP(last),
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

func VerifyNoOverlap(subnets []*net.IPNet) error {
	anyNetCIDR := "0.0.0.0/0"
	_, anyNet, err := net.ParseCIDR(anyNetCIDR)
	if err != nil {
		return errors.Wrapf(err, "failed to parse cidr %s", anyNetCIDR)
	}

	return errors.Wrapf(cidr.VerifyNoOverlap(subnets, anyNet), "failed to verify no overlap subnets")
}
