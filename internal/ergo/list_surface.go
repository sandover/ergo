package ergo

import (
	"fmt"
	"io"
)

type ListOptions struct {
	EpicID    string
	ReadyOnly bool
	ShowAll   bool
}

func RunList(listOpts ListOptions, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).List(listOpts)
	if err != nil {
		return err
	}
	RenderList(render.writer(), outcome, render.Color, render.width())
	return nil
}

func RenderList(w io.Writer, outcome ListOutcome, useColor bool, width int) {
	epicID := outcome.Options.EpicID
	readyOnly := outcome.Options.ReadyOnly
	showAll := outcome.Options.ShowAll
	graph, roots, repoDir := outcome.Graph, outcome.Roots, outcome.ProjectDir
	printSummary := func(stats taskStats, buckets []summaryBucket, addSpacing bool) {
		renderSummary(w, stats, useColor, buckets, addSpacing)
	}
	allTasks, activeTasks, readyTasks := outcome.AllTasks, outcome.ActiveTasks, outcome.ReadyTasks

	if epicID != "" {
		epicChildren, epicChildrenReady := outcome.EpicChildren, outcome.EpicReady

		renderTreeView(w, roots, graph, repoDir, useColor, width)

		switch {
		case readyOnly:
			if len(epicChildren) == 0 {
				fmt.Fprintln(w, "No tasks in this epic.")
				return
			}
			if len(epicChildrenReady) == 0 {
				fmt.Fprintln(w, "No ready tasks in this epic.")
				stats := computeStatsForTasks(epicChildren, graph)
				printSummary(stats, []summaryBucket{summaryInProgress, summaryBlocked, summaryWaiting, summaryError}, false)
				return
			}
			stats := computeStatsForTasks(epicChildrenReady, graph)
			printSummary(stats, []summaryBucket{summaryReady}, true)
			return
		default:
			if len(epicChildren) == 0 {
				fmt.Fprintln(w, "No tasks in this epic.")
				return
			}
			// Epic-focused view includes done/canceled by default.
			stats := computeStatsForTasks(epicChildren, graph)
			printSummary(stats, []summaryBucket{summaryReady, summaryInProgress, summaryBlocked, summaryWaiting, summaryError, summaryDone, summaryCanceled}, true)
			return
		}
	}

	switch {
	case readyOnly:
		if len(allTasks) == 0 {
			fmt.Fprintln(w, "No tasks.")
			return
		}
		if len(readyTasks) == 0 {
			fmt.Fprintln(w, "No ready tasks.")
			stats := computeStatsForTasks(activeTasks, graph)
			printSummary(stats, []summaryBucket{summaryInProgress, summaryBlocked, summaryWaiting, summaryError}, false)
			return
		}
		renderTreeView(w, roots, graph, repoDir, useColor, width)
		stats := computeStatsForTasks(readyTasks, graph)
		printSummary(stats, []summaryBucket{summaryReady}, true)
		return
	case showAll:
		if len(allTasks) == 0 && len(roots) == 0 {
			fmt.Fprintln(w, "No tasks.")
			return
		}
		renderTreeView(w, roots, graph, repoDir, useColor, width)
		stats := computeStatsForTasks(allTasks, graph)
		printSummary(stats, []summaryBucket{summaryReady, summaryInProgress, summaryBlocked, summaryWaiting, summaryError, summaryDone, summaryCanceled}, true)
		return
	default:
		if len(allTasks) == 0 {
			if len(roots) == 0 {
				fmt.Fprintln(w, "No tasks.")
				return
			}
			renderTreeView(w, roots, graph, repoDir, useColor, width)
			return
		}
		if len(activeTasks) == 0 {
			fmt.Fprintln(w, "No active tasks.")
			stats := computeStatsForTasks(allTasks, graph)
			printSummary(stats, []summaryBucket{summaryDone, summaryCanceled}, false)
			return
		}
		renderTreeView(w, roots, graph, repoDir, useColor, width)
		stats := computeStatsForTasks(activeTasks, graph)
		printSummary(stats, []summaryBucket{summaryReady, summaryInProgress, summaryBlocked, summaryWaiting, summaryError}, true)
		return
	}
}

// collectEpicChildren returns all tasks belonging to the given epic in dependency order.
func collectEpicChildren(epicID string, graph *Graph) []*Task {
	return topoSortTasks(graph.Children(epicID), graph)
}

func collectNonContainerTasks(graph *Graph) []*Task {
	var tasks []*Task
	for _, task := range graph.Tasks {
		if !graph.IsEpic(task.ID) {
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
		if graph.IsReady(task.ID) {
			ready = append(ready, task)
		}
	}
	return ready
}
