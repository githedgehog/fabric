//go:build linux
// +build linux

package dhcpd

import (
	"context"
	"log/slog"

	"github.com/coredhcp/coredhcp/config"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/coredhcp/coredhcp/server"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (d *Service) runCoreDHCP(ctx context.Context) error {
	log := logger.GetLogger("main")
	if d.Verbose {
		log.Logger.SetLevel(logrus.DebugLevel)
	} else {
		log.Logger.SetLevel(logrus.InfoLevel)
	}

	// TODO conf some facade to direct logrus to slog

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
