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

//go:build linux

package dhcpd

import (
	"encoding/binary"
	"net"
	"reflect"
	"testing"

	"github.com/bits-and-blooms/bitset"
)

func TestNewIPv4Range(t *testing.T) {
	type args struct {
		start     net.IP
		end       net.IP
		gateway   net.IP
		count     uint32
		prefixLen uint32
	}
	tests := []struct {
		name    string
		args    args
		want    *ipv4range
		wantErr bool
	}{
		{
			// TODO: Add test cases.
			name: "Test to check invalid input start IP",
			args: args{
				start:     net.IP{'1', '1', '1', '1', '1'},
				end:       net.IP{'1', '1', '1', '1'},
				gateway:   net.IP{1, 1, 1, 1},
				count:     1,
				prefixLen: 33,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Test to check invalid input end IP",
			args: args{
				start:     net.IP{'1', '1', '1', '1'},
				end:       net.IP{'1', '1', '1', '1', '1'},
				gateway:   net.IP{1, 1, 1, 1},
				count:     1,
				prefixLen: 33,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Test to check invalid range",
			args: args{
				start:     net.IP{'1', '1', '1', '2'},
				end:       net.IP{'1', '1', '1', '1'},
				gateway:   net.IP{'1', '1', '1', '1'},
				count:     1,
				prefixLen: 32,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Test to check prefix Length",
			args: args{
				start:     net.IPv4(1, 1, 1, 2),
				end:       net.IPv4(1, 1, 1, 4),
				gateway:   net.IPv4(1, 1, 1, 1),
				count:     1,
				prefixLen: 33,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Test to check Invalid Count",
			args: args{
				start:     net.IP{'1', '1', '1', '2'},
				end:       net.IP{'1', '1', '1', '4'},
				gateway:   net.IP{'1', '1', '1', '1'},
				count:     1,
				prefixLen: 33,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Test if the range created is valid",
			args: args{
				start:     net.IPv4(1, 1, 1, 2),
				end:       net.IPv4(1, 1, 1, 4),
				gateway:   net.IPv4(1, 1, 1, 1),
				count:     3,
				prefixLen: 24,
			},
			want: &ipv4range{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   3,
				Mask:    net.CIDRMask(24, 32),
				bitmap:  bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1)),
			},
			wantErr: false,
		},
		{
			name: "Test if the range created where the number of ip's available does not match count",
			args: args{
				start:     net.IPv4(1, 1, 1, 2),
				end:       net.IPv4(1, 1, 1, 4),
				gateway:   net.IPv4(1, 1, 1, 1),
				count:     5,
				prefixLen: 24,
			},
			want:    nil,
			wantErr: true,
		},
		// {
		// 	name: "Test if gateway is specified in middle of range that ip is reserved at creation",
		// 	args: args{
		// 		start:     net.IPv4(1, 1, 1, 2),
		// 		end:       net.IPv4(1, 1, 1, 4),
		// 		gateway:   net.IPv4(1, 1, 1, 3),
		// 		count:     3,
		// 		prefixLen: 24,
		// 	},
		// 	want: &ipv4range{
		// 		Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
		// 		End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()),
		// 		gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 3).To4()),
		// 		bitmap:  bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1)),
		// 		Count:   3,
		// 		Mask:    net.CIDRMask(24, 32),
		// 	},
		// 	wantErr: false,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newIPv4Range(tt.args.start, tt.args.end, tt.args.gateway, tt.args.count, tt.args.prefixLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewIPv4Range() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewIPv4Range() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ipv4range_AllocateIP(t *testing.T) {
	type fields struct {
		Start   uint32
		End     uint32
		gateway uint32
		Mask    net.IPMask
		Count   uint32
		bitmap  *bitset.BitSet
	}
	type args struct {
		ip net.IPNet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    net.IPNet
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "Request IP outside range ",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   3,
				Mask:    net.CIDRMask(24, 32),
				bitmap:  bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1)),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 5), Mask: net.CIDRMask(24, 32)},
			},
			want:    net.IPNet{},
			wantErr: true,
		},
		{
			name: "Request IP already reserved ip gets the first available IP",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))
					ip := binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4())
					offset := ip - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4())
					bt.Set(uint(offset))

					return bt
				}(),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 4), Mask: net.CIDRMask(24, 32)},
			},
			want:    net.IPNet{IP: net.IPv4(1, 1, 1, 2).To4(), Mask: net.CIDRMask(24, 32)},
			wantErr: false,
		},
		{
			name: "Request IP that is not reserved ip gets the that IP",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))

					return bt
				}(),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 4), Mask: net.CIDRMask(24, 32)},
			},
			want:    net.IPNet{IP: net.IPv4(1, 1, 1, 4).To4(), Mask: net.CIDRMask(24, 32)},
			wantErr: false,
		},
		{
			name: "Request IP with no free IP's",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))
					ip := binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4())
					offset := ip - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4())
					bt.Set(uint(offset))
					bt.Set(0)
					bt.Set(1)
					bt.Set(3)

					return bt
				}(),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 4), Mask: net.CIDRMask(24, 32)},
			},
			want:    net.IPNet{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ipv4range{
				Start:   tt.fields.Start,
				End:     tt.fields.End,
				gateway: tt.fields.gateway,
				Mask:    tt.fields.Mask,
				Count:   tt.fields.Count,
				bitmap:  tt.fields.bitmap,
			}
			got, err := r.AllocateIP(tt.args.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ipv4range.AllocateIP() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ipv4range.AllocateIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ipv4range_Free(t *testing.T) {
	type fields struct {
		Start   uint32
		End     uint32
		gateway uint32
		Mask    net.IPMask
		Count   uint32
		bitmap  *bitset.BitSet
	}
	type args struct {
		ip net.IPNet
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Test free for ip outside managed range",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))
					ip := binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4())
					offset := ip - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4())
					bt.Set(uint(offset))

					return bt
				}(),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 7).To4(), Mask: net.CIDRMask(24, 32)},
			},
			wantErr: true,
		},
		{
			name: "try to free Allocated IP Address",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))
					ip := binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4())
					offset := ip - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4())
					bt.Set(uint(offset))

					return bt
				}(),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 4).To4(), Mask: net.CIDRMask(24, 32)},
			},
			wantErr: false,
		},
		{
			name: "try to free non-allocated IP Address",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))
					ip := binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4())
					offset := ip - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4())
					bt.Set(uint(offset))

					return bt
				}(),
			},
			args: args{
				ip: net.IPNet{IP: net.IPv4(1, 1, 1, 5).To4(), Mask: net.CIDRMask(24, 32)},
			},
			wantErr: true,
		},
		{
			name: "Test nil ip address",
			fields: fields{
				Start:   binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()),
				End:     binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 5).To4()),
				gateway: binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 1).To4()),
				Count:   4,
				Mask:    net.CIDRMask(24, 32),
				bitmap: func() *bitset.BitSet {
					// Reserve the bit in the bitmap
					bt := bitset.New(uint(binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4()) - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4()) + 1))
					ip := binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 4).To4())
					offset := ip - binary.BigEndian.Uint32(net.IPv4(1, 1, 1, 2).To4())
					bt.Set(uint(offset))

					return bt
				}(),
			},
			// args: args{
			// 	ip: net.IPNet{IP: net.IPv4(1, 1, 1, 5).To4(), Mask: net.CIDRMask(24, 32)},
			// },
			wantErr: true,
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ipv4range{
				Start:   tt.fields.Start,
				End:     tt.fields.End,
				gateway: tt.fields.gateway,
				Mask:    tt.fields.Mask,
				Count:   tt.fields.Count,
				bitmap:  tt.fields.bitmap,
			}
			if err := r.Free(tt.args.ip); (err != nil) != tt.wantErr {
				t.Errorf("ipv4range.Free() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
