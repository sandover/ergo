// Purpose: Implement list/show/claim/set/sequence/compact/prune/where behaviors and output.
// Exports: RunSet, RunClaim, RunClaimOldestReady, RunShow, RunList, RunSequence, RunCompact, RunPrune, RunWhere.
// Role: Command layer bridging CLI wiring to graph/storage operations.
// Invariants: Mutations acquire the lock; JSON output is stable when requested.
// Notes: Read operations replay the event log to build current state.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ListOptions struct {
	EpicID    string
	ReadyOnly bool
	ShowAll   bool
}

func RunSet(id string, args []string, opts GlobalOptions) error {
	if id == "" {
		return errors.New("usage: ergo set <id> [json]")
	}

	input, verr, err := parseInlineTaskArgs(args, "usage: ergo set <id> [json]")
	if err != nil {
		return err
	}
	if verr != nil {
		if opts.JSON {
			if err := verr.WriteJSON(os.Stdout); err != nil {
				return err
			}
		}
		return verr.GoError()
	}
	if verr := input.ValidateForSet(); verr != nil {
		if opts.JSON {
			if err := verr.WriteJSON(os.Stdout); err != nil {
				return err
			}
		}
		return verr.GoError()
	}

	body, bodyProvided, err := readOptionalBodyFromStdin()
	if err != nil {
		return err
	}

	updates := input.ToUpdates()
	if bodyProvided {
		updates["body"] = body
	}
	if len(updates) == 0 {
		return errors.New("no fields to update")
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	outcome, err := applySetUpdates(dir, opts, id, updates, opts.AgentID, opts.JSON)
	if err != nil {
		return err
	}

	if opts.JSON {
		task := outcome.Graph.Tasks[id]
		if task == nil {
			return fmt.Errorf("unknown task id %s", id)
		}
		return writeJSON(os.Stdout, setOutput{
			Kind:          "set",
			ID:            id,
			UpdatedFields: buildUpdatedFields(input, bodyProvided),
			State:         task.State,
			ClaimedBy:     task.ClaimedBy,
		})
	}
	return nil
}

func RunClaim(id string, opts GlobalOptions) error {
	if id == "" {
		return errors.New("usage: ergo claim <id>")
	}

	reminder := "When you have completed this claimed task, you MUST mark it done."

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
	outcome, err := applySetUpdates(dir, opts, id, updates, agentID, true)
	if err != nil {
		return err
	}
	graph := outcome.Graph
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
			"reminder":   reminder,
		})
	}

	fmt.Println(task.ID)
	fmt.Println(task.Title)
	if task.Body != "" {
		fmt.Println(task.Body)
	}
	fmt.Println(reminder)
	return nil
}

func RunClaimOldestReady(opts GlobalOptions) error {
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)

	reminder := "When you have completed this claimed task, you MUST mark it done."

	var chosen *Task
	var now time.Time
	agentID := opts.AgentID
	if agentID == "" {
		return errors.New("claim requires --agent")
	}

	err = withLock(lockPath, opts, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		ready := readyTasks(graph)
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
				return writeJSON(os.Stdout, map[string]string{
					"status":  "no_ready",
					"message": "No ready ergo tasks.",
				})
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
			"reminder":   reminder,
		})
	}

	fmt.Println(chosen.ID)
	fmt.Println(chosen.Title)
	if chosen.Body != "" {
		fmt.Println(chosen.Body)
	}
	fmt.Println(reminder)
	return nil
}
func applySetUpdates(dir string, opts GlobalOptions, id string, updates map[string]string, agentID string, quiet bool) (mutationOutcome, error) {
	mutation, remaining, err := mutationFromUpdates(updates, agentID, identityBodyResolver)
	if err != nil {
		return mutationOutcome{}, err
	}
	if len(remaining) > 0 {
		unknown := make([]string, 0, len(remaining))
		for key := range remaining {
			unknown = append(unknown, key)
		}
		return mutationOutcome{}, fmt.Errorf("unknown keys: %s", strings.Join(unknown, ", "))
	}
	mutation.Kind = "set"
	mutation.LegacySet = true
	mutationOpts := opts
	mutationOpts.AgentID = agentID
	return applyTaskMutation(dir, mutationOpts, id, mutation, quiet)
}

