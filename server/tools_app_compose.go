package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.yaml.in/yaml/v3"
	"truenas-mcp/truenas"
)

// jobPollInterval is how often a tool waits between core.get_jobs polls. It is
// a variable so tests can shrink the wait to a few milliseconds.
var jobPollInterval = 2 * time.Second

// composeJobWait, when not zero, replaces the timeout_seconds argument of
// truenas_app_compose_update. Tests set it, because the smallest allowed
// timeout_seconds is 30 seconds and a job that never ends would stall them.
// Nothing sets it outside tests.
var composeJobWait time.Duration

const (
	composeTimeoutDefault = 600
	composeTimeoutMin     = 30
	composeTimeoutMax     = 1800
)

// registerAppComposeTools registers the write tool that replaces the compose
// file of a custom (compose) app.
//
// A catalog app's configuration is a set of schema values, which
// truenas_app_configure handles by deep-merging into the stored config. A custom
// app has no such schema: its whole definition is `custom_compose_config`, a
// compose object that app.update replaces in one step. That call needs the
// APPS_WRITE role.
//
// The tool is destructive and does not replay. app.update is called once, and a
// transport error leaves the job state unknown, so the caller is told to check
// truenas_jobs_list instead of running the tool again.
func registerAppComposeTools(s *mcp.Server, client truenas.Caller) {
	s.AddTool(&mcp.Tool{
		Name: "truenas_app_compose_update",
		Description: "Replace the whole compose file of a TrueNAS custom (compose) app, then wait for the TrueNAS job. " +
			"Custom apps only: a catalog app is refused, use truenas_app_configure for those. `compose` is the complete " +
			"new compose file as YAML text; it must be exactly 1 document, a mapping at the top level, string keys " +
			"everywhere, and a `services` mapping whose every service is a mapping. The tool sends the parsed object, " +
			"not the text, so the appliance gets exactly what the diff shows and nothing else. " +
			"Call it once with dry_run to get the current compose, the proposed canonical compose, a unified diff and " +
			"the current_sha256; read the diff, then call again with expected_sha256 set to that hash, which is refused " +
			"when the stored compose changed in between. A write needs APPS_WRITE. The result can contain plaintext " +
			"secrets from the compose, for example environment values. The call waits for the job and gives up after " +
			"timeout_seconds, but a slow job can still run: check truenas_jobs_list before you try again.",
		InputSchema: schema(map[string]any{
			"name":            stringProp("custom app name"),
			"compose":         stringProp("complete new compose file as YAML text"),
			"dry_run":         boolProp("when true, return the current and proposed compose with a diff and the current hash, and do not call app.update"),
			"expected_sha256": stringProp("the current_sha256 a dry run returned; required when dry_run is false"),
			"timeout_seconds": numberProp("how long to wait for the app.update job, 30 to 1800, default 600"),
		}, "name", "compose"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update Custom App Compose",
			ReadOnlyHint:    false,
			DestructiveHint: new(true), // replaces the stored compose whole; TrueNAS keeps no prior copy
			IdempotentHint:  false,     // every accepted call starts a new job, even when the compose matches
			OpenWorldHint:   new(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := requireString(req, "name")
		if err != nil {
			return nil, err
		}
		compose := optionalString(req, "compose")
		if strings.TrimSpace(compose) == "" {
			return nil, fmt.Errorf("parameter %q must be a non-empty compose file", "compose")
		}
		dryRun, err := strictBool(req, "dry_run", false)
		if err != nil {
			return nil, err
		}
		expectedSHA := strings.TrimSpace(optionalString(req, "expected_sha256"))
		wait, err := composeWait(req)
		if err != nil {
			return nil, err
		}

		// 1. The app must exist and must be a custom (compose) app.
		result, err := client.Call("app.query", [][]any{{"name", "=", name}})
		if err != nil {
			return nil, fmt.Errorf("app.query: %w", err)
		}
		var apps []map[string]any
		if err := json.Unmarshal(result, &apps); err != nil {
			return nil, fmt.Errorf("parsing app.query: %w", err)
		}
		if len(apps) == 0 {
			return nil, fmt.Errorf("app %q not found", name)
		}
		if custom, _ := apps[0]["custom_app"].(bool); !custom {
			return nil, fmt.Errorf("app %q is not a custom compose app; use truenas_app_configure to change a catalog app's configuration values", name)
		}

		// 2. Validate the proposed compose and normalize it to JSON values.
		proposed, err := parseCompose(compose)
		if err != nil {
			return nil, err
		}

		// 3. Read the stored compose.
		result, err = client.Call("app.config", name)
		if err != nil {
			return nil, fmt.Errorf("app.config: %w", err)
		}
		var current map[string]any
		if err := json.Unmarshal(result, &current); err != nil {
			return nil, fmt.Errorf("parsing app.config: %w", err)
		}

		// 4. Canonical text and hash of the stored compose. The same renderer
		// handles the proposal, so equal content always gives equal text.
		currentYAML, err := canonicalYAML(current)
		if err != nil {
			return nil, fmt.Errorf("rendering the current compose: %w", err)
		}
		proposedYAML, err := canonicalYAML(proposed)
		if err != nil {
			return nil, fmt.Errorf("rendering the proposed compose: %w", err)
		}
		currentSum := sha256.Sum256([]byte(currentYAML))
		currentSHA := hex.EncodeToString(currentSum[:])

		// 5. A line diff from the current to the proposed compose.
		diff := unifiedDiff("current", "proposed", currentYAML, proposedYAML)

		// 6. Nothing to do.
		if currentYAML == proposedYAML {
			return nil, fmt.Errorf("the compose for app %q is identical to the current compose; nothing to apply", name)
		}

		// 7. Preview only.
		if dryRun {
			return jsonValueResult(map[string]any{
				"name":             name,
				"dry_run":          true,
				"current_sha256":   currentSHA,
				"current_compose":  currentYAML,
				"proposed_compose": proposedYAML,
				"diff":             diff,
			})
		}

		// 8. The caller must have seen this exact stored compose.
		if expectedSHA == "" {
			return nil, fmt.Errorf("parameter %q is required when dry_run is false: run with dry_run first, read the diff, then pass the current_sha256 it returns (the current hash is %s)", "expected_sha256", currentSHA)
		}
		if expectedSHA != currentSHA {
			return nil, fmt.Errorf("expected_sha256 %q does not match the current compose of app %q, whose hash is %s: run with dry_run first, read the diff, then pass the current_sha256 it returns", expectedSHA, name, currentSHA)
		}

		// 9. One call, never replayed: a transport error may still have
		// started the job, and TrueNAS keeps no previous compose.
		result, err = client.Call("app.update", name, map[string]any{"custom_compose_config": proposed})
		if err != nil {
			return nil, fmt.Errorf("app.update: %w; the job state is unknown: check truenas_jobs_list and do not run this tool again blindly", err)
		}
		var jobID int64
		if err := json.Unmarshal(result, &jobID); err != nil {
			return nil, fmt.Errorf("parsing app.update: %w; the job may still be running, check truenas_jobs_list", err)
		}

		// 10. Wait for the job.
		outcome := waitForComposeJob(ctx, client, jobID, wait)

		// 11. Report the outcome.
		switch outcome.State {
		case "SUCCESS":
			return jsonValueResult(map[string]any{
				"name":             name,
				"job_id":           jobID,
				"state":            "SUCCESS",
				"previous_sha256":  currentSHA,
				"previous_compose": currentYAML,
				"diff":             diff,
			})
		case "FAILED", "ABORTED":
			return nil, fmt.Errorf("app.update job %v ended %s: %s; the previous compose is in the diff, so a rollback can send it back unchanged", jobID, outcome.State, jobErrorText(outcome.Error))
		}

		if outcome.Cancelled {
			return nil, fmt.Errorf("cancelled while waiting for app.update job %v, whose last state was %s%s; the job can still run, so check truenas_jobs_list and do not run this tool again blindly", jobID, outcome.State, pollErrSuffix(outcome.PollErr))
		}
		return nil, fmt.Errorf("app.update job %v did not finish within %s and its last state was %s%s; the job can still run, so check truenas_jobs_list and do not run this tool again blindly", jobID, wait, outcome.State, pollErrSuffix(outcome.PollErr))
	})
}

// composeWait returns how long the tool waits for the app.update job. The
// timeout_seconds argument must be a number from 30 to 1800.
func composeWait(req *mcp.CallToolRequest) (time.Duration, error) {
	if composeJobWait > 0 {
		return composeJobWait, nil
	}
	seconds := float64(composeTimeoutDefault)
	if raw, present := parseArgs(req)["timeout_seconds"]; present && raw != nil {
		v, ok := raw.(float64)
		if !ok {
			return 0, fmt.Errorf("parameter %q must be a number, got %T", "timeout_seconds", raw)
		}
		seconds = v
	}
	if seconds < composeTimeoutMin || seconds > composeTimeoutMax {
		return 0, fmt.Errorf("parameter %q must be between %d and %d seconds, got %v", "timeout_seconds", composeTimeoutMin, composeTimeoutMax, seconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// composeJob is the outcome of waiting for an app.update job.
type composeJob struct {
	State     string // last state core.get_jobs reported
	Error     string // the job's error text, when it carries one
	PollErr   error  // the last failed core.get_jobs call, if any
	Cancelled bool   // the request context ended before the job did
}

// waitForComposeJob polls core.get_jobs until the job reaches a final state,
// the wait runs out, or the request is cancelled. A failed poll is a read, so
// it is tried again at the next interval; its error is kept for the report.
func waitForComposeJob(ctx context.Context, client truenas.Caller, jobID int64, wait time.Duration) composeJob {
	deadline := time.Now().Add(wait)
	outcome := composeJob{State: "UNKNOWN"}
	for {
		state, errText, pollErr := pollComposeJob(client, jobID)
		if pollErr != nil {
			outcome.PollErr = pollErr
		} else {
			outcome.State, outcome.Error = state, errText
			switch state {
			case "SUCCESS", "FAILED", "ABORTED":
				return outcome
			}
		}
		if !time.Now().Before(deadline) {
			return outcome
		}
		timer := time.NewTimer(jobPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			outcome.Cancelled = true
			return outcome
		case <-timer.C:
		}
	}
}

// pollComposeJob reads the state and error text of one job.
func pollComposeJob(client truenas.Caller, jobID int64) (string, string, error) {
	raw, err := client.Call("core.get_jobs", [][]any{{"id", "=", jobID}})
	if err != nil {
		return "", "", fmt.Errorf("core.get_jobs: %w", err)
	}
	var jobs []struct {
		State string  `json:"state"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return "", "", fmt.Errorf("parsing core.get_jobs: %w", err)
	}
	if len(jobs) == 0 {
		return "", "", fmt.Errorf("job %v not found in core.get_jobs", jobID)
	}
	errText := ""
	if jobs[0].Error != nil {
		errText = *jobs[0].Error
	}
	return jobs[0].State, errText, nil
}

// jobErrorText names the job error, or says that it carried none.
func jobErrorText(text string) string {
	if text == "" {
		return "the job reported no error text"
	}
	return "the job reported: " + text
}

// pollErrSuffix reports the last failed poll, when there was one.
func pollErrSuffix(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf(" (the last core.get_jobs call failed: %v)", err)
}

// canonicalYAML renders a value as YAML with a 2-space indent and sorted map
// keys, so that equal values always produce identical text.
func canonicalYAML(value any) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// parseCompose validates a compose file and returns it as a JSON-compatible
// object, which is what app.update and the diff use.
func parseCompose(text string) (map[string]any, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parsing compose as YAML: %w", err)
	}
	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case err == nil:
		return nil, fmt.Errorf("compose must hold exactly 1 YAML document, found more")
	case !errors.Is(err, io.EOF):
		return nil, fmt.Errorf("parsing compose as YAML: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("compose must not be empty")
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("compose must be a mapping at the top level, got a %s", yamlKind(root))
	}
	if err := checkComposeKeys(root); err != nil {
		return nil, err
	}
	services := resolveAlias(mappingValue(root, "services"))
	if services == nil {
		return nil, fmt.Errorf("compose must have a top-level %q key", "services")
	}
	if services.Kind != yaml.MappingNode || len(services.Content) == 0 {
		return nil, fmt.Errorf("compose %q must be a mapping with at least 1 service", "services")
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		if resolveAlias(services.Content[i+1]).Kind != yaml.MappingNode {
			return nil, fmt.Errorf("compose service %q must be a mapping", services.Content[i].Value)
		}
	}

	var value any
	if err := root.Decode(&value); err != nil {
		return nil, fmt.Errorf("parsing compose as YAML: %w", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("compose cannot be converted to the JSON the appliance takes: %w", err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, fmt.Errorf("parsing the JSON form of the compose: %w", err)
	}
	return object, nil
}

// checkComposeKeys refuses a mapping key that is not a string, at any depth. The
// JSON form the appliance takes has string keys only.
func checkComposeKeys(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			// "<<" (tag !!merge) pulls an anchored mapping in; the decoder
			// resolves it, and the merged keys are checked where they are defined.
			if key.Kind == yaml.ScalarNode && key.Tag == "!!merge" {
				if err := checkComposeKeys(node.Content[i+1]); err != nil {
					return err
				}
				continue
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("compose keys must be strings: line %d has a %s key %q", key.Line, yamlKind(key), key.Value)
			}
			if err := checkComposeKeys(node.Content[i+1]); err != nil {
				return err
			}
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			if err := checkComposeKeys(child); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		return checkComposeKeys(node.Alias)
	}
	return nil
}

// mappingValue returns the value node of a key in a mapping node, or nil.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return resolveAlias(node.Content[i+1])
		}
	}
	return nil
}

// resolveAlias follows an anchor alias to the node it names.
func resolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	return node
}

// yamlKind names a node kind for an error message.
func yamlKind(node *yaml.Node) string {
	if node == nil {
		return "missing value"
	}
	switch node.Kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	}
	return "unknown node"
}
