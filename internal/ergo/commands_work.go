// Work command implementations for user-facing CLI actions.
// Purpose: implement list/show/claim/set/dep/compact/prune behavior and output.
// Exports: RunSet, RunDep, RunClaim, RunShow, RunList, RunCompact, RunPrune (and helpers).
// Role: command layer bridging CLI wiring to graph/storage operations.
// Invariants: write operations acquire the lock; JSON output is stable when requested.
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
	EpicID    string
	ReadyOnly bool
	ShowEpics bool
	ShowAll   bool
}

func RunSet(id string, opts GlobalOptions) error {
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

	agentID := opts.AgentID
	return applySetUpdates(dir, opts, id, updates, agentID, false)
}

func RunClaim(id string, opts GlobalOptions) error {
	if id == "" {
		return errors.New("usage: ergo claim <id>")
	}

	agentID := opts.AgentID
	if agentID == "" {
		return errors.New("claim requires --agent")
	}
	updates := map[string]string{
		"claim": agentID,
		"state": stateDoing,
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
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")

	var chosen *Task
	var now time.Time
	agentID := opts.AgentID
	if agentID == "" {
		return errors.New("claim requires --agent")
	}

	err = withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		ready := readyTasks(graph, epicID, kindTask)
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
			if opts.JSON {
				return writeJSON(os.Stdout, "No ready ergo tasks.")
			}
			fmt.Println("No ready ergo tasks.")
			return nil
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

		if _, ok := graph.Tombstones[id]; ok {
			return prunedErr(id)
		}
		task, ok := graph.Tasks[id]
		if !ok {
			return fmt.Errorf("unknown task id %s", id)
		}

		// Epics cannot have state or claim
		if isEpic(task) {
			if _, hasState := updates["state"]; hasState {
				return errors.New("epics do not have state")
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
			if agentID == "" {
				return nil, nil, errors.New("state requires claim; pass --agent or set claim explicitly")
			}
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
		if isEpic(task) {
			return nil, nil, errors.New("epics cannot be assigned to other epics")
		}
		event, err := newEvent("epic", now, EpicAssignEvent{
			ID:     id,
			EpicID: epicID,
			TS:     formatTime(now),
		})
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		delete(remainingUpdates, "epic")
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
	showEpics := listOpts.ShowEpics
	showAll := listOpts.ShowAll

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
	tasks := listTasks(graph, epicID, readyOnly)

	// Filter out epics from tasks list (tasks should only be tasks, not epics)
	var tasksOnly []*Task
	for _, task := range tasks {
		if !isEpic(task) {
			tasksOnly = append(tasksOnly, task)
		}
	}

	// Apply Active Set filtering (default behavior)
	// If --all is NOT set, and we aren't targeting specific states via --ready,
	// hide done and canceled tasks.
	if !showAll && !readyOnly {
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
	if showEpics && epicID == "" && !readyOnly {
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
	renderTreeView(os.Stdout, graph, repoDir, useColor, showAll, readyOnly)

	return nil
}

// collectEpicChildren returns all tasks belonging to the given epic, sorted by ID.
func collectEpicChildren(epicID string, graph *Graph) []*Task {
	var children []*Task
	for _, t := range graph.Tasks {
		if !isEpic(t) && t.EpicID == epicID {
			children = append(children, t)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})
	return children
}

// buildTaskShowOutput creates a taskShowOutput struct for JSON serialization.
func buildTaskShowOutput(task *Task, meta *TaskMeta, repoDir string) taskShowOutput {
	claimedAt := claimedAtForTask(task, meta)
	return taskShowOutput{
		ID:        task.ID,
		UUID:      task.UUID,
		EpicID:    task.EpicID,
		State:     task.State,
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
}

// printTaskDetails prints the human-readable show output for a single task.
func printTaskDetails(task *Task, meta *TaskMeta, repoDir string) {
	isTTY := stdoutIsTTY()
	useColor := isTTY

	// Compute derived state
	isReady := false // We don't have graph here, so can't compute properly
	icon := stateIcon(task, isReady)
	stateColor := stateColor(task)

	// Header: [icon] ID · Title
	if useColor {
		fmt.Printf("%s%s%s %s", stateColor, icon, colorReset, colorBold)
	} else {
		fmt.Printf("%s ", icon)
	}
	fmt.Printf("%s", task.ID)
	if task.Title != "" {
		fmt.Printf(" · %s", task.Title)
	}
	if useColor {
		fmt.Print(colorReset)
	}
	fmt.Println()

	// Separator
	if useColor {
		fmt.Printf("%s%s%s\n", colorDim, strings.Repeat("─", 50), colorReset)
	} else {
		fmt.Println(strings.Repeat("─", 50))
	}
	fmt.Println()

	// State
	if useColor {
		fmt.Printf("%sstate:%s %s%s%s\n", colorDim, colorReset, stateColor, task.State, colorReset)
	} else {
		fmt.Printf("state: %s\n", task.State)
	}

	// Epic
	if task.EpicID != "" {
		if useColor {
			fmt.Printf("%sepic:%s  %s\n", colorDim, colorReset, task.EpicID)
		} else {
			fmt.Printf("epic:  %s\n", task.EpicID)
		}
	}

	// Claimed by
	if task.ClaimedBy != "" {
		if useColor {
			fmt.Printf("%sclaim:%s %s\n", colorDim, colorReset, task.ClaimedBy)
		} else {
			fmt.Printf("claim: %s\n", task.ClaimedBy)
		}
	}

	fmt.Println()

	// Timestamps (dim)
	if useColor {
		fmt.Printf("%s", colorDim)
	}
	fmt.Printf("created: %s\n", relativeTime(task.CreatedAt))
	fmt.Printf("updated: %s\n", relativeTime(task.UpdatedAt))
	if useColor {
		fmt.Print(colorReset)
	}

	// Dependencies (if present, dim)
	if len(task.Deps) > 0 || len(task.RDeps) > 0 {
		fmt.Println()
		if useColor {
			fmt.Printf("%s", colorDim)
		}
		if len(task.Deps) > 0 {
			fmt.Printf("deps:  %s\n", strings.Join(task.Deps, ", "))
		}
		if len(task.RDeps) > 0 {
			fmt.Printf("rdeps: %s\n", strings.Join(task.RDeps, ", "))
		}
		if useColor {
			fmt.Print(colorReset)
		}
	}

	// Results
	if len(task.Results) > 0 {
		fmt.Println()
		fmt.Println("Results:")
		for _, r := range task.Results {
			fileURL := deriveFileURL(r.Path, repoDir)
			if useColor {
				fmt.Printf("  → %s%s%s", colorCyan, fileURL, colorReset)
			} else {
				fmt.Printf("  → %s", fileURL)
			}
			if useColor {
				fmt.Printf(" %s— %s%s\n", colorDim, r.Summary, colorReset)
			} else {
				fmt.Printf(" — %s\n", r.Summary)
			}
		}
	}

	// Separator before content
	fmt.Println()
	if useColor {
		fmt.Printf("%s%s%s\n", colorDim, strings.Repeat("─", 50), colorReset)
	} else {
		fmt.Println(strings.Repeat("─", 50))
	}
	fmt.Println()

	// Body with glamour rendering
	if isTTY && task.Body != "" {
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		out, err := r.Render(task.Body)
		if err == nil {
			fmt.Print(out)
		} else {
			fmt.Print(task.Body)
			if !strings.HasSuffix(task.Body, "\n") {
				fmt.Println()
			}
		}
	} else if task.Body != "" {
		fmt.Print(task.Body)
		if !strings.HasSuffix(task.Body, "\n") {
			fmt.Println()
		}
	}
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
	if _, ok := graph.Tombstones[id]; ok {
		return prunedErr(id)
	}
	task, ok := graph.Tasks[id]
	if !ok {
		return fmt.Errorf("unknown task id %s", id)
	}

	// Collect child tasks if this is an epic
	var childTasks []*Task
	if isEpic(task) {
		childTasks = collectEpicChildren(id, graph)
	}

	if opts.JSON {
		output := buildTaskShowOutput(task, graph.Meta[id], repoDir)

		// If it's an epic with children, wrap with children
		if isEpic(task) && len(childTasks) > 0 {
			childOutputs := make([]taskShowOutput, len(childTasks))
			for i, child := range childTasks {
				childOutputs[i] = buildTaskShowOutput(child, graph.Meta[child.ID], repoDir)
			}
			return writeJSON(os.Stdout, map[string]interface{}{
				"epic":     output,
				"children": childOutputs,
			})
		}
		return writeJSON(os.Stdout, output)
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

	// Print the main task/epic
	printTaskDetails(task, graph.Meta[id], repoDir)

	// Print child tasks if this is an epic
	for _, child := range childTasks {
		fmt.Println("---")
		printTaskDetails(child, graph.Meta[child.ID], repoDir)
	}

	return nil
}

func RunCompact(opts GlobalOptions) error {
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
		if err := os.Rename(tmpPath, eventsPath); err != nil {
			return err
		}
		return syncDir(dir)
	})
}

func RunPrune(confirm bool, opts GlobalOptions) error {
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	var plan PrunePlan
	if confirm {
		plan, err = RunPruneApply(dir, opts)
	} else {
		plan, err = RunPrunePlan(dir)
	}
	if err != nil {
		return err
	}

	if opts.JSON {
		return writeJSON(os.Stdout, pruneOutput{
			Kind:      "prune",
			DryRun:    !confirm,
			PrunedIDs: plan.PrunedIDs,
		})
	}

	printPruneSummary(confirm, plan.Items)
	return nil
}

func printPruneSummary(confirm bool, items []PruneItem) {
	useColor := stdoutIsTTY()
	termWidth := getTerminalWidth()

	if len(items) == 0 {
		printPruneEmpty(useColor)
		return
	}

	if !confirm {
		printPrunePreview(items, useColor, termWidth)
	} else {
		printPruneApplied(items, useColor)
	}
}

func printPruneEmpty(useColor bool) {
	msg := "Nothing to prune."
	if useColor {
		fmt.Print(colorDim)
	}
	fmt.Print(msg)
	if useColor {
		fmt.Print(colorReset)
	}
	fmt.Println(" All work is still active.")
}

func printPrunePreview(items []PruneItem, useColor bool, termWidth int) {
	stats := computePruneStats(items)
	total := stats.done + stats.canceled + stats.epics

	// Header - tells you exactly what this is
	if useColor {
		fmt.Print(colorBold)
	}
	fmt.Printf("Would remove %d items:\n", total)
	if useColor {
		fmt.Print(colorReset)
	}
	fmt.Println()

	// Summary stats - breaks down what those items are
	printPruneStats(stats, useColor)
	fmt.Println()

	// Item list
	printPruneItemList(items, useColor, termWidth)
	fmt.Println()

	// Safety note and call to action
	if useColor {
		fmt.Print(colorDim)
	}
	fmt.Println("This is a preview. Active work (todo, doing, blocked, error) is never pruned.")
	if useColor {
		fmt.Print(colorReset)
	}
	fmt.Println("To apply: ergo prune --yes")
}

func printPruneApplied(items []PruneItem, useColor bool) {
	stats := computePruneStats(items)
	total := stats.done + stats.canceled + stats.epics

	// Header
	if useColor {
		fmt.Print(colorBold)
	}
	fmt.Printf("Pruned %d items\n", total)
	if useColor {
		fmt.Print(colorReset)
	}
	fmt.Println()

	// Summary stats only (no item list after apply)
	printPruneStats(stats, useColor)
}

type pruneStats struct {
	done     int
	canceled int
	epics    int
}

func computePruneStats(items []PruneItem) pruneStats {
	var stats pruneStats
	for _, item := range items {
		if item.IsEpic {
			stats.epics++
		} else if item.State == stateDone {
			stats.done++
		} else if item.State == stateCanceled {
			stats.canceled++
		}
	}
	return stats
}

func printPruneStats(stats pruneStats, useColor bool) {
	if stats.done > 0 {
		fmt.Print("  ")
		if useColor {
			fmt.Print(colorGreen)
		}
		fmt.Print(iconDone)
		if useColor {
			fmt.Print(colorReset)
		}
		fmt.Printf(" %d done tasks\n", stats.done)
	}
	if stats.canceled > 0 {
		fmt.Print("  ")
		if useColor {
			fmt.Print(colorDim)
		}
		fmt.Print(iconCanceled)
		if useColor {
			fmt.Print(colorReset)
		}
		fmt.Printf(" %d canceled tasks\n", stats.canceled)
	}
	if stats.epics > 0 {
		fmt.Print("  ")
		fmt.Print(iconEpic)
		fmt.Printf("  %d empty epics\n", stats.epics)
	}
}

func printPruneItemList(items []PruneItem, useColor bool, termWidth int) {
	// Layout: icon + title padded to fill, then right-aligned ID
	idWidth := 6 // ergo IDs are 6 chars
	minGap := 2
	rightMargin := 2
	idStart := termWidth - rightMargin - idWidth

	for _, item := range items {
		var line strings.Builder

		// Icon with color
		icon := pruneItemIcon(item)
		if useColor {
			line.WriteString(pruneItemColor(item))
		}
		line.WriteString(icon)
		if useColor {
			line.WriteString(colorReset)
		}
		line.WriteString(" ")

		// Title
		title := item.Title
		if strings.TrimSpace(title) == "" {
			title = "(no title)"
		}

		// Calculate max title width
		iconWidth := visibleLen(icon) + 1 // icon + space
		maxTitleWidth := idStart - minGap - iconWidth
		if maxTitleWidth < 10 {
			maxTitleWidth = 10
		}

		// Truncate title if needed
		if visibleLen(title) > maxTitleWidth {
			title = truncateToWidth(title, maxTitleWidth)
		}

		// Apply dim color for canceled items
		if useColor && item.State == stateCanceled {
			line.WriteString(colorDim)
		}
		line.WriteString(title)
		if useColor && item.State == stateCanceled {
			line.WriteString(colorReset)
		}

		// Pad to ID column
		currentWidth := iconWidth + visibleLen(title)
		padding := idStart - currentWidth
		if padding < minGap {
			padding = minGap
		}
		line.WriteString(strings.Repeat(" ", padding))

		// ID (dimmed)
		if useColor {
			line.WriteString(colorDim)
		}
		line.WriteString(item.ID)
		if useColor {
			line.WriteString(colorReset)
		}

		fmt.Println(line.String())
	}
}

func pruneItemIcon(item PruneItem) string {
	if item.IsEpic {
		return iconEpic
	}
	switch item.State {
	case stateDone:
		return iconDone
	case stateCanceled:
		return iconCanceled
	default:
		return "?"
	}
}

func pruneItemColor(item PruneItem) string {
	if item.IsEpic {
		return ""
	}
	switch item.State {
	case stateDone:
		return colorGreen
	case stateCanceled:
		return colorDim
	default:
		return ""
	}
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