// buildSetEvents generates the event list for a set command.
// Separated from I/O to improve testability and separation of concerns.
func buildSetEvents(id string, task *Task, updates map[string]string, agentID string, now time.Time, bodyResolver func(string) (string, error)) ([]Event, map[string]string, error) {
	mutation, remaining, err := mutationFromUpdates(updates, agentID, bodyResolver)
	if err != nil {
		return nil, nil, err
	}
	if task.IsEpic && mutation.EpicSet {
		return nil, nil, errors.New("containers cannot be assigned to other containers")
	}
	mutation.LegacySet = true
	events, _, err := buildMutationEvents(id, task, mutation, agentID, now)
	return events, remaining, err
}

func mutationFromUpdates(updates map[string]string, agentID string, bodyResolver func(string) (string, error)) (taskMutation, map[string]string, error) {
	mutation := taskMutation{}
	remaining := make(map[string]string, len(updates))
	for key, value := range updates {
		remaining[key] = value
	}
	if value, ok := remaining["title"]; ok {
		mutation.Title, mutation.TitleSet = value, true
		delete(remaining, "title")
	}
	if value, ok := remaining["body"]; ok {
		resolved, err := bodyResolver(value)
		if err != nil {
			return taskMutation{}, nil, err
		}
		mutation.Body, mutation.BodySet = resolved, true
		delete(remaining, "body")
	}
	if value, ok := remaining["epic"]; ok {
		mutation.EpicID, mutation.EpicSet = value, true
		delete(remaining, "epic")
	}
	if value, ok := remaining["claim"]; ok {
		mutation.Claim, mutation.ClaimSet = value, true
		delete(remaining, "claim")
	}
	if value, ok := remaining["state"]; ok {
		mutation.State, mutation.StateSet = value, true
		delete(remaining, "state")
	}
	if value, ok := remaining["result.path"]; ok {
		mutation.ResultPath, mutation.ResultSet = value, true
		delete(remaining, "result.path")
	}
	if value, ok := remaining["result.summary"]; ok {
		if !mutation.ResultSet {
			return taskMutation{}, nil, errors.New("result.summary requires result.path=")
		}
		mutation.ResultSummary = value
		delete(remaining, "result.summary")
	}
	_ = agentID
	return mutation, remaining, nil
}

// identityBodyResolver is a no-op body resolver (body comes from JSON as-is).
// With JSON-only input, we no longer support @- or @editor syntax.
func identityBodyResolver(value string) (string, error) {
	return value, nil
}

type sequenceEdge struct {
	FromID string
	ToID   string
}

func buildSequenceEdges(order []string) []sequenceEdge {
	if len(order) < 2 {
		return nil
	}
	edges := make([]sequenceEdge, 0, len(order)-1)
	for i := 0; i < len(order)-1; i++ {
		edges = append(edges, sequenceEdge{
			FromID: order[i+1],
			ToID:   order[i],
		})
	}
	return edges
}

func RunSequence(args []string, opts GlobalOptions) error {
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}

	usage := "usage: ergo sequence <A> <B> [<C>...] | ergo sequence rm <A> <B>"
	if len(args) < 2 {
		return errors.New(usage)
	}

	action := "link"
	var edges []sequenceEdge
	if args[0] == "rm" {
		if len(args) != 3 {
			return errors.New(usage)
		}
		action = "unlink"
		edges = buildSequenceEdges([]string{args[1], args[2]})
	} else {
		if len(args) < 2 {
			return errors.New(usage)
		}
		edges = buildSequenceEdges(args)
	}

	if len(edges) == 0 {
		return errors.New(usage)
	}

	if err := writeLinkEvents(dir, opts, action, edges); err != nil {
		return err
	}

	if opts.JSON {
		outEdges := make([]sequenceEdgeOutput, 0, len(edges))
		for _, edge := range edges {
			outEdges = append(outEdges, sequenceEdgeOutput{
				FromID: edge.FromID,
				ToID:   edge.ToID,
				Type:   dependsLinkType,
			})
		}
		return writeJSON(os.Stdout, sequenceOutput{
			Kind:   "sequence",
			Action: action,
			Edges:  outEdges,
		})
	}
	return nil
}

