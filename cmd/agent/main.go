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
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/agent"
	"go.githedgehog.com/fabric/pkg/agent/systemd"
	"go.uber.org/zap"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func main() {
	app := &cli.App{
		Name:    "agent",
		Version: "0.0.0", // TODO load proper version using ld flags
		Action: func(ctx *cli.Context) error {
			return (&agent.Service{}).Run()
		},
		Commands: []*cli.Command{
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
								Value: "/etc/sonic/hedgehog/agent",
								Usage: "path to the agent binary",
							},
							&cli.StringFlag{
								Name: "user",
								Aliases: []string{
									"agent-user",
								},
								Value: "githedgehog",
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
			{
				Name:    "check",
				Aliases: []string{"c"},
				Usage:   "check",
				Action: func(ctx *cli.Context) error {
					// TODO
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		zap.S().Panic("unrecoverable error: ", err)
	}
}
