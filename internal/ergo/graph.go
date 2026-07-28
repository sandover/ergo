// Purpose: Replay events into graph state and compute readiness/compaction.
// Exports: none (package-internal graph helpers).
// Role: Core domain logic for state reconstruction and queries.
// Invariants: Tombstones remove tasks; results are ordered newest-first.
// Notes: Readiness checks direct deps and inherited container deps.
package ergo

import (
	"encoding/json"
	"fmt"
	"sort"
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
	case stateTodo, stateDoing, stateBlocked, stateDone, stateCanceled, stateError:
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
		if event.Type == "" {
			return nil, replayInvariantError(context, "", "", "event kind is empty")
		}
		if _, err := parseTime(event.TS); err != nil {
			return nil, replayDecodeError(context, event.Type, "", fmt.Errorf("invalid event ts: %w", err))
		}
		switch event.Type {
		case "new_task", "new_epic":
			var data NewTaskEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
			if event.Type == "new_epic" {
				graph.legacyEmptyEpics[data.ID] = struct{}{}
			}
			source := replayEventSource{context: context, kind: event.Type, order: eventIndex}
			taskSource[data.ID] = source
			lifecycleSource[data.ID] = source
			parentSource[data.ID] = source
		case "state":
			var data StateEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
			// todo/done/canceled clear claim
			if data.NewState == stateTodo || data.NewState == stateDone || data.NewState == stateCanceled {
				task.ClaimedBy = ""
				task.ClaimedAt = time.Time{}
			}
		case "claim":
			var data ClaimEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "link":
			var data LinkEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "unlink":
			var data LinkEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "title":
			var data TitleUpdateEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "body":
			var data BodyUpdateEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "epic":
			var data EpicAssignEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "unclaim":
			var data UnclaimEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "tombstone":
			var data TombstoneEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
			if data.ID == "" {
				return nil, replayInvariantError(context, event.Type, "", "task id is empty")
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, replayDecodeError(context, event.Type, data.ID, fmt.Errorf("invalid ts: %w", err))
			}
			applyTombstone(graph, data.ID, TombstoneInfo{AgentID: data.AgentID, At: ts})
		case "result":
			var data ResultEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
		case "message":
			var data MessageEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, replayDecodeError(context, event.Type, "", err)
			}
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
			return nil, replayInvariantError(context, event.Type, "", "unknown event kind")
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

func applyLegacyTitleMigration(graph *Graph) {
	for _, task := range graph.Tasks {
		if strings.TrimSpace(task.Title) != "" {
			continue
		}
		title, body := deriveTitleAndBodyFromLegacy(task.Body)
		task.Title = title
		task.Body = body
	}
}

func deriveTitleAndBodyFromLegacy(body string) (string, string) {
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || isLegacyHeading(trimmed) {
			continue
		}
		title := trimmed
		if i+1 >= len(lines) {
			return title, ""
		}
		return title, strings.Join(lines[i+1:], "\n")
	}
	if strings.TrimSpace(body) == "" {
		return "(untitled)", ""
	}
	return "(untitled)", body
}

func isLegacyHeading(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if !strings.HasPrefix(line, "#") {
		return false
	}
	for len(line) > 0 && line[0] == '#' {
		line = strings.TrimPrefix(line, "#")
	}
	return strings.TrimSpace(line) != ""
}

func readyTasks(graph *Graph) []*Task {
	tasks := listTasks(graph, "", true)
	if len(tasks) == 0 {
		return nil
	}
	// Exclude containers from ready list (they complete implicitly)
	tasks = filterNonContainers(tasks, graph)
	if len(tasks) == 0 {
		return nil
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

// filterNonContainers removes tasks that are containers (have children).
func filterNonContainers(tasks []*Task, graph *Graph) []*Task {
	filtered := tasks[:0]
	for _, task := range tasks {
		if !graph.IsEpic(task.ID) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func listTasks(graph *Graph, epicID string, readyOnly bool) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if epicID != "" && task.EpicID != epicID {
			continue
		}
		ready := graph.IsReady(task.ID)
		if readyOnly && !ready {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

func sortedTasks(tasks map[string]*Task) []*Task {
	values := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		values = append(values, task)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}

// isDepComplete returns true if the dependency identified by depID is fully satisfied.
// For containers: all children must be done or canceled.
// For leaves: the task must be done or canceled.
func isDepComplete(depID string, graph *Graph) bool {
	return graph != nil && graph.IsComplete(depID)
}

func isReady(task *Task, graph *Graph) bool {
	return task != nil && graph != nil && graph.IsReady(task.ID)
}

func isBlocked(task *Task, graph *Graph) bool {
	return task != nil && graph != nil && graph.IsBlocked(task.ID)
}

// isEpicComplete returns true if all tasks in the epic are done or canceled.
// An epic with no tasks is considered complete.
func isEpicComplete(epicID string, graph *Graph) bool {
	return graph != nil && graph.IsEpic(epicID) && graph.IsComplete(epicID)
}

// hasCycle returns true if adding a dependency from -> to would create a cycle.
// Uses DFS to check if 'from' is reachable from 'to' (which would mean to -> ... -> from exists).
func hasCycle(graph *Graph, from, to string) bool {
	// If from == to, it's a self-loop
	if from == to {
		return true
	}
	// Check if 'from' is reachable from 'to' via existing deps
	visited := make(map[string]bool)
	return isReachable(graph, to, from, visited)
}

// isReachable returns true if 'target' is reachable from 'start' via deps.
func isReachable(graph *Graph, start, target string, visited map[string]bool) bool {
	if start == target {
		return true
	}
	if visited[start] {
		return false
	}
	visited[start] = true
	for dep := range graph.Deps[start] {
		if isReachable(graph, dep, target, visited) {
			return true
		}
	}
	return false
}
