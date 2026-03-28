package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"truenas-mcp/server"
	"truenas-mcp/truenas"
)

type serveConfig struct {
	Host     string
	APIKey   string
	ReadOnly bool
	Timeout  int
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

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to TrueNAS at %s...\n", cfg.Host)

			client, err := truenas.Connect(cfg.Host, cfg.APIKey, cfg.Timeout)
			if err != nil {
				return fmt.Errorf("failed to connect to TrueNAS: %w", err)
			}
			defer client.Close()

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected and authenticated.\n")

			mcpServer := server.New(client, cfg.ReadOnly)

			if cfg.ReadOnly {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting MCP server on stdio (read-only mode)...\n")
			} else {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting MCP server on stdio...\n")
			}
			return server.Run(cmd.Context(), mcpServer)
		},
	}

	cmd.Flags().StringVar(&cfg.Host, "host", envOrDefault("TRUENAS_HOST", ""), "TrueNAS host address (e.g., truenas.local)")
	cmd.Flags().StringVar(&cfg.APIKey, "api-key", envOrDefault("TRUENAS_API_KEY", ""), "TrueNAS API key")
	cmd.Flags().BoolVar(&cfg.ReadOnly, "read-only", envOrDefault("TRUENAS_READ_ONLY", "") != "", "Restrict to read-only tools (no create/delete/update operations)")
	cmd.Flags().IntVar(&cfg.Timeout, "timeout", envOrDefaultInt("TRUENAS_TIMEOUT", 30), "Per-call timeout in seconds for TrueNAS API requests")

	return cmd
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return fallback
}
