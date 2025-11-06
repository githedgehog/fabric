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
	"testing"
)

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		err  bool
		want *ParsedCIDR
	}{
		{
			name: "subnet",
			arg:  "192.168.1.0/24",
			err:  false,
			want: &ParsedCIDR{
				Gateway:        net.ParseIP("192.168.1.1"),
				DHCPRangeStart: net.ParseIP("192.168.1.2"),
				DHCPRangeEnd:   net.ParseIP("192.168.1.255"),
			},
		},
		{
			name: "gateway",
			arg:  "192.168.1.1/24",
			err:  false,
			want: &ParsedCIDR{
				Gateway:        net.ParseIP("192.168.1.1"),
				DHCPRangeStart: net.ParseIP("192.168.1.2"),
				DHCPRangeEnd:   net.ParseIP("192.168.1.255"),
			},
		},
		{
			name: "first-ip",
			arg:  "192.168.1.2/24",
			err:  false,
			want: &ParsedCIDR{
				Gateway:        net.ParseIP("192.168.1.1"),
				DHCPRangeStart: net.ParseIP("192.168.1.2"),
				DHCPRangeEnd:   net.ParseIP("192.168.1.255"),
			},
		},
		{
			name: "some-ip",
			arg:  "192.168.1.3/24",
			err:  false,
			want: &ParsedCIDR{
				Gateway:        net.ParseIP("192.168.1.1"),
				DHCPRangeStart: net.ParseIP("192.168.1.2"),
				DHCPRangeEnd:   net.ParseIP("192.168.1.255"),
			},
		},
		{
			name: "small-mask",
			arg:  "192.168.1.100/27",
			err:  false,
			want: &ParsedCIDR{
				Gateway:        net.ParseIP("192.168.1.97"),
				DHCPRangeStart: net.ParseIP("192.168.1.98"),
				DHCPRangeEnd:   net.ParseIP("192.168.1.127"),
			},
		},
		{
			name: "no-mask-1",
			arg:  "192.168.1.2",
			err:  true,
		},
		{
			name: "no-mask-2",
			arg:  "192.168.1.2/",
			err:  true,
		},
		{
			name: "wrong-ip-1",
			arg:  "192.168.1.256/24",
			err:  true,
		},
		{
			name: "wrong-ip-2",
			arg:  "192.168.1/24",
			err:  true,
		},
		{
			name: "wrong-mask-1",
			arg:  "192.168.1/33",
			err:  true,
		},
		{
			name: "wrong-mask-2",
			arg:  "192.168.1/0",
			err:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cidr, err := ParseCIDR(tt.arg)

			if tt.err && err == nil {
				t.Errorf("ParseCIDR(%s) should have returned an error", tt.arg)
			}
			if !tt.err && err != nil {
				t.Errorf("ParseCIDR(%s) returned an error: %v", tt.arg, err)
			}

			if cidr == nil && tt.want != nil {
				t.Errorf("ParseCIDR(%s) returned nil, expected %v", tt.arg, tt.want)
			}
			if cidr != nil && tt.want == nil {
				t.Errorf("ParseCIDR(%s) returned %v, expected nil", tt.arg, cidr)
			}

			if tt.want != nil {
				if cidr.Gateway.String() != tt.want.Gateway.String() {
					t.Errorf("ParseCIDR(%s) returned gateway %q, expected %q", tt.arg, cidr.Gateway.String(), tt.want.Gateway.String())
				}
				if cidr.DHCPRangeStart.String() != tt.want.DHCPRangeStart.String() {
					t.Errorf("ParseCIDR(%s) returned range start %q, expected %q", tt.arg, cidr.DHCPRangeStart.String(), tt.want.DHCPRangeStart.String())
				}
				if cidr.DHCPRangeEnd.String() != tt.want.DHCPRangeEnd.String() {
					t.Errorf("ParseCIDR(%s) returned range end %q, expected %q", tt.arg, cidr.DHCPRangeEnd.String(), tt.want.DHCPRangeEnd.String())
				}
			}
		})
	}
}

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}

	return ipNet
}

func TestLastIP(t *testing.T) {
	for _, tt := range []struct {
		name     string
		ipNet    *net.IPNet
		expected net.IP
	}{
		{
			name:     "IPv4 1",
			ipNet:    mustParseCIDR("192.168.0.0/24"),
			expected: net.IPv4(192, 168, 0, 255),
		},
		{
			name:     "IPv4 2",
			ipNet:    mustParseCIDR("192.168.0.0/30"),
			expected: net.IPv4(192, 168, 0, 3),
		},
		{
			name:     "IPv4 3",
			ipNet:    mustParseCIDR("192.168.0.0/10"),
			expected: net.IPv4(192, 191, 255, 255),
		},
		{
			name:     "IPv6",
			ipNet:    mustParseCIDR("2001:db8::/32"),
			expected: net.ParseIP("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
		},
	} {
		actual := LastIP(tt.ipNet)
		if !actual.IP.Equal(tt.expected) {
			t.Errorf("LastIP(%v) = %v, want %v", tt.ipNet, actual, tt.expected)
		}
	}
}
