package server

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSchema(t *testing.T) {
	s := schema(map[string]any{
		"name": stringProp("a name"),
	}, "name")

	if s["type"] != "object" {
		t.Errorf("type = %v, want object", s["type"])
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not map[string]any")
	}
	if _, ok := props["name"]; !ok {
		t.Error("properties missing 'name'")
	}
	req, ok := s["required"].([]string)
	if !ok {
		t.Fatal("required is not []string")
	}
	if len(req) != 1 || req[0] != "name" {
		t.Errorf("required = %v, want [name]", req)
	}
}

func TestSchema_NoRequired(t *testing.T) {
	s := schema(map[string]any{
		"name": stringProp("a name"),
	})

	if _, ok := s["required"]; ok {
		t.Error("expected no 'required' key when no required args given")
	}
}

func TestNoArgs(t *testing.T) {
	s := noArgs()
	if s["type"] != "object" {
		t.Errorf("type = %v, want object", s["type"])
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not map[string]any")
	}
	if len(props) != 0 {
		t.Errorf("properties has %d entries, want 0", len(props))
	}
}

func TestStringProp(t *testing.T) {
	p := stringProp("a description")
	if p["type"] != "string" {
		t.Errorf("type = %v, want string", p["type"])
	}
	if p["description"] != "a description" {
		t.Errorf("description = %v, want 'a description'", p["description"])
	}
}

func TestNumberProp(t *testing.T) {
	p := numberProp("a number")
	if p["type"] != "number" {
		t.Errorf("type = %v, want number", p["type"])
	}
}

func TestBoolProp(t *testing.T) {
	p := boolProp("a bool")
	if p["type"] != "boolean" {
		t.Errorf("type = %v, want boolean", p["type"])
	}
}

func TestArrayProp(t *testing.T) {
	p := arrayProp("a list")
	if p["type"] != "array" {
		t.Errorf("type = %v, want array", p["type"])
	}
	items, ok := p["items"].(map[string]any)
	if !ok {
		t.Fatal("items is not map[string]any")
	}
	if items["type"] != "string" {
		t.Errorf("items.type = %v, want string", items["type"])
	}
}

func TestArgs_Valid(t *testing.T) {
	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{"name":"tank","size":42}`),
	}

	a := args(req)
	if a["name"] != "tank" {
		t.Errorf("name = %v, want tank", a["name"])
	}
	if a["size"] != 42.0 {
		t.Errorf("size = %v, want 42", a["size"])
	}
}

func TestArgs_NilArguments(t *testing.T) {
	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{}
	a := args(req)
	if a == nil {
		t.Fatal("args returned nil, want empty map")
	}
	if len(a) != 0 {
		t.Errorf("args returned %d entries, want 0", len(a))
	}
}

func TestArgs_MalformedJSON(t *testing.T) {
	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{
		Arguments: json.RawMessage(`{invalid`),
	}

	a := args(req)
	if a == nil {
		t.Fatal("args returned nil, want empty map")
	}
	if len(a) != 0 {
		t.Errorf("args returned %d entries, want 0", len(a))
	}
}

func TestJsonResult(t *testing.T) {
	raw := json.RawMessage(`{"hostname":"nas","version":"24.04"}`)
	result, err := jsonResult(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if text == "" {
		t.Error("result text is empty")
	}
	// Verify it's pretty-printed (contains newlines)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if parsed["hostname"] != "nas" {
		t.Errorf("hostname = %v, want nas", parsed["hostname"])
	}
}

func TestJsonResult_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid`)
	_, err := jsonResult(raw)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
