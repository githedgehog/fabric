package wiring

import (
	"log"
	"strings"

	"github.com/urfave/cli/v2"
	"go.githedgehog.com/fabric/pkg/wiring"
	"go.githedgehog.com/fabric/pkg/wiring/cookiecutter"
	"go.githedgehog.com/fabric/pkg/wiring/prettier"
	"go.githedgehog.com/fabric/pkg/wiring/vlab"
)

func GetWiringCommand() *cli.Command {
	fromFlag := &cli.StringFlag{
		Name:    "from",
		Aliases: []string{"f"},
		Usage:   "load configs from the file/dir or use '-' for stdin",
	}

	return &cli.Command{
		Name:  "wiring",
		Usage: "wiring diagram tools",
		Subcommands: []*cli.Command{
			{
				Name:    "prettier",
				Aliases: []string{"p"},
				Usage:   "wiring diagram visualizer/prettifier",
				Subcommands: []*cli.Command{
					{
						Name:    "tree",
						Aliases: []string{"t"},
						Usage:   "print as a tree",
						Flags: []cli.Flag{
							fromFlag,
						},
						Action: func(cCtx *cli.Context) error {
							from := cCtx.String("from")

							data, err := wiring.LoadDataFrom(from)
							if err != nil {
								return err
							}
							logLoadedSummary(data)

							p := prettier.Prettier{Data: data}

							return p.PrintTree()
						},
					},
					{
						Name:    "dot",
						Aliases: []string{"d"},
						Usage:   "print as a dot",
						Flags: []cli.Flag{
							fromFlag,
						},
						Action: func(cCtx *cli.Context) error {
							from := cCtx.String("from")

							data, err := wiring.LoadDataFrom(from)
							if err != nil {
								return err
							}
							logLoadedSummary(data)

							p := prettier.Prettier{Data: data}

							return p.PrintDot()
						},
					},
				},
			},
			{
				Name:    "cookiecutter",
				Aliases: []string{"c", "cc"},
				Usage:   "generate wiring diagram",
				Subcommands: []*cli.Command{
					{
						Name:    "spineleaf",
						Aliases: []string{"sl"},
						Usage:   "generate spine leaf topology wiring",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:  "spines",
								Usage: "number of spines",
								Value: 2,
							},
							&cli.IntFlag{
								Name:  "leafs",
								Usage: "number of leafs",
								Value: 3,
							},
							&cli.IntFlag{
								Name:  "controls",
								Usage: "number of control nodes",
								Value: 1,
							},
							&cli.IntFlag{
								Name:  "computes",
								Usage: "number of computes (test nodes)",
								Value: 3,
							},
							&cli.StringSliceFlag{
								Name:  "links",
								Usage: "links between devices, ':'-separated",
							},
						},
						Action: func(cCtx *cli.Context) error {
							links := []cookiecutter.Link{}

							if linksFlag := cCtx.StringSlice("links"); linksFlag != nil {
								for _, link := range linksFlag {
									parts := strings.Split(link, ":")
									if len(parts) != 2 {
										log.Fatalf("incorrect link: %s", link)
									}
									links = append(links, cookiecutter.Link([2]string{
										parts[0], parts[1],
									}))
								}
							}

							return cookiecutter.GenerateSpineLeaf(&cookiecutter.SpineLeaf{
								Spines:   cCtx.Int("spines"),
								Leafs:    cCtx.Int("leafs"),
								Controls: cCtx.Int("controls"),
								Computes: cCtx.Int("computes"),
								Links:    links,
							})
						},
					},
				},
			},
			{
				Name:    "vlab",
				Aliases: []string{"v"},
				Usage:   "generate vlab config",
				Flags: []cli.Flag{
					fromFlag,
				},
				Action: func(cCtx *cli.Context) error {
					from := cCtx.String("from")

					data, err := wiring.LoadDataFrom(from)
					if err != nil {
						return err
					}
					logLoadedSummary(data)

					return vlab.PrintConfig(data)
				},
			},
		},
	}
}

func logLoadedSummary(data *wiring.Data) {
	log.Printf("Loaded %d rack(s), %d switch(es), %d port(s)", data.Rack.Size(), data.Switch.Size(), data.Port.Size())
}
