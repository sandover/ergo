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
	targetState, err := lifecycleTargetState(kind)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("usage: ergo %s <id> [-m <message>] [--result <path>]", kind)
	}
	if lifecycle.ResultSet && strings.TrimSpace(lifecycle.ResultPath) == "" {
		return errors.New("--result cannot be empty")
	}
	message, messageSet, err := normalizeLifecycleMessages(lifecycle.Messages)
	if err != nil {
		return err
	}
	if stdinIsPiped() {
		return fmt.Errorf("%s does not read stdin; use ergo body %s to replace the body or -m <message> to add a lifecycle note", kind, id)
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	mutation := taskMutation{
		Kind:        kind,
		State:       targetState,
		StateSet:    true,
		ResultPath:  strings.TrimSpace(lifecycle.ResultPath),
		ResultSet:   lifecycle.ResultSet,
		MessageKind: kind,
		MessageText: message,
		MessageSet:  messageSet,
	}
	if kind == "release" {
		mutation.AllowedStates = []string{stateTodo, stateDoing, stateBlocked, stateError}
	}
	outcome, err := applyTaskMutation(dir, opts, id, mutation, true)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeMutationResult(kind, id, outcome, true)
	}
	fmt.Printf("%s %s\n", id, targetState)
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
