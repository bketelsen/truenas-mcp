package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"truenas-mcp/truenas"
)

// recordingCaller answers every middleware call with an empty list and records
// the method and the params of each call. The server calls it from its own
// goroutine, so it holds a lock.
type recordingCaller struct {
	mu    sync.Mutex
	calls []recordedCall
}

type recordedCall struct {
	method string
	params []any
}

func (r *recordingCaller) Call(method string, params ...interface{}) (json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{method: method, params: params})
	return json.RawMessage("[]"), nil
}

func (r *recordingCaller) recorded() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedCall(nil), r.calls...)
}

var _ truenas.Caller = (*recordingCaller)(nil)

// callToolRaw is callTool with arguments of any type, so a test can send
// arguments that are not a JSON object, or no arguments at all.
func callToolRaw(t *testing.T, caller truenas.Caller, readOnly bool, toolName string, args any) (*mcp.CallToolResult, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	s := New(caller, readOnly)
	ct, st := mcp.NewInMemoryTransports()

	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	c := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	cs, err := c.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	return cs.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: args})
}

// callErrorText returns the error text of a Tool call: the protocol error, or
// the text of a result with IsError set. It is empty for a call that succeeded.
func callErrorText(t *testing.T, result *mcp.CallToolResult, err error) string {
	t.Helper()
	if err != nil {
		return err.Error()
	}
	if result != nil && result.IsError {
		return resultText(t, result)
	}
	return ""
}

// undecodable holds Tool arguments that are valid JSON but do not decode into
// map[string]any: 1e309 is out of float64 range.
func undecodable(fields map[string]any) map[string]any {
	args := map[string]any{"extra": json.RawMessage("1e309")}
	for key, value := range fields {
		args[key] = value
	}
	return args
}

func TestDecodeArguments(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    map[string]any
		wantErr string
	}{
		{raw: "", want: map[string]any{}},
		{raw: "null", want: map[string]any{}},
		{raw: " null\n", want: map[string]any{}},
		{raw: "{}", want: map[string]any{}},
		{raw: `{"dataset":"tank/x","n":2}`, want: map[string]any{"dataset": "tank/x", "n": float64(2)}},
		{raw: `{"dataset":null}`, want: map[string]any{"dataset": nil}},
		{raw: "[1]", wantErr: "arguments must be a JSON object"},
		{raw: `"tank/x"`, wantErr: "arguments must be a JSON object"},
		{raw: "1", wantErr: "arguments must be a JSON object"},
		{raw: `{"dataset":"tank/x","extra":1e309}`, wantErr: `parameter "extra" does not decode`},
		{raw: `{"extra":-1e309}`, wantErr: `parameter "extra" does not decode`},
		{raw: `{"outer":{"inner":[1e309]}}`, wantErr: `parameter "outer" does not decode`},
		// Keys are checked in sorted order, so the error names the same key on every call.
		{raw: `{"zz":1e309,"aa":1e309}`, wantErr: `parameter "aa" does not decode`},
		// Every occurrence of a duplicate key is checked, not only the last one.
		{raw: `{"name":"tank/x","extra":1e309,"extra":0}`, wantErr: "arguments do not decode"},
		{raw: `{"extra":0,"extra":1e309}`, wantErr: `parameter "extra" does not decode`},
		{raw: `{"extra":1,"extra":2}`, want: map[string]any{"extra": float64(2)}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := decodeArguments(json.RawMessage(tc.raw))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want none", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for key, want := range tc.want {
				if value, ok := got[key]; !ok || value != want {
					t.Fatalf("got[%q] = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}

// A read Tool with an optional filter must refuse arguments that do not decode.
// Before the fix, parseArgs dropped the decode error, so the dataset filter
// disappeared and pool.snapshottask.query ran with no filter.
func TestUndecodableArguments_ReadToolKeepsFilter(t *testing.T) {
	caller := &recordingCaller{}
	result, err := callTool(t, caller, true, "truenas_snapshot_task_list",
		undecodable(map[string]any{"dataset": "tank/x"}))
	text := callErrorText(t, result, err)
	if !strings.Contains(text, `parameter "extra" does not decode`) {
		t.Fatalf("error = %q, want the decode error for parameter \"extra\"", text)
	}
	if calls := caller.recorded(); len(calls) != 0 {
		t.Fatalf("middleware calls = %v, want none", calls)
	}
}

// A write Tool must name the decode failure, not a missing parameter.
func TestUndecodableArguments_WriteTool(t *testing.T) {
	caller := &recordingCaller{}
	result, err := callTool(t, caller, false, "truenas_dataset_create",
		undecodable(map[string]any{"name": "tank/x"}))
	text := callErrorText(t, result, err)
	if !strings.Contains(text, `parameter "extra" does not decode`) {
		t.Fatalf("error = %q, want the decode error for parameter \"extra\"", text)
	}
	if calls := caller.recorded(); len(calls) != 0 {
		t.Fatalf("middleware calls = %v, want none", calls)
	}
}

// A duplicate key whose earlier value does not decode must not pass: the map
// keeps only the last value, so a check of that value alone would let the call
// reach the handler. The write Tool must make no middleware call.
func TestUndecodableArguments_DuplicateKey(t *testing.T) {
	for _, tc := range []struct {
		tool     string
		readOnly bool
		args     json.RawMessage
	}{
		{tool: "truenas_dataset_create", args: json.RawMessage(`{"name":"tank/x","extra":1e309,"extra":0}`)},
		{tool: "truenas_snapshot_task_list", readOnly: true, args: json.RawMessage(`{"dataset":1e309,"dataset":"tank/x"}`)},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			caller := &recordingCaller{}
			result, err := callToolRaw(t, caller, tc.readOnly, tc.tool, tc.args)
			text := callErrorText(t, result, err)
			if !strings.Contains(text, "arguments do not decode") {
				t.Fatalf("error = %q, want an error that the arguments do not decode", text)
			}
			if calls := caller.recorded(); len(calls) != 0 {
				t.Fatalf("middleware calls = %v, want none", calls)
			}
		})
	}
}

// Every registered Tool, read and write, refuses arguments that do not decode
// before any middleware call.
func TestUndecodableArguments_EveryTool(t *testing.T) {
	names := listTools(t, &recordingCaller{}, false)
	if len(names) == 0 {
		t.Fatal("no tools registered")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			caller := &recordingCaller{}
			result, err := callTool(t, caller, false, name, undecodable(nil))
			text := callErrorText(t, result, err)
			if !strings.Contains(text, `parameter "extra" does not decode`) {
				t.Fatalf("error = %q, want the decode error for parameter \"extra\"", text)
			}
			if calls := caller.recorded(); len(calls) != 0 {
				t.Fatalf("middleware calls = %v, want none", calls)
			}
		})
	}
}

