/*
Copyright 2023 Hedgehog.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha2

import "testing"

func TestSwitchLocation_GenerateUUID(t *testing.T) {
	type fields struct {
		Location string
		Aisle    string
		Row      string
		Rack     string
		Slot     string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
		want1  string
	}{
		{
			name: "example",
			fields: fields{
				Location: "DC1 Florida",
				Aisle:    "1",
				Row:      "2",
				Rack:     "12",
				Slot:     "1.5",
			},
			want:  "9ddd88a9-cbad-56f4-b2ef-055615ec8f07",
			want1: "hhloc:DC1+Florida?aisle=1&row=2&rack=12&slot=1.5",
		},
		{
			name: "testing URI escaping",
			fields: fields{
				Location: "DC1 Florida",
				Aisle:    "1&1",
				Row:      "2=4",
				Rack:     "1 2",
				Slot:     "1.5",
			},
			want:  "2936d106-5b9c-50ed-8fe5-6bbe853a7e66",
			want1: "hhloc:DC1+Florida?aisle=1%261&row=2%3D4&rack=1+2&slot=1.5",
		},
		{
			name: "not all fields are set",
			fields: fields{
				Row:  "1",
				Slot: "12",
			},
			want:  "e9352dfd-b5fc-57f8-b75c-1f227fc297f8",
			want1: "hhloc:location?row=1&slot=12",
		},
		{
			name: "empty",
			fields: fields{
				Location: "",
				Aisle:    "",
				Row:      "",
				Rack:     "",
				Slot:     "",
			},
			want:  "",
			want1: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &SwitchLocation{
				Location: tt.fields.Location,
				Aisle:    tt.fields.Aisle,
				Row:      tt.fields.Row,
				Rack:     tt.fields.Rack,
				Slot:     tt.fields.Slot,
			}
			got, got1 := l.GenerateUUID()
			if got != tt.want {
				t.Errorf("SwitchLocation.GenerateUUID() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("SwitchLocation.GenerateUUID() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
