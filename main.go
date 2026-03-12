package main

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"truenas-mcp/cmd"
)

func main() {
	root := cmd.NewRootCmd()

	if err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion("0.1.0"),
		fang.WithNotifySignal(os.Interrupt),
	); err != nil {
		os.Exit(1)
	}
}
