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
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/hhfctl"
)

var version = "(devel)"

func setupLogger(verbose bool) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	logW := os.Stdout
	logger := slog.New(
		tint.NewHandler(logW, &tint.Options{
			Level:      logLevel,
			TimeFormat: time.TimeOnly,
			NoColor:    !isatty.IsTerminal(logW.Fd()),
		}),
	)
	slog.SetDefault(logger)

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

	var name string
	nameFlag := &cli.StringFlag{
		Name:        "name",
		Aliases:     []string{"n"},
		Usage:       "name",
		Destination: &name,
	}

	var yes bool
	yesFlag := &cli.BoolFlag{
		Name:        "yes",
		Aliases:     []string{"y"},
		Usage:       "assume yes",
		Destination: &yes,
	}
	yesCheck := func(_ *cli.Context) error {
		if !yes {
			return cli.Exit("Potentially dangerous operation. Please confirm with --yes if you're sure.", 1)
		}

		return nil
	}

	var printYaml bool
	printYamlFlag := &cli.BoolFlag{
		Name:        "print",
		Aliases:     []string{"p"},
		Usage:       "print object yaml",
		Destination: &printYaml,
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   "hhfctl",
		Usage:                  "Hedgehog Fabric user client",
		Version:                version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,
		Flags: []cli.Flag{
			verboseFlag,
		},
		Commands: []*cli.Command{
			{
				Name:  "vpc",
				Usage: "VPC commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create vpc",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringFlag{
								Name:     "subnet",
								Usage:    "subnet",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "vlan",
								Usage:    "vlan",
								Required: true,
							},
							&cli.BoolFlag{
								Name:  "dhcp",
								Usage: "enable dhcp",
							},
							&cli.StringFlag{
								Name:    "dhcp-range-start",
								Aliases: []string{"dhcp-start"},
								Usage:   "dhcp range start",
							},
							&cli.StringFlag{
								Name:    "dhcp-range-end",
								Aliases: []string{"dhcp-end"},
								Usage:   "dhcp range end",
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.VPCCreate(ctx, printYaml, &hhfctl.VPCCreateOptions{
								Name:   name,
								Subnet: cCtx.String("subnet"),
								VLAN:   uint16(cCtx.Uint("vlan")),
								DHCP: vpcapi.VPCDHCP{
									Enable: cCtx.Bool("dhcp"),
									PXEURL: cCtx.String("dhcp-pxe-url"),
									Range: &vpcapi.VPCDHCPRange{
										Start: cCtx.String("dhcp-range-start"),
										End:   cCtx.String("dhcp-range-end"),
									},
								},
							}), "failed to create vpc")
						},
					},
					{
						Name:  "attach",
						Usage: "Attach connection to vpc",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringFlag{
								Name:     "vpc-subnet",
								Aliases:  []string{"subnet"},
								Usage:    "vpc/subnet",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "connection",
								Aliases:  []string{"conn"},
								Usage:    "connection",
								Required: true,
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.VPCAttach(ctx, printYaml, &hhfctl.VPCAttachOptions{
								Name:       name,
								VPCSubnet:  cCtx.String("vpc-subnet"),
								Connection: cCtx.String("connection"),
							}), "failed to attach connection to vpc")
						},
					},
					{
						Name:    "peer",
						Aliases: []string{"peering"},
						Usage:   "Enable peering between vpcs",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringSliceFlag{
								Name:     "vpc",
								Usage:    "vpc",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "remote",
								Usage: "SwitchGroup name for remote peering",
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.VPCPeer(ctx, printYaml, &hhfctl.VPCPeerOptions{
								Name:   name,
								VPCs:   cCtx.StringSlice("vpc"),
								Remote: cCtx.String("remote"),
							}), "failed to peer vpcs")
						},
					},
				},
			},
			{
				Name:    "switch",
				Aliases: []string{"sw", "agent"},
				Usage:   "Switch/Agent commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "reboot",
						Usage: "Reboot the switch",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}

							return errors.Wrapf(hhfctl.SwitchReboot(ctx, name), "failed to reboot switch")
						},
					},
					{
						Name:  "power-reset",
						Usage: "Power reset the switch (unsafe, skips graceful shutdown)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}

							return errors.Wrapf(hhfctl.SwitchPowerReset(ctx, name), "failed to power reset switch")
						},
					},
					{
						Name:  "reinstall",
						Usage: "Reinstall the switch (reboot into ONIE)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}

							return errors.Wrapf(hhfctl.SwitchReinstall(ctx, name), "failed to reinstall switch")
						},
					},
					{
						Name:  "agent-version",
						Usage: "Force agent version on the switch (empty version to reset to the default)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
							&cli.StringFlag{
								Name:     "version",
								Usage:    "version (empty to reset to the default)",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}

							return errors.Wrapf(hhfctl.SwitchForceAgentVersion(ctx, name, cCtx.String("version")), "failed to force agent version on the switch")
						},
					},
				},
			},
			{
				Name:    "connection",
				Aliases: []string{"conn"},
				Usage:   "Connection commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "get",
						Usage: "Get connections",
						Flags: []cli.Flag{
							verboseFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.ConnectionGet(ctx, &hhfctl.ConnectionGetOptions{
								Type: cCtx.Args().First(),
							}), "failed to get connections")
						},
					},
				},
			},
			{
				Name:    "switchgroup",
				Aliases: []string{"sg"},
				Usage:   "SwitchGroup commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create SwitchGroup",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf(hhfctl.SwitchGroupCreate(ctx, printYaml, &hhfctl.SwitchGroupCreateOptions{
								Name: name,
							}), "failed to create SwitchGroup")
						},
					},
				},
			},
			{
				Name:    "external",
				Aliases: []string{"ext"},
				Usage:   "External commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create External",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							printYamlFlag,
							&cli.StringFlag{
								Name:    "ipv4-namespace",
								Aliases: []string{"ipns"},
								Usage:   "ipv4 namespace",
							},
							&cli.StringFlag{
								Name:    "inbound-community",
								Aliases: []string{"in"},
								Usage:   "inbound community",
							},
							&cli.StringFlag{
								Name:    "outbound-community",
								Aliases: []string{"out"},
								Usage:   "outbound community",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.ExternalCreate(ctx, printYaml, &hhfctl.ExternalCreateOptions{
								Name:              name,
								IPv4Namespace:     cCtx.String("ipv4-namespace"),
								InboundCommunity:  cCtx.String("inbound-community"),
								OutboundCommunity: cCtx.String("outbound-community"),
							}), "failed to create External")
						},
					},
					{
						Name:    "peer",
						Aliases: []string{"peering"},
						Usage:   "Enable peering between external and vpc",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							printYamlFlag,
							&cli.StringFlag{
								Name:  "vpc",
								Usage: "vpc name",
							},
							&cli.StringFlag{
								Name:    "external",
								Aliases: []string{"ext"},
								Usage:   "external name",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-subnet",
								Aliases: []string{"subnet"},
								Usage:   "vpc subnets to enable peering for",
								Value:   cli.NewStringSlice("default"),
							},
							&cli.StringSliceFlag{
								Name:    "external-prefix",
								Aliases: []string{"prefix"},
								Usage:   "external prefixes to enable peering for, could be in a format 10.0.0.0/8_le32_ge32",
								Value:   cli.NewStringSlice("0.0.0.0/0_le32"),
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.ExternalPeering(ctx, printYaml, &hhfctl.ExternalPeeringOptions{
								VPC:              cCtx.String("vpc"),
								VPCSubnets:       cCtx.StringSlice("vpc-subnet"),
								External:         cCtx.String("external"),
								ExternalPrefixes: cCtx.StringSlice("external-prefix"),
							}), "failed to enable peering between external and vpc")
						},
					},
				},
			},
			{
				Name:  "inspect",
				Usage: "inspect commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "port",
						Usage: "Inspect port",
						Flags: []cli.Flag{
							verboseFlag,
							&cli.StringFlag{
								Name:     "name",
								Usage:    "name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							slog.Info("Inspecting port", "name", cCtx.String("name"))

							return nil
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
