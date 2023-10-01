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
