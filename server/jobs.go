package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"truenas-mcp/truenas"
)

// jobPollInterval is how often a tool waits between core.get_jobs polls. It is
// a variable so tests can shrink the wait to a few milliseconds.
var jobPollInterval = 2 * time.Second

// jobOutcome is the outcome of waiting for a middleware job.
type jobOutcome struct {
	State     string // last state core.get_jobs reported
	Error     string // the job's error text, when it carries one
	PollErr   error  // the last failed core.get_jobs call, if any
	Cancelled bool   // the request context ended before the job did
}

// pollResult is what one core.get_jobs poll found; it comes back from the
// goroutine that runs the call.
type pollResult struct {
	state   string
	errText string
	err     error
}

// waitForJob polls core.get_jobs until the job reaches a final state,
// the wait runs out, or the request is cancelled. The context and the deadline
// are checked before each poll, so no poll starts after either one, and the wait
// between polls is never longer than the time left. A failed poll is a read, so
// it is tried again at the next interval; its error is kept for the report.
//
// truenas.Caller takes no context, so a poll call cannot be stopped. Each poll
// therefore runs in its own goroutine and reports to a channel with room for 1
// result: the wait can return at the deadline or on a cancel without waiting for
// the call, and the goroutine then finishes in the background and its result is
// dropped. An abandoned poll is a read only; the write that started the job
// (app.update, pool.snapshottask.run) is never called again (the no-replay rule
// in truenas/client.go).
func waitForJob(ctx context.Context, client truenas.Caller, jobID int64, wait time.Duration) jobOutcome {
	deadline := time.Now().Add(wait)
	outcome := jobOutcome{State: "UNKNOWN"}
	for {
		if ctx.Err() != nil {
			outcome.Cancelled = true
			return outcome
		}
		left := time.Until(deadline)
		if left <= 0 {
			return outcome
		}

		results := make(chan pollResult, 1)
		go func() {
			state, errText, pollErr := pollJob(client, jobID)
			results <- pollResult{state: state, errText: errText, err: pollErr}
		}()

		timer := time.NewTimer(left)
		select {
		case result := <-results:
			timer.Stop()
			if result.err != nil {
				outcome.PollErr = result.err
				break
			}
			outcome.State, outcome.Error = result.state, result.errText
			switch result.state {
			case "SUCCESS", "FAILED", "ABORTED":
				return outcome
			}
		case <-ctx.Done():
			timer.Stop()
			outcome.Cancelled = true
			return outcome
		case <-timer.C:
			return outcome
		}

		pause := min(jobPollInterval, time.Until(deadline))
		if pause <= 0 {
			return outcome
		}
		timer = time.NewTimer(pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			outcome.Cancelled = true
			return outcome
		case <-timer.C:
		}
	}
}

// pollJob reads the state and error text of one job.
func pollJob(client truenas.Caller, jobID int64) (string, string, error) {
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