func writeLinkEvents(dir string, opts GlobalOptions, eventType string, edges []sequenceEdge) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)
	return withLock(lockPath, opts, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}

		events := make([]Event, 0, len(edges))
		now := time.Now().UTC()
		for _, edge := range edges {
			from := edge.FromID
			to := edge.ToID
			if _, ok := graph.Tombstones[from]; ok {
				return prunedErr(from)
			}
			if _, ok := graph.Tombstones[to]; ok {
				return prunedErr(to)
			}
			fromItem, ok := graph.Tasks[from]
			if !ok {
				return fmt.Errorf("unknown id %s", from)
			}
			toItem, ok := graph.Tasks[to]
			if !ok {
				return fmt.Errorf("unknown id %s", to)
			}
			if err := validateDepSelf(from, to); err != nil {
				return err
			}
			if err := validateDepAncestry(fromItem, toItem); err != nil {
				return err
			}
			if eventType == "link" && hasCycle(graph, from, to) {
				return errors.New("dependency would create a cycle")
			}
			event, err := newEvent(eventType, now, LinkEvent{
				FromID: from,
				ToID:   to,
				Type:   dependsLinkType,
			})
			if err != nil {
				return err
			}
			events = append(events, event)
			if eventType == "link" {
				if graph.Deps[from] == nil {
					graph.Deps[from] = map[string]struct{}{}
				}
				graph.Deps[from][to] = struct{}{}
			} else if graph.Deps[from] != nil {
				delete(graph.Deps[from], to)
			}
		}
		return appendEvents(eventsPath, events)
	})
}

