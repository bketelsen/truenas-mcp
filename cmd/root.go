package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "truenas-mcp",
		Short:   "MCP server for TrueNAS SCALE",
		Long:    "An MCP (Model Context Protocol) server that exposes TrueNAS SCALE management capabilities to AI assistants.",
		Version: "0.1.0",
	}

	cmd.AddCommand(NewServeCmd())

	return cmd
}
