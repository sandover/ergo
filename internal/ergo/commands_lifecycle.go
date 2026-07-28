// Purpose: Implement direct lifecycle verbs for finishing or releasing work.
// Exports: LifecycleOptions and RunLifecycle.
// Role: Translate user intent into one shared atomic task mutation.
// Invariants: done, blocked, canceled, and todo postconditions clear claims.
// Invariants: lifecycle stdin is rejected; only body may replace task bodies.
package ergo

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type LifecycleOptions struct {
	ResultPath string
	ResultSet  bool
	Messages   []string
}

func RunLifecycle(kind, id string, lifecycle LifecycleOptions, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Lifecycle(LifecycleRequest{
		Kind: kind, ID: id, ResultPath: lifecycle.ResultPath,
		ResultSet: lifecycle.ResultSet, Messages: lifecycle.Messages,
	})
	if err != nil {
		return err
	}
	RenderLifecycle(render.writer(), outcome)
	return nil
}

func RenderLifecycle(w io.Writer, outcome LifecycleOutcome) {
	task := outcome.Task
	fmt.Fprintf(w, "%s - %s\n", task.ID, task.Title)
	fmt.Fprintf(w, "State: %s\n", task.State)
	if containsString(outcome.ChangedFields, "claim") {
		fmt.Fprintln(w, "Claim: cleared")
	}
	if outcome.MessageSet {
		fmt.Fprintln(w, "Message: appended")
	}
	if outcome.ResultPath != "" {
		fmt.Fprintf(w, "Result: %s\n", outcome.ResultPath)
	}
	if len(outcome.ChangedFields) == 0 {
		fmt.Fprintln(w, "No changes.")
	}
	if outcome.Ready != nil {
		fmt.Fprintf(w, "Ready: %s - %s\n", outcome.Ready.ID, outcome.Ready.Title)
	}
}

func normalizeLifecycleMessages(messages []string) (string, bool, error) {
	if len(messages) == 0 {
		return "", false, nil
	}
	paragraphs := make([]string, len(messages))
	for i, message := range messages {
		paragraphs[i] = strings.TrimSpace(message)
		if paragraphs[i] == "" {
			return "", false, errors.New("--message cannot be blank")
		}
	}
	return strings.Join(paragraphs, "\n\n"), true, nil
}

func lifecycleTargetState(kind string) (string, error) {
	switch kind {
	case "done":
		return stateDone, nil
	case "block":
		return stateBlocked, nil
	case "cancel":
		return stateCanceled, nil
	case "release":
		return stateTodo, nil
	default:
		return "", fmt.Errorf("unknown lifecycle command %q", kind)
	}
}
