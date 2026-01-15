// Tree view rendering for human-friendly list output.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

// getTerminalWidth returns the terminal width, or a default if unavailable.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80 // default fallback
	}
	return width
}

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorCyan    = "\033[36m"
	colorRed     = "\033[31m"
	colorMagenta = "\033[35m"
)

// State icons
const (
	iconDone     = "✓"
	iconReady    = "○"
	iconDoing    = "◐"
	iconBlocked  = "·"
	iconCanceled = "✗"
	iconError    = "⚠"
)

// treeNode represents a task or epic in the tree structure.
type treeNode struct {
	task           *Task
	children       []*treeNode
	isReady        bool
	parentBlockers map[string]bool // blockers inherited from parent epic
	collapsed      bool            // for done epics: show summary instead of children
	collapsedCount int             // number of tasks in collapsed epic
}

// renderTreeView outputs tasks in a hierarchical tree format.
func renderTreeView(w io.Writer, graph *Graph, repoDir string, useColor bool, showAll bool) {
	// Build tree structure: epics contain their tasks, and epics nest by dependency
	roots := buildTree(graph)

	// Compute derived state for epics and filter/collapse unless --all
	if !showAll {
		roots = filterAndCollapseNodes(roots)
	}

	termWidth := getTerminalWidth()

	for i, root := range roots {
		renderNode(w, root, "", i == len(roots)-1, graph, repoDir, useColor, nil, termWidth)
	}

	// Summary line
	stats := computeStats(graph)
	renderSummary(w, stats, useColor)
}

// derivedEpicState computes an epic's state from its child tasks.
// Returns: "done" (all done/canceled), "canceled" (all canceled), "active" (has open work), "empty" (no tasks)
func derivedEpicState(children []*treeNode) string {
	if len(children) == 0 {
		return "empty"
	}
	allDone := true
	allCanceled := true
	for _, child := range children {
		if child.task == nil {
			continue
		}
		if child.task.State != stateDone && child.task.State != stateCanceled {
			allDone = false
			allCanceled = false
		}
		if child.task.State != stateCanceled {
			allCanceled = false
		}
	}
	if allCanceled {
		return "canceled"
	}
	if allDone {
		return "done"
	}
	return "active"
}

// filterAndCollapseNodes filters canceled tasks, hides fully-canceled epics,
// and collapses fully-done epics to a summary line.
func filterAndCollapseNodes(nodes []*treeNode) []*treeNode {
	var filtered []*treeNode
	for _, node := range nodes {
		// For epics, check derived state BEFORE filtering children
		if node.task != nil && node.task.IsEpic {
			state := derivedEpicState(node.children)
			switch state {
			case "canceled":
				// Hide fully-canceled epics
				continue
			case "done":
				// Collapse: mark as collapsed, count tasks, then clear children
				node.collapsed = true
				node.collapsedCount = countTasks(node.children)
				node.children = nil
				filtered = append(filtered, node)
				continue
			case "empty":
				// Empty epics are shown as-is
				filtered = append(filtered, node)
				continue
			}
			// "active" epics: filter children and continue
		}

		// Recursively process children
		node.children = filterAndCollapseNodes(node.children)

		// Skip canceled tasks
		if node.task != nil && !node.task.IsEpic && node.task.State == stateCanceled {
			continue
		}

		filtered = append(filtered, node)
	}
	return filtered
}

// countTasks counts non-epic tasks in a node tree.
func countTasks(nodes []*treeNode) int {
	count := 0
	for _, node := range nodes {
		if node.task != nil && !node.task.IsEpic {
			count++
		}
		count += countTasks(node.children)
	}
	return count
}

// taskStats holds aggregate counts for the summary line.
type taskStats struct {
	ready      int
	inProgress int
	blocked    int
	done       int
	canceled   int
	total      int
}

