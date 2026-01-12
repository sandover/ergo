// Work commands: dep/list/next/set/show/compact/where.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func runSet(args []string, opts GlobalOptions) error {
	if err := requireWritable(opts, "set"); err != nil {
		return err
	}
	if len(args) < 2 {
		return errors.New("usage: ergo set <id> key=value [key=value ...]")
	}

	id := args[0]
	pairs := args[1:]

	// Parse all key=value pairs
	updates, err := parseKeyValuePairs(pairs)
	if err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	return applySetUpdates(dir, opts, id, updates)
}

func applySetUpdates(dir string, opts GlobalOptions, id string, updates map[string]string) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")

	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		task, ok := graph.Tasks[id]
		if !ok {
			return fmt.Errorf("unknown task id %s", id)
		}

		now := time.Now().UTC()

		// Build events using pure function, passing I/O-dependent body resolver
		events, remainingUpdates, err := buildSetEvents(id, task, updates, now, resolveSetBody)
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

		return appendEvents(eventsPath, events)
	})
}

// buildSetEvents generates the event list for a set command.
// Separated from I/O to improve testability and separation of concerns.
func buildSetEvents(id string, task *Task, updates map[string]string, now time.Time, bodyResolver func(string) (string, error)) ([]Event, map[string]string, error) {
	var events []Event
	remainingUpdates := make(map[string]string)
	for k, v := range updates {
		remainingUpdates[k] = v
	}

	// Handle title update (can't be empty)
	if title, ok := remainingUpdates["title"]; ok {
		title = strings.TrimSpace(title)
		if title == "" {
			return nil, nil, errors.New("title cannot be empty")
		}
		// Title update via body update (reusing existing event)
		bodyToSet := title
		if body, hasBody := remainingUpdates["body"]; hasBody {
			resolvedBody, err := bodyResolver(body)
			if err != nil {
				return nil, nil, err
			}
			bodyToSet = resolvedBody
		}
		event, err := newEvent("body", now, BodyUpdateEvent{
			ID:   id,
			Body: bodyToSet,
			TS:   formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		delete(remainingUpdates, "title")
		delete(remainingUpdates, "body") // already handled
	} else if body, ok := remainingUpdates["body"]; ok {
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
		worker, err := parseWorker(workerStr)
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

func resolveSetBody(value string) (string, error) {
	switch value {
	case "@-":
		// Read from stdin
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		return string(body), nil
	case "@editor":
		// TODO: implement editor support
		return "", errors.New("@editor not yet implemented")
	default:
		return value, nil
	}
}

func runDep(args []string, opts GlobalOptions) error {
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

func runList(args []string, opts GlobalOptions) error {
	format, err := parseOutputFormat(args, outputFormatText)
	if err != nil {
		return err
	}

	epicID, err := parseFlagValue(args, "--epic")
	if err != nil {
		return err
	}

	readyOnly := hasFlag(args, "--ready")
	blockedOnly := hasFlag(args, "--blocked")
	showEpics := hasFlag(args, "--epics")

	if readyOnly && blockedOnly {
		return errors.New("cannot use both --ready and --blocked")
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	graph, err := loadGraph(dir)
	if err != nil {
		return err
	}

	// Get tasks (default: all tasks)
	tasks := listTasks(graph, epicID, readyOnly, blockedOnly, false)

	// Filter out epics from tasks list (tasks should only be tasks, not epics)
	var filteredTasks []*Task
	for _, task := range tasks {
		if !isEpic(task) {
			filteredTasks = append(filteredTasks, task)
		}
	}
	tasks = filteredTasks

	// Filter by worker only if --ready or --blocked is set
	if opts.As != workerAny && (readyOnly || blockedOnly) {
		tasks = filterTasksByWorker(tasks, opts.As)
	}

	// Handle epics if --epics flag is set
	var epics []*Task
	if showEpics {
		for _, task := range graph.Tasks {
			if isEpic(task) && (epicID == "" || task.EpicID == epicID) {
				epics = append(epics, task)
			}
		}
	}

	if format == outputFormatJSON {
		result := map[string]interface{}{
			"tasks": buildTaskListItems(tasks, graph),
		}
		if showEpics {
			result["epics"] = buildTaskListItems(epics, graph)
		}
		return writeJSON(os.Stdout, result)
	}

	// Text output
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

	if showEpics && len(epics) > 0 {
		if len(tasks) > 0 {
			fmt.Println() // separator
		}
		for _, epic := range epics {
			claimed := epic.ClaimedBy
			if claimed == "" {
				claimed = "-"
			}
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", epic.ID, epic.State, "-", claimed, firstLine(epic.Body))
		}
	}

	return nil
}

func runNext(args []string, opts GlobalOptions) error {
	peek := hasFlag(args, "--peek")

	if !peek {
		if err := requireWritable(opts, "next"); err != nil {
			return err
		}
	}

	format, err := parseOutputFormat(args, outputFormatText)
	if err != nil {
		return err
	}

	epicID, err := parseFlagValue(args, "--epic")
	if err != nil {
		return err
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	if peek {
		// Just read and show, don't claim
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		ready := readyTasks(graph, epicID, opts.As, kindTask)
		if len(ready) == 0 {
			// Exit code 3 if no ready task
			os.Exit(3)
		}

		chosen := ready[0]
		if format == outputFormatJSON {
			return writeJSON(os.Stdout, map[string]interface{}{
				"id":     chosen.ID,
				"epic":   chosen.EpicID,
				"worker": string(chosen.Worker),
				"state":  chosen.State,
				"title":  extractTitle(chosen.Body),
				"body":   chosen.Body,
			})
		}

		// Print title+body
		fmt.Println(chosen.Body)
		return nil
	}

	// Atomic claim
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	var body string
	var chosen *Task
	agentID := resolveAgentID(opts)
	var now time.Time

	err = withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		ready := readyTasks(graph, epicID, opts.As, kindTask)
		if len(ready) == 0 {
			return errors.New("no ready tasks")
		}

		task := ready[0]
		chosen = task
		body = task.Body
		now = time.Now().UTC()

		claimEvent, err := newEvent("claim", now, ClaimEvent{
			ID:      task.ID,
			AgentID: agentID,
			TS:      formatTime(now),
		})
		if err != nil {
			return err
		}

		stateEvent, err := newEvent("state", now, StateEvent{
			ID:       task.ID,
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
			// Exit code 3 if no ready task
			os.Exit(3)
		}
		return err
	}

	if format == outputFormatJSON {
		if chosen == nil {
			return errors.New("internal error: missing chosen task")
		}
		return writeJSON(os.Stdout, map[string]interface{}{
			"id":         chosen.ID,
			"epic":       chosen.EpicID,
			"worker":     string(chosen.Worker),
			"state":      stateDoing,
			"title":      extractTitle(body),
			"body":       body,
			"agent_id":   agentID,
			"claimed_at": formatTime(now),
		})
	}

	// Print title+body
	fmt.Println(body)
	return nil
}

func extractTitle(body string) string {
	lines := strings.Split(body, "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return body
}

func runShow(args []string, opts GlobalOptions) error {
	id, format, short, err := parseShowArgs(args)
	if err != nil {
		return err
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	graph, err := loadGraph(dir)
	if err != nil {
		return err
	}
	task, ok := graph.Tasks[id]
	if !ok {
		return fmt.Errorf("unknown task id %s", id)
	}
	claimedAt := claimedAtForTask(task, graph.Meta[id])
	if format == outputFormatJSON {
		return writeJSON(os.Stdout, taskShowOutput{
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
			Body:      task.Body,
		})
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
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", task.ID, task.State, epic, claimed, firstLine(task.Body))
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
	fmt.Println()
	fmt.Print(task.Body)
	if task.Body != "" && !strings.HasSuffix(task.Body, "\n") {
		fmt.Println()
	}
	return nil
}

func runCompact(args []string, opts GlobalOptions) error {
	if err := requireWritable(opts, "compact"); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("usage: ergo compact")
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
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

func runWhere(args []string, opts GlobalOptions) error {
	format, remaining, err := parseOutputFormatAndArgs(args, outputFormatText)
	if err != nil {
		return err
	}
	for _, arg := range remaining {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown flag %s", arg)
		}
		return errors.New("usage: ergo where [--json]")
	}
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
	if format == outputFormatJSON {
		return writeJSON(os.Stdout, whereOutput{
			ErgoDir: ergoDir,
			RepoDir: repoDir,
		})
	}
	fmt.Println(ergoDir)
	return nil
}
