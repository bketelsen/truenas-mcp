package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type numericConfigureCalls struct {
	methods []string
	updates []json.RawMessage
}

func numericConfigureMock(t *testing.T, current string) (*mockCaller, *numericConfigureCalls) {
	t.Helper()
	calls := &numericConfigureCalls{}
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		calls.methods = append(calls.methods, method)
		switch method {
		case "app.query":
			return json.RawMessage(`[{"name":"test-app","custom_app":false}]`), nil
		case "app.config":
			return json.RawMessage(current), nil
		case "app.update":
			if len(params) != 2 || params[0] != "test-app" {
				t.Errorf("app.update params = %v", params)
				return nil, fmt.Errorf("unexpected update parameters")
			}
			raw, err := json.Marshal(params[1])
			if err != nil {
				return nil, err
			}
			calls.updates = append(calls.updates, raw)
			return json.RawMessage(`4242`), nil
		default:
			return nil, fmt.Errorf("unexpected method %q", method)
		}
	}}
	return mock, calls
}

// assertExactConfigJSON compares JSON structure with json.Number tokens. Unlike
// a float64 comparison, this also checks every number's exact spelling.
func assertExactConfigJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	decode := func(raw []byte) any {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatalf("invalid test JSON: %v\n%s", err, raw)
		}
		return value
	}
	if !reflect.DeepEqual(decode(got), decode([]byte(want))) {
		t.Errorf("JSON = %s\nwant = %s", got, want)
	}
}

func TestAppConfigure_PreservesExactNumericTokens(t *testing.T) {
	const current = `{
		"section":{"large":9007199254740993,"adjacent":9007199254740992,
		"decimal":0.12345678901234567890123456789,"tiny":1e-400,
		"nested":[9007199254740993,{"decimal":0.12345678901234567890123456789,"tiny":1e-400}]},
		"untouched":{"number":9007199254740993,"extreme":1e999999999999999999999999999999},
		"ix_context":{"discard":9007199254740993}
	}`
	const values = `{"section":{"adjacent":9007199254740993,
		"decimal":0.12345678901234567890123456790,"tiny":2e-400,
		"nested":[9007199254740995,{"decimal":0.12345678901234567890123456790,"tiny":2e-400}]}}`
	const wantValues = `{
		"section":{"large":9007199254740993,"adjacent":9007199254740993,
		"decimal":0.12345678901234567890123456790,"tiny":2e-400,
		"nested":[9007199254740995,{"decimal":0.12345678901234567890123456790,"tiny":2e-400}]},
		"untouched":{"number":9007199254740993,"extreme":1e999999999999999999999999999999}
	}`
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry_run=%t", dryRun), func(t *testing.T) {
			mock, calls := numericConfigureMock(t, current)
			args := json.RawMessage(fmt.Sprintf(`{"name":"test-app","values":%s,"dry_run":%t}`, values, dryRun))
			result, err := callToolRaw(t, mock, false, "truenas_app_configure", args)
			if text := callErrorText(t, result, err); text != "" {
				t.Fatalf("unexpected error: %s", text)
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal([]byte(resultText(t, result)), &response); err != nil {
				t.Fatal(err)
			}
			assertExactConfigJSON(t, response["changed_paths"], `["section.adjacent","section.decimal","section.nested","section.tiny"]`)
			if dryRun {
				if len(calls.updates) != 0 {
					t.Fatal("dry run reached app.update")
				}
				assertExactConfigJSON(t, response["values"], wantValues)
			} else {
				if len(calls.updates) != 1 {
					t.Fatalf("updates = %d, want 1", len(calls.updates))
				}
				assertExactConfigJSON(t, calls.updates[0], `{"values":`+wantValues+`}`)
			}
		})
	}
}

func TestAppConfigure_AdjacentLargeIntegerIsAChange(t *testing.T) {
	mock, calls := numericConfigureMock(t, `{"n":9007199254740992}`)
	result, err := callToolRaw(t, mock, false, "truenas_app_configure", json.RawMessage(`{"name":"test-app","values":{"n":9007199254740993}}`))
	if text := callErrorText(t, result, err); text != "" {
		t.Fatalf("unexpected error: %s", text)
	}
	if len(calls.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(calls.updates))
	}
	assertExactConfigJSON(t, calls.updates[0], `{"values":{"n":9007199254740993}}`)
}

