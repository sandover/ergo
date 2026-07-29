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
}

// printTaskDocument renders the complete leaf representation used by show and claim.
func printTaskDocument(w io.Writer, task *Task, graph *Graph, repoDir string) {
	fields := []frontMatterField{
		{key: "id", value: task.ID},
		{key: "title", value: task.Title},
		{key: "state", value: task.State},
	}
	if task.EpicID != "" {
		fields = append(fields, frontMatterField{key: "parent", value: task.EpicID})
	}
	if task.ClaimedBy != "" {
		fields = append(fields, frontMatterField{key: "claimed_by", value: task.ClaimedBy})
		if claimedAt := claimedAtForTask(task); claimedAt != "" {
			fields = append(fields, frontMatterField{key: "claimed_at", value: claimedAt})
		}
	}
	fields = append(fields,
		frontMatterField{key: "created_at", value: formatTime(task.CreatedAt)},
		frontMatterField{key: "updated_at", value: formatTime(task.UpdatedAt)},
	)
	writeShowFrontMatter(w, fields)

	fmt.Fprintf(w, "# %s\n\n", showTitle(task.Title, task.ID))
	if task.Body != "" {
		printMarkdownBody(w, task.Body)
		fmt.Fprintln(w)
	}

	printTaskDependenciesMarkdown(w, task, graph, "## Dependencies")
	printTaskMessagesMarkdown(w, task.Messages, "## Messages")
	printTaskResultsMarkdown(w, task.Results, repoDir, "## Results")
}

// printContainerDocument renders a container and the complete details of each child.
func printContainerDocument(w io.Writer, epic *Task, children []*Task, graph *Graph, repoDir string) {
	writeShowFrontMatter(w, []frontMatterField{
		{key: "epic", value: "true", raw: true},
		{key: "id", value: epic.ID},
		{key: "title", value: epic.Title},
		{key: "created_at", value: formatTime(epic.CreatedAt)},
		{key: "updated_at", value: formatTime(epic.UpdatedAt)},
	})

	fmt.Fprintf(w, "# %s\n\n", showTitle(epic.Title, epic.ID))
	if epic.Body != "" {
		printMarkdownBody(w, epic.Body)
		fmt.Fprintln(w)
	}
	printTaskDependenciesMarkdown(w, epic, graph, "## Dependencies")

	fmt.Fprintln(w, "## Tasks")
	fmt.Fprintln(w)
	for index, child := range children {
		fmt.Fprintf(w, "### %s - %s\n\n", child.ID, showTitle(child.Title, child.ID))
		fmt.Fprintf(w, "- state: %s\n", child.State)
		if child.ClaimedBy != "" {
			fmt.Fprintf(w, "- claimed by: %s\n", child.ClaimedBy)
		}
		fmt.Fprintln(w)

		if child.Body != "" {
			printMarkdownBody(w, child.Body)
			fmt.Fprintln(w)
		}
		printTaskDependenciesMarkdown(w, child, graph, "#### Dependencies")
		printTaskMessagesMarkdown(w, child.Messages, "#### Messages")
		printTaskResultsMarkdown(w, child.Results, repoDir, "#### Results")
		if index < len(children)-1 {
			fmt.Fprintln(w)
		}
	}
}

func writeShowFrontMatter(w io.Writer, fields []frontMatterField) {
	fmt.Fprintln(w, "---")
	for _, field := range fields {
		if field.raw {
			fmt.Fprintf(w, "%s: %s\n", field.key, field.value)
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", field.key, yamlString(field.value))
	}
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
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

func printTaskDependenciesMarkdown(w io.Writer, task *Task, graph *Graph, heading string) {
	dependencies := graph.Dependencies(task.ID)
	dependents := graph.Dependents(task.ID)
	if len(dependencies) == 0 && len(dependents) == 0 {
		return
	}
	fmt.Fprintln(w, heading)
	for _, id := range dependencies {
		fmt.Fprintf(w, "- depends on `%s`", id)
		if dep := graph.Tasks[id]; dep != nil && dep.Title != "" {
			fmt.Fprintf(w, ": %s", dep.Title)
		}
		fmt.Fprintln(w)
	}
	for _, id := range dependents {
		fmt.Fprintf(w, "- blocks `%s`", id)
		if dependent := graph.Tasks[id]; dependent != nil && dependent.Title != "" {
			fmt.Fprintf(w, ": %s", dependent.Title)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

func printTaskMessagesMarkdown(w io.Writer, messages []Message, heading string) {
	if len(messages) == 0 {
		return
	}
	fmt.Fprintln(w, heading)
	fmt.Fprintln(w)
	for _, message := range messages {
		fmt.Fprintf(w, "**%s - %s**\n\n", message.Kind, formatTime(message.CreatedAt))
		printMarkdownBody(w, message.Text)
		fmt.Fprintln(w)
	}
}

func printTaskResultsMarkdown(w io.Writer, results []Result, repoDir string, heading string) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintln(w, heading)
	for _, result := range results {
		fileURL := deriveFileURL(result.Path, repoDir)
		fmt.Fprintf(w, "- [%s](%s)", result.Path, fileURL)
		if result.Summary != "" && result.Summary != result.Path {
			fmt.Fprintf(w, ": %s", result.Summary)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

func RunShow(id string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Show(ShowRequest{ID: id})
	if err != nil {
		return err
	}
	RenderShow(render.writer(), outcome)
	return nil
}

func RenderShow(w io.Writer, outcome ShowOutcome) {
	if outcome.Graph.IsEpic(outcome.Task.ID) {
		printContainerDocument(w, outcome.Task, outcome.Children, outcome.Graph, outcome.ProjectDir)
		return
	}
	printTaskDocument(w, outcome.Task, outcome.Graph, outcome.ProjectDir)
}

// RenderShowBody writes the stored body without adding or removing bytes.
func RenderShowBody(w io.Writer, outcome ShowBodyOutcome) error {
	_, err := io.WriteString(w, outcome.Body)
	return err
}
