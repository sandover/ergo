// Task changes build events against the locked graph before the repository writes.
// Each command owns its validation; the repository remains the single write path.
package ergo

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type taskChangeResult struct {
	events  []Event
	fields  []string
	journal []JournalEntry
}

type taskChange func(*Graph, *Task, time.Time) (taskChangeResult, error)

type mutationOutcome struct {
	Graph         *Graph
	ChangedFields []string
	Journal       []JournalEntry
}

func applyTaskChange(dir string, opts RepositoryOptions, id string, withJournal bool, build taskChange) (mutationOutcome, error) {
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return mutationOutcome{}, err
	}
	var outcome mutationOutcome
	buildLocked := func(graph *Graph) (taskChangeResult, error) {
		if _, ok := graph.Tombstones[id]; ok {
			return taskChangeResult{}, classified(ErrorNotFound, prunedErr(id))
		}
		task := graph.Tasks[id]
		if task == nil {
			return taskChangeResult{}, classified(ErrorNotFound, fmt.Errorf("unknown task id %s", id))
		}
		change, err := build(graph, task, time.Now().UTC())
		if err == nil {
			outcome.ChangedFields = change.fields
		}
		return change, err
	}

	var update UpdateOutcome
	var err error
	if withJournal {
		update, err = repository.UpdateWithJournal(func(graph *Graph) ([]Event, []JournalEntry, error) {
			change, err := buildLocked(graph)
			return change.events, change.journal, err
		})
	} else {
		update, err = repository.Update(func(graph *Graph) ([]Event, error) {
			change, err := buildLocked(graph)
			return change.events, err
		})
	}
	if err == nil {
		outcome.Graph = update.Graph
		outcome.Journal = update.Journal
	}
	return outcome, err
}

func titleChange(title string) taskChange {
	return func(_ *Graph, task *Task, now time.Time) (taskChangeResult, error) {
		cleanTitle := strings.TrimSpace(title)
		if cleanTitle == "" {
			return taskChangeResult{}, errors.New("title cannot be empty")
		}
		if err := validateUnchangedTaskLifecycle(task); err != nil {
			return taskChangeResult{}, err
		}
		if cleanTitle == task.Title {
			return taskChangeResult{}, nil
		}
		event, err := newEvent("title", now, TitleUpdateEvent{ID: task.ID, Title: cleanTitle, TS: formatTime(now)})
		if err != nil {
			return taskChangeResult{}, err
		}
		return taskChangeResult{events: []Event{event}, fields: []string{"title"}}, nil
	}
}

func bodyChange(body string, appendBody bool) taskChange {
	return func(_ *Graph, task *Task, now time.Time) (taskChangeResult, error) {
		if err := validateUnchangedTaskLifecycle(task); err != nil {
			return taskChangeResult{}, err
		}
		targetBody := body
		if appendBody {
			targetBody = task.Body + body
		}
		if targetBody == task.Body {
			return taskChangeResult{}, nil
		}
		event, err := newEvent("body", now, BodyUpdateEvent{ID: task.ID, Body: targetBody, TS: formatTime(now)})
		if err != nil {
			return taskChangeResult{}, err
		}
		return taskChangeResult{events: []Event{event}, fields: []string{"body"}}, nil
	}
}

func moveChange(destinationID string) taskChange {
	return func(graph *Graph, task *Task, now time.Time) (taskChangeResult, error) {
		if err := validateMovePlacement(graph, task, destinationID); err != nil {
			return taskChangeResult{}, err
		}
		if err := validateUnchangedTaskLifecycle(task); err != nil {
			return taskChangeResult{}, err
		}
		if destinationID == task.EpicID {
			return taskChangeResult{}, nil
		}
		event, err := newEvent("epic", now, EpicAssignEvent{ID: task.ID, EpicID: destinationID, TS: formatTime(now)})
		if err != nil {
			return taskChangeResult{}, err
		}
		return taskChangeResult{events: []Event{event}, fields: []string{"epic"}}, nil
	}
}

// Content and placement writes preserve the existing lifecycle and claim.
// Released error records may retain a claim; other states must obey the current invariant.
func validateUnchangedTaskLifecycle(task *Task) error {
	if task.State == stateError {
		return nil
	}
	return validateClaimInvariant(task.State, task.ClaimedBy)
}

