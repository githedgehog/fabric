package meta

import (
	"reflect"
	"testing"
)

func TestNormalizedVLANRanges(t *testing.T) {
	tests := []struct {
		name    string
		in, out []VLANRange
		err     bool
	}{
		{
			name: "simple-empty",
			in:   []VLANRange{},
			out:  []VLANRange{},
		},
		{
			name: "simple-only-from",
			in: []VLANRange{
				{From: 1},
			},
			out: []VLANRange{
				{From: 1, To: 1},
			},
		},
		{
			name: "simple-all-vlans",
			in: []VLANRange{
				{From: 1, To: 4094},
			},
			out: []VLANRange{
				{From: 1, To: 4094},
			},
		},
		{
			name: "simple-unsorted",
			in: []VLANRange{
				{From: 201, To: 202},
				{From: 101, To: 102},
				{From: 301, To: 302},
			},
			out: []VLANRange{
				{From: 101, To: 102},
				{From: 201, To: 202},
				{From: 301, To: 302},
			},
		},
		{
			name: "simple-invalid-vlan-from",
			in: []VLANRange{
				{From: 0, To: 4094},
			},
			err: true,
		},
		{
			name: "simple-invalid-vlan-to",
			in: []VLANRange{
				{From: 0, To: 4094},
			},
			err: true,
		},
		{
			name: "simple-invalid-vlan-to-2",
			in: []VLANRange{
				{From: 1001, To: 1000},
			},
			err: true,
		},
		{
			name: "overlaps-1",
			in: []VLANRange{
				{From: 200, To: 300},
				{From: 200, To: 200},
				{From: 150, To: 300},
				{From: 150, To: 150},
				{From: 500, To: 500},
				{From: 500, To: 600},
				{From: 700, To: 700},
				{From: 100, To: 250},
				{From: 100, To: 100},
			},
			out: []VLANRange{
				{From: 100, To: 300},
				{From: 500, To: 600},
				{From: 700, To: 700},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizedVLANRanges(tt.in)
			if err != nil && !tt.err {
				t.Errorf("unexpected error: %v", err)
			}
			if err == nil && tt.err {
				t.Errorf("expected error, got nil")
			}

			if err == nil {
				if !reflect.DeepEqual(got, tt.out) {
					t.Errorf("got = %v, want %v", got, tt.out)
				}
			}
		})
	}
}

func TestCheckVLANRangesOverlap(t *testing.T) {
	tests := []struct {
		name string
		in   []VLANRange
		err  bool
	}{
		{
			name: "simple-empty",
			in:   []VLANRange{},
		},
		{
			name: "simple-single",
			in: []VLANRange{
				{From: 1, To: 1},
			},
		},
		{
			name: "simple-unsorted",
			in: []VLANRange{
				{From: 201, To: 202},
				{From: 101, To: 102},
				{From: 301, To: 302},
			},
		},
		{
			name: "overlap-sorted-1",
			in: []VLANRange{
				{From: 100, To: 200},
				{From: 150, To: 180},
			},
			err: true,
		},
		{
			name: "overlap-sorted-2",
			in: []VLANRange{
				{From: 100, To: 200},
				{From: 150, To: 250},
			},
			err: true,
		},
		{
			name: "overlap-sorted-2",
			in: []VLANRange{
				{From: 100, To: 200},
				{From: 200, To: 250},
			},
			err: true,
		},
		{
			name: "overlap-unsorted",
			in: []VLANRange{
				{From: 250, To: 300},
				{From: 200, To: 250},
			},
			err: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckVLANRangesOverlap(tt.in)
			if err != nil && !tt.err {
				t.Errorf("unexpected error: %v", err)
			}
			if err == nil && tt.err {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
