// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package bcm

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	sonicImagePrefix = "image-"
)

var (
	sonicVersionCurr  *semver.Version
	sonicVersion450   *semver.Version
	compatInitialized = false
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
			slog.Warn("Failed to parse sonic version, using v0.0.0 as a fallback", "version", name)

			sonicVersionCurr, err = semver.NewVersion("0.0.0")
			if err != nil {
				return fmt.Errorf("parsing sonic version 0.0.0: %w", err)
			}
		}
		slog.Debug("Found sonic version", "version", sonicVersionCurr)
	}

	sonicVersion450, err = semver.NewVersion("4.5.0")
	if err != nil {
		return fmt.Errorf("parsing sonic version 4.5.0: %w", err)
	}

	compatInitialized = true

	return nil
}
