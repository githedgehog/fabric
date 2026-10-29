package cmls

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSwp(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{in: "E1/1", want: "swp1"},
		{in: "E1/32", want: "swp32"},
		{in: "E1/1/1", want: "swp1s0"},
		{in: "E1/1/4", want: "swp1s3"},
		{in: "E1/12/2", want: "swp12s1"},
		{in: "E1/abc", want: "port-invalid-uint"},
		{in: "E1/1/abc", want: "port-invalid-uint"},
		{in: "Ethernet0", want: "port-invalid-prefix"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, swp(tt.in))
		})
	}
}
