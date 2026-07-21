// Purpose: Build and apply atomic task mutations from command postconditions.
// Exports: none; command handlers use the package-internal mutation request.
// Role: Single write path for lifecycle, content, placement, and result changes.
// Invariants: doing has one claim; every other forward state is unclaimed.
// Invariants: validation completes before any event is appended under the lock.
// Notes: Legacy error states may be read, but this writer never targets error.
package ergo

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type taskMutation struct {
	Kind          string
	LegacySet     bool
	State         string
	StateSet      bool
	Claim         string
	ClaimSet      bool
	Title         string
	TitleSet      bool
	Body          string
	BodySet       bool
	EpicID        string
	EpicSet       bool
	ResultPath    string
	ResultSummary string
	ResultSet     bool
	AllowedStates []string
}

type mutationOutcome struct {
	Graph         *Graph
	UpdatedFields []string
}

func applyTaskMutation(dir string, opts GlobalOptions, id string, mutation taskMutation, quiet bool) (mutationOutcome, error) {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)
	repoDir := filepath.Dir(dir)
	var outcome mutationOutcome

	err := withLock(lockPath, opts, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if _, ok := graph.Tombstones[id]; ok {
			return prunedErr(id)
		}
		task := graph.Tasks[id]
		if task == nil {
			return fmt.Errorf("unknown task id %s", id)
		}
		if len(mutation.AllowedStates) > 0 && !containsString(mutation.AllowedStates, task.State) {
			return fmt.Errorf("%s cannot apply to state=%s", mutation.Kind, task.State)
		}
		if isContainer(task, graph) {
			if mutation.StateSet {
				return errors.New("containers do not have state")
			}
			if mutation.ClaimSet {
				return errors.New("containers cannot be claimed")
			}
			if mutation.ResultSet {
				return errors.New("containers cannot have results")
			}
		}

		now := time.Now().UTC()
		events, fields, err := buildMutationEvents(id, task, mutation, opts.AgentID, now)
		if err != nil {
			return err
		}
		if mutation.ResultSet {
			summary := mutation.ResultSummary
			if summary == "" {
				summary = mutation.ResultPath
			}
			resultEvent, err := buildResultEvent(repoDir, graph, id, summary, mutation.ResultPath, now)
			if err != nil {
				return err
			}
			events = insertBeforeLifecycleEvents(events, resultEvent)
			fields = append(fields, "result")
		}
		fields = sortedUniqueStrings(fields)

		if err := appendEvents(eventsPath, events); err != nil {
			return err
		}
		updatedGraph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		outcome = mutationOutcome{Graph: updatedGraph, UpdatedFields: fields}
		if !quiet {
			fmt.Println(id)
		}
		return nil
	})
	return outcome, err
}

func buildMutationEvents(id string, task *Task, mutation taskMutation, agentID string, now time.Time) ([]Event, []string, error) {
	var events []Event
	var fields []string

	if mutation.TitleSet {
		mutation.Title = strings.TrimSpace(mutation.Title)
		if mutation.Title == "" {
			return nil, nil, errors.New("title cannot be empty")
		}
	}
	if mutation.TitleSet && mutation.Title != task.Title {
		event, err := newEvent("title", now, TitleUpdateEvent{ID: id, Title: mutation.Title, TS: formatTime(now)})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		fields = append(fields, "title")
	}
	if mutation.BodySet && mutation.Body != task.Body {
		event, err := newEvent("body", now, BodyUpdateEvent{ID: id, Body: mutation.Body, TS: formatTime(now)})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		fields = append(fields, "body")
	}
	if mutation.EpicSet && mutation.EpicID != task.EpicID {
		event, err := newEvent("epic", now, EpicAssignEvent{ID: id, EpicID: mutation.EpicID, TS: formatTime(now)})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		fields = append(fields, "epic")
	}

	targetState, targetClaim, err := mutationPostcondition(task, mutation, agentID)
	if err != nil {
		return nil, nil, err
	}
	if targetClaim != task.ClaimedBy {
		if targetClaim == "" {
			event, err := newEvent("unclaim", now, UnclaimEvent{ID: id, TS: formatTime(now)})
			if err != nil {
				return nil, nil, err
			}
			events = append(events, event)
		} else {
			event, err := newEvent("claim", now, ClaimEvent{ID: id, AgentID: targetClaim, TS: formatTime(now)})
			if err != nil {
				return nil, nil, err
			}
			events = append(events, event)
		}
		fields = append(fields, "claim")
	}
	if targetState != task.State {
		event, err := newEvent("state", now, StateEvent{ID: id, NewState: targetState, TS: formatTime(now)})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		fields = append(fields, "state")
	}

	return events, fields, nil
}

