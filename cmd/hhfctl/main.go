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

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/hhfctl"
	"go.githedgehog.com/fabric/pkg/hhfctl/inspect"
	"go.githedgehog.com/fabric/pkg/version"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func setupLogger(verbose bool) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
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

	var username string
	usernameFlag := &cli.StringFlag{
		Name:        "username",
		Aliases:     []string{"u"},
		Usage:       "username",
		Destination: &username,
		Value:       "admin",
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
		Version:                version.Version,
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
								VLAN:   uint16(cCtx.Uint("vlan")), //nolint:gosec
								DHCP: vpcapi.VPCDHCP{
									Enable: cCtx.Bool("dhcp"),
									Range: &vpcapi.VPCDHCPRange{
										Start: cCtx.String("dhcp-range-start"),
										End:   cCtx.String("dhcp-range-end"),
									},
									Options: &vpcapi.VPCDHCPOptions{
										PXEURL: cCtx.String("dhcp-pxe-url"),
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
					{
						Name:  "wipe",
						Usage: "Delete all vpcs, their peerings (incl. external) and attachments",
						Flags: []cli.Flag{
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.VPCWipe(ctx), "failed to wipe vpcs"))
						},
					},
				},
			},
			{
				Name:    "switch",
				Aliases: []string{"sw"},
				Usage:   "Switch commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "ip",
						Usage: "Get switch management IP address",
						Flags: []cli.Flag{
							usernameFlag,
							verboseFlag,
							nameFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf(hhfctl.SwitchIP(ctx, name), "failed to get switch IP address")
						},
					},
					{
						Name:  "ssh",
						Usage: "SSH into the switch (only from control nodes, using mgmt network)",
						Flags: []cli.Flag{
							usernameFlag,
							verboseFlag,
							nameFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchSSH(ctx, name, username), "failed to ssh into the switch"))
						},
					},
					{
						Name:  "serial",
						Usage: "Run serial console for the switch (only if it's specified in the switch annotations)",
						Flags: []cli.Flag{
							usernameFlag,
							verboseFlag,
							nameFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchSerial(ctx, name), "failed to run serial for the switch"))
						},
					},
					{
						Name:  "reboot",
						Usage: "Reboot the switch (only works if switch is healthy and sends heartbeats)",
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
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchReboot(ctx, name), "failed to reboot switch"))
						},
					},
					{
						Name:  "power-reset",
						Usage: "Power reset the switch (UNSAFE, skips graceful shutdown, only works if switch is healthy and sends heartbeats)",
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
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchPowerReset(ctx, name), "failed to power reset switch"))
						},
					},
					{
						Name:  "reinstall",
						Usage: "Reinstall the switch (reboot into ONIE, only works if switch is healthy and sends heartbeats)",
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
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchReinstall(ctx, name), "failed to reinstall switch"))
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
						Name:        "get",
						Usage:       "Get connections",
						ArgsUsage:   " <type>",
						Description: "Available types: management, fabric, and vpc-loopback",
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
								Usage:   "external prefixes to enable peering for, e.g. 0.0.0.0/0 for default route",
								Value:   cli.NewStringSlice("0.0.0.0/0"),
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
				Name:  "wiring",
				Usage: "general wiring diagram helpers",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "export",
						Usage: "export wiring diagram (incl. switches, connections, vpcs, externals, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							&cli.BoolFlag{
								Name:  "vpcs",
								Usage: "include VPCs",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "externals",
								Usage: "include Externals",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "switch-profiles",
								Usage: "include SwitchProfiles (may cause issues on importing)",
								Value: false,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.WiringExport(ctx, hhfctl.WiringExportOptions{
								VPCs:           cCtx.Bool("vpcs"),
								Externals:      cCtx.Bool("externals"),
								SwitchProfiles: cCtx.Bool("switch-profiles"),
							}), "failed to export wiring")
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
						Name:  "bgp",
						Usage: "Inspect BGP neighbors",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringSliceFlag{
								Name:    "switch-name",
								Aliases: []string{"switch", "s"},
								Usage:   "Switch names to inspect BGP neighbors for (if not specified, will inspect all switches)",
							},
							&cli.BoolFlag{
								Name:  "strict",
								Usage: "strict BGP check (will fail if any neighbor is missing, not expected or not established)",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.BGP, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.BGPIn{
								Switches: cCtx.StringSlice("switch-name"),
								Strict:   cCtx.Bool("strict"),
							}, os.Stdout), "failed to inspect BGP")
						},
					},
					{
						Name:  "lldp",
						Usage: "Inspect LLDP neighbors",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringSliceFlag{
								Name:    "switch-name",
								Aliases: []string{"switch", "s"},
								Usage:   "Switch names to inspect LLDP neighbors for (if not specified, will inspect all switches)",
							},
							&cli.BoolFlag{
								Name:  "strict",
								Usage: "strict LLDP check (will fail if any neighbor is missing or not as expected ignoring external ones)",
							},
							&cli.BoolFlag{
								Name:  "fabric",
								Usage: "include fabric neighbors (fabric, mclag-domain and vpcloopback connections)",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "external",
								Usage: "include external neighbors (external and staticexternal connections)",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "server",
								Usage: "include server neighbors (unbundled, bundled, eslag and mclag connections)",
								Value: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.LLDP, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.LLDPIn{
								Switches: cCtx.StringSlice("switch-name"),
								Strict:   cCtx.Bool("strict"),
								Fabric:   cCtx.Bool("fabric"),
								External: cCtx.Bool("external"),
								Server:   cCtx.Bool("server"),
							}, os.Stdout), "failed to inspect LLDP")
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

func wrapErrWithPressToContinue(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(os.Getenv("_"), "k9s") {
		slog.Error("Failed", "err", err.Error())
		slog.Warn("Press Enter to continue...")
		_, _ = fmt.Scanln()
	}

	return err
}
