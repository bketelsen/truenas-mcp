package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const composeTestApp = "custom-app"

// composeTestCurrent is what the fake appliance returns from app.config.
const composeTestCurrent = `{
  "services": {"web": {"image": "nginx:1", "ports": ["8080:80"]}},
  "networks": {"default": {"name": "n"}}
}`

// composeTestCurrentCanonical is the canonical rendering of composeTestCurrent:
// sorted keys, 2-space indent. It is the text the hash covers.
const composeTestCurrentCanonical = `networks:
  default:
    name: "n"
services:
  web:
    image: nginx:1
    ports:
      - 8080:80
`

// composeTestProposal changes the web image and adds an environment value.
const composeTestProposal = `services:
  web:
    image: nginx:2
    ports:
      - "8080:80"
    environment:
      TZ: UTC
networks:
  default:
    name: n
`

func composeTestProposalObject() map[string]any {
	return map[string]any{
		"services": map[string]any{
			"web": map[string]any{
				"image":       "nginx:2",
				"ports":       []any{"8080:80"},
				"environment": map[string]any{"TZ": "UTC"},
			},
		},
		"networks": map[string]any{"default": map[string]any{"name": "n"}},
	}
}

// composeMockOptions describes what the fake appliance answers.
type composeMockOptions struct {
	notFound       bool
	catalogApp     bool
	stored         string   // the app.config result; empty means composeTestCurrent
	jobStates      []string // states core.get_jobs reports, in order; the last one repeats
	jobError       string
	pollErrorFirst bool
	updateError    error
}

// composeHarness records what the tool called. The poll counter is atomic: a
// poll that the tool abandoned keeps running in its own goroutine.
type composeHarness struct {
	updates     []any
	polls       atomic.Int64
	configCalls int
}

// newComposeHarness returns a mockCaller that answers the 4 methods the tool
// uses, and a recorder for its calls.
func newComposeHarness(t *testing.T, opts composeMockOptions) (*mockCaller, *composeHarness) {
	t.Helper()
	h := &composeHarness{}
	states := opts.jobStates
	if len(states) == 0 {
		states = []string{"SUCCESS"}
	}
	stored := opts.stored
	if stored == "" {
		stored = composeTestCurrent
	}

	mock := &mockCaller{
		CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
			switch method {
			case "app.query":
				if len(params) != 1 {
					t.Errorf("app.query params = %v, want 1 filter", params)
				}
				if opts.notFound {
					return json.RawMessage(`[]`), nil
				}
				custom := !opts.catalogApp
				return json.RawMessage(fmt.Sprintf(`[{"name":%q,"custom_app":%t}]`, composeTestApp, custom)), nil
			case "app.config":
				h.configCalls++
				if len(params) != 1 || params[0] != composeTestApp {
					t.Errorf("app.config params = %v", params)
				}
				return json.RawMessage(stored), nil
			case "app.update":
				if len(params) != 2 || params[0] != composeTestApp {
					t.Errorf("app.update params = %v", params)
				}
				h.updates = append(h.updates, params[1])
				if opts.updateError != nil {
					return nil, opts.updateError
				}
				return json.RawMessage(`4242`), nil
			case "core.get_jobs":
				polls := h.polls.Add(1)
				if opts.pollErrorFirst && polls == 1 {
					return nil, errors.New("connection reset by peer")
				}
				state := states[len(states)-1]
				if polls-1 < int64(len(states)) {
					state = states[polls-1]
				}
				job := map[string]any{"id": 4242, "state": state, "progress": 100}
				if opts.jobError != "" {
					job["error"] = opts.jobError
				}
				raw, err := json.Marshal([]any{job})
				if err != nil {
					t.Errorf("marshalling job: %v", err)
					return nil, err
				}
				return raw, nil
			}
			t.Errorf("unexpected method %q", method)
			return nil, fmt.Errorf("unexpected method %q", method)
		},
	}
	return mock, h
}

// fastComposePolling shrinks the job poll interval so a test does not wait 2
// seconds per poll.
func fastComposePolling(t *testing.T) {
	t.Helper()
	old := jobPollInterval
	jobPollInterval = time.Millisecond
	t.Cleanup(func() { jobPollInterval = old })
}

