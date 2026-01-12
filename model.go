// Core domain types, constants, and parsing helpers for workers/kinds/state.
package main

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

func parseWorker(value string) (Worker, error) {
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
	if task.EpicID == "" {
		return kindEpic
	}
	return kindTask
}

func isEpic(task *Task) bool {
	if task == nil {
		return false
	}
	return task.EpicID == ""
}

var (
	errNoErgoDir   = errors.New("no .ergo directory found")
	errLockBusy    = errors.New("lock busy")
	errLockTimeout = errors.New("lock timeout")
)

var validStates = map[string]struct{}{
	stateTodo:     {},
	stateDoing:    {},
	stateDone:     {},
	stateBlocked:  {},
	stateCanceled: {},
}

type GlobalOptions struct {
	StartDir    string
	ReadOnly    bool
	LockTimeout time.Duration
	As          Worker
	AgentID     string
	Quiet       bool
	Verbose     bool
}

const defaultLockTimeout = 30 * time.Second

type Task struct {
	ID        string
	UUID      string
	EpicID    string
	State     string
	Body      string
	Worker    Worker
	ClaimedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
	Deps      []string
	RDeps     []string
}

type TaskMeta struct {
	CreatedBody   string
	CreatedState  string
	CreatedWorker Worker
	CreatedAt     time.Time
	LastStateAt   time.Time
	LastClaimAt   time.Time
	LastWorkerAt  time.Time
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
