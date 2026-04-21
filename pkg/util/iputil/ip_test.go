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
	"net/netip"
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

func mustParsePrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(err)
	}

	return p
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

func TestLastIPNetip(t *testing.T) {
	for _, tt := range []struct {
		name     string
		prefix   netip.Prefix
		expected netip.Addr
	}{
		{
			name:     "IPv4 /24",
			prefix:   mustParsePrefix("10.0.0.0/24"),
			expected: netip.MustParseAddr("10.0.0.255"),
		},
		{
			name:     "IPv4 /30",
			prefix:   mustParsePrefix("192.168.0.0/30"),
			expected: netip.MustParseAddr("192.168.0.3"),
		},
		{
			name:     "IPv4 /22",
			prefix:   mustParsePrefix("10.0.0.0/22"),
			expected: netip.MustParseAddr("10.0.3.255"),
		},
		{
			name:     "IPv4 /10",
			prefix:   mustParsePrefix("192.168.0.0/10"),
			expected: netip.MustParseAddr("192.191.255.255"),
		},
		{
			name:     "IPv4 /32 host route",
			prefix:   mustParsePrefix("10.0.0.1/32"),
			expected: netip.MustParseAddr("10.0.0.1"),
		},
		{
			name:     "IPv6 /32",
			prefix:   mustParsePrefix("2001:db8::/32"),
			expected: netip.MustParseAddr("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
		},
		{
			name:     "IPv6 /128 host route",
			prefix:   mustParsePrefix("2001:db8::1/128"),
			expected: netip.MustParseAddr("2001:db8::1"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := LastIPNetip(tt.prefix)
			if actual != tt.expected {
				t.Errorf("LastIPNetip(%v) = %v, want %v", tt.prefix, actual, tt.expected)
			}
		})
	}
}

func TestIsSubset(t *testing.T) {
	for _, tt := range []struct {
		name     string
		inner    netip.Prefix
		outer    netip.Prefix
		expected bool
	}{
		{
			name:     "exact match",
			inner:    mustParsePrefix("10.0.0.0/24"),
			outer:    mustParsePrefix("10.0.0.0/24"),
			expected: true,
		},
		{
			name:     "inner is narrower same base",
			inner:    mustParsePrefix("10.0.0.0/25"),
			outer:    mustParsePrefix("10.0.0.0/24"),
			expected: true,
		},
		{
			name:     "inner is second half of outer",
			inner:    mustParsePrefix("10.0.0.128/25"),
			outer:    mustParsePrefix("10.0.0.0/24"),
			expected: true,
		},
		{
			name:     "inner is a /24 inside a /16",
			inner:    mustParsePrefix("10.0.1.0/24"),
			outer:    mustParsePrefix("10.0.0.0/16"),
			expected: true,
		},
		{
			name:     "inner wider than outer",
			inner:    mustParsePrefix("10.0.0.0/23"),
			outer:    mustParsePrefix("10.0.0.0/24"),
			expected: false,
		},
		{
			name:     "inner not in outer",
			inner:    mustParsePrefix("10.1.0.0/24"),
			outer:    mustParsePrefix("10.0.0.0/24"),
			expected: false,
		},
		{
			name:     "different address families",
			inner:    mustParsePrefix("10.0.0.0/24"),
			outer:    mustParsePrefix("2001:db8::/32"),
			expected: false,
		},
		{
			name:     "inner network addr inside outer but prefix straddles boundary",
			inner:    mustParsePrefix("10.0.0.0/23"),
			outer:    mustParsePrefix("10.0.0.0/24"),
			expected: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual := IsSubset(tt.inner, tt.outer)
			if actual != tt.expected {
				t.Errorf("IsProperSubset(%v, %v) = %v, want %v", tt.inner, tt.outer, actual, tt.expected)
			}
		})
	}
}

func TestVerifyNoOverlapNetip(t *testing.T) {
	for _, tt := range []struct {
		name    string
		subnets []netip.Prefix
		wantErr bool
	}{
		{
			name:    "empty list",
			subnets: nil,
			wantErr: false,
		},
		{
			name:    "single subnet",
			subnets: []netip.Prefix{mustParsePrefix("10.0.0.0/24")},
			wantErr: false,
		},
		{
			name: "non-overlapping adjacent subnets",
			subnets: []netip.Prefix{
				mustParsePrefix("10.0.0.0/24"),
				mustParsePrefix("10.0.1.0/24"),
				mustParsePrefix("10.0.2.0/24"),
			},
			wantErr: false,
		},
		{
			name: "identical subnets",
			subnets: []netip.Prefix{
				mustParsePrefix("10.0.0.0/24"),
				mustParsePrefix("10.0.0.0/24"),
			},
			wantErr: true,
		},
		{
			name: "one subnet contains the other",
			subnets: []netip.Prefix{
				mustParsePrefix("10.0.0.0/16"),
				mustParsePrefix("10.0.1.0/24"),
			},
			wantErr: true,
		},
		{
			name: "partially overlapping subnets",
			subnets: []netip.Prefix{
				mustParsePrefix("10.0.0.0/23"),
				mustParsePrefix("10.0.1.0/24"),
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyNoOverlapNetip(tt.subnets)
			if tt.wantErr && err == nil {
				t.Error("VerifyNoOverlapNetip: expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("VerifyNoOverlapNetip: unexpected error: %v", err)
			}
		})
	}
}