// shortComposeWait replaces the timeout_seconds argument with a test-sized wait.
func shortComposeWait(t *testing.T, wait time.Duration) {
	t.Helper()
	old := composeJobWait
	composeJobWait = wait
	t.Cleanup(func() { composeJobWait = old })
}

// resultJSON decodes a successful tool result.
func resultJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result is not JSON: %v\ntext: %s", err, text)
	}
	return got
}

// toolErrorText returns the error message of a refused call, whether the SDK
// reported it as an error or as an error result.
func toolErrorText(t *testing.T, result *mcp.CallToolResult, err error) string {
	t.Helper()
	if err != nil {
		return err.Error()
	}
	return resultText(t, result)
}

func TestAppCompose_NotFound(t *testing.T) {
	mock, h := newComposeHarness(t, composeMockOptions{notFound: true})
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected not-found refusal")
	}
	msg := toolErrorText(t, result, err)
	if !strings.Contains(msg, "not found") {
		t.Errorf("error %q does not say the app is missing", msg)
	}
	if len(h.updates) != 0 {
		t.Error("a missing app must not reach app.update")
	}
}

func TestAppCompose_CatalogAppRefused(t *testing.T) {
	mock, h := newComposeHarness(t, composeMockOptions{catalogApp: true})
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected catalog app refusal")
	}
	msg := toolErrorText(t, result, err)
	if !strings.Contains(msg, "truenas_app_configure") {
		t.Errorf("error %q does not point at truenas_app_configure", msg)
	}
	if len(h.updates) != 0 {
		t.Error("a catalog app must not reach app.update")
	}
}

func TestAppCompose_ComposeRefusals(t *testing.T) {
	cases := []struct {
		name    string
		compose string
		want    string
	}{
		{"empty", "", "non-empty"},
		{"whitespace only", " \n\t\n ", "non-empty"},
		{"unparsable", "services:\n  web: {image: a\n", "parsing compose"},
		{"two documents", "services:\n  web:\n    image: a\n---\nservices:\n  web:\n    image: b\n", "exactly 1 YAML document"},
		{"top level sequence", "- a\n- b\n", "top level"},
		{"top level scalar", "just a string\n", "top level"},
		{"no services", "version: \"3\"\n", `top-level "services"`},
		{"services not a mapping", "services: nope\n", `"services" must be a mapping`},
		{"empty services", "services: {}\n", "at least 1 service"},
		{"service not a mapping", "services:\n  web: nope\n", `service "web" must be a mapping`},
		{"non-string key", "services:\n  web:\n    image: a\n    8080: b\n", "keys must be strings"},
		{"non-string deep key", "services:\n  web:\n    labels:\n      true: b\n", "keys must be strings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, h := newComposeHarness(t, composeMockOptions{})
			result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
				"name": composeTestApp, "compose": tc.compose, "dry_run": true,
			})
			if err == nil && !result.IsError {
				t.Fatalf("expected refusal, got %q", resultText(t, result))
			}
			msg := toolErrorText(t, result, err)
			if !strings.Contains(msg, tc.want) {
				t.Errorf("error %q does not contain %q", msg, tc.want)
			}
			if len(h.updates) != 0 {
				t.Error("a refused compose must not reach app.update")
			}
			if h.configCalls != 0 {
				t.Error("the compose must be validated before app.config is read")
			}
		})
	}
}

func TestAppCompose_DryRun(t *testing.T) {
	mock, h := newComposeHarness(t, composeMockOptions{})
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	if len(h.updates) != 0 {
		t.Fatalf("dry_run called app.update %d times, want 0", len(h.updates))
	}
	got := resultJSON(t, resultText(t, result))
	if got["name"] != composeTestApp || got["dry_run"] != true {
		t.Errorf("dry_run result shape wrong: %v", got)
	}
	if got["current_compose"] != composeTestCurrentCanonical {
		t.Errorf("current_compose = %q, want %q", got["current_compose"], composeTestCurrentCanonical)
	}
	proposed, _ := got["proposed_compose"].(string)
	if !strings.Contains(proposed, "image: nginx:2") || !strings.Contains(proposed, "TZ: UTC") {
		t.Errorf("proposed_compose does not carry the new compose:\n%s", proposed)
	}
	diff, _ := got["diff"].(string)
	for _, want := range []string{"--- current\n", "+++ proposed\n", "@@ ", "-    image: nginx:1\n", "+    image: nginx:2\n"} {
		if !strings.Contains(diff, want) {
			t.Errorf("diff does not contain %q:\n%s", want, diff)
		}
	}
	sum := sha256.Sum256([]byte(composeTestCurrentCanonical))
	if got["current_sha256"] != hex.EncodeToString(sum[:]) {
		t.Errorf("current_sha256 = %v, want the hash of the canonical current compose", got["current_sha256"])
	}
}

