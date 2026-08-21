// Purpose: Decode canonical mutations into current graph state.
// Exports: none (package-internal graph helpers).
// Replay reconstructs the current graph from released and current records.
// It accepts legacy compatibility forms but enforces current graph invariants
// after replay. Tombstones remove tasks, and results remain newest-first.
// Notes: Readiness checks direct deps and inherited container deps.
package ergo

import (
	"fmt"
	"strings"
	"time"
)

func eventReplayContext(event Event, fallbackIndex int) string {
	if event.Source.Path == "" {
		return fmt.Sprintf("event %d", fallbackIndex+1)
	}
	context := fmt.Sprintf("%s:%d", event.Source.Path, event.Source.Line)
	if event.Source.TransactionIndex > 0 {
		context += fmt.Sprintf(" transaction event %d", event.Source.TransactionIndex)
	}
	return context
}

func replayDecodeError(context, kind, target string, cause error) error {
	return fmt.Errorf("%s: event %q%s: invalid payload: %w", context, kind, replayTarget(target), cause)
}

func replayInvariantError(context, kind, target, detail string) error {
	return fmt.Errorf("%s: event %q%s: %s", context, kind, replayTarget(target), detail)
}

func replayTarget(target string) string {
	if target == "" {
		return ""
	}
	return fmt.Sprintf(" for %s", target)
}

// isReadableState enumerates the current states plus the v1 error state.
// Current writers reject error, but released histories continue to normalize it
// into the in-memory task model.
func isReadableState(state string) bool {
	switch state {
	case stateTodo, stateDoing, stateBlocked, stateDone, stateFailed, stateCanceled, stateError:
		return true
	default:
		return false
	}
}

type replayEventSource struct {
	context string
	kind    string
	order   int
}

func validateReplayInvariants(
	graph *Graph,
	taskSource, lifecycleSource, parentSource map[string]replayEventSource,
	linkSource map[string]map[string]replayEventSource,
) error {
	for id, task := range graph.Tasks {
		created := taskSource[id]
		parentChanged := parentSource[id]
		if parentChanged.context == "" {
			parentChanged = created
		}
		if task.EpicID != "" {
			if task.EpicID == id {
				return replayInvariantError(parentChanged.context, parentChanged.kind, id, "task cannot be its own epic")
			}
			parent := graph.Tasks[task.EpicID]
			if parent == nil {
				return replayInvariantError(parentChanged.context, parentChanged.kind, id, fmt.Sprintf("unknown parent epic %s", task.EpicID))
			}
			if parent.EpicID != "" {
				return replayInvariantError(parentChanged.context, parentChanged.kind, id, fmt.Sprintf("nested epic relationship through %s", task.EpicID))
			}
		}

		// Released v1 histories may finish in error while retaining their
		// claim. Pre-v3 blocked histories may do the same. Both are explicit
		// read-time compatibility forms; current commands normalize them on
		// the next lifecycle mutation.
		switch task.State {
		case stateError, stateBlocked:
			// Compatibility form: claim may be empty or populated.
		default:
			if err := validateClaimInvariant(task.State, task.ClaimedBy); err != nil {
				source := lifecycleSource[id]
				if source.context == "" {
					source = created
				}
				return replayInvariantError(source.context, source.kind, id, err.Error())
			}
		}
	}

	for from, deps := range graph.Deps {
		fromTask := graph.Tasks[from]
		for to := range deps {
			toTask := graph.Tasks[to]
			if fromTask == nil || toTask == nil {
				return replayInvariantError(taskSource[from].context, "link", from+" -> "+to, "dangling dependency endpoint")
			}
			if err := validateDepAncestry(fromTask, toTask); err != nil {
				source := linkSource[from][to]
				if candidate := parentSource[from]; candidate.order > source.order {
					source = candidate
				}
				if candidate := parentSource[to]; candidate.order > source.order {
					source = candidate
				}
				return replayInvariantError(source.context, source.kind, from+" -> "+to, err.Error())
			}
		}
	}
	return nil
}

func newGraph() *Graph {
	return &Graph{
		Tasks:            map[string]*Task{},
		Deps:             map[string]map[string]struct{}{},
		Tombstones:       map[string]TombstoneInfo{},
		legacyEmptyEpics: map[string]struct{}{},
	}
}

func replayEvents(events []Event) (*Graph, error) {
	return replayEventsOnto(newGraph(), events)
}

// applyTransaction applies a proposed current transaction to an isolated graph
// copy. The caller's graph remains unchanged on both success and failure.
func applyTransaction(graph *Graph, events []Event) (*Graph, error) {
	return replayEventsOnto(cloneGraph(graph), events)
}

