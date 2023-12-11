//go:build !linux
// +build !linux

package dhcpd

import "context"

func (d *Service) runCoreDHCP(ctx context.Context) error {
	panic("unimplemented")
}