func TestAppCompose_IdenticalRefused(t *testing.T) {
	// Same content as the stored compose, in flow style and another key order.
	same := "networks: {default: {name: n}}\nservices: {web: {ports: [\"8080:80\"], image: \"nginx:1\"}}\n"
	sum := sha256.Sum256([]byte(composeTestCurrentCanonical))

	cases := []struct {
		name string
		args map[string]any
	}{
		{"dry run", map[string]any{
			"name": composeTestApp, "compose": same, "dry_run": true,
		}},
		{"write with the current hash", map[string]any{
			"name": composeTestApp, "compose": same, "expected_sha256": hex.EncodeToString(sum[:]),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, h := newComposeHarness(t, composeMockOptions{})
			result, err := callTool(t, mock, false, "truenas_app_compose_update", tc.args)
			if err == nil && !result.IsError {
				t.Fatal("expected identical-compose refusal")
			}
			msg := toolErrorText(t, result, err)
			if !strings.Contains(msg, "identical") {
				t.Errorf("error %q does not say the compose is identical", msg)
			}
			if len(h.updates) != 0 {
				t.Error("an identical compose must not reach app.update")
			}
		})
	}
}

func TestAppCompose_HashGuard(t *testing.T) {
	sum := sha256.Sum256([]byte(composeTestCurrentCanonical))
	currentSHA := hex.EncodeToString(sum[:])

	t.Run("missing hash", func(t *testing.T) {
		mock, h := newComposeHarness(t, composeMockOptions{})
		result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
			"name": composeTestApp, "compose": composeTestProposal,
		})
		if err == nil && !result.IsError {
			t.Fatal("expected refusal without expected_sha256")
		}
		msg := toolErrorText(t, result, err)
		if !strings.Contains(msg, "dry_run") || !strings.Contains(msg, currentSHA) {
			t.Errorf("error %q must name dry_run and the current hash %s", msg, currentSHA)
		}
		if len(h.updates) != 0 {
			t.Error("a missing hash must not reach app.update")
		}
	})

	t.Run("wrong hash", func(t *testing.T) {
		mock, h := newComposeHarness(t, composeMockOptions{})
		result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
			"name": composeTestApp, "compose": composeTestProposal, "expected_sha256": strings.Repeat("0", 64),
		})
		if err == nil && !result.IsError {
			t.Fatal("expected refusal with a wrong expected_sha256")
		}
		msg := toolErrorText(t, result, err)
		if !strings.Contains(msg, currentSHA) {
			t.Errorf("error %q must give the current hash %s", msg, currentSHA)
		}
		if len(h.updates) != 0 {
			t.Error("a wrong hash must not reach app.update")
		}
	})
}

// applyAfterPreview runs a dry run, takes its hash, then applies the same
// compose with that hash.
func applyAfterPreview(t *testing.T, mock *mockCaller) map[string]any {
	t.Helper()
	preview, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err != nil || preview.IsError {
		t.Fatalf("dry run failed: %v %s", err, resultText(t, preview))
	}
	hash, _ := resultJSON(t, resultText(t, preview))["current_sha256"].(string)
	if hash == "" {
		t.Fatal("dry run returned no current_sha256")
	}
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "expected_sha256": hash,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", resultText(t, result))
	}
	return resultJSON(t, resultText(t, result))
}

