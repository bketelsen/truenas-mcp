package cmd

import (
	"github.com/spf13/cobra"
	"truenas-mcp/version"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "truenas-mcp",
		Short:   "MCP server for TrueNAS SCALE",
		Long:    "An MCP (Model Context Protocol) server that exposes TrueNAS SCALE management capabilities to AI assistants.",
		Version: version.Version,
	}

	cmd.AddCommand(NewServeCmd())

	return cmd
}
