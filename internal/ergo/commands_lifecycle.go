// Purpose: Implement direct lifecycle verbs for finishing or releasing work.
// Exports: LifecycleOptions and RunLifecycle.
// Role: Translate user intent into one shared atomic task mutation.
// Invariants: done, blocked, canceled, and todo postconditions clear claims.
// Invariants: lifecycle stdin is rejected; only body may replace task bodies.
package ergo

import (
	"errors"
	"fmt"
	"strings"
)

type LifecycleOptions struct {
	ResultPath string
	ResultSet  bool
	Messages   []string
}

func RunLifecycle(kind, id string, lifecycle LifecycleOptions, opts GlobalOptions) error {
	if stdinIsPiped() {
		return fmt.Errorf("%s does not read stdin; use ergo body %s to replace the body or -m <message> to add a lifecycle note", kind, id)
	}
	outcome, err := NewApplication(opts).Lifecycle(LifecycleRequest{
		Kind: kind, ID: id, ResultPath: lifecycle.ResultPath,
		ResultSet: lifecycle.ResultSet, Messages: lifecycle.Messages,
	})
	if err != nil {
		return err
	}
	task := outcome.Task
	fmt.Printf("%s - %s\n", task.ID, task.Title)
	fmt.Printf("State: %s\n", task.State)
	if containsString(outcome.ChangedFields, "claim") {
		fmt.Println("Claim: cleared")
	}
	if outcome.MessageSet {
		fmt.Println("Message: appended")
	}
	if lifecycle.ResultSet {
		fmt.Printf("Result: %s\n", outcome.ResultPath)
	}
	if len(outcome.ChangedFields) == 0 {
		fmt.Println("No changes.")
	}
	if outcome.Ready != nil {
		fmt.Printf("Ready: %s - %s\n", outcome.Ready.ID, outcome.Ready.Title)
	}
	return nil
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