func TestAppCompose_WriteSuccess(t *testing.T) {
	fastComposePolling(t)
	mock, h := newComposeHarness(t, composeMockOptions{jobStates: []string{"RUNNING", "RUNNING", "SUCCESS"}})

	got := applyAfterPreview(t, mock)

	if len(h.updates) != 1 {
		t.Fatalf("app.update called %d times, want 1", len(h.updates))
	}
	want := map[string]any{"custom_compose_config": composeTestProposalObject()}
	if !reflect.DeepEqual(h.updates[0], want) {
		t.Errorf("app.update payload = %#v, want %#v", h.updates[0], want)
	}
	if h.polls.Load() < 3 {
		t.Errorf("core.get_jobs polled %d times, want at least 3 (RUNNING, RUNNING, SUCCESS)", h.polls.Load())
	}
	if got["job_id"] != float64(4242) {
		t.Errorf("job_id = %v, want 4242", got["job_id"])
	}
	if got["state"] != "SUCCESS" {
		t.Errorf("state = %v, want SUCCESS", got["state"])
	}
	if got["previous_compose"] != composeTestCurrentCanonical {
		t.Errorf("previous_compose = %q, want the canonical current compose", got["previous_compose"])
	}
	if got["previous_sha256"] == "" || got["previous_sha256"] == nil {
		t.Error("previous_sha256 is missing")
	}
	diff, _ := got["diff"].(string)
	if !strings.Contains(diff, "--- current\n") || !strings.Contains(diff, "+++ proposed\n") {
		t.Errorf("diff lacks its headers:\n%s", diff)
	}
}

func TestAppCompose_JobFailed(t *testing.T) {
	fastComposePolling(t)
	mock, _ := newComposeHarness(t, composeMockOptions{
		jobStates: []string{"RUNNING", "FAILED"},
		jobError:  "compose is invalid: unknown service volume",
	})
	preview, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err != nil || preview.IsError {
		t.Fatalf("dry run failed: %v", err)
	}
	hash, _ := resultJSON(t, resultText(t, preview))["current_sha256"].(string)

	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "expected_sha256": hash,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected a job failure error")
	}
	msg := toolErrorText(t, result, err)
	for _, want := range []string{"4242", "FAILED", "unknown service volume", "current_compose", "roll back"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestAppCompose_JobAborted(t *testing.T) {
	fastComposePolling(t)
	mock, _ := newComposeHarness(t, composeMockOptions{jobStates: []string{"ABORTED"}})
	preview, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err != nil || preview.IsError {
		t.Fatalf("dry run failed: %v", err)
	}
	hash, _ := resultJSON(t, resultText(t, preview))["current_sha256"].(string)

	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "expected_sha256": hash,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected an aborted job error")
	}
	msg := toolErrorText(t, result, err)
	for _, want := range []string{"4242", "ABORTED", "current_compose", "roll back"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestAppCompose_JobTimeout(t *testing.T) {
	fastComposePolling(t)
	shortComposeWait(t, 30*time.Millisecond)
	mock, h := newComposeHarness(t, composeMockOptions{jobStates: []string{"RUNNING"}})
	preview, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err != nil || preview.IsError {
		t.Fatalf("dry run failed: %v", err)
	}
	hash, _ := resultJSON(t, resultText(t, preview))["current_sha256"].(string)

	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "expected_sha256": hash,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected a timeout error")
	}
	msg := toolErrorText(t, result, err)
	for _, want := range []string{"4242", "RUNNING", "did not finish", "truenas_jobs_list"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	if h.polls.Load() < 2 {
		t.Errorf("core.get_jobs polled %d times, want several", h.polls.Load())
	}
	if len(h.updates) != 1 {
		t.Errorf("app.update called %d times, want 1 (a timeout is not replayed)", len(h.updates))
	}
}

func TestAppCompose_UpdateTransportError(t *testing.T) {
	fastComposePolling(t)
	mock, h := newComposeHarness(t, composeMockOptions{updateError: errors.New("websocket: close 1006")})
	preview, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err != nil || preview.IsError {
		t.Fatalf("dry run failed: %v", err)
	}
	hash, _ := resultJSON(t, resultText(t, preview))["current_sha256"].(string)

	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "expected_sha256": hash,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected the app.update error")
	}
	msg := toolErrorText(t, result, err)
	for _, want := range []string{"app.update", "unknown", "truenas_jobs_list"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	if len(h.updates) != 1 {
		t.Errorf("app.update called %d times, want exactly 1: it must never be replayed", len(h.updates))
	}
	if h.polls.Load() != 0 {
		t.Errorf("core.get_jobs polled %d times, want 0 without a job ID", h.polls.Load())
	}
}

func TestAppCompose_PollErrorThenSuccess(t *testing.T) {
	fastComposePolling(t)
	mock, h := newComposeHarness(t, composeMockOptions{pollErrorFirst: true, jobStates: []string{"SUCCESS"}})

	got := applyAfterPreview(t, mock)

	if got["state"] != "SUCCESS" {
		t.Errorf("state = %v, want SUCCESS after a failed poll", got["state"])
	}
	if h.polls.Load() != 2 {
		t.Errorf("core.get_jobs polled %d times, want 2 (1 failure, 1 success)", h.polls.Load())
	}
}

func TestAppCompose_TimeoutOutOfRange(t *testing.T) {
	for _, seconds := range []float64{10, 29.5, 1801, 0, -5} {
		t.Run(fmt.Sprintf("%v", seconds), func(t *testing.T) {
			mock, h := newComposeHarness(t, composeMockOptions{})
			result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
				"name": composeTestApp, "compose": composeTestProposal, "dry_run": true, "timeout_seconds": seconds,
			})
			if err == nil && !result.IsError {
				t.Fatal("expected a timeout range refusal")
			}
			msg := toolErrorText(t, result, err)
			if !strings.Contains(msg, "between 30 and 1800") {
				t.Errorf("error %q does not give the allowed range", msg)
			}
			if h.configCalls != 0 || len(h.updates) != 0 {
				t.Error("an out-of-range timeout must refuse before any middleware write")
			}
		})
	}
}