func computeStats(graph *Graph) taskStats {
	var stats taskStats
	for _, task := range graph.Tasks {
		if task.IsEpic {
			continue
		}
		stats.total++
		switch task.State {
		case stateDone:
			stats.done++
		case stateCanceled:
			stats.canceled++
		case stateDoing:
			stats.inProgress++
		case stateTodo:
			if isReady(task, graph) {
				stats.ready++
			} else {
				stats.blocked++
			}
		default:
			stats.blocked++
		}
	}
	return stats
}

func renderSummary(w io.Writer, stats taskStats, useColor bool) {
	if stats.total == 0 {
		return
	}
	fmt.Fprintln(w)

	var parts []string

	if stats.ready > 0 {
		part := fmt.Sprintf("%d ready", stats.ready)
		if useColor {
			part = colorYellow + part + colorReset
		}
		parts = append(parts, part)
	}

	if stats.inProgress > 0 {
		part := fmt.Sprintf("%d in progress", stats.inProgress)
		if useColor {
			part = colorCyan + part + colorReset
		}
		parts = append(parts, part)
	}

	if stats.blocked > 0 {
		part := fmt.Sprintf("%d blocked", stats.blocked)
		if useColor {
			part = colorDim + part + colorReset
		}
		parts = append(parts, part)
	}

	if stats.done > 0 {
		part := fmt.Sprintf("%d done", stats.done)
		if useColor {
			part = colorGreen + part + colorReset
		}
		parts = append(parts, part)
	}

	fmt.Fprintln(w, strings.Join(parts, " · "))
}

// buildTree constructs a forest of tree nodes from the graph.
// All epics appear as siblings at root level (flat structure).
// Tasks are nested under their owning epic.
// Orphan tasks (no epic) appear at root level.
// Dependencies between epics are shown via ⧗ annotations, not nesting.
func buildTree(graph *Graph) []*treeNode {
	// Separate epics and tasks
	var epics, orphanTasks []*Task
	epicTasks := make(map[string][]*Task) // epicID -> tasks

	for _, task := range graph.Tasks {
		if task.IsEpic {
			epics = append(epics, task)
		} else if task.EpicID == "" {
			orphanTasks = append(orphanTasks, task)
		} else {
			epicTasks[task.EpicID] = append(epicTasks[task.EpicID], task)
		}
	}

	// Sort epics by topo order: dependencies first (what you do first appears first)
	epics = topoSortTasks(epics, graph)

	// Build epic nodes with their tasks (flat - all epics at root level)
	var rootEpics []*treeNode

	for _, epic := range epics {
		node := &treeNode{task: epic, isReady: isReady(epic, graph)}

		// Add tasks under this epic, sorted by dependency order
		tasks := epicTasks[epic.ID]
		tasks = topoSortTasks(tasks, graph)
		for _, t := range tasks {
			childNode := &treeNode{
				task:    t,
				isReady: isReady(t, graph),
			}
			node.children = append(node.children, childNode)
		}

		rootEpics = append(rootEpics, node)
	}

	// Build orphan task nodes
	var orphanNodes []*treeNode
	orphanTasks = topoSortTasks(orphanTasks, graph)
	for _, t := range orphanTasks {
		orphanNodes = append(orphanNodes, &treeNode{
			task:    t,
			isReady: isReady(t, graph),
		})
	}

	// Combine: orphan tasks first, then epic trees
	var roots []*treeNode
	roots = append(roots, orphanNodes...)
	roots = append(roots, rootEpics...)

	return roots
}

