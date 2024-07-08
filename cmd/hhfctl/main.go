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
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/hhfctl"
	"go.githedgehog.com/fabric/pkg/hhfctl/inspect"
)

var version = "(devel)"

func setupLogger(verbose bool) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	logW := os.Stderr
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

	outputTypes := []string{}
	for _, t := range inspect.OutputTypes {
		outputTypes = append(outputTypes, string(t))
	}

	var output string
	outputFlag := &cli.StringFlag{
		Name:        "output",
		Aliases:     []string{"o"},
		Usage:       "output format, one of " + strings.Join(outputTypes, ", "),
		Value:       "text",
		Destination: &output,
	}

	appName := "hhfctl"
	usage := "Hedgehog Fabric API CLI client"
	if len(os.Args) > 0 {
		if strings.HasSuffix(os.Args[0], "kubectl-fabric") {
			appName = "kubectl fabric"
			usage = "Hedgehog Fabric API kubectl plugin"
		} else if strings.HasSuffix(os.Args[0], "fabric") {
			appName = "fabric"
		}
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   appName,
		Usage:                  usage,
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
				Name:    "inspect",
				Aliases: []string{"i"},
				Usage:   "Inspect Fabric API Objects and Primitives",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "fabric",
						Usage: "Inspect Fabric (overall control nodes and switches overview incl. status, serials, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Fabric, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.FabricIn{}, os.Stdout), "failed to inspect Fabric")
						},
					},
					{
						Name:  "switch",
						Usage: "Inspect Switch (status, used ports, counters, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "switch name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Switch, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.SwitchIn{
								Name: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Switch")
						},
					},
					{
						Name:    "port",
						Aliases: []string{"switchport"},
						Usage:   "Inspect Switch Port (connection if used in one, counters, VPC and External attachments, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "full switch port name (<switch-name>/<port-name>, e.g. 's5248-02/E1/2')",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Port, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.PortIn{
								Port: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Switch Port")
						},
					},
					{
						Name:  "server",
						Usage: "Inspect Server (connection if used in one, VPC attachments, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "server name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Server, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.ServerIn{
								Name: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Server")
						},
					},
					{
						Name:    "connection",
						Aliases: []string{"conn"},
						Usage:   "Inspect Connection (incl. VPC and External attachments, Loobpback Workaround usage, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "connection name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Connection, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.ConnectionIn{
								Name: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Connection")
						},
					},
					{
						Name:    "vpc",
						Aliases: []string{"subnet", "vpcsubnet"},
						Usage:   "Inspect VPC/VPCSubnet (incl. where is it attached and what's reachable from it)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "VPC name (if no subnet specified, will inspect all subnets)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "subnet",
								Aliases: []string{"s"},
								Usage:   "Subnet name (without VPC) to only inspect this subnet",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.VPC, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.VPCIn{
								Name:   cCtx.String("name"),
								Subnet: cCtx.String("subnet"),
							}, os.Stdout), "failed to inspect VPC")
						},
					},
					{
						Name:  "ip",
						Usage: "Inspect IP Address (incl. IPv4Namespace, VPCSubnet and DHCPLease or External/StaticExternal usage)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "address",
								Aliases:  []string{"a", "addr"},
								Usage:    "IP address to inspect",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.IP, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.IPIn{
								IP: cCtx.String("address"),
							}, os.Stdout), "failed to inspect IP address")
						},
					},
					{
						Name:  "mac",
						Usage: "Inspect MAC Address (incl. switch ports and DHCP leases)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "address",
								Aliases:  []string{"a", "addr"},
								Usage:    "MAC address",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.MAC, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.MACIn{
								Value: cCtx.String("address"),
							}, os.Stdout), "failed to inspect MAC Address")
						},
					},
					{
						Name:  "access",
						Usage: "Inspect access between pair of IPs, Server names or VPCSubnets (everything except external IPs will be translated to VPCSubnets)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s", "src"},
								Usage:    "Source IP (only from VPC subnets), full VPC subnet name (<vpc-name>/<subnet-name>) or Server Name",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "destination",
								Aliases:  []string{"d", "dest"},
								Usage:    "Destination IP (from VPC subnets, Externals or StaticExternals), full VPC subnet name (<vpc-name>/<subnet-name>) or Server Name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Access, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.AccessIn{
								Source:      cCtx.String("source"),
								Destination: cCtx.String("destination"),
							}, os.Stdout), "failed to inspect access")
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
