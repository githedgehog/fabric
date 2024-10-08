// Copyright 2024 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package boot

import "go.githedgehog.com/fabric/api/meta"

type ServerConfig struct {
	ControlVIP           string                  `json:"controlVIP,omitempty"`
	NOSRepos             map[meta.NOSType]string `json:"nosRepos,omitempty"`
	NOSVersions          map[meta.NOSType]string `json:"nosVersions,omitempty"`
	ONIERepos            map[string]string       `json:"onieRepos,omitempty"`
	ONIEPlatformVersions map[string]string       `json:"oniePlatformVersions,omitempty"`
}
