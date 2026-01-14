// Tree view rendering for human-friendly list output.
package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

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
	task     *Task
	children []*treeNode
	isReady  bool
}

// renderTreeView outputs tasks in a hierarchical tree format.
func renderTreeView(w io.Writer, graph *Graph, repoDir string, useColor bool) {
	// Build tree structure: epics contain their tasks, and epics nest by dependency
	roots := buildTree(graph)

	for i, root := range roots {
		renderNode(w, root, "", i == len(roots)-1, graph, repoDir, useColor)
	}
}

// buildTree constructs a forest of tree nodes from the graph.
// Epics that depend on other epics are nested under their dependencies.
// Tasks are nested under their epic.
// Orphan tasks (no epic) appear at root level.
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

	// Sort epics by topological order (dependencies first)
	epics = topoSortTasks(epics, graph)

	// Build epic nodes with their tasks
	epicNodes := make(map[string]*treeNode)
	var rootEpics []*treeNode

	for _, epic := range epics {
		node := &treeNode{task: epic}

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

		epicNodes[epic.ID] = node

		// Check if this epic depends on another epic
		hasEpicParent := false
		for depID := range graph.Deps[epic.ID] {
			if depTask, ok := graph.Tasks[depID]; ok && depTask.IsEpic {
				// This epic depends on another epic - it will be nested
				hasEpicParent = true
				break
			}
		}
		if !hasEpicParent {
			rootEpics = append(rootEpics, node)
		}
	}

	// Nest epics under their dependency epics
	for _, epic := range epics {
		node := epicNodes[epic.ID]
		for depID := range graph.Deps[epic.ID] {
			if depTask, ok := graph.Tasks[depID]; ok && depTask.IsEpic {
				parentNode := epicNodes[depID]
				if parentNode != nil {
					parentNode.children = append(parentNode.children, node)
				}
			}
		}
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
func renderNode(w io.Writer, node *treeNode, prefix string, isLast bool, graph *Graph, repoDir string, useColor bool) {
	task := node.task

	// Determine connector
	connector := "├"
	if isLast {
		connector = "└"
	}

	// Build the line
	icon := stateIcon(task, node.isReady)
	id := task.ID
	title := firstLine(task.Body)

	// Annotations
	var annotations []string

	// Worker type (only show if human)
	if task.Worker == workerHuman {
		annotations = append(annotations, "[human]")
	}

	// Claimed by
	if task.ClaimedBy != "" {
		annotations = append(annotations, "@"+task.ClaimedBy)
	}

	// Blocking info for blocked tasks
	if !node.isReady && !task.IsEpic && task.State == stateTodo {
		blockers := getBlockers(task, graph)
		if len(blockers) > 0 {
			annotations = append(annotations, "⧗ "+strings.Join(blockers, ", "))
		}
	}

	// Format with optional color
	line := formatTreeLine(prefix, connector, icon, id, title, annotations, task, useColor)
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
		resultLine := formatResultLine(resultPrefix, latest.Summary, fileURL, useColor)
		fmt.Fprintln(w, resultLine)
	}

	// Render children
	childPrefix := prefix
	if isLast {
		childPrefix += "  "
	} else {
		childPrefix += "│ "
	}

	for i, child := range node.children {
		renderNode(w, child, childPrefix, i == len(node.children)-1, graph, repoDir, useColor)
	}
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

// formatTreeLine formats a tree line with optional color.
func formatTreeLine(prefix, connector, icon, id, title string, annotations []string, task *Task, useColor bool) string {
	var sb strings.Builder

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

	// ID
	if useColor {
		sb.WriteString(colorBlue)
	}
	sb.WriteString(id)
	if useColor {
		sb.WriteString(colorReset)
	}
	sb.WriteString("  ")

	// Title
	if useColor && (task.State == stateTodo && !isTaskReady(task)) {
		sb.WriteString(colorDim)
	}
	if useColor && task.IsEpic {
		sb.WriteString(colorBold)
	}
	sb.WriteString(title)
	if useColor {
		sb.WriteString(colorReset)
	}

	// Annotations
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

	return sb.String()
}

// isTaskReady is a simple helper that doesn't need graph (used for color only).
func isTaskReady(task *Task) bool {
	return task.State == stateTodo && task.ClaimedBy == ""
}

// formatResultLine formats the result file line.
func formatResultLine(prefix, summary, fileURL string, useColor bool) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString("  → ")
	if useColor {
		sb.WriteString(colorGreen)
	}
	sb.WriteString(summary)
	if useColor {
		sb.WriteString(colorReset)
	}
	sb.WriteString("\n")
	sb.WriteString(prefix)
	sb.WriteString("    ")
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
