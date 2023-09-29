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
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	slogmulti "github.com/samber/slog-multi"
	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/agent"
	"go.githedgehog.com/fabric/pkg/agent/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/systemd"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	DEFAULT_BASEDIR            = "/etc/sonic/hedgehog/"
	DEFAULT_BIN_PATH           = "/opt/hedgehog/bin/agent"
	DEFAULT_AGENT_SERVICE_USER = "root"
)

//go:embed motd.txt
var motd []byte

var version = "(devel)"

func setupLogger(verbose bool, logToFile bool) error {
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
		}
		// TODO do we need to close logFile?

		handlers = append(handlers, slog.NewTextHandler(logFile, &slog.HandlerOptions{
			Level: logLevel,
		}))
	}

	logger := slog.New(slogmulti.Fanout(handlers...))

	slog.SetDefault(logger)

	_, err := logConsole.Write([]byte(motd))
	if err != nil {
		return err
	}

	return nil
}

func main() {
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
		Value:       DEFAULT_BASEDIR,
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "agent",
		Usage:                  "hedgehog fabric agent",
		Version:                version,
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
				Before: func(cCtx *cli.Context) error {
					return setupLogger(verbose, true)
				},
				Action: func(cCtx *cli.Context) error {
					slog.Info("Starting", "version", version)

					return (&agent.Service{
						Basedir: basedir,
						Version: version,
					}).Run(ctx, func() (*gnmi.Client, error) {
						return gnmi.NewInSONiC(ctx, basedir, false)
					})
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
				Before: func(cCtx *cli.Context) error {
					return setupLogger(verbose, false)
				},
				Action: func(cCtx *cli.Context) error {
					slog.Info("Applying", "version", version)

					getGNMIClient := func() (*gnmi.Client, error) {
						if cCtx.Bool("gnmi-direct") {
							return gnmi.New(ctx,
								cCtx.String("gnmi-server"),
								cCtx.String("gnmi-username"),
								cCtx.String("gnmi-password"))
						}

						return gnmi.NewInSONiC(ctx, basedir, true)
					}

					return (&agent.Service{
						Basedir:         basedir,
						Version:         version,
						DryRun:          cCtx.Bool("dry-run"),
						SkipControlLink: cCtx.Bool("skip-contol-link"),
						ApplyOnce:       cCtx.Bool("apply-once"),
						SkipActions:     true,
					}).Run(ctx, getGNMIClient)
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
								Value: DEFAULT_BIN_PATH,
								Usage: "path to the agent binary",
							},
							&cli.StringFlag{
								Name: "user",
								Aliases: []string{
									"agent-user",
								},
								Value: DEFAULT_AGENT_SERVICE_USER,
								Usage: "user to run agent",
							},
						},
						Action: func(cCtx *cli.Context) error {
							unit, err := systemd.Generate(systemd.UnitConfig{
								BinPath: cCtx.String("bin-path"),
								User:    cCtx.String("user"),
							})
							if err != nil {
								return err
							}

							fmt.Println(unit)

							return nil
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Failed", "err", err)
		os.Exit(1)
	}
}
