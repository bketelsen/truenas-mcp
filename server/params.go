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

// strictBool returns the boolean field, or def when the key is absent or null. A
// present value of another type is an error: the SDK does not validate arguments
// against InputSchema, so a string "false" would otherwise become the default.
func strictBool(req *mcp.CallToolRequest, field string, def bool) (bool, error) {
	m := parseArgs(req)
	v, present := m[field]
	if !present || v == nil {
		return def, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("parameter %q must be a boolean, got %T", field, v)
	}
	return b, nil
}

// strictStringList returns the string items of an array field, or nil when the
// key is absent or null. A present value that is not an array of strings is an
// error, so a mistyped list cannot be dropped silently.
func strictStringList(req *mcp.CallToolRequest, field string) ([]string, error) {
	m := parseArgs(req)
	v, present := m[field]
	if !present || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("parameter %q must be an array of strings, got %T", field, v)
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("parameter %q must contain only strings (item %d is %T)", field, i, item)
		}
		out = append(out, s)
	}
	return out, nil
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
