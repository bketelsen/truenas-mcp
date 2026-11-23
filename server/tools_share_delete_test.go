package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestShareDelete_ExactIntegerIDs(t *testing.T) {
	for _, kind := range []string{"smb", "nfs"} {
		for _, tc := range []struct {
			literal string
			want    int64
		}{
			{"-9223372036854775808", math.MinInt64},
			{"9223372036854775807", math.MaxInt64},
			{"9007199254740993", 9007199254740993},
			{"0", 0},
			{"-0", 0},
			{"1", 1},
			{"-1", -1},
		} {
			t.Run(kind+"/"+tc.literal, func(t *testing.T) {
				calls := 0
				mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
					calls++
					if method != "sharing."+kind+".delete" || len(params) != 1 {
						t.Errorf("call = %s %v, want sharing.%s.delete with 1 ID", method, params, kind)
						return nil, fmt.Errorf("unexpected middleware call")
					}
					id, ok := params[0].(int64)
					if !ok || id != tc.want {
						t.Errorf("ID = %v (%T), want int64(%d)", params[0], params[0], tc.want)
					}
					return json.RawMessage(`true`), nil
				}}
				args := json.RawMessage(`{"id": ` + tc.literal + ` }`)
				result, err := callToolRaw(t, mock, false, "truenas_"+kind+"_delete", args)
				if text := callErrorText(t, result, err); text != "" {
					t.Fatalf("unexpected error: %s", text)
				}
				if calls != 1 {
					t.Errorf("middleware calls = %d, want 1", calls)
				}
			})
		}
	}
}

func TestShareDelete_MissingID(t *testing.T) {
	for _, kind := range []string{"smb", "nfs"} {
		for _, tc := range []struct {
			name string
			args string
		}{
			{"omitted arguments", ``},
			{"null arguments", `,"arguments":null`},
			{"empty object", `,"arguments":{}`},
			{"null ID", `,"arguments":{"id":null}`},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				caller := &recordingCaller{}
				// Send the literal request: ClientSession.CallTool would replace omitted arguments with {}.
				request := openShareDeleteWire(t, caller)
				var reply struct {
					ID    int
					Error struct{ Message string }
				}
				request(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":%q%s}}`, "truenas_"+kind+"_delete", tc.args), &reply)
				if reply.ID != 2 {
					t.Errorf("response id = %d, want 2", reply.ID)
				}
				if got, want := reply.Error.Message, `required parameter "id" missing`; !strings.HasSuffix(got, want) {
					t.Errorf("error = %q, want %q", got, want)
				}
				if calls := caller.recorded(); len(calls) != 0 {
					t.Errorf("middleware calls = %v, want none", calls)
				}
			})
		}
	}
}

func TestShareDelete_RejectsInvalidIDs(t *testing.T) {
	for _, kind := range []string{"smb", "nfs"} {
		for _, tc := range []struct {
			name string
			args string
		}{
			{"below minimum", `{"id":-9223372036854775809}`},
			{"above maximum", `{"id":9223372036854775808}`},
			{"fraction", `{"id":1.5}`},
			{"negative fraction", `{"id":-1.5}`},
			{"decimal integer", `{"id":1.0}`},
			{"fraction rounds to integer", `{"id":1.00000000000000000001}`},
			{"fraction underflows", `{"id":0.` + strings.Repeat("0", 400) + `1}`},
			{"negative fraction underflows", `{"id":-0.` + strings.Repeat("0", 400) + `1}`},
			{"exponent integer", `{"id":1e3}`},
			{"zero exponent", `{"id":1e0}`},
			{"uppercase exponent", `{"id":1E0}`},
			{"positive exponent", `{"id":1e+0}`},
			{"negative exponent", `{"id":1e-1}`},
			{"exponent underflows", `{"id":1e-999}`},
			{"zero with large exponent", `{"id":0e999}`},
			{"exponent overflows", `{"id":1e309}`},
			{"string", `{"id":"1"}`},
			{"empty string", `{"id":""}`},
			{"true", `{"id":true}`},
			{"false", `{"id":false}`},
			{"array", `{"id":[1]}`},
			{"object", `{"id":{"value":1}}`},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				caller := &recordingCaller{}
				result, err := callToolRaw(t, caller, false, "truenas_"+kind+"_delete", json.RawMessage(tc.args))
				text := callErrorText(t, result, err)
				if !strings.Contains(text, `"id"`) {
					t.Errorf("error = %q, want an error naming id", text)
				}
				if calls := caller.recorded(); len(calls) != 0 {
					t.Errorf("middleware calls = %v, want none", calls)
				}
			})
		}
	}
}

