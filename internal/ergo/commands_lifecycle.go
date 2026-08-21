// Lifecycle verbs share one mutation path so postconditions and journal text
// stay aligned. Every lifecycle target clears the claim.
// Lifecycle commands reject stdin because only body commands may replace prose.
package ergo

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

type LifecycleOptions struct {
	Messages []string
}

func RunLifecycle(kind, id string, lifecycle LifecycleOptions, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Lifecycle(LifecycleRequest{
		Kind: kind, ID: id, Messages: lifecycle.Messages,
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
	if len(outcome.ChangedFields) == 0 {
		fmt.Fprintln(w, "No changes.")
	}
	if outcome.Ready != nil {
		fmt.Fprintf(w, "Ready: %s - %s\n", outcome.Ready.ID, outcome.Ready.Title)
	}
}

func RenderResult(w io.Writer, outcome ResultOutcome) {
	fmt.Fprintf(w, "%s - Result recorded: %s\n", outcome.TaskID, outcome.Text)
	if outcome.FilePath != "" {
		fmt.Fprintf(w, "File: %s\n", outcome.FilePath)
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
	case "fail":
		return stateFailed, nil
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
