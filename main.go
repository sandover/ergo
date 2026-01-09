package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
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

var validStates = map[string]struct{}{
	stateTodo:     {},
	stateDoing:    {},
	stateDone:     {},
	stateBlocked:  {},
	stateCanceled: {},
}

type Task struct {
	ID        string
	UUID      string
	EpicID    string
	State     string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClaimedBy string
	Deps      []string
	RDeps     []string
}

type TaskMeta struct {
	CreatedBody  string
	CreatedState string
	CreatedAt    time.Time
	LastEditAt   time.Time
	LastStateAt  time.Time
	LastClaimAt  time.Time
}

type Graph struct {
	Tasks map[string]*Task
	Deps  map[string]map[string]struct{}
	RDeps map[string]map[string]struct{}
	Meta  map[string]*TaskMeta
}

type Event struct {
	TS   string          `json:"ts"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type NewTaskEvent struct {
	ID        string `json:"id"`
	UUID      string `json:"uuid"`
	EpicID    string `json:"epic_id,omitempty"`
	State     string `json:"state"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type StateEvent struct {
	ID       string `json:"id"`
	NewState string `json:"new_state"`
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

type EditEvent struct {
	ID   string `json:"id"`
	Body string `json:"body"`
	TS   string `json:"ts"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "help", "-h", "--help":
		printUsage()
		return
	case "init":
		if err := runInit(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "epic":
		if err := runEpic(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "task":
		if err := runTask(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "edit":
		if err := runEdit(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "state":
		if err := runState(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "link":
		if err := runLink(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "unlink":
		if err := runUnlink(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "ls":
		if err := runList(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "take":
		if err := runTake(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "show":
		if err := runShow(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "deps":
		if err := runDeps(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "rdeps":
		if err := runRDeps(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "plan":
		if err := runPlan(os.Args[2:]); err != nil {
			exitErr(err)
		}
	case "compact":
		if err := runCompact(os.Args[2:]); err != nil {
			exitErr(err)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`ergo — minimal multi-agent DAG task planner

Stores append-only events at .ergo/events.jsonl (writes are locked via .ergo/lock).
Run commands from the directory that contains .ergo (no auto-discovery yet).

Concepts:
  - IDs are 6-char human IDs (UUID shown in show).
  - States: todo | doing | done | blocked | canceled
  - deps: link A depends B => A waits for B (done/canceled)
  - READY: todo + unclaimed + all deps done/canceled
  - Bodies: stdin-first; otherwise $EDITOR (default nano). state=todo clears claim.

Commands:
  init [dir]                         create [dir]/.ergo
  epic new                           create epic (prints id)
  task new --epic <epic_id>          create task (prints id)
  edit <id>                          replace body
  state <id> <state>                 set state (todo clears claim)
  link <from> depends <to>           add dep edge
  unlink <from> depends <to>         remove dep edge
  ls [--epic <id>] [--ready|--blocked|--all]
                                    TSV: id state epic|- claimed_by|- title
  take [--epic <id>]                 claim oldest READY, set doing, print body
  show <id>                          show details + body
  deps <id> | rdeps <id>             list deps / reverse deps
  plan [--epic <id>]                 summary + per-task READY/BLOCKED
  compact                            rewrite log to current state (drops history)

Examples:
  ergo init
  epic=$(ergo epic new <<'EOF'
  My epic
  EOF
  )
  cat task.md | ergo task new --epic "$epic"
  ergo take
  ergo state <task_id> done`)
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func runInit(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	target := filepath.Join(dir, dataDirName)
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	eventsPath := filepath.Join(target, "events.jsonl")
	lockPath := filepath.Join(target, "lock")
	if _, err := os.Stat(eventsPath); err == nil {
		return fmt.Errorf("%s already exists", eventsPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(lockPath, []byte{}, 0644); err != nil {
		return err
	}
	fmt.Println("Initialized ergo at", target)
	return nil
}

func runEpic(args []string) error {
	if len(args) < 1 || args[0] != "new" {
		return errors.New("usage: ergo epic new")
	}
	return createTask("", true)
}

func runTask(args []string) error {
	if len(args) < 1 || args[0] != "new" {
		return errors.New("usage: ergo task new --epic <id>")
	}
	epicID, err := parseFlagValue(args[1:], "--epic")
	if err != nil {
		return err
	}
	if epicID == "" {
		return errors.New("missing --epic <id>")
	}
	return createTask(epicID, false)
}

func runEdit(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ergo edit <id>")
	}
	id := args[0]
	dir := dataDir()
	graph, err := loadGraph(dir)
	if err != nil {
		return err
	}
	task, ok := graph.Tasks[id]
	if !ok {
		return fmt.Errorf("unknown task id %s", id)
	}
	body, err := readBody(task.Body)
	if err != nil {
		return err
	}
	if body == task.Body {
		return nil
	}
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, func() error {
		now := time.Now().UTC()
		event, err := newEvent("edit", now, EditEvent{
			ID:   id,
			Body: body,
			TS:   formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func runState(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: ergo state <id> <state>")
	}
	id := args[0]
	state := args[1]
	if _, ok := validStates[state]; !ok {
		return fmt.Errorf("invalid state %s", state)
	}
	dir := dataDir()
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if _, ok := graph.Tasks[id]; !ok {
			return fmt.Errorf("unknown task id %s", id)
		}
		now := time.Now().UTC()
		event, err := newEvent("state", now, StateEvent{
			ID:       id,
			NewState: state,
			TS:       formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func runLink(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: ergo link <from> depends <to>")
	}
	from := args[0]
	linkType := args[1]
	to := args[2]
	if linkType != dependsLinkType {
		return fmt.Errorf("unsupported link type %s", linkType)
	}
	return writeLinkEvent("link", from, to)
}

func runUnlink(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: ergo unlink <from> depends <to>")
	}
	from := args[0]
	linkType := args[1]
	to := args[2]
	if linkType != dependsLinkType {
		return fmt.Errorf("unsupported link type %s", linkType)
	}
	return writeLinkEvent("unlink", from, to)
}

func runList(args []string) error {
	epicID, err := parseFlagValue(args, "--epic")
	if err != nil {
		return err
	}
	readyOnly := hasFlag(args, "--ready")
	blockedOnly := hasFlag(args, "--blocked")
	includeAll := hasFlag(args, "--all")
	flags := countTrue(readyOnly, blockedOnly, includeAll)
	if flags > 1 {
		return errors.New("only one of --ready, --blocked, --all is allowed")
	}
	graph, err := loadGraph(dataDir())
	if err != nil {
		return err
	}
	tasks := listTasks(graph, epicID, readyOnly, blockedOnly, includeAll)
	for _, task := range tasks {
		epic := task.EpicID
		if epic == "" {
			epic = "-"
		}
		claimed := task.ClaimedBy
		if claimed == "" {
			claimed = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", task.ID, task.State, epic, claimed, firstLine(task.Body))
	}
	return nil
}

func runTake(args []string) error {
	epicID, err := parseFlagValue(args, "--epic")
	if err != nil {
		return err
	}
	dir := dataDir()
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	var body string
	err = withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		ready := readyTasks(graph, epicID)
		if len(ready) == 0 {
			return errors.New("no ready tasks")
		}
		chosen := ready[0]
		body = chosen.Body
		now := time.Now().UTC()
		claimEvent, err := newEvent("claim", now, ClaimEvent{
			ID:      chosen.ID,
			AgentID: defaultAgentID(),
			TS:      formatTime(now),
		})
		if err != nil {
			return err
		}
		stateEvent, err := newEvent("state", now, StateEvent{
			ID:       chosen.ID,
			NewState: stateDoing,
			TS:       formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{claimEvent, stateEvent})
	})
	if err != nil {
		return err
	}
	fmt.Print(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		fmt.Println()
	}
	return nil
}

func runShow(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ergo show <id>")
	}
	id := args[0]
	graph, err := loadGraph(dataDir())
	if err != nil {
		return err
	}
	task, ok := graph.Tasks[id]
	if !ok {
		return fmt.Errorf("unknown task id %s", id)
	}
	fmt.Printf("id: %s\n", task.ID)
	fmt.Printf("uuid: %s\n", task.UUID)
	if task.EpicID != "" {
		fmt.Printf("epic: %s\n", task.EpicID)
	}
	fmt.Printf("state: %s\n", task.State)
	if task.ClaimedBy != "" {
		fmt.Printf("claimed_by: %s\n", task.ClaimedBy)
	}
	fmt.Printf("created_at: %s\n", formatTime(task.CreatedAt))
	fmt.Printf("updated_at: %s\n", formatTime(task.UpdatedAt))
	if len(task.Deps) > 0 {
		fmt.Printf("deps: %s\n", strings.Join(task.Deps, ","))
	}
	if len(task.RDeps) > 0 {
		fmt.Printf("rdeps: %s\n", strings.Join(task.RDeps, ","))
	}
	fmt.Println()
	fmt.Print(task.Body)
	if task.Body != "" && !strings.HasSuffix(task.Body, "\n") {
		fmt.Println()
	}
	return nil
}

func runDeps(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ergo deps <id>")
	}
	return listDepIDs(args[0], false)
}

func runRDeps(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ergo rdeps <id>")
	}
	return listDepIDs(args[0], true)
}

func runPlan(args []string) error {
	epicID, err := parseFlagValue(args, "--epic")
	if err != nil {
		return err
	}
	graph, err := loadGraph(dataDir())
	if err != nil {
		return err
	}
	tasks := listTasks(graph, epicID, false, false, true)
	counts := map[string]int{}
	readyCount := 0
	blockedCount := 0
	blockedStateCount := 0
	for _, task := range tasks {
		counts[task.State]++
		if task.State == stateBlocked {
			blockedStateCount++
		}
		if isReady(task, graph) {
			readyCount++
		}
		if isBlocked(task, graph) {
			blockedCount++
		}
	}
	fmt.Printf("total:%d todo:%d doing:%d done:%d blocked_state:%d canceled:%d ready:%d blocked_any:%d\n",
		len(tasks),
		counts[stateTodo],
		counts[stateDoing],
		counts[stateDone],
		blockedStateCount,
		counts[stateCanceled],
		readyCount,
		blockedCount,
	)
	for _, task := range tasks {
		status := ""
		if isReady(task, graph) {
			status = "READY"
		} else if isBlocked(task, graph) {
			status = "BLOCKED"
		}
		if status == "" {
			status = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", task.ID, task.State, status, firstLine(task.Body))
	}
	return nil
}

func runCompact(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ergo compact")
	}
	dir := dataDir()
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, func() error {
		events, err := readEvents(eventsPath)
		if err != nil {
			return err
		}
		graph, err := replayEvents(events)
		if err != nil {
			return err
		}
		compacted, err := compactEvents(graph)
		if err != nil {
			return err
		}
		tmpPath := eventsPath + ".tmp"
		if err := writeEventsFile(tmpPath, compacted); err != nil {
			return err
		}
		return os.Rename(tmpPath, eventsPath)
	})
}

func writeLinkEvent(eventType, from, to string) error {
	dir := dataDir()
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if _, ok := graph.Tasks[from]; !ok {
			return fmt.Errorf("unknown task id %s", from)
		}
		if _, ok := graph.Tasks[to]; !ok {
			return fmt.Errorf("unknown task id %s", to)
		}
		now := time.Now().UTC()
		event, err := newEvent(eventType, now, LinkEvent{
			FromID: from,
			ToID:   to,
			Type:   dependsLinkType,
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func createTask(epicID string, isEpic bool) error {
	dir := dataDir()
	eventsPath := filepath.Join(dir, "events.jsonl")
	lockPath := filepath.Join(dir, "lock")
	body, err := readBody("")
	if err != nil {
		return err
	}
	return withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if !isEpic && epicID != "" {
			epic, ok := graph.Tasks[epicID]
			if !ok {
				return fmt.Errorf("unknown epic id %s", epicID)
			}
			if epic.EpicID != "" {
				return fmt.Errorf("task %s is not an epic", epicID)
			}
		}
		id, err := newShortID(graph.Tasks)
		if err != nil {
			return err
		}
		uuid, err := newUUID()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		payload := NewTaskEvent{
			ID:        id,
			UUID:      uuid,
			EpicID:    epicID,
			State:     stateTodo,
			Body:      body,
			CreatedAt: formatTime(now),
		}
		if isEpic {
			payload.EpicID = ""
		}
		eventType := "new_task"
		if isEpic {
			eventType = "new_epic"
		}
		event, err := newEvent(eventType, now, payload)
		if err != nil {
			return err
		}
		if err := appendEvents(eventsPath, []Event{event}); err != nil {
			return err
		}
		fmt.Println(id)
		return nil
	})
}

func listDepIDs(id string, reverse bool) error {
	graph, err := loadGraph(dataDir())
	if err != nil {
		return err
	}
	if _, ok := graph.Tasks[id]; !ok {
		return fmt.Errorf("unknown task id %s", id)
	}
	var ids []string
	if reverse {
		ids = sortedKeys(graph.RDeps[id])
	} else {
		ids = sortedKeys(graph.Deps[id])
	}
	for _, depID := range ids {
		fmt.Println(depID)
	}
	return nil
}

const (
	dataDirName = ".ergo"
)

func dataDir() string {
	return dataDirName
}

func loadGraph(dir string) (*Graph, error) {
	eventsPath := filepath.Join(dir, "events.jsonl")
	events, err := readEvents(eventsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("missing %s/events.jsonl (run ergo init)", dir)
		}
		return nil, err
	}
	return replayEvents(events)
}

func readEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var events []Event
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			var event Event
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				if readErr == io.EOF {
					break
				}
				return nil, fmt.Errorf("invalid event: %w", err)
			}
			events = append(events, event)
		}
		if readErr == io.EOF {
			break
		}
	}
	return events, nil
}

func appendEvents(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, event := range events {
		blob, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(blob, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeEventsFile(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, event := range events {
		blob, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(blob, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

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
			task := &Task{
				ID:        data.ID,
				UUID:      data.UUID,
				EpicID:    data.EpicID,
				State:     data.State,
				Body:      data.Body,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}
			graph.Tasks[data.ID] = task
			graph.Meta[data.ID] = &TaskMeta{
				CreatedBody:  data.Body,
				CreatedState: data.State,
				CreatedAt:    createdAt,
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
		case "edit":
			var data EditEvent
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
				meta.LastEditAt = ts
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
		var lastEditAt time.Time
		var lastStateAt time.Time
		var lastClaimAt time.Time
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
			lastEditAt = meta.LastEditAt
			lastStateAt = meta.LastStateAt
			lastClaimAt = meta.LastClaimAt
		}

		payload := NewTaskEvent{
			ID:        task.ID,
			UUID:      task.UUID,
			EpicID:    task.EpicID,
			State:     createdState,
			Body:      createdBody,
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

		if task.Body != createdBody {
			ts := pickTime(lastEditAt, task.UpdatedAt)
			editEvent, err := newEvent("edit", ts, EditEvent{
				ID:   task.ID,
				Body: task.Body,
				TS:   formatTime(ts),
			})
			if err != nil {
				return nil, err
			}
			events = append(events, editEvent)
		}

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

func readyTasks(graph *Graph, epicID string) []*Task {
	tasks := listTasks(graph, epicID, true, false, true)
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

func listTasks(graph *Graph, epicID string, readyOnly, blockedOnly, includeAll bool) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if epicID != "" && task.EpicID != epicID {
			continue
		}
		if readyOnly && !isReady(task, graph) {
			continue
		}
		if blockedOnly && !isBlocked(task, graph) {
			continue
		}
		if !readyOnly && !blockedOnly && !includeAll {
			if task.State == stateDone || task.State == stateCanceled {
				continue
			}
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

func sortedTasks(tasks map[string]*Task) []*Task {
	list := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		list = append(list, task)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID < list[j].ID
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list
}

func isReady(task *Task, graph *Graph) bool {
	if task.State != stateTodo {
		return false
	}
	if task.ClaimedBy != "" {
		return false
	}
	deps := graph.Deps[task.ID]
	for depID := range deps {
		dep, ok := graph.Tasks[depID]
		if !ok {
			return false
		}
		if dep.State != stateDone && dep.State != stateCanceled {
			return false
		}
	}
	return true
}

func isBlocked(task *Task, graph *Graph) bool {
	if task.State == stateBlocked {
		return true
	}
	if task.State == stateTodo && !isReady(task, graph) {
		return true
	}
	return false
}

func readBody(initial string) (string, error) {
	if stdinIsPiped() {
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		if len(body) == 0 && initial != "" {
			return initial, nil
		}
		return string(body), nil
	}

	tmp, err := os.CreateTemp("", "ergo-edit-*.md")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(initial); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}
	cmd := exec.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	updated, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", err
	}
	return string(updated), nil
}

func stdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

func newShortID(existing map[string]*Task) (string, error) {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	const size = 6
	for i := 0; i < 10; i++ {
		buf := make([]byte, size)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		var b strings.Builder
		for _, c := range buf {
			b.WriteByte(alphabet[int(c)%len(alphabet)])
		}
		id := b.String()
		if _, exists := existing[id]; !exists {
			return id, nil
		}
	}
	return "", errors.New("failed to generate unique id")
}

func newUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}

func newEvent(eventType string, ts time.Time, payload interface{}) (Event, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	return Event{
		TS:   formatTime(ts),
		Type: eventType,
		Data: data,
	}, nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("missing timestamp")
	}
	return time.Parse(time.RFC3339Nano, value)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func sortedKeys(items map[string]struct{}) []string {
	if items == nil {
		return nil
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(items map[string]map[string]struct{}) []string {
	if items == nil {
		return nil
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func withLock(path string, lockType int, fn func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), lockType); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return fn()
}

func parseFlagValue(args []string, name string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			if i+1 >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[i+1], nil
		}
	}
	return "", nil
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func countTrue(values ...bool) int {
	total := 0
	for _, value := range values {
		if value {
			total++
		}
	}
	return total
}

func firstLine(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "-"
	}
	if idx := strings.Index(body, "\n"); idx >= 0 {
		return body[:idx]
	}
	return body
}

func defaultAgentID() string {
	user := os.Getenv("USER")
	host, _ := os.Hostname()
	if user == "" {
		user = "unknown"
	}
	if host == "" {
		return user
	}
	return user + "@" + host
}

func pickTime(candidate, fallback time.Time) time.Time {
	if !candidate.IsZero() {
		return candidate
	}
	if !fallback.IsZero() {
		return fallback
	}
	return time.Now().UTC()
}

func maxTime(current, next time.Time) time.Time {
	if current.IsZero() {
		return next
	}
	if next.IsZero() {
		return current
	}
	if next.After(current) {
		return next
	}
	return current
}
