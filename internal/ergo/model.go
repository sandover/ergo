// Core domain types, constants, and parsing helpers for workers/kinds/state.
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

type Worker string

const (
	workerAny   Worker = "any"
	workerAgent Worker = "agent"
	workerHuman Worker = "human"
)

type Kind string

const (
	kindAny  Kind = "any"
	kindTask Kind = "task"
	kindEpic Kind = "epic"
)

func ParseWorker(value string) (Worker, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "any":
		return workerAny, nil
	case "agent":
		return workerAgent, nil
	case "human":
		return workerHuman, nil
	default:
		return "", fmt.Errorf("invalid worker %s (use any, agent, or human)", value)
	}
}

func isWorkerAllowed(taskWorker Worker, as Worker) bool {
	if taskWorker == "" {
		taskWorker = workerAny
	}
	if as == "" || as == workerAny {
		return true
	}
	return taskWorker == workerAny || taskWorker == as
}

func parseKind(value string) (Kind, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "any":
		return kindAny, nil
	case "task", "tasks":
		return kindTask, nil
	case "epic", "epics":
		return kindEpic, nil
	default:
		return "", fmt.Errorf("invalid kind %s (use any, task, or epic)", value)
	}
}

func kindForTask(task *Task) Kind {
	if task == nil {
		return kindTask
	}
	if task.IsEpic {
		return kindEpic
	}
	return kindTask
}

func isEpic(task *Task) bool {
	if task == nil {
		return false
	}
	return task.IsEpic
}

var (
	ErrNoErgoDir   = errors.New("no .ergo directory found")
	ErrLockBusy    = errors.New("lock busy")
	ErrLockTimeout = errors.New("lock timeout")
)

var validStates = map[string]struct{}{
	stateTodo:     {},
	stateDoing:    {},
	stateDone:     {},
	stateBlocked:  {},
	stateCanceled: {},
	stateError:    {},
}

// State machine: valid transitions and claim invariants.
// Design decisions:
// - done/canceled are NOT terminal (can reopen via →todo)
// - todo→done allowed (quick completion without claiming)
// - blocked can transition to any non-terminal state
// - error represents failed work; can retry (→doing), reassign (→todo), or give up (→canceled)
// - error preserves claim to show who failed
var validTransitions = map[string]map[string]struct{}{
	stateTodo:     {stateDoing: {}, stateDone: {}, stateBlocked: {}, stateCanceled: {}},
	stateDoing:    {stateTodo: {}, stateDone: {}, stateBlocked: {}, stateCanceled: {}, stateError: {}},
	stateBlocked:  {stateTodo: {}, stateDoing: {}, stateDone: {}, stateCanceled: {}},
	stateDone:     {stateTodo: {}},                                    // reopen only
	stateCanceled: {stateTodo: {}},                                    // reopen only
	stateError:    {stateTodo: {}, stateDoing: {}, stateCanceled: {}}, // retry, reassign, or give up
}

// validateTransition checks if from→to is a valid state transition.
// Returns nil if valid, error describing why if not.
func validateTransition(from, to string) error {
	if from == to {
		return nil // no-op is always valid
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("unknown state: %s", from)
	}
	if _, valid := allowed[to]; !valid {
		return fmt.Errorf("invalid transition: %s → %s", from, to)
	}
	return nil
}

// validateClaimInvariant checks that the claim/state relationship is valid.
// doing requires a claim; todo/done/canceled must have no claim.
// error keeps claim (shows who failed); blocked can have or not have claim.
func validateClaimInvariant(state, claimedBy string) error {
	switch state {
	case stateDoing:
		if claimedBy == "" {
			return errors.New("state=doing requires a claim")
		}
	case stateError:
		if claimedBy == "" {
			return errors.New("state=error requires a claim (shows who failed)")
		}
	case stateTodo, stateDone, stateCanceled:
		if claimedBy != "" {
			return fmt.Errorf("state=%s must have no claim", state)
		}
	}
	// blocked can have or not have a claim
	return nil
}

// Dependency rules: defines valid dependency relationships.
// Design decisions:
// - task→task: allowed (standard dependency)
// - epic→epic: allowed (epic hierarchies)
// - task→epic: forbidden (tasks cannot depend on epics)
// - epic→task: forbidden (epics cannot depend on tasks)
// - self-dep: forbidden (A cannot depend on A)
// - cycles: forbidden (A→B→...→A not allowed)

// validateDepKinds checks if a dependency between from and to is valid based on their kinds.
// Both must be the same kind (both tasks or both epics).
func validateDepKinds(fromIsEpic, toIsEpic bool) error {
	if fromIsEpic != toIsEpic {
		if fromIsEpic {
			return errors.New("epic cannot depend on task")
		}
		return errors.New("task cannot depend on epic")
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
	StartDir    string
	ReadOnly    bool
	LockTimeout time.Duration
	As          Worker
	AgentID     string
	Quiet       bool
	Verbose     bool
	JSON        bool
}

const DefaultLockTimeout = 30 * time.Second

type Task struct {
	ID        string
	UUID      string
	EpicID    string
	IsEpic    bool
	State     string
	Title     string
	Body      string
	Worker    Worker
	ClaimedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
	Deps      []string
	RDeps     []string
	Results   []Result // Attached results/artifacts, newest first
}

type TaskMeta struct {
	CreatedTitle     string
	CreatedBody      string
	CreatedState     string
	CreatedWorker    Worker
	CreatedEpicID    string
	CreatedEpicIDSet bool
	CreatedAt        time.Time
	LastStateAt      time.Time
	LastClaimAt      time.Time
	LastWorkerAt     time.Time
	LastTitleAt      time.Time
	LastBodyAt       time.Time
	LastEpicAt       time.Time
}

type Graph struct {
	Tasks map[string]*Task
	Deps  map[string]map[string]struct{}
	RDeps map[string]map[string]struct{}
	Meta  map[string]*TaskMeta
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
	Worker    string `json:"worker"`
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

type WorkerEvent struct {
	ID     string `json:"id"`
	Worker string `json:"worker"`
	TS     string `json:"ts"`
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
