// JSON output shapes and list-item formatting helpers.
package ergo

import (
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
	"strings"
)

type taskListItem struct {
	Kind       string `json:"kind,omitempty"`
	ID         string `json:"id"`
	EpicID     string `json:"epic_id,omitempty"`
	State      string `json:"state"`
	ClaimedBy  string `json:"claimed_by,omitempty"`
	Title      string `json:"title"`
	Worker     string `json:"worker,omitempty"`
	Ready      bool   `json:"ready"`
	Blocked    bool   `json:"blocked"`
	HasResults bool   `json:"has_results,omitempty"`
}

// resultOutputItem is the JSON representation of a result with derived file_url.
type resultOutputItem struct {
	Summary           string `json:"summary"`
	Path              string `json:"path"`
	FileURL           string `json:"file_url"`
	Sha256AtAttach    string `json:"sha256_at_attach"`
	MtimeAtAttach     string `json:"mtime_at_attach,omitempty"`
	GitCommitAtAttach string `json:"git_commit_at_attach,omitempty"`
	CreatedAt         string `json:"created_at"`
}

type taskShowOutput struct {
	ID        string             `json:"id"`
	UUID      string             `json:"uuid"`
	EpicID    string             `json:"epic_id"`
	State     string             `json:"state"`
	Worker    string             `json:"worker"`
	ClaimedBy string             `json:"claimed_by"`
	ClaimedAt string             `json:"claimed_at"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
	Deps      []string           `json:"deps"`
	RDeps     []string           `json:"rdeps"`
	Body      string             `json:"body"`
	Results   []resultOutputItem `json:"results,omitempty"`
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

// buildResultOutputItem converts a Result to its JSON output form with derived file_url.
func buildResultOutputItem(result Result, repoDir string) resultOutputItem {
	return resultOutputItem{
		Summary:           result.Summary,
		Path:              result.Path,
		FileURL:           deriveFileURL(result.Path, repoDir),
		Sha256AtAttach:    result.Sha256AtAttach,
		MtimeAtAttach:     result.MtimeAtAttach,
		GitCommitAtAttach: result.GitCommitAtAttach,
		CreatedAt:         formatTime(result.CreatedAt),
	}
}

// buildResultOutputItems converts a slice of Results to their JSON output form.
func buildResultOutputItems(results []Result, repoDir string) []resultOutputItem {
	if len(results) == 0 {
		return nil
	}
	items := make([]resultOutputItem, len(results))
	for i, result := range results {
		items[i] = buildResultOutputItem(result, repoDir)
	}
	return items
}

// deriveFileURL creates a file:// URL from a relative path and repo directory.
func deriveFileURL(relPath, repoDir string) string {
	absPath := filepath.Join(repoDir, relPath)
	u := url.URL{
		Scheme: "file",
		Path:   absPath,
	}
	return u.String()
}

func buildTaskListItems(tasks []*Task, graph *Graph, repoDir string) []taskListItem {
	items := make([]taskListItem, 0, len(tasks))
	for _, task := range tasks {
		item := taskListItem{
			Kind:       string(kindForTask(task)),
			ID:         task.ID,
			EpicID:     task.EpicID,
			State:      task.State,
			ClaimedBy:  task.ClaimedBy,
			Title:      firstLine(task.Body),
			Worker:     string(task.Worker),
			Ready:      isReady(task, graph),
			Blocked:    isBlocked(task, graph),
			HasResults: len(task.Results) > 0,
		}
		items = append(items, item)
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