func lifecycleChange(kind, targetState, message string, messageSet bool) taskChange {
	return func(graph *Graph, task *Task, now time.Time) (taskChangeResult, error) {
		if !lifecycleAllowsState(kind, task.State) {
			return taskChangeResult{}, classified(ErrorConflict, lifecycleStateError(kind, task.ID, task.State))
		}
		if graph.IsEpic(task.ID) {
			return taskChangeResult{}, classified(ErrorConflict, errors.New("epics do not have state"))
		}
		recordMessage := messageSet
		recordedText := message
		if kind == "open" && task.State == stateTodo {
			recordMessage = false
			recordedText = ""
		}
		change, err := buildStateChange(task, targetState, "", now)
		if err != nil {
			return taskChangeResult{}, err
		}
		if len(change.events) > 0 || recordMessage {
			change.journal = []JournalEntry{newJournalEntry(task.ID, kind, task.ClaimedBy, recordedText, now)}
		}
		if recordMessage {
			change.fields = append(change.fields, "message")
		}
		return change, nil
	}
}

func lifecycleAllowsState(kind, state string) bool {
	switch kind {
	case "open":
		return state == stateTodo || state == stateDraft || state == stateDoing || state == stateBlocked
	case "done", "fail", "block":
		return state == stateTodo || state == stateDoing || state == stateBlocked ||
			state == stateDone || state == stateFailed || state == stateCanceled || state == stateError
	case "cancel":
		return state == stateTodo || state == stateDraft || state == stateDoing || state == stateBlocked ||
			state == stateDone || state == stateFailed || state == stateCanceled || state == stateError
	default:
		return false
	}
}

func claimChange(agentID string) taskChange {
	return func(graph *Graph, task *Task, now time.Time) (taskChangeResult, error) {
		if task.State != stateTodo && task.State != stateDoing && task.State != stateDone &&
			task.State != stateFailed && task.State != stateCanceled && task.State != stateError {
			return taskChangeResult{}, classified(ErrorConflict, lifecycleStateError("claim", task.ID, task.State))
		}
		if task.ClaimedBy != "" && task.ClaimedBy != agentID {
			return taskChangeResult{}, classified(ErrorConflict, fmt.Errorf("task %s is already claimed by %s", task.ID, task.ClaimedBy))
		}
		if graph.IsEpic(task.ID) {
			return taskChangeResult{}, classified(ErrorConflict, errors.New("epics cannot be claimed"))
		}
		change, err := buildStateChange(task, stateDoing, agentID, now)
		if err != nil {
			return taskChangeResult{}, err
		}
		if len(change.events) > 0 {
			change.journal = []JournalEntry{newJournalEntry(task.ID, "claim", agentID, "", now)}
		}
		return change, nil
	}
}

func buildStateChange(task *Task, targetState, targetClaim string, now time.Time) (taskChangeResult, error) {
	if err := validateForwardState(targetState); err != nil {
		return taskChangeResult{}, err
	}
	if err := validateClaimInvariant(targetState, targetClaim); err != nil {
		return taskChangeResult{}, err
	}
	var change taskChangeResult
	if targetClaim != task.ClaimedBy {
		var event Event
		var err error
		if targetClaim == "" {
			event, err = newEvent("unclaim", now, UnclaimEvent{ID: task.ID, TS: formatTime(now)})
		} else {
			event, err = newEvent("claim", now, ClaimEvent{ID: task.ID, AgentID: targetClaim, TS: formatTime(now)})
		}
		if err != nil {
			return taskChangeResult{}, err
		}
		change.events = append(change.events, event)
		change.fields = append(change.fields, "claim")
	}
	if targetState != task.State {
		event, err := newEvent("state", now, StateEvent{ID: task.ID, NewState: targetState, TS: formatTime(now)})
		if err != nil {
			return taskChangeResult{}, err
		}
		change.events = append(change.events, event)
		change.fields = append(change.fields, "state")
	}
	return change, nil
}

func validateForwardState(state string) error {
	switch state {
	case stateTodo, stateDraft, stateDoing, stateBlocked, stateDone, stateFailed, stateCanceled:
		return nil
	case stateError:
		return errors.New("state=error is legacy-only; use block or open")
	default:
		return fmt.Errorf("invalid state: %s", state)
	}
}

func lifecycleStateError(kind, id, state string) error {
	switch {
	case kind == "claim" && (state == stateDraft || state == stateBlocked):
		return fmt.Errorf("claim cannot apply to state=%s; use open %s first", state, id)
	case kind == "open" && (state == stateDone || state == stateFailed || state == stateCanceled):
		return fmt.Errorf("open cannot apply to state=%s; use claim %s --agent <identity> to retry", state, id)
	case kind == "open" && state == stateError:
		return fmt.Errorf("open cannot apply to state=error; use claim %s --agent <identity> to recover legacy work", id)
	case state == stateDraft && kind != "cancel" && kind != "open":
		return fmt.Errorf("%s cannot apply to state=draft; use open %s first", kind, id)
	default:
		return fmt.Errorf("%s cannot apply to state=%s", kind, state)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
