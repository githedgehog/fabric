package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/wiring/sample"
	"golang.org/x/exp/slices"
)

func GetWiringCommand() *cli.Command {
	from := "-"
	fromFlag := &cli.StringFlag{
		Name:        "from",
		Aliases:     []string{"f"},
		Usage:       "load from the `FILE` (or dir), use '-' for stdin",
		Value:       "-",
		Destination: &from,
	}

	topologyType := ""
	topologyTypeFlag := &cli.StringFlag{
		Name:        "type",
		Aliases:     []string{"t"},
		Usage:       "topology `TYPE`",
		Value:       "collapsedcore",
		Destination: &topologyType,
		Action: func(ctx *cli.Context, v string) error {
			if v != "collapsedcore" {
				return fmt.Errorf("topology type '%s' isn't supported", v)
			}

			return nil
		},
	}

	preset := ""
	presetFlag := &cli.StringFlag{
		Name:        "preset",
		Aliases:     []string{"p"},
		Usage:       "preset `TYPE`",
		Value:       string(sample.SAMPLE_CC_VLAB_MGMT),
		Destination: &preset,
		Action: func(ctx *cli.Context, v string) error {
			if !slices.Contains(sample.PresetsAll, sample.Preset(preset)) {
				return fmt.Errorf("preset '%s' isn't supported", v)
			}

			return nil
		},
	}

	return &cli.Command{
		Name:    "wiring",
		Aliases: []string{"w"},
		Usage:   "wiring diagram tools",
		Subcommands: []*cli.Command{
			{
				Name:    "sample",
				Aliases: []string{"s"},
				Usage:   "wiring diagram sample for specified topology",
				Flags: []cli.Flag{
					topologyTypeFlag,
					presetFlag,
				},
				Action: func(cCtx *cli.Context) error {
					log.Println("Generating sample type", topologyType, "preset", preset)
					data, err := sample.CollapsedCore(sample.Preset(preset))
					if err != nil {
						return err
					}

					return data.Write(os.Stdout)
				},
			},
			{
				Name:    "graph",
				Aliases: []string{"g"},
				Usage:   "wiring diagram graph (dot) for specified topology",
				Flags: []cli.Flag{
					topologyTypeFlag,
					fromFlag,
				},
				Action: func(cCtx *cli.Context) error {
					log.Println("Generating graph for", topologyType, "from", from)
					return errors.New("not implemented")
				},
			},
		},
	}
}
