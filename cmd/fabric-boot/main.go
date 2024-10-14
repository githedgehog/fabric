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
	"time"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/boot/server"
	"go.githedgehog.com/fabric/pkg/version"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func setupLogger(verbose, brief bool) error {
	if verbose && brief {
		return cli.Exit("verbose and brief are mutually exclusive", 1)
	}

	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	} else if brief {
		logLevel = slog.LevelWarn
	}

	logW := os.Stderr
	handler := tint.NewHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.TimeOnly,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	ctrl.SetLogger(logr.FromSlogHandler(handler))
	klog.SetSlogLogger(logger)

	return nil
}

func main() {
	ctx := context.Background()

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

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "fabric-boot",
		Usage:                  "Hedgehog Fabric boot server",
		Version:                version.Version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,

		Flags: []cli.Flag{
			verboseFlag,
			briefFlag,
		},
		Before: func(_ *cli.Context) error {
			return setupLogger(verbose, brief)
		},
		Action: func(_ *cli.Context) error {
			slog.Info("Running fabric-boot", "version", version.Version)

			if err := server.Run(ctx); err != nil {
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
