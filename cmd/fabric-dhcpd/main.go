/*
Copyright 2023.

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

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/dhcp"
	dhcpcfg "go.githedgehog.com/fabric/pkg/dhcp/config"
	"go.githedgehog.com/fabric/pkg/version"
	"k8s.io/klog/v2"
	kctrl "sigs.k8s.io/controller-runtime"
)

const (
	DefaultBasedir = "/etc/hedgehog/"
)

//go:embed motd.txt
var motd []byte

func setupLogger(verbose bool, printMotd bool) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	logW := os.Stdout

	slog.SetDefault(slog.New(tint.NewTextHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.StampMilli,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})))

	kubeHandler := tint.NewTextHandler(logW, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.StampMilli,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})
	kctrl.SetLogger(logr.FromSlogHandler(kubeHandler))
	klog.SetSlogLogger(slog.New(kubeHandler))

	if printMotd {
		_, err := logW.Write(motd)
		if err != nil {
			return errors.Wrapf(err, "failed to write motd")
		}
	}

	return nil
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("Panic", "err", err, "stack", string(debug.Stack()))
			os.Exit(1)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var verbose bool
	verboseFlag := &cli.BoolFlag{
		Name:        "verbose",
		Aliases:     []string{"v"},
		Usage:       "verbose output (includes debug)",
		Value:       true, // TODO disable debug by default
		Destination: &verbose,
	}

	// passed by the chart from the same value as the probes port, so it's not in the config file
	var healthListen string
	healthListenFlag := &cli.StringFlag{
		Name:        "health-listen",
		Usage:       "address for the liveness/readiness HTTP server",
		Value:       dhcpcfg.DefaultHealthListen,
		Destination: &healthListen,
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "hhdhcpd",
		Usage:                  "hedgehog fabric dhcp server",
		Version:                version.Version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "start dhcp server",
				Flags: []cli.Flag{
					verboseFlag,
					healthListenFlag,
				},
				Before: func(_ *cli.Context) error {
					return setupLogger(verbose, true)
				},
				Action: func(_ *cli.Context) error {
					cfg, err := dhcpcfg.Load(dhcpcfg.Path, healthListen)
					if err != nil {
						return fmt.Errorf("loading config: %w", err)
					}

					if err := (&dhcp.Server{Config: *cfg}).Run(ctx); err != nil {
						return fmt.Errorf("running dhcp server: %w", err)
					}

					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Failed", "err", err.Error())
		os.Exit(1) //nolint:gocritic
	}
}
