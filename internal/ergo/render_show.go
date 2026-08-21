package ergo

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

type frontMatterField struct {
	key   string
	value string
	raw   bool
	style string
}

// printTaskDocument renders the complete leaf representation used by show and claim.
func printTaskDocument(w io.Writer, task *Task, graph *Graph, journal []JournalEntry, repoDir string, useColor bool) {
	fields := []frontMatterField{
		{key: "id", value: task.ID, style: colorCyan},
		{key: "title", value: task.Title},
		{key: "state", value: task.State, style: stateColor(task)},
	}
	if task.EpicID != "" {
		fields = append(fields, frontMatterField{key: "parent", value: task.EpicID, style: colorCyan})
	}
	if task.ClaimedBy != "" {
		fields = append(fields, frontMatterField{key: "claimed_by", value: task.ClaimedBy})
		if claimedAt := claimedAtForTask(task); claimedAt != "" {
			fields = append(fields, frontMatterField{key: "claimed_at", value: claimedAt, style: colorDim})
		}
	}
	fields = append(fields,
		frontMatterField{key: "created_at", value: formatTime(task.CreatedAt), style: colorDim},
		frontMatterField{key: "updated_at", value: formatTime(task.UpdatedAt), style: colorDim},
	)
	writeShowFrontMatter(w, fields, useColor)

	writeMarkdownHeading(w, "# ", showTitle(task.Title, task.ID), useColor)
	if task.Body != "" {
		printMarkdownBody(w, task.Body)
		fmt.Fprintln(w)
	}

	printTaskDependenciesMarkdown(w, task, graph, "## Dependencies", useColor)
	printJournalMarkdown(w, journalForTask(journal, task.ID), repoDir, "## Journal", useColor)
}

// printContainerDocument renders a container and the complete details of each child.
func printContainerDocument(w io.Writer, epic *Task, children []*Task, graph *Graph, journal []JournalEntry, repoDir string, useColor bool) {
	fields := []frontMatterField{
		{key: "epic", value: "true", raw: true},
		{key: "id", value: epic.ID, style: colorCyan},
		{key: "title", value: epic.Title},
	}
	if state := graph.EpicState(epic.ID); state == stateFailed {
		fields = append(fields, frontMatterField{key: "state", value: state, style: colorRed})
	}
	fields = append(fields,
		frontMatterField{key: "created_at", value: formatTime(epic.CreatedAt), style: colorDim},
		frontMatterField{key: "updated_at", value: formatTime(epic.UpdatedAt), style: colorDim},
	)
	writeShowFrontMatter(w, fields, useColor)

	writeMarkdownHeading(w, "# ", showTitle(epic.Title, epic.ID), useColor)
	if epic.Body != "" {
		printMarkdownBody(w, epic.Body)
		fmt.Fprintln(w)
	}
	printTaskDependenciesMarkdown(w, epic, graph, "## Dependencies", useColor)

	writeGeneratedLine(w, "## Tasks", colorBold+colorCyan, useColor)
	fmt.Fprintln(w)
	for index, child := range children {
		writeChildHeading(w, child, useColor)
		fmt.Fprint(w, "- state: ")
		writeGeneratedLine(w, child.State, stateColor(child), useColor)
		if child.ClaimedBy != "" {
			fmt.Fprintf(w, "- claimed by: %s\n", child.ClaimedBy)
		}
		fmt.Fprintln(w)

		if child.Body != "" {
			printMarkdownBody(w, child.Body)
			fmt.Fprintln(w)
		}
		printTaskDependenciesMarkdown(w, child, graph, "#### Dependencies", useColor)
		if result := latestExplicitResult(journal, child.ID); result != nil {
			printJournalMarkdown(w, []JournalEntry{*result}, repoDir, "#### Latest result", useColor)
		}
		if index < len(children)-1 {
			fmt.Fprintln(w)
		}
	}
}

func writeShowFrontMatter(w io.Writer, fields []frontMatterField, useColor bool) {
	writeGeneratedLine(w, "---", colorDim, useColor)
	for _, field := range fields {
		writeGenerated(w, field.key, colorCyan, useColor)
		if field.raw {
			fmt.Fprint(w, ": ")
			writeGeneratedLine(w, field.value, field.style, useColor)
			continue
		}
		fmt.Fprint(w, ": ")
		writeGeneratedLine(w, yamlString(field.value), field.style, useColor)
	}
	writeGeneratedLine(w, "---", colorDim, useColor)
	fmt.Fprintln(w)
}