func cloneGraph(graph *Graph) *Graph {
	clone := newGraph()
	if graph == nil {
		return clone
	}
	for id, task := range graph.Tasks {
		copied := *task
		copied.Results = append([]Result(nil), task.Results...)
		copied.Messages = append([]Message(nil), task.Messages...)
		clone.Tasks[id] = &copied
	}
	for from, deps := range graph.Deps {
		clone.Deps[from] = map[string]struct{}{}
		for to := range deps {
			clone.Deps[from][to] = struct{}{}
		}
	}
	for id, info := range graph.Tombstones {
		clone.Tombstones[id] = info
	}
	for id := range graph.legacyEmptyEpics {
		clone.legacyEmptyEpics[id] = struct{}{}
	}
	clone.rebuildIndexes()
	return clone
}

func replayEventsOnto(graph *Graph, events []Event) (*Graph, error) {
	taskSource := map[string]replayEventSource{}
	lifecycleSource := map[string]replayEventSource{}
	parentSource := map[string]replayEventSource{}
	linkSource := map[string]map[string]replayEventSource{}
	for id := range graph.Tasks {
		source := replayEventSource{context: "snapshot", kind: snapshotTaskRecordType, order: -1}
		taskSource[id] = source
		lifecycleSource[id] = source
		parentSource[id] = source
	}
	for from, deps := range graph.Deps {
		linkSource[from] = map[string]replayEventSource{}
		for to := range deps {
			linkSource[from][to] = replayEventSource{context: "snapshot", kind: snapshotDependencyRecordType, order: -1}
		}
	}
	for eventIndex, event := range events {
		context := eventReplayContext(event, eventIndex)
		decoded, err := decodeEvent(event, eventIndex)
		if err != nil {
			return nil, err
		}
		switch decoded.kind {
		case eventNewTask:
			data := decoded.payload.(NewTaskEvent)
			if data.ID == "" {
				return nil, replayInvariantError(context, event.Type, "", "task id is empty")
			}
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			if _, exists := graph.Tasks[data.ID]; exists {
				return nil, replayInvariantError(context, event.Type, data.ID, "duplicate task id")
			}
			createdAt, err := parseTime(data.CreatedAt)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid created_at: %w", err))
			}
			if !isReadableState(data.State) {
				return nil, replayInvariantError(context, event.Type, data.ID, fmt.Sprintf("invalid state %q", data.State))
			}
			task := &Task{
				ID:        data.ID,
				UUID:      data.UUID,
				EpicID:    data.EpicID,
				State:     data.State,
				Title:     data.Title,
				Body:      data.Body,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}
			graph.Tasks[data.ID] = task
			if decoded.legacyExplicitEpic {
				graph.legacyEmptyEpics[data.ID] = struct{}{}
			}
			source := replayEventSource{context: context, kind: decoded.wireKind, order: eventIndex}
			taskSource[data.ID] = source
			lifecycleSource[data.ID] = source
			parentSource[data.ID] = source
		case eventState:
			data := decoded.payload.(StateEvent)
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.ID, "orphan state event")
			}
			if !isReadableState(data.NewState) {
				return nil, replayInvariantError(context, event.Type, data.ID, fmt.Sprintf("invalid state %q", data.NewState))
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			task.State = data.NewState
			lifecycleSource[data.ID] = replayEventSource{context: context, kind: event.Type}
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			// Current non-doing lifecycle states clear the claim.
			if data.NewState == stateTodo || isFinishedState(data.NewState) {
				task.ClaimedBy = ""
				task.ClaimedAt = time.Time{}
			}
		case eventClaim:
			data := decoded.payload.(ClaimEvent)
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.ID, "orphan claim event")
			}
			if strings.TrimSpace(data.AgentID) == "" {
				return nil, replayInvariantError(context, event.Type, data.ID, "claim agent is empty")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			task.ClaimedBy = data.AgentID
			task.ClaimedAt = ts
			lifecycleSource[data.ID] = replayEventSource{context: context, kind: event.Type}
		case eventLink:
			data := decoded.payload.(LinkEvent)
			if _, tombstoned := graph.Tombstones[data.FromID]; tombstoned {
				continue
			}
			if _, tombstoned := graph.Tombstones[data.ToID]; tombstoned {
				continue
			}
			if data.Type != dependsLinkType {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, fmt.Sprintf("unknown link type %q", data.Type))
			}
			fromTask, fromOK := graph.Tasks[data.FromID]
			toTask, toOK := graph.Tasks[data.ToID]
			if !fromOK || !toOK {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, "dangling dependency endpoint")
			}
			if err := validateDepSelf(data.FromID, data.ToID); err != nil {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, err.Error())
			}
			if err := validateDepAncestry(fromTask, toTask); err != nil {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, err.Error())
			}
			if hasCycle(graph, data.FromID, data.ToID) {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, "dependency cycle")
			}
			if graph.Deps[data.FromID] == nil {
				graph.Deps[data.FromID] = map[string]struct{}{}
			}
			graph.Deps[data.FromID][data.ToID] = struct{}{}
			if linkSource[data.FromID] == nil {
				linkSource[data.FromID] = map[string]replayEventSource{}
			}
			linkSource[data.FromID][data.ToID] = replayEventSource{context: context, kind: event.Type, order: eventIndex}
		case eventUnlink:
			data := decoded.payload.(LinkEvent)
			if _, tombstoned := graph.Tombstones[data.FromID]; tombstoned {
				continue
			}
			if _, tombstoned := graph.Tombstones[data.ToID]; tombstoned {
				continue
			}
			if data.Type != dependsLinkType {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, fmt.Sprintf("unknown link type %q", data.Type))
			}
			if graph.Tasks[data.FromID] == nil || graph.Tasks[data.ToID] == nil {
				return nil, replayInvariantError(context, event.Type, data.FromID+" -> "+data.ToID, "dangling dependency endpoint")
			}
			if graph.Deps[data.FromID] != nil {
				delete(graph.Deps[data.FromID], data.ToID)
			}
			if linkSource[data.FromID] != nil {
				delete(linkSource[data.FromID], data.ToID)
			}
		case eventTitle:
			data := decoded.payload.(TitleUpdateEvent)
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.ID, "orphan title event")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			task.Title = data.Title
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
		case eventBody:
			data := decoded.payload.(BodyUpdateEvent)
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.ID, "orphan body event")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			task.Body = data.Body
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
		case eventEpic:
			data := decoded.payload.(EpicAssignEvent)
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.ID, "orphan epic assignment")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			task.EpicID = data.EpicID
			parentSource[data.ID] = replayEventSource{context: context, kind: event.Type, order: eventIndex}
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
		case eventUnclaim:
			data := decoded.payload.(UnclaimEvent)
			if _, tombstoned := graph.Tombstones[data.ID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.ID, "orphan unclaim event")
			}
			if _, err := parseTime(data.TS); err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			task.ClaimedBy = ""
			task.ClaimedAt = time.Time{}
			lifecycleSource[data.ID] = replayEventSource{context: context, kind: event.Type}
		case eventTombstone:
			data := decoded.payload.(TombstoneEvent)
			if data.ID == "" {
				return nil, replayInvariantError(context, event.Type, "", "task id is empty")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			applyTombstone(graph, data.ID, TombstoneInfo{AgentID: data.AgentID, At: ts})
		case eventResult:
			data := decoded.payload.(ResultEvent)
			if _, tombstoned := graph.Tombstones[data.TaskID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.TaskID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.TaskID, "orphan result event")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.TaskID, fmt.Errorf("invalid ts: %w", err))
			}
			// Prepend to keep newest first
			result := Result{
				Summary:           data.Summary,
				Path:              data.Path,
				Sha256AtAttach:    data.Sha256AtAttach,
				MtimeAtAttach:     data.MtimeAtAttach,
				GitCommitAtAttach: data.GitCommitAtAttach,
				CreatedAt:         ts,
			}
			task.Results = append([]Result{result}, task.Results...)
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
		case eventMessage:
			data := decoded.payload.(MessageEvent)
			if _, tombstoned := graph.Tombstones[data.TaskID]; tombstoned {
				continue
			}
			task, ok := graph.Tasks[data.TaskID]
			if !ok {
				return nil, replayInvariantError(context, event.Type, data.TaskID, "orphan message event")
			}
			if err := validateMessageKind(data.Kind); err != nil {
				return nil, replayInvariantError(context, event.Type, data.TaskID, err.Error())
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.TaskID, fmt.Errorf("invalid ts: %w", err))
			}
			task.Messages = append([]Message{{Kind: data.Kind, Text: data.Text, CreatedAt: ts}}, task.Messages...)
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
		default:
			return nil, replayInvariantError(context, decoded.wireKind, "", "unsupported canonical event kind")
		}
	}

	if err := validateReplayInvariants(graph, taskSource, lifecycleSource, parentSource, linkSource); err != nil {
		return nil, err
	}

	applyLegacyTitleMigration(graph)
	graph.rebuildIndexes()

	return graph, nil
}

func applyTombstone(graph *Graph, id string, info TombstoneInfo) {
	if graph == nil {
		return
	}
	graph.Tombstones[id] = info
	delete(graph.Tasks, id)
	delete(graph.legacyEmptyEpics, id)
	delete(graph.Deps, id)
	for from, deps := range graph.Deps {
		if _, ok := deps[id]; ok {
			delete(deps, id)
			if len(deps) == 0 {
				delete(graph.Deps, from)
			}
		}
	}
}
