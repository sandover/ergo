package ergo

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"io"
	"strings"
)

// renderTreeView outputs tasks in a hierarchical tree format.
func renderTreeView(w io.Writer, roots []*treeNode, graph *Graph, repoDir string, useColor bool, widths ...int) {
	termWidth := 80
	if len(widths) > 0 && widths[0] > 0 {
		termWidth = widths[0]
	}
	for i, root := range roots {
		renderNode(w, root, "", i == len(roots)-1, true, graph, repoDir, useColor, nil, termWidth)
	}
}

func renderSummary(w io.Writer, stats taskStats, useColor bool, buckets []summaryBucket, addSpacing bool) {
	var parts []string

	for _, bucket := range buckets {
		var (
			count int
			label string
			color string
		)
		switch bucket {
		case summaryReady:
			count = stats.ready
			label = "ready"
			color = colorYellow
		case summaryInProgress:
			count = stats.inProgress
			label = "in progress"
			color = colorCyan
		case summaryBlocked:
			count = stats.blocked
			label = "blocked"
			color = colorDim
		case summaryWaiting:
			count = stats.waiting
			label = "waiting"
			color = colorDim
		case summaryError:
			count = stats.errors
			label = "error"
			color = colorRed
		case summaryFailed:
			count = stats.failed
			label = "failed"
			color = colorRed
		case summaryDone:
			count = stats.done
			label = "done"
			color = colorGreen
		case summaryCanceled:
			count = stats.canceled
			label = "canceled"
			color = colorDim
		}
		if count <= 0 {
			continue
		}
		part := fmt.Sprintf("%d %s", count, label)
		if useColor {
			part = color + part + colorReset
		}
		parts = append(parts, part)
	}

	if len(parts) == 0 {
		return
	}
	if addSpacing {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, strings.Join(parts, " · "))
}

// renderNode renders a single node and its children.
// parentBlockers tracks blockers already shown at a parent level to avoid repetition.
func renderNode(w io.Writer, node *treeNode, prefix string, isLast bool, isRoot bool, graph *Graph, repoDir string, useColor bool, parentBlockers map[string]bool, termWidth int) {
	task := node.task

	// Determine connector
	connector := "├"
	if isLast {
		connector = "└"
	}
	showConnector := !isRoot

	// Handle collapsed (done) epics
	if node.collapsed && node.isEpic {
		title := task.Title
		countStr := fmt.Sprintf("[%d tasks]", node.collapsedCount)
		line := formatCollapsedEpicLine(prefix, connector, showConnector, task.ID, title, countStr, useColor, termWidth)
		fmt.Fprintln(w, line)
		return
	}

	// Build the line
	displayTask := task
	if node.isEpic && graph.EpicState(task.ID) == stateFailed {
		copy := *task
		copy.State = stateFailed
		displayTask = &copy
	}
	icon := stateIcon(displayTask, node.isReady, node.isEpic)
	title := task.Title

	annotations := []string{}

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
					name := abbreviate(blocker.Title, 20)
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
	line := formatTreeLine(prefix, connector, showConnector, icon, task.ID, title, annotations, blockerAnnotation, displayTask, node.isReady, node.isEpic, useColor, termWidth)
	fmt.Fprintln(w, line)

	// Show result file URL on separate line for done tasks
	if latest, ok := latestAttachedResult(task.Results); ok {
		fileURL := deriveFileURL(latest.Path, repoDir)
		resultPrefix := prefix
		if isRoot {
			resultPrefix += "  "
		} else if isLast {
			resultPrefix += "  "
		} else {
			resultPrefix += "│ "
		}
		resultLine := formatResultLine(resultPrefix, fileURL, useColor)
		fmt.Fprintln(w, resultLine)
	}

	// Render children, passing down our blockers so they don't repeat
	childPrefix := prefix
	if isRoot {
		childPrefix = ""
	} else if isLast {
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
		renderNode(w, child, childPrefix, i == len(node.children)-1, false, graph, repoDir, useColor, childBlockers, termWidth)
	}
}

func latestAttachedResult(results []Result) (Result, bool) {
	for _, result := range results {
		if result.Path != "" {
			return result, true
		}
	}
	return Result{}, false
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
func stateIcon(task *Task, isReady, isEpic bool) string {
	if isEpic {
		if task.State == stateFailed {
			return iconFailed
		}
		return iconEpic
	}
	switch task.State {
	case stateDone:
		return iconDone
	case stateCanceled:
		return iconCanceled
	case stateFailed:
		return iconFailed
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
		return iconWaiting
	default:
		return "?"
	}
}

// getBlockers returns the IDs that are blocking this task.
func getBlockers(task *Task, graph *Graph) []string {
	if task == nil {
		return nil
	}
	return graph.Blockers(task.ID)
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
// Format: ├ ◈  ✓ Epic title [3 tasks]                                    EPICID
func formatCollapsedEpicLine(prefix, connector string, showConnector bool, id, title, countStr string, useColor bool, termWidth int) string {
	// Layout contract: ids are right-aligned at idStart.
	minGap := idMinGap
	rightMargin := idRightMargin
	idWidth := len(id)
	idStart := termWidth - rightMargin - idWidth - minGap
	if idStart < 0 {
		idStart = 0
	}

	var base strings.Builder
	if showConnector {
		if useColor {
			base.WriteString(colorDim)
		}
		base.WriteString(prefix)
		base.WriteString(connector)
		base.WriteString(" ")
		if useColor {
			base.WriteString(colorReset)
		}
	}
	base.WriteString(iconEpic)
	base.WriteString("  ")
	if useColor {
		base.WriteString(colorGreen)
	}
	base.WriteString(iconDone)
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
// Visual hierarchy: icon → title → @claimer → [blocker column] → ID (right-aligned)
// Ensures the line never exceeds termWidth by truncating content as needed.
func formatTreeLine(prefix, connector string, showConnector bool, icon, id, title string, annotations []string, blockerAnnotation string, task *Task, isReady, isEpic, useColor bool, termWidth int) string {
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
		if isEpic {
			iconStr += " "
		}
	}

	annotationStr := ""
	if len(annotations) > 0 {
		annotationStr = "  " + strings.Join(annotations, "  ")
	}

	// Build base prefix (tree + icon).
	var base strings.Builder
	if showConnector {
		if useColor {
			base.WriteString(colorDim)
		}
		base.WriteString(prefix)
		base.WriteString(connector)
		base.WriteString(" ")
		if useColor {
			base.WriteString(colorReset)
		}
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

	isBlocked := task.State == stateTodo && !isReady && !isEpic
	if useColor {
		if isBlocked {
			left.WriteString(colorDim)
		} else if isEpic {
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
		if isEpic {
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
	case stateFailed:
		return colorRed
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
