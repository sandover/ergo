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
		return errors.New("usage: ergo set <id> (JSON stdin; flags; or --body-stdin)")
	}

	if opts.BodyStdin {
		if err := validateBodyStdinExclusions(opts.BodyFlag); err != nil {
			return err
		}
		body, err := readBodyFromStdinOrEmpty()
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			return errors.New("set --body-stdin requires non-empty body")
		}
		updates := buildFlagUpdates(opts)
		updates["body"] = body

		updatedFields := []string{"body"}
		if strings.TrimSpace(opts.TitleFlag) != "" {
			updatedFields = append(updatedFields, "title")
		}
		if opts.EpicFlag != "" {
			updatedFields = append(updatedFields, "epic")
		}
		if opts.StateFlag != "" {
			updatedFields = append(updatedFields, "state")
		}
		if opts.ClaimFlag != "" {
			updatedFields = append(updatedFields, "claim")
		}
		if opts.ResultPathFlag != "" {
			updatedFields = append(updatedFields, "result_path")
		}
		if opts.ResultSummaryFlag != "" {
			updatedFields = append(updatedFields, "result_summary")
		}

		dir, err := ergoDir(opts)
		if err != nil {
			return err
		}
		agentID := opts.AgentID
		if err := applySetUpdates(dir, opts, id, updates, agentID, opts.JSON); err != nil {
			return err
		}

		if opts.JSON {
			graph, err := loadGraph(dir)
			if err != nil {
				return err
			}
			task := graph.Tasks[id]
			if task == nil {
				return fmt.Errorf("unknown task id %s", id)
			}
			return writeJSON(os.Stdout, setOutput{
				Kind:          "set",
				ID:            id,
				UpdatedFields: updatedFields,
				State:         task.State,
				ClaimedBy:     task.ClaimedBy,
			})
		}
		return nil
	}

	hasFlagInput := strings.TrimSpace(opts.TitleFlag) != "" ||
		opts.BodyFlag != "" ||
		opts.EpicFlag != "" ||
		opts.StateFlag != "" ||
		opts.ClaimFlag != "" ||
		opts.ResultPathFlag != "" ||
		opts.ResultSummaryFlag != ""
	if !stdinIsPiped() && hasFlagInput {
		updates := buildFlagUpdates(opts)
		if opts.BodyFlag != "" {
			updates["body"] = opts.BodyFlag
		}

		if len(updates) == 0 {
			return errors.New("no fields to update")
		}

		var updatedFields []string
		if strings.TrimSpace(opts.TitleFlag) != "" {
			updatedFields = append(updatedFields, "title")
		}
		if opts.BodyFlag != "" {
			updatedFields = append(updatedFields, "body")
		}
		if opts.EpicFlag != "" {
			updatedFields = append(updatedFields, "epic")
		}
		if opts.StateFlag != "" {
			updatedFields = append(updatedFields, "state")
		}
		if opts.ClaimFlag != "" {
			updatedFields = append(updatedFields, "claim")
		}
		if opts.ResultPathFlag != "" {
			updatedFields = append(updatedFields, "result_path")
		}
		if opts.ResultSummaryFlag != "" {
			updatedFields = append(updatedFields, "result_summary")
		}

		dir, err := ergoDir(opts)
		if err != nil {
			return err
		}
		agentID := opts.AgentID
		if err := applySetUpdates(dir, opts, id, updates, agentID, opts.JSON); err != nil {
			return err
		}

		if opts.JSON {
			graph, err := loadGraph(dir)
			if err != nil {
				return err
			}
			task := graph.Tasks[id]
			if task == nil {
				return fmt.Errorf("unknown task id %s", id)
			}
			return writeJSON(os.Stdout, setOutput{
				Kind:          "set",
				ID:            id,
				UpdatedFields: updatedFields,
				State:         task.State,
				ClaimedBy:     task.ClaimedBy,
			})
		}
		return nil
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
	if err := applySetUpdates(dir, opts, id, updates, agentID, opts.JSON); err != nil {
		return err
	}

	if opts.JSON {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		task := graph.Tasks[id]
		if task == nil {
			return fmt.Errorf("unknown task id %s", id)
		}
		return writeJSON(os.Stdout, setOutput{
			Kind:          "set",
			ID:            id,
			UpdatedFields: buildUpdatedFields(input),
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

func RunClaimOldestReady(epicID string, opts GlobalOptions) error {
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
				return writeJSON(os.Stdout, map[string]string{
					"kind":    "claim",
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

func buildUpdatedFields(input *TaskInput) []string {
	if input == nil {
		return nil
	}
	var fields []string
	if input.Title != nil {
		fields = append(fields, "title")
	}
	if input.Body != nil {
		fields = append(fields, "body")
	}
	if input.Epic != nil {
		fields = append(fields, "epic")
	}
	if input.State != nil {
		fields = append(fields, "state")
	}
	if input.Claim != nil {
		fields = append(fields, "claim")
	}
	if input.ResultPath != nil {
		fields = append(fields, "result_path")
	}
	if input.ResultSummary != nil {
		fields = append(fields, "result_summary")
	}
	return fields
}

func applySetUpdates(dir string, opts GlobalOptions, id string, updates map[string]string, agentID string, quiet bool) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)

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

	for _, edge := range edges {
		if err := writeLinkEvent(dir, opts, action, edge.FromID, edge.ToID); err != nil {
			return err
		}
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

func RunList(listOpts ListOptions, opts GlobalOptions) error {
	epicID := listOpts.EpicID
	readyOnly := listOpts.ReadyOnly
	showEpics := listOpts.ShowEpics
	showAll := listOpts.ShowAll

	if readyOnly && showAll {
		return errors.New("conflicting flags: --ready and --all")
	}
	if showEpics {
		if readyOnly {
			return errors.New("conflicting flags: --epics and --ready")
		}
		if showAll {
			return errors.New("conflicting flags: --epics and --all")
		}
		if epicID != "" {
			return errors.New("conflicting flags: --epics and --epic")
		}
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
		fmt.Fprintln(os.Stderr, "Coding agents should call 'ergo --json list' instead for structured output.")
	}

	// If --epics only, show simple epic list instead of tree
	if showEpics && epicID == "" && !readyOnly {
		useColor := stdoutIsTTY()
		termWidth := getTerminalWidth()
		for _, epic := range epics {
			icon := stateIcon(epic, false)
			line := formatTreeLine("", "", false, icon, epic.ID, epic.Title, nil, "", epic, false, useColor, termWidth)
			fmt.Fprintln(os.Stdout, line)
		}
		if len(epics) == 0 {
			fmt.Println("No epics.")
		}
		return nil
	}

	// Tree view (human-friendly hierarchical output)
	if epicID != "" {
		epic := graph.Tasks[epicID]
		if epic == nil || !epic.IsEpic {
			return fmt.Errorf("no such epic: %s", epicID)
		}
	}

	useColor := stdoutIsTTY()
	roots := buildListRoots(graph, showAll, readyOnly, epicID)

	printSummary := func(stats taskStats, buckets []summaryBucket, addSpacing bool) {
		if opts.Quiet {
			return
		}
		renderSummary(os.Stdout, stats, useColor, buckets, addSpacing)
	}

	allTasks := collectNonEpicTasks(graph)
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
		if !isEpic(t) && t.EpicID == epicID {
			children = append(children, t)
		}
	}
	return topoSortTasks(children, graph)
}

func collectNonEpicTasks(graph *Graph) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if !isEpic(task) {
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

// printTaskDetails prints the human-readable show output for a single task.
func printTaskDetails(task *Task, meta *TaskMeta, repoDir string) {
	isTTY := stdoutIsTTY()
	useColor := isTTY

	// Compute derived state
	isReady := false // We don't have graph here, so can't compute properly
	icon := stateIcon(task, isReady)
	stateColor := stateColor(task)

	// Header: [icon] ID [state] · Title
	if useColor {
		fmt.Printf("%s%s%s %s", stateColor, icon, colorReset, colorBold)
	} else {
		fmt.Printf("%s ", icon)
	}
	fmt.Printf("%s", task.ID)
	if useColor {
		fmt.Printf(" %s[%s]%s", stateColor, task.State, colorReset)
		if task.Title != "" {
			fmt.Printf(" %s· %s%s", colorBold, task.Title, colorReset)
		}
	} else {
		fmt.Printf(" [%s]", task.State)
		if task.Title != "" {
			fmt.Printf(" · %s", task.Title)
		}
	}
	if useColor {
		fmt.Print(colorReset)
	}
	fmt.Println()

	// Separator
	printDimSeparator(50, useColor)

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
	printDimSeparator(50, useColor)

	// Body with glamour rendering
	if task.Body != "" {
		printMarkdownBody(task.Body, isTTY)
	}
}

// printEpicDetails renders a document-first epic view for human output.
func printEpicDetails(epic *Task, children []*Task, graph *Graph) {
	isTTY := stdoutIsTTY()
	useColor := isTTY
	termWidth := getTerminalWidth()
	if termWidth < 24 {
		termWidth = 24
	}

	if useColor {
		fmt.Printf("%s# %s%s\n", colorBold, epic.Title, colorReset)
	} else {
		fmt.Printf("# %s\n", epic.Title)
	}
	printDimSeparator(50, useColor)

	if epic.Body != "" {
		printMarkdownBody(epic.Body, isTTY)
		fmt.Println()
	}

	stats := computeStatsForTasks(children, graph)
	summary := renderSummaryLine(stats, useColor)
	if summary != "" {
		fmt.Printf("Tasks  ·  %s\n", summary)
	} else {
		fmt.Println("Tasks")
	}

	rowWidth := termWidth - 2
	if rowWidth < 20 {
		rowWidth = 20
	}
	for _, child := range children {
		row := formatEpicTaskRow(child, graph, useColor, rowWidth)
		fmt.Printf("  %s\n", row)
	}

	printDimSeparator(50, useColor)
	footer := fmt.Sprintf("%s  ·  created %s  ·  updated %s", epic.ID, relativeTime(epic.CreatedAt), relativeTime(epic.UpdatedAt))
	if useColor {
		fmt.Printf("%s%s%s\n", colorDim, footer, colorReset)
	} else {
		fmt.Println(footer)
	}
}

func printDimSeparator(width int, useColor bool) {
	if width < 1 {
		width = 1
	}
	if useColor {
		fmt.Printf("%s%s%s\n", colorDim, strings.Repeat("─", width), colorReset)
	} else {
		fmt.Println(strings.Repeat("─", width))
	}
}

func printMarkdownBody(body string, isTTY bool) {
	if body == "" {
		return
	}
	if isTTY {
		r, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		out, err := r.Render(body)
		if err == nil {
			fmt.Print(out)
			return
		}
	}
	fmt.Print(body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Println()
	}
}

func renderSummaryLine(stats taskStats, useColor bool) string {
	var b strings.Builder
	renderSummary(&b, stats, useColor, []summaryBucket{
		summaryReady,
		summaryInProgress,
		summaryBlocked,
		summaryError,
		summaryDone,
		summaryCanceled,
	}, false)
	return strings.TrimSpace(b.String())
}

func formatEpicTaskRow(task *Task, graph *Graph, useColor bool, termWidth int) string {
	ready := isReady(task, graph)
	icon := stateIcon(task, ready)
	var annotations []string
	if task.ClaimedBy != "" {
		annotations = append(annotations, "@"+task.ClaimedBy)
	}

	var blockerAnnotation string
	if !ready && task.State == stateTodo {
		blockers := getBlockers(task, graph)
		var names []string
		for _, blockerID := range blockers {
			if blocker := graph.Tasks[blockerID]; blocker != nil {
				names = append(names, abbreviate(blocker.Title, 20))
			} else {
				names = append(names, blockerID)
			}
		}
		if len(names) > 2 {
			blockerAnnotation = fmt.Sprintf("⧗ %d blockers", len(names))
		} else if len(names) > 0 {
			blockerAnnotation = "⧗ " + strings.Join(names, ", ")
		}
	}

	return formatTreeLine("", "", false, icon, task.ID, task.Title, annotations, blockerAnnotation, task, ready, useColor, termWidth)
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
	if !opts.Quiet {
		fmt.Fprintln(os.Stderr, "Coding agents should call 'ergo --json show <id>' instead for structured output.")
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

	if isEpic(task) {
		printEpicDetails(task, childTasks, graph)
		return nil
	}
	printTaskDetails(task, graph.Meta[id], repoDir)
	return nil
}

func RunCompact(opts GlobalOptions) error {
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "lock")
	eventsPath := getEventsPath(dir)
	if err := withLock(lockPath, syscall.LOCK_EX, func() error {
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
