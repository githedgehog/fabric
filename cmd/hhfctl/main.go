package main

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
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
	yesCheck := func(cCtx *cli.Context) error {
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.VPCCreate(ctx, printYaml, &hhfctl.VPCCreateOptions{
								Name:   name,
								Subnet: cCtx.String("subnet"),
								VLAN:   cCtx.String("vlan"),
								DHCP: vpcapi.VPCDHCP{
									Enable: cCtx.Bool("dhcp"),
									Range: &vpcapi.VPCDHCPRange{
										Start: cCtx.String("dhcp-range-start"),
										End:   cCtx.String("dhcp-range-end"),
									},
								},
							})
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.VPCAttach(ctx, printYaml, &hhfctl.VPCAttachOptions{
								Name:       name,
								VPCSubnet:  cCtx.String("vpc-subnet"),
								Connection: cCtx.String("connection"),
							})
						},
					},
					{
						Name:    "peer",
						Aliases: []string{"peering"},
						Usage:   "Peering connection between vpcs",
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.VPCPeer(ctx, printYaml, &hhfctl.VPCPeerOptions{
								Name:   name,
								VPCs:   cCtx.StringSlice("vpc"),
								Remote: cCtx.String("remote"),
							})
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}
							return hhfctl.SwitchReboot(ctx, yes, name)
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}
							return hhfctl.SwitchReinstall(ctx, yes, name)
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return err
							}
							return hhfctl.SwitchForceAgentVersion(ctx, yes, name, cCtx.String("version"))
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
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.ConnectionGet(ctx, &hhfctl.ConnectionGetOptions{
								Type: cCtx.Args().First(),
							})
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
						},
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.SwitchGroupCreate(ctx, printYaml, &hhfctl.SwitchGroupCreateOptions{
								Name: name,
							})
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
