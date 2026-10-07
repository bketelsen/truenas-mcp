package server

import (
	"encoding/json"
	"testing"
)

// toolHints is the expected safety classification of one tool. It repeats the
// Annotations in tools_*.go on purpose: adding a tool, or reclassifying one,
// means editing both places, and the README's "Tool Annotations" section with
// them. destructive and idempotent apply to write tools only.
type toolHints struct {
	readOnly    bool
	destructive bool
	idempotent  bool
}

// wantToolHints covers every tool the server registers with writes enabled.
var wantToolHints = map[string]toolHints{
	// Read-only tools: registered in both modes.
	"truenas_health_report":      {readOnly: true},
	"truenas_system_info":        {readOnly: true},
	"truenas_disk_list":          {readOnly: true},
	"truenas_network_list":       {readOnly: true},
	"truenas_pool_list":          {readOnly: true},
	"truenas_pool_get":           {readOnly: true},
	"truenas_dataset_list":       {readOnly: true},
	"truenas_dataset_get":        {readOnly: true},
	"truenas_snapshot_list":      {readOnly: true},
	"truenas_snapshot_get":       {readOnly: true},
	"truenas_smb_list":           {readOnly: true},
	"truenas_nfs_list":           {readOnly: true},
	"truenas_alert_list":         {readOnly: true},
	"truenas_app_list":           {readOnly: true},
	"truenas_app_get":            {readOnly: true},
	"truenas_app_config":         {readOnly: true},
	"truenas_apps_update_report": {readOnly: true},
	"truenas_jobs_list":          {readOnly: true},

	// Destructive writes: can lose data or settings, or are hard to undo.
	"truenas_dataset_delete":  {destructive: true, idempotent: true},
	"truenas_snapshot_delete": {destructive: true, idempotent: true},
	"truenas_smb_delete":      {destructive: true, idempotent: true},
	"truenas_nfs_delete":      {destructive: true, idempotent: true},
	"truenas_app_configure":   {destructive: true},
	"truenas_app_update":      {destructive: true, idempotent: true},
	"truenas_app_update_all":  {destructive: true, idempotent: true},

	// Non-destructive writes: additive or recoverable.
	"truenas_dataset_create":  {idempotent: true},
	"truenas_snapshot_create": {},
	"truenas_smb_create":      {idempotent: true},
	"truenas_nfs_create":      {},
	"truenas_alert_dismiss":   {idempotent: true},
	"truenas_app_start":       {},
	"truenas_app_stop":        {idempotent: true},
	"truenas_app_restart":     {},
}

func annotationsMock() *mockCaller {
	return &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
}

// TestToolAnnotations_Invariants checks the rules every tool must follow in
// each registration mode, so a new tool without annotations fails here.
func TestToolAnnotations_Invariants(t *testing.T) {
	mock := annotationsMock()

	inReadOnlyMode := map[string]bool{}
	for _, tool := range listToolDefs(t, mock, true) {
		inReadOnlyMode[tool.Name] = true
	}

	modes := []struct {
		name     string
		readOnly bool
	}{
		{name: "read-only", readOnly: true},
		{name: "with writes", readOnly: false},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			tools := listToolDefs(t, mock, mode.readOnly)
			if len(tools) == 0 {
				t.Fatal("no tools registered")
			}
			for _, tool := range tools {
				t.Run(tool.Name, func(t *testing.T) {
					a := tool.Annotations
					if a == nil {
						t.Fatal("Annotations is nil; every tool needs &mcp.ToolAnnotations{...}")
					}
					if a.Title == "" {
						t.Error("Annotations.Title is empty")
					}
					if a.OpenWorldHint == nil || *a.OpenWorldHint {
						t.Error("OpenWorldHint must be explicitly false: the server talks only to one appliance")
					}
					if inReadOnlyMode[tool.Name] {
						if !a.ReadOnlyHint {
							t.Error("registered in read-only mode but ReadOnlyHint is false")
						}
						return
					}
					if a.ReadOnlyHint {
						t.Error("registered only with writes but ReadOnlyHint is true")
					}
					if a.DestructiveHint == nil {
						t.Error("write tool must set DestructiveHint explicitly (the MCP default is true)")
					}
				})
			}
		})
	}
}

// TestToolAnnotations_Classification pins each tool's hints to wantToolHints,
// so reclassifying a tool, or adding one, is a deliberate, reviewed change.
func TestToolAnnotations_Classification(t *testing.T) {
	isWriteToolName := make(map[string]bool, len(writeToolNames))
	for _, name := range writeToolNames {
		isWriteToolName[name] = true
	}

	registered := map[string]bool{}
	for _, tool := range listToolDefs(t, annotationsMock(), false) {
		registered[tool.Name] = true
		t.Run(tool.Name, func(t *testing.T) {
			want, ok := wantToolHints[tool.Name]
			if !ok {
				t.Fatal("no entry in wantToolHints; classify the tool there and in the README's Tool Annotations section")
			}
			if !want.readOnly && !isWriteToolName[tool.Name] {
				t.Error("write tool missing from writeToolNames in mock_test.go")
			}
			a := tool.Annotations
			if a == nil {
				t.Fatal("Annotations is nil")
			}
			if a.ReadOnlyHint != want.readOnly {
				t.Errorf("ReadOnlyHint = %v, want %v", a.ReadOnlyHint, want.readOnly)
			}
			if want.readOnly {
				return
			}
			if a.DestructiveHint == nil {
				t.Fatalf("DestructiveHint is nil, want %v", want.destructive)
			}
			if *a.DestructiveHint != want.destructive {
				t.Errorf("DestructiveHint = %v, want %v", *a.DestructiveHint, want.destructive)
			}
			if a.IdempotentHint != want.idempotent {
				t.Errorf("IdempotentHint = %v, want %v", a.IdempotentHint, want.idempotent)
			}
		})
	}

	for name := range wantToolHints {
		if !registered[name] {
			t.Errorf("wantToolHints lists %q, which the server does not register", name)
		}
	}
}
