// Work commands: dep/list/claim/set/show/compact/where.
//
// The set command uses stdin-only JSON input:
//
//	echo '{"state":"done"}' | ergo set T-xyz
//
// See json_input.go for the unified TaskInput schema.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/glamour"
)

type ListOptions struct {
	EpicID      string
	ReadyOnly   bool
	BlockedOnly bool
	ShowEpics   bool
	ShowAll     bool
}

func RunSet(id string, opts GlobalOptions) error {
	if err := requireWritable(opts, "set"); err != nil {
		return err
	}

	if id == "" {
		return errors.New("usage: echo '{\"state\":\"done\"}' | ergo set <id>")
	}

	// Parse JSON from stdin
	input, verr := ParseTaskInput()
	if verr != nil {
		if opts.JSON {
			if err := verr.WriteJSON(os.Stdout); err != nil {
				return err
			}
		}
		return verr.GoError()
	}

	// Validate for set (all fields optional)
	if verr := input.ValidateForSet(); verr != nil {
		if opts.JSON {
			if err := verr.WriteJSON(os.Stdout); err != nil {
				return err
			}
		}
		return verr.GoError()
	}

	updates := input.ToKeyValueMap()
	if len(updates) == 0 {
		return errors.New("no fields to update")
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	agentID := resolveAgentID(opts)
	return applySetUpdates(dir, opts, id, updates, agentID, false)
}

func RunClaim(id string, state string, opts GlobalOptions) error {
	if err := requireWritable(opts, "claim"); err != nil {
		return err
	}
	if id == "" {
		return errors.New("usage: ergo claim <id> [--state <doing|error|blocked>]")
	}

	agentID := resolveAgentID(opts)
	updates := map[string]string{
		"claim": agentID,
	}
	if state == "" {
		state = stateDoing
	}
	switch state {
	case stateDoing, stateError, stateBlocked:
		updates["state"] = state
	default:
		return fmt.Errorf("invalid state %q, expected: doing, error, blocked", state)
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	if err := applySetUpdates(dir, opts, id, updates, agentID, true); err != nil {
		return err
	}

	graph, err := loadGraph(dir)
	if err != nil {
		return err
	}
	task := graph.Tasks[id]
	if task == nil {
		return errors.New("internal error: missing claimed task")
	}

	if opts.JSON {
		claimedAt := claimedAtForTask(task, graph.Meta[id])
		return writeJSON(os.Stdout, map[string]interface{}{
			"id":         task.ID,
			"epic":       task.EpicID,
			"worker":     string(task.Worker),
			"state":      task.State,
			"title":      task.Title,
			"body":       task.Body,
			"agent_id":   agentID,
			"claimed_at": claimedAt,
		})
	}

	fmt.Println(task.ID)
	fmt.Println(task.Title)
	if task.Body != "" {
		fmt.Println(task.Body)
	}
	return nil
}

func RunClaimOldestReady(epicID string, opts GlobalOptions) error {
	if err := requireWritable(opts, "claim"); err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")

	var chosen *Task
	var now time.Time
	agentID := resolveAgentID(opts)

	err = withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		ready := readyTasks(graph, epicID, opts.As, kindTask)
		if len(ready) == 0 {
			return errors.New("no ready tasks")
		}

		chosen = ready[0]
		now = time.Now().UTC()

		claimEvent, err := newEvent("claim", now, ClaimEvent{
			ID:      chosen.ID,
			AgentID: agentID,
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
		if err.Error() == "no ready tasks" {
			os.Exit(3)
		}
		return err
	}

	if chosen == nil {
		return errors.New("internal error: missing chosen task")
	}

	if opts.JSON {
		return writeJSON(os.Stdout, map[string]interface{}{
			"id":         chosen.ID,
			"epic":       chosen.EpicID,
			"worker":     string(chosen.Worker),
			"state":      stateDoing,
			"title":      chosen.Title,
			"body":       chosen.Body,
			"agent_id":   agentID,
			"claimed_at": formatTime(now),
		})
	}

	fmt.Println(chosen.ID)
	fmt.Println(chosen.Title)
	if chosen.Body != "" {
		fmt.Println(chosen.Body)
	}
	return nil
}

func applySetUpdates(dir string, opts GlobalOptions, id string, updates map[string]string, agentID string, quiet bool) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")

	// Handle result.path + result.summary (requires file I/O before lock)
	resultPath, hasPath := updates["result.path"]
	resultSummary, hasSummary := updates["result.summary"]
	if hasPath || hasSummary {
		if !hasPath {
			return errors.New("result.summary requires result.path=")
		}
		if !hasSummary {
			return errors.New("result.path requires result.summary=")
		}
		if err := writeResultEvent(dir, opts, id, resultSummary, resultPath); err != nil {
			return err
		}
		delete(updates, "result.path")
		delete(updates, "result.summary")
		// If no other updates, we're done
		if len(updates) == 0 {
			if !quiet {
				fmt.Println(id)
			}
			return nil
		}
	}

	return withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		task, ok := graph.Tasks[id]
		if !ok {
			return fmt.Errorf("unknown task id %s", id)
		}

		// Epics cannot have state, worker, or claim
		if isEpic(task) {
			if _, hasState := updates["state"]; hasState {
				return errors.New("epics do not have state")
			}
			if _, hasWorker := updates["worker"]; hasWorker {
				return errors.New("epics do not have workers")
			}
			if _, hasClaim := updates["claim"]; hasClaim {
				return errors.New("epics cannot be claimed")
			}
		}

		now := time.Now().UTC()

		// Build events using pure function, passing I/O-dependent body resolver
		events, remainingUpdates, err := buildSetEvents(id, task, updates, agentID, now, identityBodyResolver)
		if err != nil {
			return err
		}

		// Check for any unhandled keys
		if len(remainingUpdates) > 0 {
			var unknown []string
			for key := range remainingUpdates {
				unknown = append(unknown, key)
			}
			return fmt.Errorf("unknown keys: %s", strings.Join(unknown, ", "))
		}

		if err := appendEvents(eventsPath, events); err != nil {
			return err
		}
		if !quiet {
			fmt.Println(id)
		}
		return nil
	})
}

