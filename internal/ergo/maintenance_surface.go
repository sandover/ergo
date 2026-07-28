package ergo

import (
	"fmt"
	"io"
	"strings"
)

func RunCompact(opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Compact()
	if err != nil {
		return err
	}
	RenderCompact(render.writer(), outcome)
	return nil
}

func RenderCompact(w io.Writer, outcome CompactOutcomeResult) {
	fmt.Fprintf(w, "Compacted %s: %d source records -> %d snapshot records\n", outcome.Path, outcome.SourceRecords, outcome.SnapshotRecords)
}

func RunPrune(confirm bool, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Prune(PruneRequest{Confirm: confirm})
	if err != nil {
		return err
	}
	RenderPrune(render.writer(), outcome, render.Color, render.width())
	return nil
}

func RenderPrune(w io.Writer, outcome PruneOutcome, useColor bool, termWidth int) {
	confirm, items := outcome.Confirmed, outcome.Items
	if len(items) == 0 {
		printPruneEmpty(w, useColor)
		return
	}

	if !confirm {
		printPrunePreview(w, items, useColor, termWidth)
	} else {
		printPruneApplied(w, items, useColor)
	}
}

func printPruneEmpty(w io.Writer, useColor bool) {
	msg := "Nothing to prune."
	if useColor {
		fmt.Fprint(w, colorDim)
	}
	fmt.Fprint(w, msg)
	if useColor {
		fmt.Fprint(w, colorReset)
	}
	fmt.Fprintln(w, " All work is still active.")
}

func printPrunePreview(w io.Writer, items []PruneItem, useColor bool, termWidth int) {
	stats := computePruneStats(items)
	total := stats.done + stats.canceled + stats.containers

	// Header - tells you exactly what this is
	if useColor {
		fmt.Fprint(w, colorBold)
	}
	fmt.Fprintf(w, "Would remove %d items:\n", total)
	if useColor {
		fmt.Fprint(w, colorReset)
	}
	fmt.Fprintln(w)

	// Summary stats - breaks down what those items are
	printPruneStats(w, stats, useColor)
	fmt.Fprintln(w)

	// Item list
	printPruneItemList(w, items, useColor, termWidth)
	fmt.Fprintln(w)

	// Safety note and call to action
	if useColor {
		fmt.Fprint(w, colorDim)
	}
	fmt.Fprintln(w, "This is a preview. Active work (todo, doing, blocked, error) is never pruned.")
	if useColor {
		fmt.Fprint(w, colorReset)
	}
	fmt.Fprintln(w, "To apply: ergo prune --yes")
}

func printPruneApplied(w io.Writer, items []PruneItem, useColor bool) {
	stats := computePruneStats(items)
	total := stats.done + stats.canceled + stats.containers

	// Header
	if useColor {
		fmt.Fprint(w, colorBold)
	}
	fmt.Fprintf(w, "Pruned %d items\n", total)
	if useColor {
		fmt.Fprint(w, colorReset)
	}
	fmt.Fprintln(w)

	// Summary stats only (no item list after apply)
	printPruneStats(w, stats, useColor)
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

func printPruneStats(w io.Writer, stats pruneStats, useColor bool) {
	if stats.done > 0 {
		fmt.Fprint(w, "  ")
		if useColor {
			fmt.Fprint(w, colorGreen)
		}
		fmt.Fprint(w, iconDone)
		if useColor {
			fmt.Fprint(w, colorReset)
		}
		fmt.Fprintf(w, " %d done tasks\n", stats.done)
	}
	if stats.canceled > 0 {
		fmt.Fprint(w, "  ")
		if useColor {
			fmt.Fprint(w, colorDim)
		}
		fmt.Fprint(w, iconCanceled)
		if useColor {
			fmt.Fprint(w, colorReset)
		}
		fmt.Fprintf(w, " %d canceled tasks\n", stats.canceled)
	}
	if stats.containers > 0 {
		fmt.Fprint(w, "  ")
		fmt.Fprint(w, iconEpic)
		fmt.Fprintf(w, "  %d empty epics\n", stats.containers)
	}
}

func printPruneItemList(w io.Writer, items []PruneItem, useColor bool, termWidth int) {
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

		fmt.Fprintln(w, line.String())
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

func RunWhere(opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Where()
	if err != nil {
		return err
	}
	RenderWhere(render.writer(), outcome)
	return nil
}

func RenderWhere(w io.Writer, outcome WhereOutcome) {
	fmt.Fprintln(w, outcome.Path)
}
