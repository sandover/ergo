// Event replay, compaction, and readiness/blocking logic.
package ergo

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func replayEvents(events []Event) (*Graph, error) {
	graph := &Graph{
		Tasks: map[string]*Task{},
		Deps:  map[string]map[string]struct{}{},
		RDeps: map[string]map[string]struct{}{},
		Meta:  map[string]*TaskMeta{},
	}

	for _, event := range events {
		switch event.Type {
		case "new_task", "new_epic":
			var data NewTaskEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			if _, exists := graph.Tasks[data.ID]; exists {
				return nil, fmt.Errorf("duplicate task id %s", data.ID)
			}
			createdAt, err := parseTime(data.CreatedAt)
			if err != nil {
				return nil, err
			}
			taskWorker, err := ParseWorker(data.Worker)
			if err != nil {
				return nil, err
			}
			task := &Task{
				ID:        data.ID,
				UUID:      data.UUID,
				EpicID:    data.EpicID,
				IsEpic:    event.Type == "new_epic",
				State:     data.State,
				Title:     data.Title,
				Body:      data.Body,
				Worker:    taskWorker,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}
			graph.Tasks[data.ID] = task
			graph.Meta[data.ID] = &TaskMeta{
				CreatedTitle:     data.Title,
				CreatedBody:      data.Body,
				CreatedState:     data.State,
				CreatedWorker:    taskWorker,
				CreatedEpicID:    data.EpicID,
				CreatedEpicIDSet: true,
				CreatedAt:        createdAt,
			}
		case "state":
			var data StateEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
			}
			task.State = data.NewState
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			// todo/done/canceled clear claim
			if data.NewState == stateTodo || data.NewState == stateDone || data.NewState == stateCanceled {
				task.ClaimedBy = ""
			}
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastStateAt = ts
			}
		case "claim":
			var data ClaimEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
			}
			task.ClaimedBy = data.AgentID
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastClaimAt = ts
			}
		case "link":
			var data LinkEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			if data.Type != dependsLinkType {
				continue
			}
			if graph.Deps[data.FromID] == nil {
				graph.Deps[data.FromID] = map[string]struct{}{}
			}
			graph.Deps[data.FromID][data.ToID] = struct{}{}
		case "unlink":
			var data LinkEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			if data.Type != dependsLinkType {
				continue
			}
			if graph.Deps[data.FromID] != nil {
				delete(graph.Deps[data.FromID], data.ToID)
			}
		case "worker":
			var data WorkerEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
			}
			taskWorker, err := ParseWorker(data.Worker)
			if err != nil {
				return nil, err
			}
			task.Worker = taskWorker
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastWorkerAt = ts
			}
		case "title":
			var data TitleUpdateEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
			}
			task.Title = data.Title
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastTitleAt = ts
			}
		case "body":
			var data BodyUpdateEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
			}
			task.Body = data.Body
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastBodyAt = ts
			}
		case "epic":
			var data EpicAssignEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
			}
			task.EpicID = data.EpicID
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastEpicAt = ts
			}
		case "unclaim":
			var data UnclaimEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.ID]
			if !ok {
				continue
			}
			task.ClaimedBy = ""
		case "result":
			var data ResultEvent
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return nil, err
			}
			task, ok := graph.Tasks[data.TaskID]
			if !ok {
				continue
			}
			ts, err := parseTime(data.TS)
			if err != nil {
				return nil, err
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
		}
	}

	for from, deps := range graph.Deps {
		for to := range deps {
			if graph.RDeps[to] == nil {
				graph.RDeps[to] = map[string]struct{}{}
			}
			graph.RDeps[to][from] = struct{}{}
		}
	}

	for id, task := range graph.Tasks {
		task.Deps = sortedKeys(graph.Deps[id])
		task.RDeps = sortedKeys(graph.RDeps[id])
	}

	applyLegacyTitleMigration(graph)

	return graph, nil
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

