// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
)

const (
	sonicImagePrefix = "image-"
)

var (
	sonicVersionCurr  *semver.Version
	sonicVersion450   *semver.Version
	compatInitialized = false

	Compat_INSTALL_PROTOCOL_TYPE_ATTACHED_HOST      oc.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ATTACHED_HOST      //nolint:revive,stylecheck
	Compat_INSTALL_PROTOCOL_TYPE_BGP                oc.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP                //nolint:revive,stylecheck
	Compat_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED oc.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED //nolint:revive,stylecheck
	Compat_INSTALL_PROTOCOL_TYPE_STATIC             oc.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE = oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC             //nolint:revive,stylecheck
)

// Ugly temporary compatibility fix for sonic versions using singleton to avoid refactoring processor code
func initCompat() error {
	if compatInitialized {
		return nil
	}

	entries, err := os.ReadDir("/host")
	if err != nil {
		return fmt.Errorf("reading host dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		if !strings.HasPrefix(name, sonicImagePrefix) {
			continue
		}

		if sonicVersionCurr != nil {
			return fmt.Errorf("multiple sonic images found in /host: %q and %q", sonicVersionCurr, name) //nolint:err113
		}

		name = strings.Split(strings.TrimPrefix(name, sonicImagePrefix), "-")[0]
		sonicVersionCurr, err = semver.NewVersion(name)
		if err != nil {
			return fmt.Errorf("parsing sonic version %q: %w", name, err)
		}
		slog.Debug("Found sonic version", "version", sonicVersionCurr)
	}

	sonicVersion450, err = semver.NewVersion("4.5.0")
	if err != nil {
		return fmt.Errorf("parsing sonic version 4.5.0: %w", err)
	}

	if sonicVersionCurr.Compare(sonicVersion450) < 0 {
		Compat_INSTALL_PROTOCOL_TYPE_ATTACHED_HOST = 0
		Compat_INSTALL_PROTOCOL_TYPE_BGP = 1
		Compat_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED = 3
		Compat_INSTALL_PROTOCOL_TYPE_STATIC = 15
	}

	compatInitialized = true

	return nil
}
