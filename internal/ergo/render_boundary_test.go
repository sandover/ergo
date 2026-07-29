package ergo

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderPruneHonorsWidthAndColorCapabilities(t *testing.T) {
	outcome := PruneOutcome{
		Items: []PruneItem{{ID: "ABC123", Title: "A deliberately long title that must be truncated", State: stateDone}},
	}
	var plain bytes.Buffer
	RenderPrune(&plain, outcome, false, 32)
	for _, line := range strings.Split(strings.TrimSuffix(plain.String(), "\n"), "\n") {
		if visibleLen(line) > 32 && strings.Contains(line, "ABC123") {
			t.Fatalf("rendered item exceeds stable layout: %q", line)
		}
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatalf("plain rendering contains ANSI: %q", plain.String())
	}

	var colored bytes.Buffer
	RenderPrune(&colored, outcome, true, 32)
	if !strings.Contains(colored.String(), colorGreen) {
		t.Fatalf("color rendering lacks state color: %q", colored.String())
	}
}

func TestRenderShowPreservesStructuredDocument(t *testing.T) {
	task := &Task{ID: "ABC123", Title: "Structured task", State: stateTodo}
	graph := &Graph{
		Tasks: map[string]*Task{task.ID: task},
		Deps:  map[string]map[string]struct{}{task.ID: {}},
	}
	var output bytes.Buffer
	RenderShow(&output, ShowOutcome{Graph: graph, Task: task, ProjectDir: "/project"}, false)
	text := output.String()
	for _, fragment := range []string{"---\n", `id: "ABC123"`, `title: "Structured task"`, "# Structured task"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("structured output missing %q:\n%s", fragment, text)
		}
	}
}