// topoSortTasks sorts tasks by dependency order (dependencies before dependents).
// Tasks with no dependencies come first, sorted by readiness then ID.
func topoSortTasks(tasks []*Task, graph *Graph) []*Task {
	if len(tasks) == 0 {
		return tasks
	}

	// Build in-degree map for these tasks only
	taskSet := make(map[string]bool)
	for _, t := range tasks {
		taskSet[t.ID] = true
	}

	inDegree := make(map[string]int)
	for _, t := range tasks {
		inDegree[t.ID] = 0
	}
	for _, t := range tasks {
		for depID := range graph.Deps[t.ID] {
			if taskSet[depID] {
				inDegree[t.ID]++
			}
		}
	}

	// Kahn's algorithm
	var queue []*Task
	for _, t := range tasks {
		if inDegree[t.ID] == 0 {
			queue = append(queue, t)
		}
	}

	// Sort initial queue: ready first, then by ID
	sort.Slice(queue, func(i, j int) bool {
		iReady := isReady(queue[i], graph)
		jReady := isReady(queue[j], graph)
		if iReady != jReady {
			return iReady // ready tasks first
		}
		return queue[i].ID < queue[j].ID
	})

	var result []*Task
	for len(queue) > 0 {
		// Pop first
		t := queue[0]
		queue = queue[1:]
		result = append(result, t)

		// Reduce in-degree of dependents
		for _, other := range tasks {
			if graph.Deps[other.ID] != nil {
				if _, ok := graph.Deps[other.ID][t.ID]; ok {
					inDegree[other.ID]--
					if inDegree[other.ID] == 0 {
						queue = append(queue, other)
					}
				}
			}
		}

		// Re-sort queue
		sort.Slice(queue, func(i, j int) bool {
			iReady := isReady(queue[i], graph)
			jReady := isReady(queue[j], graph)
			if iReady != jReady {
				return iReady
			}
			return queue[i].ID < queue[j].ID
		})
	}

	return result
}

// renderNode renders a single node and its children.
// parentBlockers tracks blockers already shown at a parent level to avoid repetition.
func renderNode(w io.Writer, node *treeNode, prefix string, isLast bool, graph *Graph, repoDir string, useColor bool, parentBlockers map[string]bool, termWidth int) {
	task := node.task

	// Determine connector
	connector := "├"
	if isLast {
		connector = "└"
	}

	// Handle collapsed (done) epics
	if node.collapsed && task.IsEpic {
		title := firstLine(task.Body)
		countStr := fmt.Sprintf("[%d tasks]", node.collapsedCount)
		line := formatCollapsedEpicLine(prefix, connector, task.ID, title, countStr, useColor, termWidth)
		fmt.Fprintln(w, line)
		return
	}

	// Build the line
	icon := stateIcon(task, node.isReady)
	title := firstLine(task.Body)

	// Worker indicator - compact, shown before title
	workerIndicator := ""
	if task.Worker == workerHuman && !task.IsEpic {
		workerIndicator = "[h]"
	}

	// Annotations - keep minimal to reduce noise
	var annotations []string

	// Claimed by (always show - this is actionable info)
	if task.ClaimedBy != "" {
		annotations = append(annotations, "@"+task.ClaimedBy)
	}

	// Blocking info - only show blockers that aren't already shown by parent
	var thisBlockers map[string]bool
	var blockerAnnotation string
	if !node.isReady && task.State == stateTodo {
		allBlockers := getBlockers(task, graph)
		var newBlockers []string
		thisBlockers = make(map[string]bool)
		for _, bid := range allBlockers {
			thisBlockers[bid] = true
			if parentBlockers == nil || !parentBlockers[bid] {
				// Show by name, not ID
				if blocker := graph.Tasks[bid]; blocker != nil {
					name := abbreviate(firstLine(blocker.Body), 20)
					newBlockers = append(newBlockers, name)
				} else {
					newBlockers = append(newBlockers, bid)
				}
			}
		}
		if len(newBlockers) > 2 {
			// Too many blockers - show count instead of list
			blockerAnnotation = fmt.Sprintf("⧗ %d blockers", len(newBlockers))
		} else if len(newBlockers) > 0 {
			blockerAnnotation = "⧗ " + strings.Join(newBlockers, ", ")
		}
	}

	// Format with optional color
	line := formatTreeLine(prefix, connector, icon, task.ID, title, workerIndicator, annotations, blockerAnnotation, task, node.isReady, useColor, termWidth)
	fmt.Fprintln(w, line)

	// Show result file URL on separate line for done tasks
	if (task.State == stateDone || task.State == stateCanceled) && len(task.Results) > 0 {
		latest := task.Results[0]
		fileURL := deriveFileURL(latest.Path, repoDir)
		resultPrefix := prefix
		if isLast {
			resultPrefix += "  "
		} else {
			resultPrefix += "│ "
		}
		resultLine := formatResultLine(resultPrefix, fileURL, useColor)
		fmt.Fprintln(w, resultLine)
	}

	// Render children, passing down our blockers so they don't repeat
	childPrefix := prefix
	if isLast {
		childPrefix += "  "
	} else {
		childPrefix += "│ "
	}

	// Merge parent blockers with this node's blockers for children
	childBlockers := make(map[string]bool)
	for k := range parentBlockers {
		childBlockers[k] = true
	}
	for k := range thisBlockers {
		childBlockers[k] = true
	}

	for i, child := range node.children {
		renderNode(w, child, childPrefix, i == len(node.children)-1, graph, repoDir, useColor, childBlockers, termWidth)
	}
}

