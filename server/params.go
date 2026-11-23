package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// decodeArguments decodes the raw arguments of a Tool call into a map. Absent
// arguments and a JSON null both mean no arguments and give an empty map.
// Anything else must be a JSON object that decodes as a whole: a value that
// json cannot put in a Go value, such as the number 1e309 (out of float64
// range), is an error.
//
// The whole object is decoded into map[string]any, so every occurrence of a
// key is checked: in {"extra":1e309,"extra":0} the first value fails, although
// the last one wins in the map. Only the error text comes from a second pass
// over the keys in sorted order, which names the first key whose last value
// does not decode; when no last value fails (a duplicate key), the error does
// not name a key.
func decodeArguments(raw json.RawMessage) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return map[string]any{}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	var m map[string]any
	err := json.Unmarshal(trimmed, &m)
	if err == nil {
		return m, nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		var value any
		if fieldErr := json.Unmarshal(fields[key], &value); fieldErr != nil {
			return nil, fmt.Errorf("parameter %q does not decode: %w", key, fieldErr)
		}
	}
	return nil, fmt.Errorf("arguments do not decode: %w", err)
}

// rejectUndecodableArguments is receiving middleware for tools/call. It runs
// decodeArguments on the arguments of every Tool call and returns its error to
// the caller before the Tool handler runs, so before any middleware call. The
// accessors below read through parseArgs, which has no error return: without
// this check, arguments that do not decode would read as no arguments, and an
// optional filter (for example `dataset` of truenas_snapshot_task_list) would
// disappear.
func rejectUndecodableArguments(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
			if _, err := decodeArguments(call.Params.Arguments); err != nil {
				return nil, err
			}
		}
		return next(ctx, method, req)
	}
}

// parseArgs returns the decoded arguments of a Tool call. A server from New
// refuses arguments that do not decode before the handler runs
// (rejectUndecodableArguments), so a handler never sees the empty map that
// parseArgs returns on a decode error.
func parseArgs(req *mcp.CallToolRequest) map[string]any {
	m, err := decodeArguments(req.Params.Arguments)
	if err != nil {
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

// requireShareID reads a share deletion ID from raw JSON, without the float64
// rounding in parseArgs. Only integer literals are supported: decimal points and
// exponent notation are refused even when their numeric value is an integer.
// The signed int64 range is the Server's conversion bound, not a statement about
// which IDs the Appliance accepts.
func requireShareID(req *mcp.CallToolRequest) (int64, error) {
	var fields map[string]json.RawMessage
	if args := req.Params.Arguments; len(args) != 0 {
		if err := json.Unmarshal(args, &fields); err != nil {
			return 0, fmt.Errorf("reading share ID arguments: %w", err)
		}
	}
	rawID := bytes.TrimSpace(fields["id"])
	if len(rawID) == 0 || bytes.Equal(rawID, []byte("null")) {
		return 0, fmt.Errorf("required parameter %q missing", "id")
	}
	id, err := strconv.ParseInt(string(rawID), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parameter %q must be a JSON integer literal in the signed int64 range (no decimal point or exponent): %w", "id", err)
	}
	return id, nil
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
