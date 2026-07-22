// Purpose: Define command result records and file-link formatting helpers.
// Exports: none (package-internal output helpers).
// Role: Shared data passed from storage operations to readable CLI renderers.
// Invariants: Command result records contain only data needed after a write.
// Notes: File URLs are derived from repo-relative paths.
package ergo

import (
	"net/url"
	"path/filepath"
	"strings"
)

type createOutput struct {
	Container bool
	ID        string
	UUID      string
	EpicID    string
	State     string
	Title     string
	Body      string
	CreatedAt string
}

// bulkCreateChildOutput is a compact child task entry in a bulk-create result.
type bulkCreateChildOutput struct {
	ID    string
	Title string
}

// bulkCreateOutput is the result of creating a container and its children.
type bulkCreateOutput struct {
	ID       string
	Title    string
	Children []bulkCreateChildOutput
	Edges    []sequenceEdge
}

// deriveFileURL creates a file:// URL from a relative path and repo directory.
func deriveFileURL(relPath, repoDir string) string {
	absPath := filepath.Join(repoDir, relPath)
	absPath = strings.ReplaceAll(absPath, "\\", "/")
	absPath = filepath.ToSlash(absPath)
	if len(absPath) >= 2 && absPath[1] == ':' && !strings.HasPrefix(absPath, "/") {
		absPath = "/" + absPath
	}
	u := url.URL{
		Scheme: "file",
		Path:   absPath,
	}
	return u.String()
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
