// Purpose: Implement direct lifecycle verbs for finishing or releasing work.
// Exports: LifecycleOptions and RunLifecycle.
// Role: Translate user intent into one shared atomic task mutation.
// Invariants: done, blocked, canceled, and todo postconditions clear claims.
// Invariants: summary is accepted only with a validated result file.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type LifecycleOptions struct {
	ResultPath string
	ResultSet  bool
	Summary    string
	SummarySet bool
}

func RunLifecycle(kind, id string, lifecycle LifecycleOptions, opts GlobalOptions) error {
	targetState, err := lifecycleTargetState(kind)
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("usage: ergo %s <id> [--result <path>] [--summary <text>]", kind)
	}
	if lifecycle.SummarySet && !lifecycle.ResultSet {
		return errors.New("--summary requires --result")
	}
	if lifecycle.ResultSet && strings.TrimSpace(lifecycle.ResultPath) == "" {
		return errors.New("--result cannot be empty")
	}
	body, bodySet, err := readOptionalBodyFromStdin()
	if err != nil {
		return err
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	mutation := taskMutation{
		Kind:          kind,
		State:         targetState,
		StateSet:      true,
		Body:          body,
		BodySet:       bodySet,
		ResultPath:    strings.TrimSpace(lifecycle.ResultPath),
		ResultSummary: lifecycle.Summary,
		ResultSet:     lifecycle.ResultSet,
	}
	if kind == "release" {
		mutation.AllowedStates = []string{stateTodo, stateDoing, stateBlocked, stateError}
	}
	outcome, err := applyTaskMutation(dir, opts, id, mutation, opts.JSON)
	if err != nil {
		return err
	}
	if !opts.JSON {
		return nil
	}
	task := outcome.Graph.Tasks[id]
	if task == nil {
		return errors.New("internal error: missing mutated task")
	}
	return writeJSON(os.Stdout, setOutput{
		Kind:          kind,
		ID:            id,
		UpdatedFields: outcome.UpdatedFields,
		State:         task.State,
		ClaimedBy:     task.ClaimedBy,
	})
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