func TestAppCompose_NotRegisteredReadOnly(t *testing.T) {
	mock, h := newComposeHarness(t, composeMockOptions{})
	result, err := callTool(t, mock, true, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("the compose tool must be unavailable in read-only mode")
	}
	if h.configCalls != 0 || len(h.updates) != 0 {
		t.Error("read-only mode must never reach the appliance")
	}
	for _, tool := range listToolDefs(t, mock, true) {
		if tool.Name == "truenas_app_compose_update" {
			t.Fatal("read-only server registered truenas_app_compose_update")
		}
	}
}

// A "<<" merge key is not a string key, but compose files use it with anchors;
// the decoder resolves it, so the tool accepts it and sends the merged object.
func TestAppCompose_MergeKeyAccepted(t *testing.T) {
	obj, err := parseCompose("x-base: &base\n  restart: always\nservices:\n  web:\n    <<: *base\n    image: a\n")
	if err != nil {
		t.Fatalf("parseCompose: %v", err)
	}
	web := obj["services"].(map[string]any)["web"].(map[string]any)
	if web["restart"] != "always" || web["image"] != "a" {
		t.Errorf("merge not resolved: %v", web)
	}
	if _, err := parseCompose("x-base: &base\n  1: a\nservices:\n  web:\n    <<: *base\n"); err == nil {
		t.Error("a non-string key inside a merged anchor must still be refused")
	}
}

func TestAppCompose_CyclicAliasRefused(t *testing.T) {
	// The anchor "loop" contains an alias to itself. A walk that follows
	// node.Alias without cycle detection recurses until the stack overflows.
	cyclic := "services:\n  web:\n    image: nginx\n    x-loop: &loop [*loop]\n"

	result, err := callTool(t, newComposeHarnessFor(t, composeMockOptions{}), false,
		"truenas_app_compose_update", map[string]any{
			"name": composeTestApp, "compose": cyclic, "dry_run": true,
		})
	if err == nil && !result.IsError {
		t.Fatal("expected a refusal for a cyclic alias")
	}
	msg := toolErrorText(t, result, err)
	if !strings.Contains(msg, "cyclic alias") {
		t.Errorf("error %q does not name the cyclic alias", msg)
	}

	mock, h := newComposeHarness(t, composeMockOptions{})
	result, err = callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": cyclic, "dry_run": true,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected a refusal for a cyclic alias")
	}
	if h.configCalls != 0 || len(h.updates) != 0 {
		t.Error("a cyclic compose must be refused before app.config, and must never reach app.update")
	}
}

// newComposeHarnessFor returns only the mock of a fresh harness.
func newComposeHarnessFor(t *testing.T, opts composeMockOptions) *mockCaller {
	t.Helper()
	mock, _ := newComposeHarness(t, opts)
	return mock
}

