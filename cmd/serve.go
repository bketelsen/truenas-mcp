package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"truenas-mcp/server"
	"truenas-mcp/truenas"
)

type serveConfig struct {
	Host     string
	APIKey   string
	ReadOnly bool
}

func NewServeCmd() *cobra.Command {
	cfg := &serveConfig{}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server",
		Long:  "Start the MCP server over stdio, connecting to the configured TrueNAS SCALE instance.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Host == "" {
				return fmt.Errorf("TrueNAS host is required (use --host or TRUENAS_HOST env var)")
			}
			if cfg.APIKey == "" {
				return fmt.Errorf("TrueNAS API key is required (use --api-key or TRUENAS_API_KEY env var)")
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to TrueNAS at %s...\n", cfg.Host)

			client, err := truenas.Connect(cfg.Host, cfg.APIKey)
			if err != nil {
				return fmt.Errorf("failed to connect to TrueNAS: %w", err)
			}
			defer client.Close()

			fmt.Fprintf(cmd.ErrOrStderr(), "Connected and authenticated.\n")

			mcpServer := server.New(client, cfg.ReadOnly)

			if cfg.ReadOnly {
				fmt.Fprintf(cmd.ErrOrStderr(), "Starting MCP server on stdio (read-only mode)...\n")
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "Starting MCP server on stdio...\n")
			}
			return server.Run(cmd.Context(), mcpServer)
		},
	}

	cmd.Flags().StringVar(&cfg.Host, "host", envOrDefault("TRUENAS_HOST", ""), "TrueNAS host address (e.g., truenas.local)")
	cmd.Flags().StringVar(&cfg.APIKey, "api-key", envOrDefault("TRUENAS_API_KEY", ""), "TrueNAS API key")
	cmd.Flags().BoolVar(&cfg.ReadOnly, "read-only", envOrDefault("TRUENAS_READ_ONLY", "") != "", "Restrict to read-only tools (no create/delete/update operations)")

	return cmd
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
