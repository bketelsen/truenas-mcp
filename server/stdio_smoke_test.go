package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const stdioSmokeChildEnv = "TRUENAS_MCP_STDIO_SMOKE_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(stdioSmokeChildEnv) == "1" {
		mock := &mockCaller{
			CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
				return json.RawMessage(`null`), nil
			},
		}
		server := New(mock, true)
		if err := Run(context.Background(), server); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestStdioSmoke_ReadOnlyToolList(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.Command(os.Args[0], "-test.run=TestStdioSmoke_ReadOnlyToolList")
	cmd.Env = append(os.Environ(), stdioSmokeChildEnv+"=1")

	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-smoke-test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command:           cmd,
		TerminateDuration: time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("connect stdio command transport: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	tools := map[string]bool{}
	for _, tool := range result.Tools {
		tools[tool.Name] = true
	}

	writeTools := []string{
		"truenas_dataset_create",
		"truenas_dataset_delete",
		"truenas_snapshot_create",
		"truenas_snapshot_delete",
		"truenas_smb_create",
		"truenas_smb_delete",
		"truenas_nfs_create",
		"truenas_nfs_delete",
		"truenas_alert_dismiss",
		"truenas_app_start",
		"truenas_app_stop",
		"truenas_app_restart",
	}
	for _, name := range writeTools {
		if tools[name] {
			t.Fatalf("read-only stdio server registered write tool %q", name)
		}
	}

	readTools := []string{
		"truenas_health_report",
		"truenas_system_info",
		"truenas_pool_list",
		"truenas_dataset_list",
		"truenas_apps_update_report",
		"truenas_jobs_list",
	}
	for _, name := range readTools {
		if !tools[name] {
			t.Fatalf("read-only stdio server missing read tool %q", name)
		}
	}
}