// buildSetEvents generates the event list for a set command.
// Separated from I/O to improve testability and separation of concerns.
func buildSetEvents(id string, task *Task, updates map[string]string, agentID string, now time.Time, bodyResolver func(string) (string, error)) ([]Event, map[string]string, error) {
	var events []Event
	remainingUpdates := make(map[string]string)
	for k, v := range updates {
		remainingUpdates[k] = v
	}

	// Handle implicit claim: if transitioning to doing/error and unclaimed, and no claim provided, use session identity
	if !isEpic(task) && task.ClaimedBy == "" {
		newState, hasState := remainingUpdates["state"]
		_, hasClaim := remainingUpdates["claim"]
		if hasState && (newState == stateDoing || newState == stateError) && !hasClaim {
			remainingUpdates["claim"] = agentID
		}
	}

	// Handle title/body updates (explicit fields).
	if title, hasTitle := remainingUpdates["title"]; hasTitle {
		title = strings.TrimSpace(title)
		if title == "" {
			return nil, nil, errors.New("title cannot be empty")
		}
		event, err := newEvent("title", now, TitleUpdateEvent{
			ID:    id,
			Title: title,
			TS:    formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		delete(remainingUpdates, "title")
	}

	if body, hasBody := remainingUpdates["body"]; hasBody {
		resolvedBody, err := bodyResolver(body)
		if err != nil {
			return nil, nil, err
		}
		event, err := newEvent("body", now, BodyUpdateEvent{
			ID:   id,
			Body: resolvedBody,
			TS:   formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		delete(remainingUpdates, "body")
	}

	// Handle epic assignment
	if epicID, ok := remainingUpdates["epic"]; ok {
		if !isEpic(task) {
			event, err := newEvent("epic", now, EpicAssignEvent{
				ID:     id,
				EpicID: epicID,
				TS:     formatTime(now),
			})
			if err != nil {
				return nil, nil, err
			}
			events = append(events, event)
		}
		delete(remainingUpdates, "epic")
	}

	// Handle worker
	if workerStr, ok := remainingUpdates["worker"]; ok {
		worker, err := ParseWorker(workerStr)
		if err != nil {
			return nil, nil, err
		}
		event, err := newEvent("worker", now, WorkerEvent{
			ID:     id,
			Worker: string(worker),
			TS:     formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		delete(remainingUpdates, "worker")
	}

	// Handle claim
	claimWasSet := false
	claimValue := ""
	if cv, ok := remainingUpdates["claim"]; ok {
		claimValue = cv
		if !isEpic(task) {
			if claimValue == "" {
				// Clear claim
				event, err := newEvent("unclaim", now, UnclaimEvent{
					ID: id,
					TS: formatTime(now),
				})
				if err != nil {
					return nil, nil, err
				}
				events = append(events, event)
			} else {
				event, err := newEvent("claim", now, ClaimEvent{
					ID:      id,
					AgentID: claimValue,
					TS:      formatTime(now),
				})
				if err != nil {
					return nil, nil, err
				}
				events = append(events, event)
			}
			claimWasSet = true
		}
		delete(remainingUpdates, "claim")
	}

	// Handle state (must come last)
	stateWasSet := false
	if stateStr, ok := remainingUpdates["state"]; ok {
		if _, valid := validStates[stateStr]; !valid {
			return nil, nil, fmt.Errorf("invalid state: %s", stateStr)
		}
		// Validate state transition
		if err := validateTransition(task.State, stateStr); err != nil {
			return nil, nil, err
		}
		// Validate claim invariant for new state
		newClaimedBy := task.ClaimedBy
		if claimWasSet {
			newClaimedBy = claimValue
		}
		// done/canceled/todo will clear claim, so check with empty
		if stateStr == stateTodo || stateStr == stateDone || stateStr == stateCanceled {
			newClaimedBy = ""
		}
		if err := validateClaimInvariant(stateStr, newClaimedBy); err != nil {
			return nil, nil, err
		}
		event, err := newEvent("state", now, StateEvent{
			ID:       id,
			NewState: stateStr,
			TS:       formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		delete(remainingUpdates, "state")
		stateWasSet = true
	}

	// If claim was set to a non-empty value and state wasn't explicitly set, default to doing
	if claimWasSet && claimValue != "" && !stateWasSet {
		event, err := newEvent("state", now, StateEvent{
			ID:       id,
			NewState: stateDoing,
			TS:       formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
	}

	return events, remainingUpdates, nil
}

// identityBodyResolver is a no-op body resolver (body comes from JSON as-is).
// With JSON-only input, we no longer support @- or @editor syntax.
func identityBodyResolver(value string) (string, error) {
	return value, nil
}

func RunDep(args []string, opts GlobalOptions) error {
	if err := requireWritable(opts, "dep"); err != nil {
		return err
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	usage := "usage: ergo dep <A> <B> | ergo dep rm <A> <B>"
	if len(args) < 2 {
		return errors.New(usage)
	}

	if len(args) == 2 {
		// dep <A> <B>: A depends on B (B blocks A)
		return writeLinkEvent(dir, opts, "link", args[0], args[1])
	}

	if len(args) == 3 && args[0] == "rm" {
		// dep rm <A> <B>: remove dependency
		return writeLinkEvent(dir, opts, "unlink", args[1], args[2])
	}

	return errors.New(usage)
}

func RunList(listOpts ListOptions, opts GlobalOptions) error {
	epicID := listOpts.EpicID
	readyOnly := listOpts.ReadyOnly
	blockedOnly := listOpts.BlockedOnly
	showEpics := listOpts.ShowEpics
	showAll := listOpts.ShowAll

	if readyOnly && blockedOnly {
		return errors.New("cannot use both --ready and --blocked")
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(dir)

	graph, err := loadGraph(dir)
	if err != nil {
		return err
	}

	// Get tasks - includeAll=true so we get everything, then filter
	// We need all tasks for tree view hierarchy and JSON output
	tasks := listTasks(graph, epicID, readyOnly, blockedOnly, true)

	// Filter out epics from tasks list (tasks should only be tasks, not epics)
	var tasksOnly []*Task
	for _, task := range tasks {
		if !isEpic(task) {
			tasksOnly = append(tasksOnly, task)
		}
	}

	// Filter by worker only if --ready or --blocked is set
	if opts.As != workerAny && (readyOnly || blockedOnly) {
		tasksOnly = filterTasksByWorker(tasksOnly, opts.As)
	}

	// Apply Active Set filtering (default behavior)
	// If --all is NOT set, and we aren't targeting specific states via --ready/--blocked,
	// hide done and canceled tasks.
	if !showAll && !readyOnly && !blockedOnly {
		var active []*Task
		for _, task := range tasksOnly {
			if task.State != stateDone && task.State != stateCanceled {
				active = append(active, task)
			}
		}
		tasksOnly = active
	}

	// Handle epics if --epics flag is set
	var epics []*Task
	if showEpics {
		for _, task := range graph.Tasks {
			if isEpic(task) && (epicID == "" || task.EpicID == epicID) {
				epics = append(epics, task)
			}
		}
		// Sort epics by creation time
		sortByCreatedAt(epics)
	}

	if opts.JSON {
		// JSON output includes all tasks (agents filter themselves)
		// Return bare array for simplicity and consistency with show --json
		if showEpics {
			// When filtering to epics only, return just the epics array
			return writeJSON(os.Stdout, buildTaskListItems(epics, graph, repoDir))
		}
		// Default: return array of tasks
		return writeJSON(os.Stdout, buildTaskListItems(tasksOnly, graph, repoDir))
	}

	if !opts.Quiet {
		fmt.Fprintln(os.Stderr, "agents: use 'ergo --json list' for structured output")
	}

	// If --epics only, show simple epic list instead of tree
	if showEpics && epicID == "" && !readyOnly && !blockedOnly {
		for _, epic := range epics {
			fmt.Printf("%s  %s\n", epic.ID, epic.Title)
		}
		if len(epics) == 0 {
			fmt.Println("no epics")
		}
		return nil
	}

	// Tree view (human-friendly hierarchical output)
	useColor := stdoutIsTTY()
	renderTreeView(os.Stdout, graph, repoDir, useColor, showAll)

	return nil
}

func RunShow(id string, short bool, opts GlobalOptions) error {
	if id == "" {
		return errors.New("usage: ergo show <id> [--short] [--json]")
	}
	if short && opts.JSON {
		return errors.New("conflicting flags: --short and --json")
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(dir)
	graph, err := loadGraph(dir)
	if err != nil {
		return err
	}
	task, ok := graph.Tasks[id]
	if !ok {
		return fmt.Errorf("unknown task id %s", id)
	}
	claimedAt := claimedAtForTask(task, graph.Meta[id])

	// For epics, collect child tasks
	var childTasks []*Task
	if isEpic(task) {
		for _, t := range graph.Tasks {
			if !isEpic(t) && t.EpicID == id {
				childTasks = append(childTasks, t)
			}
		}
		// Sort by ID for consistent output
		sort.Slice(childTasks, func(i, j int) bool {
			return childTasks[i].ID < childTasks[j].ID
		})
	}

	if opts.JSON {
		epicOutput := taskShowOutput{
			ID:        task.ID,
			UUID:      task.UUID,
			EpicID:    task.EpicID,
			State:     task.State,
			Worker:    string(task.Worker),
			ClaimedBy: task.ClaimedBy,
			ClaimedAt: claimedAt,
			CreatedAt: formatTime(task.CreatedAt),
			UpdatedAt: formatTime(task.UpdatedAt),
			Deps:      task.Deps,
			RDeps:     task.RDeps,
			Title:     task.Title,
			Body:      task.Body,
			Results:   buildResultOutputItems(task.Results, repoDir),
		}

		// If it's an epic with children, wrap with children
		if isEpic(task) && len(childTasks) > 0 {
			childOutputs := make([]taskShowOutput, len(childTasks))
			for i, child := range childTasks {
				childClaimedAt := claimedAtForTask(child, graph.Meta[child.ID])
				childOutputs[i] = taskShowOutput{
					ID:        child.ID,
					UUID:      child.UUID,
					EpicID:    child.EpicID,
					State:     child.State,
					Worker:    string(child.Worker),
					ClaimedBy: child.ClaimedBy,
					ClaimedAt: childClaimedAt,
					CreatedAt: formatTime(child.CreatedAt),
					UpdatedAt: formatTime(child.UpdatedAt),
					Deps:      child.Deps,
					RDeps:     child.RDeps,
					Title:     child.Title,
					Body:      child.Body,
					Results:   buildResultOutputItems(child.Results, repoDir),
				}
			}
			// Return object with epic and children
			return writeJSON(os.Stdout, map[string]interface{}{
				"epic":     epicOutput,
				"children": childOutputs,
			})
		}
		return writeJSON(os.Stdout, epicOutput)
	}
	if short {
		epic := task.EpicID
		if epic == "" {
			epic = "-"
		}
		claimed := task.ClaimedBy
		if claimed == "" {
			claimed = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", task.ID, task.State, epic, claimed, task.Title)
		return nil
	}
	fmt.Printf("id: %s\n", task.ID)
	fmt.Printf("uuid: %s\n", task.UUID)
	if task.EpicID != "" {
		fmt.Printf("epic: %s\n", task.EpicID)
	}
	fmt.Printf("state: %s\n", task.State)
	fmt.Printf("worker: %s\n", task.Worker)
	if task.ClaimedBy != "" {
		fmt.Printf("claimed_by: %s\n", task.ClaimedBy)
		if claimedAt != "" {
			fmt.Printf("claimed_at: %s\n", claimedAt)
		}
	}
	fmt.Printf("created_at: %s\n", formatTime(task.CreatedAt))
	fmt.Printf("updated_at: %s\n", formatTime(task.UpdatedAt))
	if len(task.Deps) > 0 {
		fmt.Printf("deps: %s\n", strings.Join(task.Deps, ","))
	}
	if len(task.RDeps) > 0 {
		fmt.Printf("rdeps: %s\n", strings.Join(task.RDeps, ","))
	}
	if len(task.Results) > 0 {
		fmt.Printf("results: %d\n", len(task.Results))
		for i, r := range task.Results {
			fileURL := deriveFileURL(r.Path, repoDir)
			fmt.Printf("  [%d] %s\n", i+1, r.Summary)
			fmt.Printf("      path: %s\n", r.Path)
			fmt.Printf("      file_url: %s\n", fileURL)
			fmt.Printf("      sha256: %s\n", r.Sha256AtAttach)
			fmt.Printf("      attached: %s\n", formatTime(r.CreatedAt))
		}
	}
	fmt.Println()

	// --- GLAMOUR RENDERING START ---
	isTTY := stdoutIsTTY()
	if task.Title != "" {
		fmt.Println(task.Title)
		if task.Body != "" {
			fmt.Println()
		}
	}
	if isTTY && task.Body != "" {
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		out, err := r.Render(task.Body)
		if err == nil {
			fmt.Print(out)
		} else {
			// Fallback if rendering fails
			fmt.Print(task.Body)
			if !strings.HasSuffix(task.Body, "\n") {
				fmt.Println()
			}
		}
	} else {
		// Non-TTY or empty body: print raw
		fmt.Print(task.Body)
		if task.Body != "" && !strings.HasSuffix(task.Body, "\n") {
			fmt.Println()
		}
	}
	// --- GLAMOUR RENDERING END ---

	// Show child tasks if this is an epic
	if isEpic(task) && len(childTasks) > 0 {
		for _, child := range childTasks {
			fmt.Println("---")
			childClaimedAt := claimedAtForTask(child, graph.Meta[child.ID])
			fmt.Printf("id: %s\n", child.ID)
			fmt.Printf("uuid: %s\n", child.UUID)
			if child.EpicID != "" {
				fmt.Printf("epic: %s\n", child.EpicID)
			}
			fmt.Printf("state: %s\n", child.State)
			fmt.Printf("worker: %s\n", child.Worker)
			if child.ClaimedBy != "" {
				fmt.Printf("claimed_by: %s\n", child.ClaimedBy)
				if childClaimedAt != "" {
					fmt.Printf("claimed_at: %s\n", childClaimedAt)
				}
			}
			fmt.Printf("created_at: %s\n", formatTime(child.CreatedAt))
			fmt.Printf("updated_at: %s\n", formatTime(child.UpdatedAt))
			if len(child.Deps) > 0 {
				fmt.Printf("deps: %s\n", strings.Join(child.Deps, ","))
			}
			if len(child.RDeps) > 0 {
				fmt.Printf("rdeps: %s\n", strings.Join(child.RDeps, ","))
			}
			if len(child.Results) > 0 {
				fmt.Printf("results: %d\n", len(child.Results))
				for i, r := range child.Results {
					fileURL := deriveFileURL(r.Path, repoDir)
					fmt.Printf("  [%d] %s\n", i+1, r.Summary)
					fmt.Printf("      path: %s\n", r.Path)
					fmt.Printf("      file_url: %s\n", fileURL)
					fmt.Printf("      sha256: %s\n", r.Sha256AtAttach)
					fmt.Printf("      attached: %s\n", formatTime(r.CreatedAt))
				}
			}
			fmt.Println()

			// Render child task content
			isTTY := stdoutIsTTY()
			if child.Title != "" {
				fmt.Println(child.Title)
				if child.Body != "" {
					fmt.Println()
				}
			}
			if isTTY && child.Body != "" {
				r, _ := glamour.NewTermRenderer(
					glamour.WithAutoStyle(),
					glamour.WithWordWrap(80),
				)
				out, err := r.Render(child.Body)
				if err == nil {
					fmt.Print(out)
				} else {
					fmt.Print(child.Body)
					if !strings.HasSuffix(child.Body, "\n") {
						fmt.Println()
					}
				}
			} else {
				fmt.Print(child.Body)
				if child.Body != "" && !strings.HasSuffix(child.Body, "\n") {
					fmt.Println()
				}
			}
		}
	}

	return nil
}

func RunCompact(opts GlobalOptions) error {
	if err := requireWritable(opts, "compact"); err != nil {
		return err
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
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

func RunWhere(opts GlobalOptions) error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}
	if opts.StartDir != "" {
		start = opts.StartDir
	}
	ergoDir, err := resolveErgoDir(start)
	if err != nil {
		return err
	}
	ergoDir, err = filepath.Abs(ergoDir)
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(ergoDir)
	debugf(opts, "where start=%s ergo_dir=%s repo_dir=%s", start, ergoDir, repoDir)
	if opts.JSON {
		return writeJSON(os.Stdout, whereOutput{
			ErgoDir: ergoDir,
			RepoDir: repoDir,
		})
	}
	fmt.Println(ergoDir)
	return nil
}
