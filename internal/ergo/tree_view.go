// Tree view rendering for human-friendly list output.
package ergo

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
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

const (
	// Layout contract: IDs start at (termWidth - rightMargin - idWidth - minGap).
	// All truncation and padding must use visibleLen/ truncateToWidth.
	idMinGap      = 2
	idRightMargin = 2
)

// State icons
const (
	iconDone     = "✓"
	iconReady    = "○"
	iconDoing    = "◐"
	iconBlocked  = "·"
	iconCanceled = "✗"
	iconError    = "⚠"
	iconEpic     = "Ⓔ"
)

// treeNode represents a task or epic in the tree structure.
type treeNode struct {
	task           *Task
	children       []*treeNode
	isReady        bool
	collapsed      bool // for done epics: show summary instead of children
	collapsedCount int  // number of tasks in collapsed epic
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
		title := titleForBody(task.Body)
		countStr := fmt.Sprintf("[%d tasks]", node.collapsedCount)
		line := formatCollapsedEpicLine(prefix, connector, task.ID, title, countStr, useColor, termWidth)
		fmt.Fprintln(w, line)
		return
	}

	// Build the line
	icon := stateIcon(task, node.isReady)
	title := titleForBody(task.Body)

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
					name := abbreviate(titleForBody(blocker.Body), 20)
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
		return iconEpic
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

// visibleLen returns the display width of a string, excluding ANSI escape codes.
func visibleLen(s string) int {
	return runewidth.StringWidth(stripANSICodes(s))
}

func stripANSICodes(s string) string {
	var b strings.Builder
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
		b.WriteRune(r)
	}
	return b.String()
}

// truncateToWidth truncates a string to maxWidth visible characters, adding ellipsis if truncated.
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if maxWidth <= 1 {
		return "…"
	}
	if visibleLen(s) <= maxWidth {
		return s
	}
	ellipsisWidth := runewidth.RuneWidth('…')
	targetWidth := maxWidth - ellipsisWidth
	if targetWidth < 1 {
		return "…"
	}
	var result []rune
	width := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			result = append(result, r)
			continue
		}
		if inEscape {
			result = append(result, r)
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		rw := runewidth.RuneWidth(r)
		if width+rw > targetWidth {
			break
		}
		result = append(result, r)
		width += rw
	}
	return string(result) + "…"
}

// formatCollapsedEpicLine formats a done epic as a single collapsed line.
// Format: ├ ✓ Epic title [3 tasks]                                    EPICID
func formatCollapsedEpicLine(prefix, connector, id, title, countStr string, useColor bool, termWidth int) string {
	// Layout contract: ids are right-aligned at idStart.
	minGap := idMinGap
	rightMargin := idRightMargin
	idWidth := len(id)
	idStart := termWidth - rightMargin - idWidth - minGap
	if idStart < 0 {
		idStart = 0
	}

	var base strings.Builder
	if useColor {
		base.WriteString(colorDim)
	}
	base.WriteString(prefix)
	base.WriteString(connector)
	base.WriteString(" ")
	if useColor {
		base.WriteString(colorReset)
	}
	if useColor {
		base.WriteString(colorGreen)
	}
	base.WriteString("✓")
	base.WriteString(" ")
	if useColor {
		base.WriteString(colorReset)
	}
	baseStr := base.String()
	titleSep := ""
	if !strings.HasSuffix(stripANSICodes(baseStr), " ") {
		titleSep = " "
	}
	baseWidth := visibleLen(baseStr) + visibleLen(titleSep)

	content := title + " " + countStr
	maxContent := idStart - minGap - baseWidth
	if maxContent < 0 {
		maxContent = 0
	}
	if visibleLen(content) > maxContent {
		countWidth := visibleLen(countStr)
		if maxContent >= countWidth+1 {
			title = truncateToWidth(title, maxContent-countWidth-1)
			content = title + " " + countStr
		} else if maxContent > 0 {
			content = truncateToWidth(countStr, maxContent)
		} else {
			content = ""
		}
	}

	var sb strings.Builder
	sb.WriteString(baseStr)
	sb.WriteString(content)

	padding := idStart - visibleLen(sb.String())
	if padding < 0 {
		padding = 0
	}
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(strings.Repeat(" ", minGap))

	if useColor {
		sb.WriteString(colorReset)
	}
	sb.WriteString(id)
	if useColor {
		sb.WriteString(colorReset)
	}

	return sb.String()
}