func TestAppCompose_SharedAliasesFinishFast(t *testing.T) {
	// 9 levels, each a list of 9 aliases to the level below: 9^9 leaves if the
	// walk expands every alias instead of checking each anchor once.
	var doc strings.Builder
	doc.WriteString("l0: &l0 [a, a, a, a, a, a, a, a, a]\n")
	for level := 1; level <= 9; level++ {
		fmt.Fprintf(&doc, "l%d: &l%d [", level, level)
		for i := 0; i < 9; i++ {
			if i > 0 {
				doc.WriteString(", ")
			}
			fmt.Fprintf(&doc, "*l%d", level-1)
		}
		doc.WriteString("]\n")
	}
	doc.WriteString("services:\n  web:\n    image: nginx\n")

	mock, h := newComposeHarness(t, composeMockOptions{})
	start := time.Now()
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": doc.String(), "dry_run": true,
	})
	elapsed := time.Since(start)

	if err == nil && !result.IsError {
		t.Fatal("expected a refusal: the yaml decoder caps alias expansion")
	}
	if elapsed > 5*time.Second {
		t.Errorf("the shared-alias document took %s, want far less", elapsed)
	}
	if h.configCalls != 0 || len(h.updates) != 0 {
		t.Error("a refused compose must not reach app.config or app.update")
	}
}

func TestAppCompose_BigIntegerPreserved(t *testing.T) {
	const big = "9007199254740993" // 2^53 + 1: a float64 reads it as 2^53
	fastComposePolling(t)
	mock, h := newComposeHarness(t, composeMockOptions{
		stored: `{"services":{"web":{"image":"nginx:1","replicas":` + big + `}}}`,
	})

	preview, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name":    composeTestApp,
		"compose": "services:\n  web:\n    image: nginx:2\n    replicas: " + big + "\n",
		"dry_run": true,
	})
	if err != nil || preview.IsError {
		t.Fatalf("dry run failed: %v %s", err, resultText(t, preview))
	}
	previewJSON := resultJSON(t, resultText(t, preview))
	current, _ := previewJSON["current_compose"].(string)
	if !strings.Contains(current, big) {
		t.Errorf("current_compose lost digits:\n%s", current)
	}
	if strings.Contains(current, "9007199254740992") {
		t.Errorf("current_compose rounded 2^53+1 down:\n%s", current)
	}

	hash, _ := previewJSON["current_sha256"].(string)
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name":            composeTestApp,
		"compose":         "services:\n  web:\n    image: nginx:2\n    replicas: " + big + "\n",
		"expected_sha256": hash,
	})
	if err != nil || result.IsError {
		t.Fatalf("write failed: %v %s", err, resultText(t, result))
	}
	if len(h.updates) != 1 {
		t.Fatalf("app.update called %d times, want 1", len(h.updates))
	}
	payload, err := json.Marshal(h.updates[0])
	if err != nil {
		t.Fatalf("marshalling the app.update payload: %v", err)
	}
	if !strings.Contains(string(payload), `"replicas":`+big) {
		t.Errorf("app.update payload lost digits: %s", payload)
	}
}

func TestAppCompose_HashDistinguishesBigIntegers(t *testing.T) {
	hashes := make(map[string]string, 2)
	for _, big := range []string{"9007199254740992", "9007199254740993"} {
		mock, _ := newComposeHarness(t, composeMockOptions{
			stored: `{"services":{"web":{"image":"nginx:1","replicas":` + big + `}}}`,
		})
		result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
			"name":    composeTestApp,
			"compose": "services:\n  web:\n    image: nginx:2\n    replicas: " + big + "\n",
			"dry_run": true,
		})
		if err != nil || result.IsError {
			t.Fatalf("dry run failed for %s: %v", big, err)
		}
		hashes[big], _ = resultJSON(t, resultText(t, result))["current_sha256"].(string)
	}
	if hashes["9007199254740992"] == "" || hashes["9007199254740992"] == hashes["9007199254740993"] {
		t.Errorf("the two stored composes hash the same: %v", hashes)
	}
}

func TestAppCompose_ProposalNumberRefusals(t *testing.T) {
	cases := []struct {
		name    string
		compose string
		want    []string
	}{
		{
			name:    "integer above int64",
			compose: "services:\n  web:\n    image: a\n    big: 9223372036854775808\n",
			want:    []string{"9223372036854775808", "64-bit integer"},
		},
		{
			name:    "integer written as a float above 2^53",
			compose: "services:\n  web:\n    image: a\n    big: 1e24\n",
			want:    []string{"2^53", "cannot hold it exactly"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, h := newComposeHarness(t, composeMockOptions{})
			result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
				"name": composeTestApp, "compose": tc.compose, "dry_run": true,
			})
			if err == nil && !result.IsError {
				t.Fatal("expected the number to be refused")
			}
			msg := toolErrorText(t, result, err)
			for _, want := range tc.want {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
			if h.configCalls != 0 || len(h.updates) != 0 {
				t.Error("a refused proposal must not reach app.config or app.update")
			}
		})
	}
}