func compactEvents(graph *Graph) ([]Event, error) {
	tasks := sortedTasks(graph.Tasks)
	var events []Event

	for _, task := range tasks {
		meta := graph.Meta[task.ID]
		createdAt := task.CreatedAt
		createdState := task.State
		createdTitle := task.Title
		createdBody := task.Body
		createdWorker := task.Worker
		createdEpicID := task.EpicID
		var lastStateAt time.Time
		var lastClaimAt time.Time
		var lastWorkerAt time.Time
		var lastTitleAt time.Time
		var lastBodyAt time.Time
		var lastEpicAt time.Time
		if meta != nil {
			if !meta.CreatedAt.IsZero() {
				createdAt = meta.CreatedAt
			}
			if meta.CreatedState != "" {
				createdState = meta.CreatedState
			}
			if meta.CreatedTitle != "" {
				createdTitle = meta.CreatedTitle
			}
			if meta.CreatedBody != "" {
				createdBody = meta.CreatedBody
			}
			if meta.CreatedWorker != "" {
				createdWorker = meta.CreatedWorker
			}
			if meta.CreatedEpicIDSet {
				createdEpicID = meta.CreatedEpicID
			}
			lastStateAt = meta.LastStateAt
			lastClaimAt = meta.LastClaimAt
			lastWorkerAt = meta.LastWorkerAt
			lastTitleAt = meta.LastTitleAt
			lastBodyAt = meta.LastBodyAt
			lastEpicAt = meta.LastEpicAt
		}

		payload := NewTaskEvent{
			ID:        task.ID,
			UUID:      task.UUID,
			EpicID:    createdEpicID,
			State:     createdState,
			Title:     createdTitle,
			Body:      createdBody,
			Worker:    string(createdWorker),
			CreatedAt: formatTime(createdAt),
		}
		eventType := "new_task"
		if task.IsEpic {
			eventType = "new_epic"
		}
		event, err := newEvent(eventType, createdAt, payload)
		if err != nil {
			return nil, err
		}
		events = append(events, event)

		if task.Title != createdTitle || (!lastTitleAt.IsZero() && lastTitleAt.After(createdAt)) {
			ts := pickTime(lastTitleAt, task.UpdatedAt)
			titleEvent, err := newEvent("title", ts, TitleUpdateEvent{
				ID:    task.ID,
				Title: task.Title,
				TS:    formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, titleEvent)
		}

		if task.Body != createdBody || (!lastBodyAt.IsZero() && lastBodyAt.After(createdAt)) {
			ts := pickTime(lastBodyAt, task.UpdatedAt)
			bodyEvent, err := newEvent("body", ts, BodyUpdateEvent{
				ID:   task.ID,
				Body: task.Body,
				TS:   formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, bodyEvent)
		}

		if !task.IsEpic && (task.EpicID != createdEpicID || (!lastEpicAt.IsZero() && lastEpicAt.After(createdAt))) {
			ts := pickTime(lastEpicAt, task.UpdatedAt)
			epicEvent, err := newEvent("epic", ts, EpicAssignEvent{
				ID:     task.ID,
				EpicID: task.EpicID,
				TS:     formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, epicEvent)
		}

		if (task.Worker != createdWorker || (!lastWorkerAt.IsZero() && lastWorkerAt.After(createdAt))) && task.Worker != "" {
			ts := pickTime(lastWorkerAt, task.UpdatedAt)
			workerEvent, err := newEvent("worker", ts, WorkerEvent{
				ID:     task.ID,
				Worker: string(task.Worker),
				TS:     formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, workerEvent)
		}

		if task.ClaimedBy != "" {
			ts := pickTime(lastClaimAt, task.UpdatedAt)
			claimEvent, err := newEvent("claim", ts, ClaimEvent{
				ID:      task.ID,
				AgentID: task.ClaimedBy,
				TS:      formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, claimEvent)
		}

		if task.State != createdState || (!lastStateAt.IsZero() && lastStateAt.After(createdAt)) {
			ts := pickTime(lastStateAt, task.UpdatedAt)
			stateEvent, err := newEvent("state", ts, StateEvent{
				ID:       task.ID,
				NewState: task.State,
				TS:       formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, stateEvent)
		}

		// Emit result events (in chronological order, oldest first)
		for i := len(task.Results) - 1; i >= 0; i-- {
			result := task.Results[i]
			resultEvent, err := newEvent("result", result.CreatedAt, ResultEvent{
				TaskID:            task.ID,
				Summary:           result.Summary,
				Path:              result.Path,
				Sha256AtAttach:    result.Sha256AtAttach,
				MtimeAtAttach:     result.MtimeAtAttach,
				GitCommitAtAttach: result.GitCommitAtAttach,
				TS:                formatTime(result.CreatedAt),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, resultEvent)
		}
	}

	fromIDs := sortedMapKeys(graph.Deps)
	for _, from := range fromIDs {
		toIDs := sortedKeys(graph.Deps[from])
		for _, to := range toIDs {
			now := time.Now().UTC()
			linkEvent, err := newEvent("link", now, LinkEvent{
				FromID: from,
				ToID:   to,
				Type:   dependsLinkType,
			})
			if err != nil {
				return nil, err
			}
			events = append(events, linkEvent)
		}
	}

	return events, nil
}

func readyTasks(graph *Graph, epicID string, kind Kind) []*Task {
	tasks := listTasks(graph, epicID, true, false, true)
	if len(tasks) == 0 {
		return nil
	}
	tasks = filterTasksByKind(tasks, kind)
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

func filterTasksByKind(tasks []*Task, kind Kind) []*Task {
	if kind == "" || kind == kindAny {
		return tasks
	}
	filtered := tasks[:0]
	for _, task := range tasks {
		if kindForTask(task) == kind {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func listTasks(graph *Graph, epicID string, readyOnly, blockedOnly, includeAll bool) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if epicID != "" && task.EpicID != epicID {
			continue
		}
		ready := isReady(task, graph)
		blocked := isBlocked(task, graph)
		if readyOnly && !ready {
			continue
		}
		if blockedOnly && !blocked {
			continue
		}
		if !includeAll && !ready && !blocked {
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

func sortByCreatedAt(tasks []*Task) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
}

func isReady(task *Task, graph *Graph) bool {
	if task == nil {
		return false
	}
	if task.State != stateTodo {
		return false
	}
	if task.ClaimedBy != "" {
		return false
	}
	for dep := range graph.Deps[task.ID] {
		other, ok := graph.Tasks[dep]
		if !ok {
			continue
		}
		if other.State != stateDone && other.State != stateCanceled {
			return false
		}
	}
	// For tasks in an epic, check if epic's epic-deps are complete
	if task.EpicID != "" {
		if !areEpicDepsComplete(task.EpicID, graph) {
			return false
		}
	}
	return true
}

func isBlocked(task *Task, graph *Graph) bool {
	if task == nil {
		return false
	}
	if task.State == stateBlocked {
		return true
	}
	if task.State != stateTodo || task.ClaimedBy != "" {
		return false
	}
	for dep := range graph.Deps[task.ID] {
		other, ok := graph.Tasks[dep]
		if !ok {
			continue
		}
		if other.State != stateDone && other.State != stateCanceled {
			return true
		}
	}
	// For tasks in an epic, check if epic's epic-deps are incomplete
	if task.EpicID != "" {
		if !areEpicDepsComplete(task.EpicID, graph) {
			return true
		}
	}
	return false
}

// isEpicComplete returns true if all tasks in the epic are done or canceled.
// An epic with no tasks is considered complete.
func isEpicComplete(epicID string, graph *Graph) bool {
	for _, task := range graph.Tasks {
		if task.EpicID == epicID {
			if task.State != stateDone && task.State != stateCanceled {
				return false
			}
		}
	}
	return true
}

// areEpicDepsComplete returns true if all epics that the given epic depends on are complete.
func areEpicDepsComplete(epicID string, graph *Graph) bool {
	for depID := range graph.Deps[epicID] {
		depEpic, ok := graph.Tasks[depID]
		if !ok {
			continue
		}
		if !isEpic(depEpic) {
			continue
		}
		if !isEpicComplete(depID, graph) {
			return false
		}
	}
	return true
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
