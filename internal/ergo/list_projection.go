package ergo

import "strings"

func taskClaimedFields(task *Task) (claimedBy, claimedAt *string) {
	if task == nil || task.ClaimedBy == "" {
		return nil, nil
	}
	by := task.ClaimedBy
	claimedBy = &by
	if !task.ClaimedAt.IsZero() {
		at := formatTime(task.ClaimedAt)
		claimedAt = &at
	}
	return claimedBy, claimedAt
}

func taskDependsOnJSON(graph *Graph, taskID string) []string {
	if graph == nil {
		return []string{}
	}
	deps := graph.Dependencies(taskID)
	if len(deps) == 0 {
		return []string{}
	}
	return deps
}

type taskJSONProjection struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind"`
	State      string   `json:"state,omitempty"`
	EpicID     string   `json:"epic_id,omitempty"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	ClaimedAt  *string  `json:"claimed_at"`
	ClaimedBy  *string  `json:"claimed_by"`
	DependsOn  []string `json:"depends_on"`
	Body       string   `json:"body"`
}

func projectTaskJSON(graph *Graph, task *Task, includeBody bool) taskJSONProjection {
	if task == nil {
		return taskJSONProjection{}
	}
	claimedBy, claimedAt := taskClaimedFields(task)
	projection := taskJSONProjection{
		ID:        task.ID,
		Title:     task.Title,
		CreatedAt: formatTime(task.CreatedAt),
		UpdatedAt: formatTime(task.UpdatedAt),
		ClaimedAt: claimedAt,
		ClaimedBy: claimedBy,
		DependsOn: taskDependsOnJSON(graph, task.ID),
	}
	if graph != nil && graph.IsEpic(task.ID) {
		projection.Kind = "epic"
		projection.State = graph.EpicState(task.ID)
	} else {
		projection.Kind = "task"
		projection.State = task.State
		projection.EpicID = task.EpicID
	}
	if includeBody {
		projection.Body = task.Body
	}
	return projection
}

func formatBatchShowMissingWarning(id string) string {
	id = strings.TrimSpace(id)
	return "ergo batch-show: unknown task id " + id
}