// formatTreeLine formats a tree line with optional color.
// Visual hierarchy: icon → [h] → title → @claimer → [blocker column] → ID (right-aligned)
// Ensures the line never exceeds termWidth by truncating content as needed.
func formatTreeLine(prefix, connector, icon, id, title, workerIndicator string, annotations []string, blockerAnnotation string, task *Task, isReady bool, useColor bool, termWidth int) string {
	// Layout contract: ids are right-aligned at idStart.
	minGap := idMinGap
	rightMargin := idRightMargin
	idWidth := len(id)
	idStart := termWidth - rightMargin - idWidth - minGap
	if idStart < 0 {
		idStart = 0
	}

	iconStr := ""
	if icon != "" {
		iconStr = icon + " "
		if task.IsEpic {
			iconStr += " "
		}
	}

	annotationStr := ""
	if len(annotations) > 0 {
		annotationStr = "  " + strings.Join(annotations, "  ")
	}

	// Build base prefix (tree + icon + worker).
	var base strings.Builder
	if useColor {
		base.WriteString(colorDim)
	}
	base.WriteString(prefix)
	base.WriteString(connector)
	base.WriteString(" ")
	if useColor {
		base.WriteString(colorReset)
	}
	if iconStr != "" {
		if useColor {
			base.WriteString(stateColor(task))
		}
		base.WriteString(iconStr)
		if useColor {
			base.WriteString(colorReset)
		}
	}
	if workerIndicator != "" {
		if useColor {
			base.WriteString(colorDim)
		}
		base.WriteString(workerIndicator)
		base.WriteString(" ")
		if useColor {
			base.WriteString(colorReset)
		}
	}
	baseStr := base.String()
	titleSep := ""
	if !strings.HasSuffix(stripANSICodes(baseStr), " ") {
		titleSep = " "
	}
	baseWidth := visibleLen(baseStr) + visibleLen(titleSep)

	maxContent := idStart - minGap - baseWidth
	if maxContent < 0 {
		maxContent = 0
	}

	// Fit title + annotations within maxContent.
	if visibleLen(title)+visibleLen(annotationStr) > maxContent {
		maxAnnotation := maxContent - visibleLen(title)
		if maxAnnotation > 0 && annotationStr != "" {
			annotationStr = truncateToWidth(annotationStr, maxAnnotation)
		} else {
			annotationStr = ""
		}
		if visibleLen(title) > maxContent {
			title = truncateToWidth(title, maxContent)
		}
	}

	// Build left side with color.
	var left strings.Builder
	left.WriteString(baseStr)
	if titleSep != "" {
		left.WriteString(titleSep)
	}

	isBlocked := task.State == stateTodo && !isReady && !task.IsEpic
	if useColor {
		if isBlocked {
			left.WriteString(colorDim)
		} else if task.IsEpic {
			left.WriteString(colorBold)
		}
	}
	left.WriteString(title)
	if useColor {
		left.WriteString(colorReset)
	}

	if annotationStr != "" {
		if useColor {
			left.WriteString(colorDim)
		}
		left.WriteString(annotationStr)
		if useColor {
			left.WriteString(colorReset)
		}
	}

	var sb strings.Builder
	sb.WriteString(left.String())

	// Optional blocker annotation within remaining space before ID.
	if blockerAnnotation != "" {
		available := idStart - minGap - visibleLen(sb.String())
		if available > 6 {
			blockerCol := termWidth * 55 / 100
			maxBlockerStart := idStart - minGap - 1
			if blockerCol > maxBlockerStart {
				blockerCol = maxBlockerStart
			}
			paddingToBlocker := blockerCol - visibleLen(sb.String())
			if paddingToBlocker > 1 {
				sb.WriteString(strings.Repeat(" ", paddingToBlocker))
			} else {
				sb.WriteString("  ")
			}
			maxBlockerLen := idStart - minGap - visibleLen(sb.String())
			if maxBlockerLen > 0 {
				if useColor {
					sb.WriteString(colorDim)
				}
				if visibleLen(blockerAnnotation) > maxBlockerLen {
					sb.WriteString(truncateToWidth(blockerAnnotation, maxBlockerLen))
				} else {
					sb.WriteString(blockerAnnotation)
				}
				if useColor {
					sb.WriteString(colorReset)
				}
			}
		}
	}

	// Pad to ID column and append ID.
	currentLen := visibleLen(sb.String())
	padding := idStart - currentLen
	if padding < 0 {
		padding = 0
	}
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(strings.Repeat(" ", minGap))
	if useColor {
		if task.IsEpic {
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