func openShareDeleteWire(t *testing.T, caller *recordingCaller) func(string, any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	client, pipe := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() { _ = pipe.Close() })
	deadline, _ := ctx.Deadline()
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}

	s := New(caller, false)
	session, err := s.Connect(ctx, &mcp.IOTransport{Reader: pipe, Writer: pipe}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	// Read from the transport directly, before an MCP client can decode schema
	// numbers as float64 (which cannot represent MaxInt64).
	dec := json.NewDecoder(client)
	request := func(message string, reply any) {
		t.Helper()
		if _, err := fmt.Fprintln(client, message); err != nil {
			t.Fatal(err)
		}
		if reply != nil {
			if err := dec.Decode(reply); err != nil {
				t.Fatalf("decode response to %s: %v", message, err)
			}
		}
	}
	var initialized struct {
		Result json.RawMessage
		Error  json.RawMessage
	}
	request(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"wire-test","version":"1"}}}`, &initialized)
	if len(initialized.Result) == 0 || len(initialized.Error) != 0 {
		t.Fatalf("initialize: result=%s error=%s", initialized.Result, initialized.Error)
	}
	request(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	return request
}

func TestShareDelete_IDSchemaWireBounds(t *testing.T) {
	request := openShareDeleteWire(t, &recordingCaller{})
	var listed struct {
		ID     int
		Error  json.RawMessage
		Result struct {
			Tools []struct {
				Name        string
				InputSchema struct {
					Properties map[string]struct {
						Minimum json.RawMessage
						Maximum json.RawMessage
					}
				}
			}
		}
	}
	request(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, &listed)
	if listed.ID != 2 || len(listed.Error) != 0 {
		t.Fatalf("tools/list: id=%d error=%s", listed.ID, listed.Error)
	}
	for _, name := range []string{"truenas_smb_delete", "truenas_nfs_delete"} {
		t.Run(name, func(t *testing.T) {
			for _, tool := range listed.Result.Tools {
				if tool.Name == name {
					id := tool.InputSchema.Properties["id"]
					if string(id.Minimum) != "-9223372036854775808" || string(id.Maximum) != "9223372036854775807" {
						t.Errorf("wire bounds = [%s, %s], want [-9223372036854775808, 9223372036854775807]", id.Minimum, id.Maximum)
					}
					return
				}
			}
			t.Fatal("tool missing from tools/list response")
		})
	}
}

func TestShareDelete_IDSchema(t *testing.T) {
	// Check the source schema's exact JSON bounds before the SDK client decodes
	// schema numbers as float64. Client-side float64 cannot represent MaxInt64.
	raw, err := json.Marshal(shareIDProp("share ID"))
	if err != nil {
		t.Fatal(err)
	}
	var exact map[string]json.RawMessage
	if err := json.Unmarshal(raw, &exact); err != nil {
		t.Fatal(err)
	}
	if string(exact["minimum"]) != "-9223372036854775808" || string(exact["maximum"]) != "9223372036854775807" {
		t.Fatalf("schema bounds are not exact int64 limits: %s", raw)
	}

	tools := listToolDefs(t, &recordingCaller{}, false)
	for _, name := range []string{"truenas_smb_delete", "truenas_nfs_delete"} {
		t.Run(name, func(t *testing.T) {
			for _, tool := range tools {
				if tool.Name != name {
					continue
				}
				s, ok := tool.InputSchema.(map[string]any)
				if !ok {
					t.Fatalf("input schema is %T, want object", tool.InputSchema)
				}
				if !reflect.DeepEqual(s["required"], []any{"id"}) {
					t.Errorf("required = %v, want [id]", s["required"])
				}
				props := s["properties"].(map[string]any)
				id := props["id"].(map[string]any)
				if id["type"] != "integer" || id["minimum"] != float64(math.MinInt64) || id["maximum"] != float64(math.MaxInt64) {
					t.Errorf("ID schema = %v, want integer with signed int64 bounds", id)
				}
				desc, _ := id["description"].(string)
				for _, term := range []string{"JSON integer literal", "without a decimal point or exponent", "Server's conversion range", "not the Appliance's valid ID range"} {
					if !strings.Contains(desc, term) {
						t.Errorf("ID description %q must contain %q", desc, term)
					}
				}
				return
			}
			t.Fatal("tool not registered")
		})
	}
}
