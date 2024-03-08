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
// +build linux

package dhcpd

import (
	"context"
	"log/slog"
	"os"

	"github.com/coredhcp/coredhcp/config"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/coredhcp/coredhcp/server"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const defaultConfig = `
server4:
  listen:
    - "0.0.0.0"
  plugins:
    - hhdhcp: ""
`

func (d *Service) runCoreDHCP(ctx context.Context) error {
	log := logger.GetLogger("main")
	if d.Verbose {
		log.Logger.SetLevel(logrus.DebugLevel)
	} else {
		log.Logger.SetLevel(logrus.InfoLevel)
	}

	// TODO conf some facade to direct logrus to slog

	if _, err := os.Stat(d.Config); errors.Is(err, os.ErrNotExist) {
		d.Config = "/etc/coredhcp.conf"

		if err := os.WriteFile(d.Config, []byte(defaultConfig), 0644); err != nil {
			return errors.Wrapf(err, "failed to write default config")
		}
	}

	config, err := config.Load(d.Config)
	if err != nil {
		return errors.Wrapf(err, "failed to load configuration")
	}

	desiredPlugins := []*plugins.Plugin{
		{
			Name:   "hhdhcp",
			Setup6: nil,
			Setup4: setup(d),
		},
	}
	for _, plugin := range desiredPlugins {
		if err := plugins.RegisterPlugin(plugin); err != nil {
			return errors.Wrapf(err, "failed to register plugin '%s'", plugin.Name)
		}
	}

	slog.Info("Starting DHCP server")

	srv, err := server.Start(config)
	if err != nil {
		return errors.Wrapf(err, "failed to start server")
	}
	if err := srv.Wait(); err != nil {
		return errors.Wrapf(err, "failed to wait for server")
	}

	return nil
}
