package server

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func parseArgs(req *mcp.CallToolRequest) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func requireString(req *mcp.CallToolRequest, field string) (string, error) {
	m := parseArgs(req)
	v, ok := m[field].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("required parameter %q missing", field)
	}
	return v, nil
}

func optionalString(req *mcp.CallToolRequest, field string) string {
	m := parseArgs(req)
	v, _ := m[field].(string)
	return v
}

func requireFloat64(req *mcp.CallToolRequest, field string) (float64, error) {
	m := parseArgs(req)
	v, ok := m[field].(float64)
	if !ok {
		return 0, fmt.Errorf("required parameter %q missing", field)
	}
	return v, nil
}

func optionalFloat64(req *mcp.CallToolRequest, field string) (float64, bool) {
	m := parseArgs(req)
	v, ok := m[field].(float64)
	return v, ok && v > 0
}

func optionalBool(req *mcp.CallToolRequest, field string) bool {
	m := parseArgs(req)
	v, _ := m[field].(bool)
	return v
}

func optionalSlice(req *mcp.CallToolRequest, field string) []any {
	m := parseArgs(req)
	v, _ := m[field].([]any)
	return v
}

func requireObject(req *mcp.CallToolRequest, field string) (map[string]any, error) {
	m := parseArgs(req)
	v, ok := m[field].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("required parameter %q missing or not an object", field)
	}
	return v, nil
}