func TestAppCompose_StoredNumberRefused(t *testing.T) {
	// app.config sends 1e24: an integer-valued float64 above 2^53, which the
	// tool cannot carry exactly.
	mock, h := newComposeHarness(t, composeMockOptions{
		stored: `{"services":{"web":{"image":"nginx:1","value":1e24}}}`,
	})
	result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
		"name": composeTestApp, "compose": composeTestProposal, "dry_run": true,
	})
	if err == nil && !result.IsError {
		t.Fatal("expected the stored number to be refused")
	}
	msg := toolErrorText(t, result, err)
	for _, want := range []string{"stored compose", "cannot carry exactly"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	if len(h.updates) != 0 {
		t.Error("a refused stored compose must not reach app.update")
	}
}

// pollCountingMock answers core.get_jobs with one state and counts the polls.
func pollCountingMock(t *testing.T, state string, onPoll func()) (*mockCaller, *atomic.Int64) {
	t.Helper()
	polls := new(atomic.Int64)
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if method != "core.get_jobs" {
			t.Errorf("unexpected method %q", method)
		}
		polls.Add(1)
		if onPoll != nil {
			onPoll()
		}
		return json.RawMessage(`[{"id":1,"state":"` + state + `"}]`), nil
	}}
	return mock, polls
}

func TestWaitForComposeJob_CancelledBeforeFirstPoll(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	mock, polls := pollCountingMock(t, "RUNNING", nil)
	outcome := waitForComposeJob(ctx, mock, 1, time.Minute)

	if !outcome.Cancelled {
		t.Error("a cancelled context must end the wait as cancelled")
	}
	if polls.Load() != 0 {
		t.Errorf("core.get_jobs polled %d times after cancellation, want 0", polls.Load())
	}
}

func TestWaitForComposeJob_CancelledBetweenPolls(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// The cancel lands while the first poll runs; the wait between polls must
	// then end at once instead of sleeping out the interval.
	mock, polls := pollCountingMock(t, "RUNNING", cancel)
	start := time.Now()
	outcome := waitForComposeJob(ctx, mock, 1, time.Minute)
	elapsed := time.Since(start)

	if !outcome.Cancelled {
		t.Error("the wait must report the cancellation")
	}
	if polls.Load() != 1 {
		t.Errorf("core.get_jobs polled %d times, want 1", polls.Load())
	}
	if elapsed > time.Second {
		t.Errorf("the wait took %s after a cancel, want an immediate return", elapsed)
	}
}

func TestWaitForComposeJob_NoPollAfterDeadline(t *testing.T) {
	old := jobPollInterval
	jobPollInterval = time.Hour // longer than the whole wait
	t.Cleanup(func() { jobPollInterval = old })

	mock, polls := pollCountingMock(t, "RUNNING", nil)
	start := time.Now()
	outcome := waitForComposeJob(t.Context(), mock, 1, 20*time.Millisecond)
	elapsed := time.Since(start)

	if outcome.Cancelled {
		t.Error("a timeout is not a cancellation")
	}
	if outcome.State != "RUNNING" {
		t.Errorf("last state = %q, want RUNNING", outcome.State)
	}
	if polls.Load() != 1 {
		t.Errorf("core.get_jobs polled %d times, want 1: the wait must not poll after the deadline", polls.Load())
	}
	if elapsed > 5*time.Second {
		t.Errorf("the wait took %s, want about 20ms", elapsed)
	}
}

func TestWaitForComposeJob_PollsUntilDeadline(t *testing.T) {
	old := jobPollInterval
	jobPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { jobPollInterval = old })

	mock, polls := pollCountingMock(t, "RUNNING", nil)
	outcome := waitForComposeJob(t.Context(), mock, 1, 50*time.Millisecond)

	if outcome.Cancelled {
		t.Error("a timeout is not a cancellation")
	}
	// Polls are at most one interval apart, so 50ms allows at most 11 polls.
	if polls.Load() < 1 || polls.Load() > 11 {
		t.Errorf("core.get_jobs polled %d times in 50ms at a 5ms interval", polls.Load())
	}
}