// abbreviate truncates a string to maxLen, adding "…" if truncated.
func abbreviate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// stateIcon returns the appropriate icon for a task's state.
func stateIcon(task *Task, isReady bool) string {
	if task.IsEpic {
		return "" // Epics don't get icons
	}
	switch task.State {
	case stateDone:
		return iconDone
	case stateCanceled:
		return iconCanceled
	case stateError:
		return iconError
	case stateDoing:
		return iconDoing
	case stateBlocked:
		return iconBlocked
	case stateTodo:
		if isReady {
			return iconReady
		}
		return iconBlocked
	default:
		return "?"
	}
}

// getBlockers returns the IDs that are blocking this task.
func getBlockers(task *Task, graph *Graph) []string {
	var blockers []string

	// Check task deps
	for depID := range graph.Deps[task.ID] {
		dep := graph.Tasks[depID]
		if dep != nil && dep.State != stateDone && dep.State != stateCanceled {
			blockers = append(blockers, depID)
		}
	}

	// Check epic deps (if task is in an epic)
	if task.EpicID != "" {
		for epicDepID := range graph.Deps[task.EpicID] {
			epicDep := graph.Tasks[epicDepID]
			if epicDep != nil && epicDep.IsEpic && !isEpicComplete(epicDepID, graph) {
				blockers = append(blockers, epicDepID)
			}
		}
	}

	sort.Strings(blockers)
	return blockers
}

