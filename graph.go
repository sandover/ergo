// Event replay, compaction, and readiness/blocking logic.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
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
			taskWorker, err := parseWorker(data.Worker)
			if err != nil {
				return nil, err
			}
			task := &Task{
				ID:        data.ID,
				UUID:      data.UUID,
				EpicID:    data.EpicID,
				State:     data.State,
				Body:      data.Body,
				Worker:    taskWorker,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}
			graph.Tasks[data.ID] = task
			graph.Meta[data.ID] = &TaskMeta{
				CreatedBody:   data.Body,
				CreatedState:  data.State,
				CreatedWorker: taskWorker,
				CreatedAt:     createdAt,
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
			if data.NewState == stateTodo {
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
			taskWorker, err := parseWorker(data.Worker)
			if err != nil {
				return nil, err
			}
			task.Worker = taskWorker
			task.UpdatedAt = maxTime(task.UpdatedAt, ts)
			meta := graph.Meta[data.ID]
			if meta != nil {
				meta.LastWorkerAt = ts
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

	return graph, nil
}

func compactEvents(graph *Graph) ([]Event, error) {
	tasks := sortedTasks(graph.Tasks)
	var events []Event

	for _, task := range tasks {
		meta := graph.Meta[task.ID]
		createdAt := task.CreatedAt
		createdState := task.State
		createdBody := task.Body
		createdWorker := task.Worker
		var lastStateAt time.Time
		var lastClaimAt time.Time
		var lastWorkerAt time.Time
		if meta != nil {
			if !meta.CreatedAt.IsZero() {
				createdAt = meta.CreatedAt
			}
			if meta.CreatedState != "" {
				createdState = meta.CreatedState
			}
			if meta.CreatedBody != "" {
				createdBody = meta.CreatedBody
			}
			if meta.CreatedWorker != "" {
				createdWorker = meta.CreatedWorker
			}
			lastStateAt = meta.LastStateAt
			lastClaimAt = meta.LastClaimAt
			lastWorkerAt = meta.LastWorkerAt
		}

		payload := NewTaskEvent{
			ID:        task.ID,
			UUID:      task.UUID,
			EpicID:    task.EpicID,
			State:     createdState,
			Body:      createdBody,
			Worker:    string(createdWorker),
			CreatedAt: formatTime(createdAt),
		}
		eventType := "new_task"
		if task.EpicID == "" {
			eventType = "new_epic"
		}
		event, err := newEvent(eventType, createdAt, payload)
		if err != nil {
			return nil, err
		}
		events = append(events, event)

		if task.State != createdState {
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

		if task.Worker != createdWorker && task.Worker != "" {
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

func readyTasks(graph *Graph, epicID string, as Worker, kind Kind) []*Task {
	tasks := listTasks(graph, epicID, true, false, true)
	if len(tasks) == 0 {
		return nil
	}
	tasks = filterTasksByKind(tasks, kind)
	if len(tasks) == 0 {
		return nil
	}
	if as != "" && as != workerAny {
		tasks = filterTasksByWorker(tasks, as)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

func filterTasksByWorker(tasks []*Task, as Worker) []*Task {
	if as == "" || as == workerAny {
		return tasks
	}
	filtered := tasks[:0]
	for _, task := range tasks {
		if isWorkerAllowed(task.Worker, as) {
			filtered = append(filtered, task)
		}
	}
	return filtered
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
	return false
}