func RunList(listOpts ListOptions, opts GlobalOptions) error {
	epicID := listOpts.EpicID
	readyOnly := listOpts.ReadyOnly
	showAll := listOpts.ShowAll

	if readyOnly && showAll {
		return errors.New("conflicting flags: --ready and --all")
	}

	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(dir)

	graph, err := loadGraphLocked(dir, opts)
	if err != nil {
		return err
	}

	// Get tasks - includeAll=true so we get everything, then filter
	// We need all tasks for tree view hierarchy and JSON output
	tasks := listTasks(graph, epicID, readyOnly)

	// Filter out containers from tasks list (default listing shows leaf tasks)
	var tasksOnly []*Task
	for _, task := range tasks {
		if !isContainer(task, graph) {
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

	if opts.JSON {
		// Default: return array of tasks
		return writeJSON(os.Stdout, buildTaskListItems(tasksOnly, graph, repoDir))
	}

	fmt.Fprintln(os.Stderr, "Coding agents should call 'ergo --json list' instead for structured output.")

	// Tree view (human-friendly hierarchical output)
	if epicID != "" {
		epic := graph.Tasks[epicID]
		if epic == nil || !isContainer(epic, graph) {
			return fmt.Errorf("no such container: %s", epicID)
		}
	}

	useColor := stdoutIsTTY()
	roots := buildListRoots(graph, showAll, readyOnly, epicID)

	printSummary := func(stats taskStats, buckets []summaryBucket, addSpacing bool) {
		renderSummary(os.Stdout, stats, useColor, buckets, addSpacing)
	}

	allTasks := collectNonContainerTasks(graph)
	activeTasks := filterActiveTasks(allTasks)
	readyTasks := filterReadyTasks(allTasks, graph)

	if epicID != "" {
		epicChildren := collectEpicChildren(epicID, graph)
		epicChildrenReady := filterReadyTasks(epicChildren, graph)

		renderTreeView(os.Stdout, roots, graph, repoDir, useColor)

		switch {
		case readyOnly:
			if len(epicChildren) == 0 {
				fmt.Fprintln(os.Stdout, "No tasks in this epic.")
				return nil
			}
			if len(epicChildrenReady) == 0 {
				fmt.Fprintln(os.Stdout, "No ready tasks in this epic.")
				stats := computeStatsForTasks(epicChildren, graph)
				printSummary(stats, []summaryBucket{summaryInProgress, summaryBlocked, summaryError}, false)
				return nil
			}
			stats := computeStatsForTasks(epicChildrenReady, graph)
			printSummary(stats, []summaryBucket{summaryReady}, true)
			return nil
		default:
			if len(epicChildren) == 0 {
				fmt.Fprintln(os.Stdout, "No tasks in this epic.")
				return nil
			}
			// Epic-focused view includes done/canceled by default.
			stats := computeStatsForTasks(epicChildren, graph)
			printSummary(stats, []summaryBucket{summaryReady, summaryInProgress, summaryBlocked, summaryError, summaryDone, summaryCanceled}, true)
			return nil
		}
	}

	switch {
	case readyOnly:
		if len(allTasks) == 0 {
			fmt.Fprintln(os.Stdout, "No tasks.")
			return nil
		}
		if len(readyTasks) == 0 {
			fmt.Fprintln(os.Stdout, "No ready tasks.")
			stats := computeStatsForTasks(activeTasks, graph)
			printSummary(stats, []summaryBucket{summaryInProgress, summaryBlocked, summaryError}, false)
			return nil
		}
		renderTreeView(os.Stdout, roots, graph, repoDir, useColor)
		stats := computeStatsForTasks(readyTasks, graph)
		printSummary(stats, []summaryBucket{summaryReady}, true)
		return nil
	case showAll:
		if len(allTasks) == 0 && len(roots) == 0 {
			fmt.Fprintln(os.Stdout, "No tasks.")
			return nil
		}
		renderTreeView(os.Stdout, roots, graph, repoDir, useColor)
		stats := computeStatsForTasks(allTasks, graph)
		printSummary(stats, []summaryBucket{summaryReady, summaryInProgress, summaryBlocked, summaryError, summaryDone, summaryCanceled}, true)
		return nil
	default:
		if len(allTasks) == 0 {
			if len(roots) == 0 {
				fmt.Fprintln(os.Stdout, "No tasks.")
				return nil
			}
			renderTreeView(os.Stdout, roots, graph, repoDir, useColor)
			return nil
		}
		if len(activeTasks) == 0 {
			fmt.Fprintln(os.Stdout, "No active tasks.")
			stats := computeStatsForTasks(allTasks, graph)
			printSummary(stats, []summaryBucket{summaryDone, summaryCanceled}, false)
			return nil
		}
		renderTreeView(os.Stdout, roots, graph, repoDir, useColor)
		stats := computeStatsForTasks(activeTasks, graph)
		printSummary(stats, []summaryBucket{summaryReady, summaryInProgress, summaryBlocked, summaryError}, true)
		return nil
	}
}

// collectEpicChildren returns all tasks belonging to the given epic in dependency order.
func collectEpicChildren(epicID string, graph *Graph) []*Task {
	var children []*Task
	for _, t := range graph.Tasks {
		if t.EpicID == epicID {
			children = append(children, t)
		}
	}
	return topoSortTasks(children, graph)
}

func collectNonContainerTasks(graph *Graph) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if !isContainer(task, graph) {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func filterActiveTasks(tasks []*Task) []*Task {
	var active []*Task
	for _, task := range tasks {
		if task.State != stateDone && task.State != stateCanceled {
			active = append(active, task)
		}
	}
	return active
}

func filterReadyTasks(tasks []*Task, graph *Graph) []*Task {
	var ready []*Task
	for _, task := range tasks {
		if isReady(task, graph) {
			ready = append(ready, task)
		}
	}
	return ready
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

type frontMatterField struct {
	key   string
	value string
}

// printTaskDetails prints task show output as a Markdown document.
func printTaskDetails(task *Task, repoDir string) {
	writeShowFrontMatter([]frontMatterField{
		{key: "id", value: task.ID},
		{key: "title", value: task.Title},
		{key: "state", value: task.State},
		{key: "epic_id", value: task.EpicID},
		{key: "created_at", value: formatTime(task.CreatedAt)},
		{key: "updated_at", value: formatTime(task.UpdatedAt)},
	})

	fmt.Printf("# %s\n\n", showTitle(task.Title, task.ID))
	if task.Body != "" {
		printMarkdownBody(task.Body)
		fmt.Println()
	}

	printTaskResultsMarkdown(task.Results, repoDir, "## Results")
}

// printEpicDetails renders epic show output as a Markdown document.
func printEpicDetails(epic *Task, children []*Task, repoDir string) {
	writeShowFrontMatter([]frontMatterField{
		{key: "container", value: "true"},
		{key: "id", value: epic.ID},
		{key: "title", value: epic.Title},
		{key: "created_at", value: formatTime(epic.CreatedAt)},
		{key: "updated_at", value: formatTime(epic.UpdatedAt)},
	})

	fmt.Printf("# %s\n\n", showTitle(epic.Title, epic.ID))
	if epic.Body != "" {
		printMarkdownBody(epic.Body)
		fmt.Println()
	}

	fmt.Println("## Tasks")
	fmt.Println()
	for index, child := range children {
		fmt.Printf("### %s - %s\n\n", child.ID, showTitle(child.Title, child.ID))
		fmt.Printf("- state: %s\n", child.State)
		if len(child.Results) > 0 {
			fmt.Println("- results:")
			for _, result := range child.Results {
				fileURL := deriveFileURL(result.Path, repoDir)
				fmt.Printf("  - [%s](%s): %s\n", result.Path, fileURL, result.Summary)
			}
		}
		fmt.Println()

		if child.Body != "" {
			printMarkdownBody(child.Body)
		}
		if index < len(children)-1 {
			fmt.Println()
		}
	}
}

func writeShowFrontMatter(fields []frontMatterField) {
	fmt.Println("---")
	for _, field := range fields {
		fmt.Printf("%s: %s\n", field.key, yamlString(field.value))
	}
	fmt.Println("---")
	fmt.Println()
}

func yamlString(value string) string {
	return strconv.Quote(value)
}

func showTitle(title string, fallback string) string {
	if strings.TrimSpace(title) == "" {
		return fallback
	}
	return title
}

func printMarkdownBody(body string) {
	fmt.Print(body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Println()
	}
}

func printTaskResultsMarkdown(results []Result, repoDir string, heading string) {
	if len(results) == 0 {
		return
	}
	fmt.Println(heading)
	for _, result := range results {
		fileURL := deriveFileURL(result.Path, repoDir)
		fmt.Printf("- [%s](%s): %s\n", result.Path, fileURL, result.Summary)
	}
	fmt.Println()
}

func RunShow(id string, opts GlobalOptions) error {
	if id == "" {
		return errors.New("usage: ergo show <id> [--json]")
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	repoDir := filepath.Dir(dir)
	graph, err := loadGraphLocked(dir, opts)
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

	// Collect child tasks if this is a container
	var childTasks []*Task
	if isContainer(task, graph) {
		childTasks = collectEpicChildren(id, graph)
	}

	if opts.JSON {
		output := buildTaskShowOutput(task, graph.Meta[id], repoDir)

		// If it's a container with children, wrap with children
		if isContainer(task, graph) && len(childTasks) > 0 {
			childOutputs := make([]taskShowOutput, len(childTasks))
			for i, child := range childTasks {
				childOutputs[i] = buildTaskShowOutput(child, graph.Meta[child.ID], repoDir)
			}
			return writeJSON(os.Stdout, map[string]interface{}{
				"container": output,
				"children":  childOutputs,
			})
		}
		return writeJSON(os.Stdout, output)
	}
	fmt.Fprintln(os.Stderr, "Coding agents should call 'ergo --json show <id>' instead for structured output.")

	if isContainer(task, graph) {
		printEpicDetails(task, childTasks, repoDir)
		return nil
	}
	printTaskDetails(task, repoDir)
	return nil
}

func RunCompact(opts GlobalOptions) error {
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)
	if err := withLock(lockPath, opts, func() error {
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
		return replaceEventsAtomically(eventsPath, compacted)
	}); err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(os.Stdout, compactOutput{
			Kind:   "compact",
			Status: "ok",
		})
	}
	return nil
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
	total := stats.done + stats.canceled + stats.containers

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
	total := stats.done + stats.canceled + stats.containers

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
	done       int
	canceled   int
	containers int
}

func computePruneStats(items []PruneItem) pruneStats {
	var stats pruneStats
	for _, item := range items {
		if item.IsContainer {
			stats.containers++
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
	if stats.containers > 0 {
		fmt.Print("  ")
		fmt.Print(iconEpic)
		fmt.Printf("  %d empty containers\n", stats.containers)
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
	if item.IsContainer {
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
	if item.IsContainer {
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
	if opts.JSON {
		return writeJSON(os.Stdout, whereOutput{
			ErgoDir: ergoDir,
			RepoDir: repoDir,
		})
	}
	fmt.Println(ergoDir)
	return nil
}
