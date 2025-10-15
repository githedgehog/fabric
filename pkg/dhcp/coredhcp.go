// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/coredhcp/coredhcp/config"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/coredhcp/coredhcp/server"
	"github.com/sirupsen/logrus"
)

const cfgTmpl = `
server4:
  listen:
    - "%s"
  plugins:
    - hh: ""
`

func (s *Server) startCoreDHCP(ctx context.Context) error {
	// TODO conf some facade to direct logrus to slog

	log := logger.GetLogger("main")
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		log.Logger.SetLevel(logrus.DebugLevel)
	} else {
		log.Logger.SetLevel(logrus.InfoLevel)
	}

	cfgFile := "/etc/coredhcp.conf"

	if err := os.WriteFile(cfgFile, fmt.Appendf(nil, cfgTmpl, "%"+s.ListenInterface), 0o600); err != nil {
		return fmt.Errorf("writing config") //nolint:err113
	}

	config, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading configuration") //nolint:err113
	}

	desiredPlugins := []*plugins.Plugin{
		{
			Name:   "hh",
			Setup6: nil,
			Setup4: s.setupDHCP4Plugin(ctx),
		},
	}
	for _, plugin := range desiredPlugins {
		if err := plugins.RegisterPlugin(plugin); err != nil {
			return fmt.Errorf("registering plugin: %s", plugin.Name) //nolint:err113
		}
	}

	slog.Info("Starting DHCP server", "iface", s.ListenInterface)

	srv, err := server.Start(config)
	if err != nil {
		return fmt.Errorf("starting coredhcp") //nolint:err113
	}
	if err := srv.Wait(); err != nil {
		return fmt.Errorf("waiting for coredhcp") //nolint:err113
	}

	return fmt.Errorf("coredhcp finished unexpectedly") //nolint:err113
}