// Arguments that are not a JSON object are refused before any middleware call.
func TestNonObjectArguments(t *testing.T) {
	for _, args := range []json.RawMessage{json.RawMessage("[1]"), json.RawMessage(`"tank/x"`), json.RawMessage("7")} {
		t.Run(string(args), func(t *testing.T) {
			caller := &recordingCaller{}
			result, err := callToolRaw(t, caller, true, "truenas_snapshot_task_list", args)
			text := callErrorText(t, result, err)
			if !strings.Contains(text, "arguments must be a JSON object") {
				t.Fatalf("error = %q, want an error about a JSON object", text)
			}
			if calls := caller.recorded(); len(calls) != 0 {
				t.Fatalf("middleware calls = %v, want none", calls)
			}
		})
	}
}

// Absent and null arguments keep their meaning: no arguments, so no filter.
// A decodable filter still reaches the query.
func TestDecodableArguments_KeepMeaning(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       any
		wantParams int
	}{
		{name: "absent", args: nil, wantParams: 0},
		{name: "null", args: json.RawMessage("null"), wantParams: 0},
		{name: "empty object", args: map[string]any{}, wantParams: 0},
		{name: "null filter", args: map[string]any{"dataset": nil}, wantParams: 0},
		{name: "filter", args: map[string]any{"dataset": "tank/x"}, wantParams: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &recordingCaller{}
			result, err := callToolRaw(t, caller, true, "truenas_snapshot_task_list", tc.args)
			if text := callErrorText(t, result, err); text != "" {
				t.Fatalf("error = %q, want none", text)
			}
			calls := caller.recorded()
			if len(calls) != 1 || calls[0].method != "pool.snapshottask.query" {
				t.Fatalf("middleware calls = %v, want 1 pool.snapshottask.query", calls)
			}
			if len(calls[0].params) != tc.wantParams {
				t.Fatalf("query params = %v, want %d", calls[0].params, tc.wantParams)
			}
		})
	}
}
