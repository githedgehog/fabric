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
	slogmulti "github.com/samber/slog-multi"
	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/agent"
	"go.githedgehog.com/fabric/pkg/agent/control"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/systemd"
	"go.githedgehog.com/fabric/pkg/version"
	"gopkg.in/natefinch/lumberjack.v2"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	DefaultBasedir          = "/etc/sonic/hedgehog/"
	DefaultBinPath          = "/opt/hedgehog/bin/agent"
	DefaultAgentServiceUser = "root"
)

//go:embed motd.txt
var motd []byte

func setupLogger(verbose bool, logToFile bool, printMotd bool) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	logConsole := os.Stdout

	handlers := []slog.Handler{
		tint.NewHandler(logConsole, &tint.Options{
			Level:      logLevel,
			TimeFormat: time.DateTime,
			NoColor:    !isatty.IsTerminal(logConsole.Fd()),
		}),
	}

	if logToFile {
		logFile := &lumberjack.Logger{
			Filename:   "/var/log/agent.log",
			MaxSize:    5, // MB
			MaxBackups: 4,
			MaxAge:     30, // days
			Compress:   true,
			FileMode:   0o644,
		}
		// TODO do we need to close logFile?

		handlers = append(handlers, slog.NewTextHandler(logFile, &slog.HandlerOptions{
			Level: logLevel,
		}))
	}

	handler := slogmulti.Fanout(handlers...)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	ctrl.SetLogger(logr.FromSlogHandler(handler))
	klog.SetSlogLogger(logger)

	if printMotd {
		_, err := logConsole.Write(motd)
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

	var basedir string
	basedirFlag := &cli.StringFlag{
		Name:        "basedir",
		Usage:       "base directory for the agent files",
		Destination: &basedir,
		Value:       DefaultBasedir,
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "agent",
		Usage:                  "hedgehog fabric agent",
		Version:                version.Version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "start agent to watch for config changes and apply them",
				Flags: []cli.Flag{
					verboseFlag,
					basedirFlag,
				},
				Before: func(_ *cli.Context) error {
					return setupLogger(verbose, true, true)
				},
				Action: func(_ *cli.Context) error {
					return errors.Wrapf((&agent.Service{
						Basedir: basedir,
					}).Run(ctx, func() (*gnmi.Client, error) {
						client, err := gnmi.NewInSONiC(ctx, basedir, false)
						if err != nil {
							return nil, errors.Wrapf(err, "failed to create GNMI client")
						}

						return client, nil
					}), "failed to run agent")
				},
			},
			{
				Name:  "apply",
				Usage: "apply config once from file without starting agent",
				Flags: []cli.Flag{
					verboseFlag,
					basedirFlag,
					&cli.BoolFlag{
						Name:  "dry-run",
						Value: true,
					},
					&cli.BoolFlag{
						Name:  "skip-contol-link",
						Value: true,
					},
					&cli.BoolFlag{
						Name:  "apply-once",
						Value: true,
					},
					&cli.BoolFlag{
						Name:  "gnmi-direct",
						Value: false,
					},
					&cli.StringFlag{
						Name:  "gnmi-server",
						Value: "127.0.0.1:8080",
					},
					&cli.StringFlag{
						Name:    "gnmi-username",
						Aliases: []string{"u"},
						Value:   "admin",
					},
					&cli.StringFlag{
						Name:    "gnmi-password",
						Aliases: []string{"p"},
						Value:   "YourPaSsWoRd",
					},
				},
				Before: func(_ *cli.Context) error {
					return setupLogger(verbose, false, true)
				},
				Action: func(cCtx *cli.Context) error {
					slog.Info("Applying", "version", version.Version)

					getGNMIClient := func() (*gnmi.Client, error) {
						if cCtx.Bool("gnmi-direct") {
							client, err := gnmi.New(ctx,
								cCtx.String("gnmi-server"),
								cCtx.String("gnmi-username"),
								cCtx.String("gnmi-password"))
							if err != nil {
								return nil, errors.Wrapf(err, "failed to create GNMI client")
							}

							return client, nil
						}

						client, err := gnmi.NewInSONiC(ctx, basedir, true)
						if err != nil {
							return nil, errors.Wrapf(err, "failed to create GNMI client")
						}

						return client, nil
					}

					return errors.Wrapf((&agent.Service{
						Basedir:         basedir,
						DryRun:          cCtx.Bool("dry-run"),
						SkipControlLink: cCtx.Bool("skip-contol-link"),
						ApplyOnce:       cCtx.Bool("apply-once"),
						SkipActions:     true,
					}).Run(ctx, getGNMIClient), "failed to apply config")
				},
			},
			{
				Name:    "generate",
				Aliases: []string{"gen"},
				Usage:   "generate config/systemd-unit/etc",
				Subcommands: []*cli.Command{
					{
						Name:  "systemd-unit",
						Usage: "generate systemd-unit",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name: "bin-path",
								Aliases: []string{
									"agent-path",
								},
								Value: DefaultBinPath,
								Usage: "path to the agent binary",
							},
							&cli.StringFlag{
								Name: "user",
								Aliases: []string{
									"agent-user",
								},
								Value: DefaultAgentServiceUser,
								Usage: "user to run agent",
							},
							&cli.BoolFlag{
								Name:  "control",
								Usage: "generate control agent systemd-unit",
							},
						},
						Action: func(cCtx *cli.Context) error {
							unit, err := systemd.Generate(systemd.UnitConfig{
								BinPath: cCtx.String("bin-path"),
								User:    cCtx.String("user"),
								Control: cCtx.Bool("control"),
							})
							if err != nil {
								return errors.Wrapf(err, "failed to generate systemd unit")
							}

							fmt.Println(unit)

							return nil
						},
					},
				},
			},
			{
				Name:  "install",
				Usage: "install systemd unit",
				Flags: []cli.Flag{
					verboseFlag,
					basedirFlag,
					&cli.StringFlag{
						Name: "bin-path",
						Aliases: []string{
							"agent-path",
						},
						Value: DefaultBinPath,
						Usage: "path to the agent binary",
					},
					&cli.StringFlag{
						Name: "user",
						Aliases: []string{
							"agent-user",
						},
						Value: DefaultAgentServiceUser,
						Usage: "user to run agent",
					},
					&cli.BoolFlag{
						Name:  "control",
						Usage: "install control agent systemd-unit",
					},
				},
				Before: func(_ *cli.Context) error {
					return setupLogger(verbose, true, false)
				},
				Action: func(cCtx *cli.Context) error {
					return errors.Wrapf(systemd.Install(systemd.UnitConfig{
						BinPath: cCtx.String("bin-path"),
						User:    cCtx.String("user"),
						Control: cCtx.Bool("control"),
					}), "failed to install systemd unit")
				},
			},
			{
				Name:  "control",
				Usage: "control agent",
				Flags: []cli.Flag{
					verboseFlag,
					basedirFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "start",
						Usage: "start control agent to watch for config changes and apply them",
						Flags: []cli.Flag{
							verboseFlag,
							basedirFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose, true, true)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf((&control.Service{}).Run(ctx), "failed to run control agent")
						},
					},
					{
						Name:  "apply",
						Usage: "apply control agent config once",
						Flags: []cli.Flag{
							verboseFlag,
							basedirFlag,
							&cli.BoolFlag{
								Name:  "dry-run",
								Value: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose, false, true)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf((&control.Service{
								ApplyOnce: true,
								DryRun:    cCtx.Bool("dry-run"),
							}).Run(ctx), "failed to apply control agent config")
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Failed", "err", err.Error())
		os.Exit(1)
	}
}
