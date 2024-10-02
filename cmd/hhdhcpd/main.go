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
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	slogmulti "github.com/samber/slog-multi"
	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/dhcpd"
)

const (
	DefaultBasedir = "/etc/hedgehog/"
)

//go:embed motd.txt
var motd []byte

var version = "(devel)"

func setupLogger(verbose bool, printMotd bool) error {
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

	logger := slog.New(slogmulti.Fanout(handlers...))

	slog.SetDefault(logger)

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

	var configPath, listenInterface string
	configPathFlag := &cli.StringFlag{
		Name:        "config",
		Aliases:     []string{"c"},
		Usage:       "config file",
		Value:       "/etc/hedgehog/dhcpd.yaml",
		Destination: &configPath,
	}
	listenInterfaceFlag := &cli.StringFlag{
		Name:        "listen",
		Aliases:     []string{"l"},
		Usage:       "listen interface",
		Value:       "127.0.0.1",
		Destination: &listenInterface,
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "hhdhcpd",
		Usage:                  "hedgehog fabric dhcp server",
		Version:                version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "start dhcp server",
				Flags: []cli.Flag{
					verboseFlag,
					configPathFlag,
					listenInterfaceFlag,
				},
				Before: func(_ *cli.Context) error {
					return setupLogger(verbose, true)
				},
				Action: func(_ *cli.Context) error {
					return errors.Wrapf((&dhcpd.Service{
						Verbose:         verbose,
						Config:          configPath,
						ListenInterface: listenInterface,
					}).Run(ctx), "failed to run dhcp server")
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Failed", "err", err.Error())
		os.Exit(1)
	}
}
