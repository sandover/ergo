// Core domain values live here so every command shares one state vocabulary.
// Only doing work may carry a claim. Finished-state membership controls both
// dependency release and epic completion, so callers must use isFinishedState
// instead of spelling their own state lists. Legacy error remains readable but
// current writers must never create it.
package ergo

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	stateTodo     = "todo"
	stateDoing    = "doing"
	stateDone     = "done"
	stateFailed   = "failed"
	stateBlocked  = "blocked"
	stateCanceled = "canceled"
	stateError    = "error"

	dependsLinkType = "depends"
)

func isFinishedState(state string) bool {
	switch state {
	case stateDone, stateFailed, stateCanceled:
		return true
	default:
		return false
	}
}

var (
	ErrNoErgoDir = errors.New("no .ergo directory found")
	ErrLockBusy  = errors.New("lock busy")
)

// validateClaimInvariant checks that the claim/state relationship is valid.
// Forward state writes require a claim exactly when the state is doing.
func validateClaimInvariant(state, claimedBy string) error {
	if state == stateDoing && claimedBy == "" {
		return errors.New("state=doing requires a claim")
	}
	if state != stateDoing && claimedBy != "" {
		return fmt.Errorf("state=%s must have no claim", state)
	}
	return nil
}

// Dependency rules: defines valid dependency relationships.
// Design decisions (1.0 unified model):
// - Any two non-ancestor tasks may depend on each other
// - A task cannot depend on its own container (parent) or vice versa
// - self-dep: forbidden (A cannot depend on A)
// - cycles: forbidden (A→B→...→A not allowed)

// validateDepAncestry checks that neither task is the other's container.
// A task cannot depend on its parent epic, nor can a parent depend on its child.
func validateDepAncestry(from, to *Task) error {
	if from == nil || to == nil {
		return nil
	}
	if from.EpicID == to.ID {
		return errors.New("task cannot depend on its own epic")
	}
	if to.EpicID == from.ID {
		return errors.New("epic cannot depend on its own child")
	}
	return nil
}

// validateDepSelf checks for self-dependencies.
func validateDepSelf(from, to string) error {
	if from == to {
		return errors.New("cannot depend on self")
	}
	return nil
}

// RepositoryOptions configures repository discovery and locking.
type RepositoryOptions struct {
	StartDir string
}

// GlobalOptions remains as a compatibility alias while command adapters move
// to the Application surface. It contains repository configuration only.
type GlobalOptions = RepositoryOptions

type Task struct {
	ID        string
	UUID      string
	EpicID    string
	State     string
	Title     string
	Body      string
	ClaimedBy string
	ClaimedAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Results   []Result  // Attached results/artifacts, newest first
	Messages  []Message // Lifecycle messages, newest first
}

type Graph struct {
	Tasks      map[string]*Task
	Deps       map[string]map[string]struct{}
	Tombstones map[string]TombstoneInfo

	reverseDeps      map[string]map[string]struct{}
	childrenByEpic   map[string][]*Task
	legacyEmptyEpics map[string]struct{}
}

type TombstoneInfo struct {
	AgentID string
	At      time.Time
}

// Result represents an attached result/artifact for a task.
// Path is relative to the project root; file_url is derived at read time.
type Result struct {
	Summary           string    `json:"summary"`
	Path              string    `json:"path"`                           // relative to project root
	Sha256AtAttach    string    `json:"sha256_at_attach"`               // hash when attached
	MtimeAtAttach     string    `json:"mtime_at_attach,omitempty"`      // optional
	GitCommitAtAttach string    `json:"git_commit_at_attach,omitempty"` // optional
	CreatedAt         time.Time `json:"created_at"`
}

// Message is a durable lifecycle note reconstructed from the event log.
type Message struct {
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func validateMessageKind(kind string) error {
	switch kind {
	case "done", "fail", "block", "cancel", "release":
		return nil
	default:
		return fmt.Errorf("invalid lifecycle message kind: %s", kind)
	}
}

const maxResultSummaryLen = 120

// validateResultSummary ensures summary is non-empty, single-line, and ≤120 chars.
func validateResultSummary(summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return errors.New("result summary required")
	}
	if strings.ContainsAny(summary, "\n\r") {
		return errors.New("result summary must be single line")
	}
	if len(summary) > maxResultSummaryLen {
		return fmt.Errorf("result summary too long (max %d chars)", maxResultSummaryLen)
	}
	return nil
}
