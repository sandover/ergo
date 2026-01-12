// JSON output shapes and list-item formatting helpers.
package main

import (
	"encoding/json"
	"io"
	"strings"
)

type taskListItem struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	EpicID     string `json:"epic_id"`
	State      string `json:"state"`
	ClaimedBy  string `json:"claimed_by"`
	ClaimedAt  string `json:"claimed_at"`
	Title      string `json:"title"`
	Worker     string `json:"worker"`
	Ready      bool   `json:"ready"`
	Blocked    bool   `json:"blocked"`
	HasResults bool   `json:"has_results,omitempty"`
}

type taskShowOutput struct {
	ID        string   `json:"id"`
	UUID      string   `json:"uuid"`
	EpicID    string   `json:"epic_id"`
	State     string   `json:"state"`
	Worker    string   `json:"worker"`
	ClaimedBy string   `json:"claimed_by"`
	ClaimedAt string   `json:"claimed_at"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Deps      []string `json:"deps"`
	RDeps     []string `json:"rdeps"`
	Body      string   `json:"body"`
	Results   []Result `json:"results,omitempty"`
}

type initOutput struct {
	ErgoDir string `json:"ergo_dir"`
}

type createOutput struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	UUID      string `json:"uuid"`
	EpicID    string `json:"epic_id"`
	State     string `json:"state"`
	Worker    string `json:"worker"`
	CreatedAt string `json:"created_at"`
}

type whereOutput struct {
	ErgoDir string `json:"ergo_dir"`
	RepoDir string `json:"repo_dir"`
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(v)
}

func buildTaskListItems(tasks []*Task, graph *Graph) []taskListItem {
	items := make([]taskListItem, 0, len(tasks))
	for _, task := range tasks {
		meta := graph.Meta[task.ID]
		items = append(items, taskListItem{
			Kind:       string(kindForTask(task)),
			ID:         task.ID,
			EpicID:     task.EpicID,
			State:      task.State,
			ClaimedBy:  task.ClaimedBy,
			ClaimedAt:  claimedAtForTask(task, meta),
			Title:      firstLine(task.Body),
			Worker:     string(task.Worker),
			Ready:      isReady(task, graph),
			Blocked:    isBlocked(task, graph),
			HasResults: len(task.Results) > 0,
		})
	}
	return items
}

func firstLine(body string) string {
	if body == "" {
		return ""
	}
	line, _, _ := strings.Cut(body, "\n")
	return strings.TrimSpace(line)
}

func claimedAtForTask(task *Task, meta *TaskMeta) string {
	if task == nil || meta == nil || task.ClaimedBy == "" {
		return ""
	}
	if meta.LastClaimAt.IsZero() {
		return ""
	}
	return formatTime(meta.LastClaimAt)
}