func mutationPostcondition(task *Task, mutation taskMutation, agentID string) (string, string, error) {
	if mutation.LegacySet {
		return legacySetPostcondition(task, mutation, agentID)
	}
	targetState := task.State
	targetClaim := task.ClaimedBy

	if mutation.StateSet {
		if err := validateForwardState(mutation.State); err != nil {
			return "", "", err
		}
		targetState = mutation.State
		if targetState == stateDoing {
			switch {
			case mutation.ClaimSet && mutation.Claim != "":
				targetClaim = mutation.Claim
			case mutation.ClaimSet:
				return "", "", errors.New("state=doing requires a claim")
			case targetClaim != "":
			case agentID != "":
				targetClaim = agentID
			default:
				return "", "", errors.New("state=doing requires a claim; pass --agent")
			}
		} else {
			if mutation.ClaimSet && mutation.Claim != "" {
				return "", "", fmt.Errorf("state=%s must have no claim", targetState)
			}
			targetClaim = ""
		}
	} else if mutation.ClaimSet {
		if mutation.Claim == "" {
			targetClaim = ""
			if targetState == stateDoing {
				targetState = stateTodo
			}
		} else {
			targetState = stateDoing
			targetClaim = mutation.Claim
		}
	}

	if targetState != stateError {
		if err := validateClaimInvariant(targetState, targetClaim); err != nil {
			return "", "", err
		}
	}
	return targetState, targetClaim, nil
}

func legacySetPostcondition(task *Task, mutation taskMutation, agentID string) (string, string, error) {
	targetState := task.State
	targetClaim := task.ClaimedBy
	if mutation.ClaimSet {
		targetClaim = mutation.Claim
	}
	if mutation.StateSet {
		if _, ok := knownStates[mutation.State]; !ok {
			return "", "", fmt.Errorf("invalid state: %s", mutation.State)
		}
		if err := validateLegacySetTransition(task.State, mutation.State); err != nil {
			return "", "", err
		}
		targetState = mutation.State
		if (targetState == stateDoing || targetState == stateError) && targetClaim == "" {
			if agentID == "" {
				return "", "", errors.New("state requires claim; pass --agent or set claim explicitly")
			}
			targetClaim = agentID
		}
		if targetState == stateTodo || targetState == stateDone || targetState == stateCanceled {
			targetClaim = ""
		}
		if err := validateLegacyClaimInvariant(targetState, targetClaim); err != nil {
			return "", "", err
		}
	} else if mutation.ClaimSet && mutation.Claim != "" {
		targetState = stateDoing
	}
	return targetState, targetClaim, nil
}

func validateLegacySetTransition(from, to string) error {
	if from == to {
		return nil
	}
	valid := false
	switch from {
	case stateTodo:
		valid = to == stateDoing || to == stateDone || to == stateBlocked || to == stateCanceled
	case stateDoing:
		valid = to == stateTodo || to == stateDone || to == stateBlocked || to == stateCanceled || to == stateError
	case stateBlocked:
		valid = to == stateTodo || to == stateDoing || to == stateDone || to == stateCanceled
	case stateDone, stateCanceled:
		valid = to == stateTodo
	case stateError:
		valid = to == stateTodo || to == stateDoing || to == stateCanceled
	default:
		return fmt.Errorf("unknown state: %s", from)
	}
	if !valid {
		return fmt.Errorf("invalid transition: %s → %s", from, to)
	}
	return nil
}

func validateLegacyClaimInvariant(state, claimedBy string) error {
	switch state {
	case stateDoing:
		if claimedBy == "" {
			return errors.New("state=doing requires a claim")
		}
	case stateError:
		if claimedBy == "" {
			return errors.New("state=error requires a claim (shows who failed)")
		}
	case stateTodo, stateDone, stateCanceled:
		if claimedBy != "" {
			return fmt.Errorf("state=%s must have no claim", state)
		}
	}
	return nil
}

func validateForwardState(state string) error {
	switch state {
	case stateTodo, stateDoing, stateBlocked, stateDone, stateCanceled:
		return nil
	case stateError:
		return errors.New("state=error is legacy-only; use block or release")
	default:
		return fmt.Errorf("invalid state: %s", state)
	}
}

func insertBeforeLifecycleEvents(events []Event, event Event) []Event {
	for i, existing := range events {
		if existing.Type == "claim" || existing.Type == "unclaim" || existing.Type == "state" {
			events = append(events, Event{})
			copy(events[i+1:], events[i:])
			events[i] = event
			return events
		}
	}
	return append(events, event)
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
