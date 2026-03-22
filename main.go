package main

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"truenas-mcp/cmd"
	"truenas-mcp/version"
)

func main() {
	root := cmd.NewRootCmd()

	if err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(version.Version),
		fang.WithNotifySignal(os.Interrupt),
	); err != nil {
		os.Exit(1)
	}
}