func writeGenerated(w io.Writer, text, style string, useColor bool) {
	if useColor && style != "" {
		fmt.Fprint(w, style, text, colorReset)
		return
	}
	fmt.Fprint(w, text)
}

func writeGeneratedLine(w io.Writer, text, style string, useColor bool) {
	writeGenerated(w, text, style, useColor)
	fmt.Fprintln(w)
}

func writeMarkdownHeading(w io.Writer, marker, title string, useColor bool) {
	writeGenerated(w, marker, colorBold+colorCyan, useColor)
	fmt.Fprintf(w, "%s\n\n", title)
}

func writeChildHeading(w io.Writer, child *Task, useColor bool) {
	writeGenerated(w, "### ", colorBold+colorCyan, useColor)
	writeGenerated(w, child.ID, colorCyan, useColor)
	fmt.Fprintf(w, " - %s\n\n", showTitle(child.Title, child.ID))
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

func printMarkdownBody(w io.Writer, body string) {
	fmt.Fprint(w, body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Fprintln(w)
	}
}

func printTaskDependenciesMarkdown(w io.Writer, task *Task, graph *Graph, heading string, useColor bool) {
	dependencies := graph.Dependencies(task.ID)
	dependents := graph.Dependents(task.ID)
	if len(dependencies) == 0 && len(dependents) == 0 {
		return
	}
	writeGeneratedLine(w, heading, colorBold+colorCyan, useColor)
	for _, id := range dependencies {
		fmt.Fprint(w, "- ")
		writeGenerated(w, "depends on", colorDim, useColor)
		fmt.Fprint(w, " `")
		writeGenerated(w, id, colorCyan, useColor)
		fmt.Fprint(w, "`")
		if dep := graph.Tasks[id]; dep != nil && dep.Title != "" {
			fmt.Fprintf(w, ": %s", dep.Title)
		}
		fmt.Fprintln(w)
	}
	for _, id := range dependents {
		fmt.Fprint(w, "- ")
		writeGenerated(w, "blocks", colorDim, useColor)
		fmt.Fprint(w, " `")
		writeGenerated(w, id, colorCyan, useColor)
		fmt.Fprint(w, "`")
		if dependent := graph.Tasks[id]; dependent != nil && dependent.Title != "" {
			fmt.Fprintf(w, ": %s", dependent.Title)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

func printJournalMarkdown(w io.Writer, entries []JournalEntry, repoDir, heading string, useColor bool) {
	if len(entries) == 0 {
		return
	}
	writeGeneratedLine(w, heading, colorBold+colorCyan, useColor)
	fmt.Fprintln(w)
	for _, entry := range entries {
		fmt.Fprint(w, "- **")
		writeGenerated(w, entry.Kind, stateColor(&Task{State: journalKindState(entry.Kind)}), useColor)
		fmt.Fprint(w, "** — ")
		writeGenerated(w, entry.At, colorDim, useColor)
		if entry.Agent != "" {
			fmt.Fprintf(w, " — `%s`", entry.Agent)
		}
		if entry.Text != "" {
			fmt.Fprintf(w, ": %s", entry.Text)
		}
		if entry.File != nil {
			fmt.Fprintf(w, " — [%s](", entry.File.Path)
			writeGenerated(w, deriveFileURL(entry.File.Path, repoDir), colorCyan, useColor)
			fmt.Fprint(w, ")")
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

func journalKindState(kind string) string {
	switch kind {
	case "fail":
		return stateFailed
	case "done":
		return stateDone
	case "block":
		return stateBlocked
	case "cancel":
		return stateCanceled
	default:
		return stateTodo
	}
}

func RunShow(id string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Show(ShowRequest{ID: id})
	if err != nil {
		return err
	}
	RenderShow(render.writer(), outcome, render.Color)
	return nil
}

func RenderShow(w io.Writer, outcome ShowOutcome, useColor bool) {
	if outcome.Graph.IsEpic(outcome.Task.ID) {
		printContainerDocument(w, outcome.Task, outcome.Children, outcome.Graph, outcome.Journal, outcome.ProjectDir, useColor)
		return
	}
	printTaskDocument(w, outcome.Task, outcome.Graph, outcome.Journal, outcome.ProjectDir, useColor)
}

// RenderShowBody writes the stored body without adding or removing bytes.
func RenderShowBody(w io.Writer, outcome ShowBodyOutcome) error {
	_, err := io.WriteString(w, outcome.Body)
	return err
}
