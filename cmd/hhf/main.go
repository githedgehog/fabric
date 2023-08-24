package main

import (
	"log"
	"os"

	cli "github.com/urfave/cli/v2"

	"go.githedgehog.com/fabric/cmd/hhf/wiring"
)

func main() {
	app := &cli.App{
		Name:  "hhf",
		Usage: "hedgehog fabric tools",
		Commands: []*cli.Command{
			wiring.GetWiringCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
