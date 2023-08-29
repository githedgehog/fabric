package main

import (
	"log"
	"os"

	cli "github.com/urfave/cli/v2"
)

var version = "(devel)"

func main() {
	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}

	app := &cli.App{
		Name:                   "hhf",
		Usage:                  "hedgehog fabric tools",
		Version:                version,
		Suggest:                true,
		UseShortOptionHandling: true,
		Commands: []*cli.Command{
			GetWiringCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal("Failed with error: ", err)
	}
}
