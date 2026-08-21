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
	"sort"
	"strings"
	"time"
)

type taskMutation struct {
	Kind          string
	State         string
	StateSet      bool
	Claim         string
	ClaimSet      bool
	Title         string
	TitleSet      bool
	Body          string
	BodySet       bool
	BodyAppend    bool
	EpicID        string
	EpicSet       bool
	ValidateMove  bool
	MessageKind   string
	MessageText   string
	MessageSet    bool
	AllowedStates []string
	ClaimConflict bool
}

type mutationOutcome struct {
	Graph         *Graph
	ChangedFields []string
	Journal       []JournalEntry
}

func applyTaskMutation(dir string, opts RepositoryOptions, id string, mutation taskMutation, agentID string) (mutationOutcome, error) {
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return mutationOutcome{}, err
	}
	var outcome mutationOutcome

	update, err := repository.UpdateWithJournal(func(graph *Graph) ([]Event, []JournalEntry, error) {
		if _, ok := graph.Tombstones[id]; ok {
			return nil, nil, classified(ErrorNotFound, prunedErr(id))
		}
		task := graph.Tasks[id]
		if task == nil {
			return nil, nil, classified(ErrorNotFound, fmt.Errorf("unknown task id %s", id))
		}
		if len(mutation.AllowedStates) > 0 && !containsString(mutation.AllowedStates, task.State) {
			return nil, nil, classified(ErrorConflict, fmt.Errorf("%s cannot apply to state=%s", mutation.Kind, task.State))
		}
		if mutation.ClaimConflict && task.ClaimedBy != "" && task.ClaimedBy != mutation.Claim {
			return nil, nil, classified(ErrorConflict, fmt.Errorf("task %s is already claimed by %s", id, task.ClaimedBy))
		}
		if mutation.EpicSet && mutation.ValidateMove {
			if err := validateMovePlacement(graph, task, mutation.EpicID); err != nil {
				return nil, nil, err
			}
		}
		if graph.IsEpic(task.ID) {
			if mutation.ClaimSet {
				return nil, nil, classified(ErrorConflict, errors.New("epics cannot be claimed"))
			}
			if mutation.StateSet {
				return nil, nil, classified(ErrorConflict, errors.New("epics do not have state"))
			}
			if mutation.MessageSet {
				return nil, nil, classified(ErrorConflict, errors.New("epics cannot have lifecycle messages"))
			}
		}

		now := time.Now().UTC()
		events, fields, err := buildMutationEvents(id, task, mutation, agentID, now)
		if err != nil {
			return nil, nil, err
		}
		outcome.ChangedFields = fields
		var journal []JournalEntry
		if isAutomaticJournalKind(mutation.Kind) && (len(events) > 0 || mutation.MessageSet) {
			responsible := agentID
			if responsible == "" {
				responsible = task.ClaimedBy
			}
			journal = []JournalEntry{newJournalEntry(id, mutation.Kind, responsible, mutation.MessageText, now)}
		}
		if mutation.MessageSet {
			outcome.ChangedFields = append(outcome.ChangedFields, "message")
		}
		return events, journal, nil
	})
	if err == nil {
		outcome.Graph = update.Graph
		outcome.Journal = update.Journal
	}
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
	targetBody := mutation.Body
	if mutation.BodyAppend {
		targetBody = task.Body + mutation.Body
	}
	if mutation.BodySet && targetBody != task.Body {
		event, err := newEvent("body", now, BodyUpdateEvent{ID: id, Body: targetBody, TS: formatTime(now)})
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

func validateForwardState(state string) error {
	switch state {
	case stateTodo, stateDoing, stateBlocked, stateDone, stateFailed, stateCanceled:
		return nil
	case stateError:
		return errors.New("state=error is legacy-only; use block or release")
	default:
		return fmt.Errorf("invalid state: %s", state)
	}
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
