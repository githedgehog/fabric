//go:build !linux

package uefiutil

import "github.com/pkg/errors"

func MakeONIEDefaultBootEntryAndCleanup() error {
	return errors.New("uefi: not implemented")
}
