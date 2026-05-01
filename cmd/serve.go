package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"truenas-mcp/server"
	"truenas-mcp/truenas"
)

type serveConfig struct {
	Host         string
	APIKey       string
	EnableWrites bool
	TLSInsecure  bool
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

			client, err := truenas.Connect(cfg.Host, cfg.APIKey, cfg.TLSInsecure)
			if err != nil {
				return fmt.Errorf("failed to connect to TrueNAS: %w", err)
			}
			defer client.Close()

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected and authenticated.\n")

			mcpServer := server.New(client, !cfg.EnableWrites)

			if cfg.EnableWrites {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting MCP server on stdio (writes enabled)...\n")
			} else {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Starting MCP server on stdio (read-only mode)...\n")
			}
			return server.Run(cmd.Context(), mcpServer)
		},
	}

	cmd.Flags().StringVar(&cfg.Host, "host", envOrDefault("TRUENAS_HOST", ""), "TrueNAS host address (e.g., truenas.local)")
	cmd.Flags().StringVar(&cfg.APIKey, "api-key", envOrDefault("TRUENAS_API_KEY", ""), "TrueNAS API key")
	cmd.Flags().BoolVar(&cfg.EnableWrites, "enable-writes", envBool("TRUENAS_ENABLE_WRITES", false), "Register tools that create, delete, or modify TrueNAS resources")
	cmd.Flags().BoolVar(&cfg.TLSInsecure, "tls-insecure", envBool("TRUENAS_TLS_INSECURE", false), "Skip TLS certificate verification when connecting to TrueNAS")

	return cmd
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	switch val {
	case "1", "t", "T", "true", "TRUE", "True", "y", "Y", "yes", "YES", "Yes", "on", "ON", "On":
		return true
	case "0", "f", "F", "false", "FALSE", "False", "n", "N", "no", "NO", "No", "off", "OFF", "Off":
		return false
	default:
		return fallback
	}
}