func TestAppConfigure_NumericNoOps(t *testing.T) {
	for _, spelling := range []string{"1", "1.0", "1e0"} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry_run=%t", spelling, dryRun), func(t *testing.T) {
				mock, calls := numericConfigureMock(t, `{"n":1,"nested":[1,{"numbers":[1.0,1e0]}],"large":9007199254740993}`)
				values := fmt.Sprintf(`{"n":%s,"nested":[%s,{"numbers":[%s,%s]}],"large":9007199254740993}`, spelling, spelling, spelling, spelling)
				args := json.RawMessage(fmt.Sprintf(`{"name":"test-app","values":%s,"dry_run":%t}`, values, dryRun))
				result, err := callToolRaw(t, mock, false, "truenas_app_configure", args)
				text := callErrorText(t, result, err)
				if len(calls.updates) != 0 {
					t.Fatal("numeric no-op reached app.update")
				}
				if !dryRun {
					if !strings.Contains(text, "identical") {
						t.Fatalf("error = %q, want no-op refusal", text)
					}
					return
				}
				if text != "" {
					t.Fatalf("unexpected error: %s", text)
				}
				var response map[string]json.RawMessage
				if err := json.Unmarshal([]byte(resultText(t, result)), &response); err != nil {
					t.Fatal(err)
				}
				assertExactConfigJSON(t, response["values"], values)
				var paths []string
				if err := json.Unmarshal(response["changed_paths"], &paths); err != nil || len(paths) != 0 {
					t.Errorf("changed paths = %s, error = %v, want none", response["changed_paths"], err)
				}
			})
		}
	}
}

func TestAppConfigure_ExtremeExponentNoOps(t *testing.T) {
	for _, tc := range []struct {
		before string
		after  string
	}{
		{"1e-400", "10e-401"},
		{"1e-999999999999999999999999999999", "10e-1000000000000000000000000000000"},
		{"0", "-0e999999999999999999999999999999"},
	} {
		t.Run(tc.after, func(t *testing.T) {
			mock, calls := numericConfigureMock(t, `{"n":`+tc.before+`}`)
			args := json.RawMessage(`{"name":"test-app","values":{"n":` + tc.after + `}}`)
			result, err := callToolRaw(t, mock, false, "truenas_app_configure", args)
			if text := callErrorText(t, result, err); !strings.Contains(text, "identical") {
				t.Errorf("error = %q, want exact numeric no-op refusal", text)
			}
			if !reflect.DeepEqual(calls.methods, []string{"app.query", "app.config"}) {
				t.Errorf("middleware calls = %v, want query and config only", calls.methods)
			}
		})
	}
}

func TestAppConfigure_InvalidStoredConfig(t *testing.T) {
	for _, current := range []string{`null`, `[]`, `1`, `"text"`, ``, `{"n":}`, `{"n":1} {}`, `{"n":1} null`, `{"n":1} garbage`} {
		t.Run(current, func(t *testing.T) {
			mock, calls := numericConfigureMock(t, current)
			result, err := callToolRaw(t, mock, false, "truenas_app_configure", json.RawMessage(`{"name":"test-app","values":{"n":2}}`))
			if text := callErrorText(t, result, err); !strings.Contains(text, "parsing app.config") {
				t.Errorf("error = %q, want stored config decode refusal", text)
			}
			if !reflect.DeepEqual(calls.methods, []string{"app.query", "app.config"}) {
				t.Errorf("middleware calls = %v, want query and config only", calls.methods)
			}
		})
	}
}

func TestAppConfigure_LosslessDecodeKeepsArgumentGuard(t *testing.T) {
	for _, args := range []string{
		`{"name":"test-app","values":{"n":1e309}}`,
		`{"name":"test-app","values":{"n":1e309,"n":0}}`,
		`{"name":"test-app","values":{"n":1e309},"values":{"n":0}}`,
		`{"name":"test-app","values":{"n":0},"extra":1e309,"extra":0}`,
	} {
		t.Run(args, func(t *testing.T) {
			caller := &recordingCaller{}
			result, err := callToolRaw(t, caller, false, "truenas_app_configure", json.RawMessage(args))
			if text := callErrorText(t, result, err); !strings.Contains(text, "does not decode") && !strings.Contains(text, "arguments do not decode") {
				t.Errorf("error = %q, want retained global decode rejection", text)
			}
			if calls := caller.recorded(); len(calls) != 0 {
				t.Errorf("middleware calls = %v, want none", calls)
			}
		})
	}
}
