// Purpose: Define core domain types, constants, and validation rules.
// Exports: GlobalOptions, Task, Graph, Event, and related structs.
// Role: Shared model and state machine definitions.
// Invariants: lifecycle postconditions and claim ownership must stay consistent.
// Notes: Error values are stable sentinel constants.
package ergo

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	stateTodo     = "todo"
	stateDoing    = "doing"
	stateDone     = "done"
	stateBlocked  = "blocked"
	stateCanceled = "canceled"
	stateError    = "error"

	dependsLinkType = "depends"
)

// isContainer returns true if the task is a container.
// Current containers are derived from children; legacy new_epic events also
// mark IsEpic so old empty containers remain visible after replay.
func isContainer(task *Task, graph *Graph) bool {
	if task == nil || graph == nil {
		return false
	}
	// Legacy: tasks created via new_epic event have IsEpic=true
	if task.IsEpic {
		return true
	}
	// Derived: any task with children assigned to it is a container
	for _, t := range graph.Tasks {
		if t.EpicID == task.ID {
			return true
		}
	}
	return false
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
		return errors.New("task cannot depend on its own container")
	}
	if to.EpicID == from.ID {
		return errors.New("container cannot depend on its own child")
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

type GlobalOptions struct {
	StartDir string
	AgentID  string
}

type Task struct {
	ID     string
	UUID   string
	EpicID string
	// IsEpic is a compatibility/display cache set during replay for legacy
	// new_epic events and derived containers. Behavioral checks should prefer
	// isContainer(task, graph).
	IsEpic    bool
	State     string
	Title     string
	Body      string
	ClaimedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
	Deps      []string
	RDeps     []string
	Results   []Result  // Attached results/artifacts, newest first
	Messages  []Message // Lifecycle messages, newest first
}

type TaskMeta struct {
	CreatedTitle     string
	CreatedBody      string
	CreatedState     string
	CreatedEpicID    string
	CreatedEpicIDSet bool
	CreatedAt        time.Time
	LastStateAt      time.Time
	LastClaimAt      time.Time
	LastTitleAt      time.Time
	LastBodyAt       time.Time
	LastEpicAt       time.Time
}

type Graph struct {
	Tasks      map[string]*Task
	Deps       map[string]map[string]struct{}
	RDeps      map[string]map[string]struct{}
	Meta       map[string]*TaskMeta
	Tombstones map[string]TombstoneInfo
}

type TombstoneInfo struct {
	AgentID string
	At      time.Time
}

type Event struct {
	Type string          `json:"type"`
	TS   string          `json:"ts"`
	Data json.RawMessage `json:"data"`
}

type NewTaskEvent struct {
	ID        string `json:"id"`
	UUID      string `json:"uuid"`
	EpicID    string `json:"epic_id"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type StateEvent struct {
	ID       string `json:"id"`
	NewState string `json:"state"`
	TS       string `json:"ts"`
}

type LinkEvent struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Type   string `json:"type"`
}

type ClaimEvent struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	TS      string `json:"ts"`
}

type TitleUpdateEvent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	TS    string `json:"ts"`
}

type BodyUpdateEvent struct {
	ID   string `json:"id"`
	Body string `json:"body"`
	TS   string `json:"ts"`
}

type EpicAssignEvent struct {
	ID     string `json:"id"`
	EpicID string `json:"epic_id"`
	TS     string `json:"ts"`
}

type UnclaimEvent struct {
	ID string `json:"id"`
	TS string `json:"ts"`
}

// TombstoneEvent marks an entity as deleted in the event log.
// Interpretation is handled during replay.
type TombstoneEvent struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id,omitempty"`
	TS      string `json:"ts"`
}

// ResultEvent records a result attachment in the event log.
type ResultEvent struct {
	TaskID            string `json:"task_id"`
	Summary           string `json:"summary"`
	Path              string `json:"path"`                           // relative to project root
	Sha256AtAttach    string `json:"sha256_at_attach"`               // required
	MtimeAtAttach     string `json:"mtime_at_attach,omitempty"`      // optional
	GitCommitAtAttach string `json:"git_commit_at_attach,omitempty"` // optional
	TS                string `json:"ts"`
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

// MessageEvent records a note attached to a lifecycle postcondition.
type MessageEvent struct {
	TaskID string `json:"task_id"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	TS     string `json:"ts"`
}

// Message is a durable lifecycle note reconstructed from the event log.
type Message struct {
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func validateMessageKind(kind string) error {
	switch kind {
	case "done", "block", "cancel", "release":
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
