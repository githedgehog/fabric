package main

import (
	"context"

	"go.githedgehog.com/fabric/pkg/agent/gnmi"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := gnmi.New(ctx, gnmi.DEFAULT_ADDRESS, "admin", "HH.Labs!")
	if err != nil {
		panic(err)
	}
	defer client.Close()

	err = client.Set(ctx, gnmi.EntPortChannel("PortChannel1", "PortChannel1", "1..4094"))
	if err != nil {
		panic(err)
	}
}
