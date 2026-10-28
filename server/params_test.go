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

func TestStrictBool_present(t *testing.T) {
	req := makeReq(t, map[string]any{"enabled": false, "allow_empty": true})
	if got, err := strictBool(req, "enabled", true); err != nil || got {
		t.Errorf("explicit false must win over a default of true: got %v, err %v", got, err)
	}
	if got, err := strictBool(req, "allow_empty", false); err != nil || !got {
		t.Errorf("explicit true must win over a default of false: got %v, err %v", got, err)
	}
}

func TestStrictBool_absent(t *testing.T) {
	req := makeReq(t, map[string]any{"dry_run": nil})
	if got, err := strictBool(req, "enabled", true); err != nil || !got {
		t.Errorf("absent key must return the default true: got %v, err %v", got, err)
	}
	if got, err := strictBool(req, "enabled", false); err != nil || got {
		t.Errorf("absent key must return the default false: got %v, err %v", got, err)
	}
	if got, err := strictBool(req, "dry_run", true); err != nil || !got {
		t.Errorf("null must count as absent: got %v, err %v", got, err)
	}
}

func TestStrictBool_wrongType(t *testing.T) {
	for _, value := range []any{"true", "false", float64(1), []any{true}} {
		req := makeReq(t, map[string]any{"dry_run": value})
		got, err := strictBool(req, "dry_run", false)
		if err == nil {
			t.Errorf("strictBool(%v) = %v, want an error", value, got)
		}
	}
}

func TestStrictStringList_present(t *testing.T) {
	req := makeReq(t, map[string]any{"exclude": []any{"tank/a", "tank/b"}})
	got, err := strictStringList(req, "exclude")
	if err != nil || len(got) != 2 || got[0] != "tank/a" || got[1] != "tank/b" {
		t.Fatalf("got %v, err %v", got, err)
	}
}

func TestStrictStringList_absent(t *testing.T) {
	for _, fields := range []map[string]any{{}, {"exclude": nil}} {
		got, err := strictStringList(makeReq(t, fields), "exclude")
		if err != nil || got != nil {
			t.Errorf("got %v, err %v, want nil and no error", got, err)
		}
	}
}

func TestStrictStringList_wrongType(t *testing.T) {
	for _, value := range []any{"tank/a", float64(3), []any{float64(1)}, []any{true}} {
		if _, err := strictStringList(makeReq(t, map[string]any{"exclude": value}), "exclude"); err == nil {
			t.Errorf("strictStringList(%v) gave no error", value)
		}
	}
}
