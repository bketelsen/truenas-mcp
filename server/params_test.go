package server

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func makeReq(t *testing.T, fields map[string]any) *mcp.CallToolRequest {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: b,
		},
	}
}

func TestRequireString_present(t *testing.T) {
	req := makeReq(t, map[string]any{"name": "myapp"})
	got, err := requireString(req, "name")
	if err != nil || got != "myapp" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestRequireString_missing(t *testing.T) {
	req := makeReq(t, map[string]any{})
	_, err := requireString(req, "name")
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
}

func TestRequireString_empty(t *testing.T) {
	req := makeReq(t, map[string]any{"name": ""})
	_, err := requireString(req, "name")
	if err == nil {
		t.Fatal("expected error for empty required param")
	}
}

func TestOptionalString_present(t *testing.T) {
	req := makeReq(t, map[string]any{"level": "WARNING"})
	got := optionalString(req, "level")
	if got != "WARNING" {
		t.Fatalf("got %q", got)
	}
}

func TestOptionalString_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	got := optionalString(req, "level")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestRequireFloat64_present(t *testing.T) {
	req := makeReq(t, map[string]any{"id": float64(42)})
	got, err := requireFloat64(req, "id")
	if err != nil || got != 42 {
		t.Fatalf("got %v, err %v", got, err)
	}
}

func TestRequireFloat64_missing(t *testing.T) {
	req := makeReq(t, map[string]any{})
	_, err := requireFloat64(req, "id")
	if err == nil {
		t.Fatal("expected error for missing required float64 param")
	}
}

func TestOptionalBool_true(t *testing.T) {
	req := makeReq(t, map[string]any{"guest_ok": true})
	if !optionalBool(req, "guest_ok") {
		t.Fatal("expected true")
	}
}

func TestOptionalBool_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	if optionalBool(req, "guest_ok") {
		t.Fatal("expected false")
	}
}

func TestOptionalSlice_present(t *testing.T) {
	req := makeReq(t, map[string]any{"hosts": []any{"10.0.0.1"}})
	got := optionalSlice(req, "hosts")
	if len(got) != 1 {
		t.Fatalf("expected 1 element, got %v", got)
	}
}

func TestOptionalSlice_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	got := optionalSlice(req, "hosts")
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestOptionalFloat64_present(t *testing.T) {
	req := makeReq(t, map[string]any{"limit": float64(25)})
	got, ok := optionalFloat64(req, "limit")
	if !ok || got != 25 {
		t.Fatalf("got %v, ok %v", got, ok)
	}
}

func TestOptionalFloat64_absent(t *testing.T) {
	req := makeReq(t, map[string]any{})
	_, ok := optionalFloat64(req, "limit")
	if ok {
		t.Fatal("expected ok=false for absent field")
	}
}