func TestAppCompose_FloatAndBigIntegerRefusals(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"float above 2^53", "9007199254740993.0", "2^53"},
		{"exponent float above 2^53", "9.007199254740993e15", "2^53"},
		{"negative float above 2^53", "-9007199254740993.0", "2^53"},
		{"infinite float", ".inf", "infinity"},
		{"not a number", ".nan", "NaN"},
		{"int64 max plus 1", "9223372036854775808", "signed 64-bit integer"},
		{"2^64", "18446744073709551616", "2^53"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock, h := newComposeHarness(t, composeMockOptions{})
			result, err := callTool(t, mock, false, "truenas_app_compose_update", map[string]any{
				"name":    composeTestApp,
				"compose": "services:\n  web:\n    image: a\n    value: " + tc.value + "\n",
				"dry_run": true,
			})
			if err == nil && !result.IsError {
				t.Fatal("expected the number to be refused")
			}
			msg := toolErrorText(t, result, err)
			if !strings.Contains(msg, tc.want) {
				t.Errorf("error %q does not contain %q", msg, tc.want)
			}
			if !strings.Contains(msg, tc.value) {
				t.Errorf("error %q does not name the value %q", msg, tc.value)
			}
			if !strings.Contains(msg, "line 4") {
				t.Errorf("error %q does not name the line", msg)
			}
			if h.configCalls != 0 || len(h.updates) != 0 {
				t.Error("a refused number must be caught before app.config and app.update")
			}
		})
	}
}

func TestAppCompose_SmallFloatsAccepted(t *testing.T) {
	object, err := parseCompose("services:\n  web:\n    image: a\n    whole: 5.0\n    fraction: 1.5\n")
	if err != nil {
		t.Fatalf("parseCompose: %v", err)
	}
	web, ok := object["services"].(map[string]any)["web"].(map[string]any)
	if !ok {
		t.Fatalf("services.web is not a mapping: %v", object["services"])
	}
	// 5.0 has no fraction, so the JSON form is the integer 5.
	if web["whole"] != int64(5) {
		t.Errorf("whole = %v (%T), want int64(5)", web["whole"], web["whole"])
	}
	if web["fraction"] != 1.5 {
		t.Errorf("fraction = %v (%T), want 1.5", web["fraction"], web["fraction"])
	}
}

func TestWaitForComposeJob_CancelWhilePollBlocks(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	entered := make(chan struct{}, 1)

	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		entered <- struct{}{}
		<-release
		return json.RawMessage(`[{"id":1,"state":"RUNNING"}]`), nil
	}}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		<-entered
		cancel()
	}()

	start := time.Now()
	outcome := waitForComposeJob(ctx, mock, 1, time.Minute)
	elapsed := time.Since(start)

	if !outcome.Cancelled {
		t.Error("a cancel during a blocking poll must end the wait as cancelled")
	}
	if elapsed > time.Second {
		t.Errorf("the wait took %s after a cancel while the poll blocks, want a prompt return", elapsed)
	}
}

func TestWaitForComposeJob_DeadlineWhilePollBlocks(t *testing.T) {
	old := jobPollInterval
	jobPollInterval = time.Millisecond
	t.Cleanup(func() { jobPollInterval = old })

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var calls atomic.Int64

	// The first poll answers at once; every later poll blocks until the test
	// ends, so the deadline has to win over a poll that never returns.
	mock := &mockCaller{CallFunc: func(method string, params ...interface{}) (json.RawMessage, error) {
		if calls.Add(1) == 1 {
			return json.RawMessage(`[{"id":1,"state":"RUNNING"}]`), nil
		}
		<-release
		return json.RawMessage(`[{"id":1,"state":"SUCCESS"}]`), nil
	}}

	start := time.Now()
	outcome := waitForComposeJob(t.Context(), mock, 1, 50*time.Millisecond)
	elapsed := time.Since(start)

	if outcome.Cancelled {
		t.Error("a deadline is not a cancellation")
	}
	if outcome.State != "RUNNING" {
		t.Errorf("last state = %q, want RUNNING from the poll that answered", outcome.State)
	}
	if elapsed > time.Second {
		t.Errorf("the wait took %s across a blocking poll at the deadline, want a prompt return", elapsed)
	}
}
