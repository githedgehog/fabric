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

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	slogmulti "github.com/samber/slog-multi"
	slogwh "github.com/samber/slog-webhook/v2"
	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/boot/nosinstall"
	"go.githedgehog.com/fabric/pkg/version"
	"gopkg.in/natefinch/lumberjack.v2"
)

//go:embed motd.txt
var motd []byte

func setupLogger(verbose, brief bool, env nosinstall.Env) error {
	if verbose && brief {
		return cli.Exit("verbose and brief are mutually exclusive", 1)
	}

	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	} else if brief {
		logLevel = slog.LevelWarn
	}

	logConsole := os.Stderr

	logFile := &lumberjack.Logger{
		Filename:   "/var/log/install.log",
		MaxSize:    5, // MB
		MaxBackups: 4,
		MaxAge:     30, // days
		Compress:   true,
		FileMode:   0o644,
	}

	handlers := []slog.Handler{
		tint.NewHandler(logConsole, &tint.Options{
			Level:      logLevel,
			TimeFormat: time.DateTime,
			NoColor:    !isatty.IsTerminal(logConsole.Fd()),
		}),
		slog.NewTextHandler(logFile, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	}

	if nosinstall.WebhookLog && strings.HasPrefix(env.ExecURL, "http://") && strings.HasSuffix(env.ExecURL, nosinstall.OnieURLSuffix) {
		logURL := strings.TrimSuffix(env.ExecURL, nosinstall.OnieURLSuffix) + nosinstall.LogURLSuffix

		if env.Serial != "" || env.EthAddr != "" {
			handlers = append(handlers,
				slogwh.Option{
					Level:    slog.LevelInfo,
					Endpoint: logURL,
					AttrFromContext: []func(ctx context.Context) []slog.Attr{
						func(_ context.Context) []slog.Attr {
							return []slog.Attr{
								slog.String(nosinstall.KeySerial, env.Serial),
								slog.String(nosinstall.KeyEthAddr, env.EthAddr),
							}
						},
					},
				}.NewWebhookHandler(),
			)
		}
	}

	logger := slog.New(slogmulti.Fanout(handlers...))

	slog.SetDefault(logger)

	_, err := logConsole.Write(motd)
	if err != nil {
		return fmt.Errorf("writing motd: %w", err)
	}

	slog.Info("Running fabric-nos-install", "version", version.Version)

	if len(handlers) == 2 {
		slog.Info("Not sending logs to fabric-boot (no mac or serial available)")
	} else {
		slog.Info("Sending logs to fabric-boot")
	}

	return nil
}

func main() {
	ctx := context.Background()

	env := nosinstall.ReadEnv(ctx)

	var verbose, brief bool
	verboseFlag := &cli.BoolFlag{
		Name:        "verbose",
		Aliases:     []string{"v"},
		Usage:       "verbose output (includes debug)",
		Destination: &verbose,
	}
	briefFlag := &cli.BoolFlag{
		Name:        "brief",
		Aliases:     []string{"b"},
		Usage:       "brief output (only warn and error)",
		Destination: &brief,
	}

	var dryRun bool
	dryRunFlag := &cli.BoolFlag{
		Name:        "dry-run",
		Aliases:     []string{"n"},
		Usage:       "dry run (don't actually run anything)",
		Destination: &dryRun,
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "fabric-nos-install",
		Usage:                  "Hedgehog Fabric Switch NOS installer",
		Version:                version.Version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,

		Flags: []cli.Flag{
			verboseFlag,
			briefFlag,
			dryRunFlag,
		},
		Before: func(_ *cli.Context) error {
			return setupLogger(verbose, brief, env)
		},
		Action: func(_ *cli.Context) error {
			if err := nosinstall.Run(ctx, env, dryRun); err != nil {
				return fmt.Errorf("running: %w", err)
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Failed", "err", err.Error())
		os.Exit(1)
	}
}