// visibleLen returns the visible length of a string, excluding ANSI escape codes.
func visibleLen(s string) int {
	length := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

// formatCollapsedEpicLine formats a done epic as a single collapsed line.
// Format: ├ ✓ Epic title [3 tasks]                                    EPICID
func formatCollapsedEpicLine(prefix, connector, id, title, countStr string, useColor bool, termWidth int) string {
	var sb strings.Builder

	// Tree structure (dim)
	if useColor {
		sb.WriteString(colorDim)
	}
	sb.WriteString(prefix)
	sb.WriteString(connector)
	sb.WriteString(" ")
	if useColor {
		sb.WriteString(colorReset)
	}

	// Done icon
	if useColor {
		sb.WriteString(colorGreen)
	}
	sb.WriteString("✓")
	sb.WriteString(" ")
	if useColor {
		sb.WriteString(colorReset)
	}

	// Title (dim for done epic)
	if useColor {
		sb.WriteString(colorDim)
	}
	sb.WriteString(title)
	sb.WriteString(" ")
	sb.WriteString(countStr)
	if useColor {
		sb.WriteString(colorReset)
	}

	// Calculate padding for right-aligned ID
	leftLen := len(prefix) + len(connector) + 1 + 2 + len(title) + 1 + len(countStr)
	padding := termWidth - leftLen - len(id) - 2
	if padding < 2 {
		padding = 2
	}
	sb.WriteString(strings.Repeat(" ", padding))

	// ID (dim, right-aligned)
	if useColor {
		sb.WriteString(colorDim)
	}
	sb.WriteString(id)
	if useColor {
		sb.WriteString(colorReset)
	}

	return sb.String()
}

// formatTreeLine formats a tree line with optional color.
// Visual hierarchy: icon → [h] → title → @claimer → [blocker column] → ID (right-aligned)
func formatTreeLine(prefix, connector, icon, id, title, workerIndicator string, annotations []string, blockerAnnotation string, task *Task, isReady bool, useColor bool, termWidth int) string {
	var sb strings.Builder

	// Tree structure (dim)
	if useColor {
		sb.WriteString(colorDim)
	}
	sb.WriteString(prefix)
	sb.WriteString(connector)
	sb.WriteString(" ")
	if useColor {
		sb.WriteString(colorReset)
	}

	// Icon with state-based color
	if icon != "" {
		if useColor {
			sb.WriteString(stateColor(task))
		}
		sb.WriteString(icon)
		sb.WriteString(" ")
		if useColor {
			sb.WriteString(colorReset)
		}
	}

	// Worker indicator (e.g., [h] for human) - before title, dim
	if workerIndicator != "" {
		if useColor {
			sb.WriteString(colorDim)
		}
		sb.WriteString(workerIndicator)
		sb.WriteString(" ")
		if useColor {
			sb.WriteString(colorReset)
		}
	}

	// Title (prominent for ready/epics, dim for blocked)
	isBlocked := task.State == stateTodo && !isReady && !task.IsEpic
	if useColor {
		if isBlocked {
			sb.WriteString(colorDim)
		} else if task.IsEpic {
			sb.WriteString(colorBold)
		}
	}
	sb.WriteString(title)
	if useColor {
		sb.WriteString(colorReset)
	}

	// Annotations (dim) - like @claimer - immediately after title
	if len(annotations) > 0 {
		sb.WriteString("  ")
		if useColor {
			sb.WriteString(colorDim)
		}
		sb.WriteString(strings.Join(annotations, "  "))
		if useColor {
			sb.WriteString(colorReset)
		}
	}

	// Blocker annotation at a fixed column (about 60% of terminal width)
	// This creates a visual column for ⧗ to align
	blockerCol := termWidth * 55 / 100 // blocker column at ~55% of width
	idCol := termWidth - len(id) - 2   // ID ends 2 chars from right edge

	if blockerAnnotation != "" {
		// Pad to blocker column
		currentLen := visibleLen(sb.String())
		paddingToBlocker := blockerCol - currentLen
		if paddingToBlocker > 1 {
			sb.WriteString(strings.Repeat(" ", paddingToBlocker))
		} else {
			sb.WriteString("  ")
		}

		// Truncate blocker text if it would collide with ID
		maxBlockerLen := idCol - blockerCol - 2
		if useColor {
			sb.WriteString(colorDim)
		}
		if len(blockerAnnotation) > maxBlockerLen && maxBlockerLen > 3 {
			sb.WriteString(blockerAnnotation[:maxBlockerLen-1])
			sb.WriteString("…")
		} else {
			sb.WriteString(blockerAnnotation)
		}
		if useColor {
			sb.WriteString(colorReset)
		}
	}

	// ID (right-aligned; white for epics, dim for tasks)
	currentLen := visibleLen(sb.String())
	padding := idCol - currentLen

	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	} else {
		sb.WriteString("  ") // minimum spacing if line is too long
	}
	if useColor {
		if task.IsEpic {
			// White/normal for epic IDs - they stand out more
			sb.WriteString(colorReset)
		} else {
			sb.WriteString(colorDim)
		}
	}
	sb.WriteString(id)
	if useColor {
		sb.WriteString(colorReset)
	}

	return sb.String()
}

// formatResultLine formats the result file line - just arrow and file link.
func formatResultLine(prefix, fileURL string, useColor bool) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString("  → ")
	if useColor {
		sb.WriteString(colorCyan)
	}
	sb.WriteString(fileURL)
	if useColor {
		sb.WriteString(colorReset)
	}
	return sb.String()
}

// stateColor returns the ANSI color for a task's state.
func stateColor(task *Task) string {
	switch task.State {
	case stateDone:
		return colorGreen
	case stateCanceled:
		return colorDim
	case stateError:
		return colorRed
	case stateDoing:
		return colorCyan
	case stateBlocked:
		return colorDim
	case stateTodo:
		return colorYellow
	default:
		return ""
	}
}
