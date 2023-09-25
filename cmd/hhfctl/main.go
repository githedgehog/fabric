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
			TimeFormat: time.DateTime,
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
								Aliases:  []string{"s"},
								Usage:    "subnet",
								Required: true,
							},
							&cli.BoolFlag{
								Name:    "dhcp",
								Aliases: []string{"d"},
								Usage:   "enable dhcp",
							},
							&cli.StringFlag{
								Name:    "dhcp-range-start",
								Aliases: []string{"dhcp-start", "start"},
								Usage:   "dhcp range start",
							},
							&cli.StringFlag{
								Name:    "dhcp-range-end",
								Aliases: []string{"dhcp-end", "end"},
								Usage:   "dhcp range end",
							},
						},
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.VPCCreate(ctx, &hhfctl.VPCCreateOptions{
								Name:   name,
								Subnet: cCtx.String("subnet"),
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
								Name:     "vpc",
								Usage:    "vpc",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "connection",
								Aliases:  []string{"conn", "c"},
								Usage:    "connection",
								Required: true,
							},
						},
						Before: func(cCtx *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return hhfctl.VPCAttach(ctx, &hhfctl.VPCAttachOptions{
								Name:       name,
								VPC:        cCtx.String("vpc"),
								Connection: cCtx.String("connection"),
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
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Warn("Failed", "error", err)
		os.Exit(1)
	}
}
