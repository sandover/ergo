// Purpose: Implement init and create commands for tasks.
// Exports: RunInit, RunNewTask, RunNewEpic.
// Role: Command layer for creation workflows and repo initialization.
// Invariants: Writes are append-only under lock; create is safe under concurrent writers.
// Notes: Titles are positional, bodies are optional stdin, and draft creation
// keeps new work unavailable until a planner explicitly opens it.
package ergo

import (
	"errors"
	"fmt"
	"io"
)

func RunInit(args []string, opts GlobalOptions, render RenderOptions) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	if len(args) > 1 {
		return errors.New("usage: ergo init [dir]")
	}
	outcome, err := NewApplication(opts).Initialize(InitializeRequest{Dir: dir})
	if err != nil {
		return err
	}
	RenderInitialize(render.writer(), outcome)
	return nil
}

func RenderInitialize(w io.Writer, outcome InitializeResult) {
	switch outcome.Status {
	case "initialized":
		fmt.Fprintf(w, "Initialized Ergo at %s\n", outcome.Path)
	case "repaired":
		fmt.Fprintf(w, "Repaired Ergo at %s\n", outcome.Path)
	default:
		fmt.Fprintf(w, "Ergo already initialized at %s\n", outcome.Path)
	}
}

func RunNewTask(title, epicID, body string, opts GlobalOptions, render RenderOptions) error {
	created, err := NewApplication(opts).CreateTask(CreateTaskRequest{
		Title: title, EpicID: epicID, Body: body,
	})
	if err != nil {
		return err
	}

	RenderCreateTask(render.writer(), created)
	return nil
}

func RenderCreateTask(w io.Writer, outcome CreateTaskOutcome) {
	fmt.Fprintln(w, outcome.ID)
}

func RunNewEpic(title, filePath, body string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).CreateEpic(CreateEpicRequest{
		Title: title, FilePath: filePath, Body: body,
	})
	if err != nil {
		return err
	}
	RenderCreateEpic(render.writer(), outcome)
	return nil
}

func RenderCreateEpic(w io.Writer, out bulkCreateOutput) {
	fmt.Fprintf(w, "Epic %s - %s\n", out.ID, out.Title)
	for _, child := range out.Children {
		fmt.Fprintf(w, "  %s - %s\n", child.ID, child.Title)
	}
	fmt.Fprintf(w, "%d tasks, %d dependencies\n", len(out.Children), len(out.Edges))
}
