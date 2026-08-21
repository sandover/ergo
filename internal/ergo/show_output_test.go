// Purpose: Verify the canonical Markdown task document consumed by agents.
// Exports: none.
// Role: Presentation coverage shared by show and claim.
// Invariants: IDs stay fixed in front matter and task prose remains unescaped.
// Invariants: new path-only results and distinct legacy summaries render clearly.
package ergo

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTaskDocumentRendersCompleteAgentContext(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	task := &Task{
		ID: "ABCDEF", UUID: "uuid", EpicID: "PARENT", State: stateDoing,
		ClaimedBy: "agent@host", ClaimedAt: now.Add(30 * time.Second),
		Title: "Implement login", Body: "Line one\n\n- literal Markdown",
		CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		Messages: []Message{{Kind: "release", Text: "Retry with the new token.", CreatedAt: now.Add(time.Minute)}},
		Results: []Result{
			{Path: "docs/new.md", Summary: "docs/new.md"},
			{Path: "docs/legacy.md", Summary: "Legacy caption"},
		},
	}
	graph := &Graph{
		Tasks: map[string]*Task{
			"ABCDEF": task,
			"BEFORE": {ID: "BEFORE", Title: "Prepare schema"},
			"AFTER1": {ID: "AFTER1", Title: "Run rollout"},
		},
		Deps: map[string]map[string]struct{}{
			"ABCDEF": {"BEFORE": {}},
			"AFTER1": {"ABCDEF": {}},
		},
	}
	journal := []JournalEntry{
		newJournalEntry("ABCDEF", "release", "agent@host", "Retry with the new token.", now.Add(time.Minute)),
		{Version: journalVersion, TaskID: "ABCDEF", Kind: "result", At: formatTime(now.Add(2 * time.Minute)), Text: "Legacy caption", File: &JournalFile{Path: "docs/legacy.md", SHA256: "abc"}},
		{Version: journalVersion, TaskID: "ABCDEF", Kind: "result", At: formatTime(now.Add(3 * time.Minute)), Text: "Current result", File: &JournalFile{Path: "docs/new.md", SHA256: "def"}},
	}
	var buf bytes.Buffer
	printTaskDocument(&buf, task, graph, journal, "/repo", false)
	output := buf.String()
	if !strings.HasPrefix(output, "---\nid: \"ABCDEF\"\n") {
		t.Fatalf("ID is not fixed at the start of front matter: %s", output)
	}
	for _, want := range []string{
		"state: \"doing\"", "parent: \"PARENT\"", "claimed_by: \"agent@host\"",
		"Line one\n\n- literal Markdown", "depends on `BEFORE`: Prepare schema",
		"blocks `AFTER1`: Run rollout", "## Journal", "Retry with the new token.",
		"[docs/new.md](file:///repo/docs/new.md)", "[docs/legacy.md](file:///repo/docs/legacy.md)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %s", want, output)
		}
	}
}

func TestColoredTaskDocumentsReduceExactlyToPlainOutput(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	epic := &Task{
		ID: "EPIC01", Title: "User epic title", Body: "Epic **body**", State: stateTodo,
		CreatedAt: now, UpdatedAt: now,
	}
	child := &Task{
		ID: "TASK01", EpicID: epic.ID, Title: "User task title", State: stateDoing,
		ClaimedBy: "agent@host", Body: "Literal {{CYAN}} body\n\n# User heading",
		CreatedAt: now, UpdatedAt: now,
		Messages: []Message{{Kind: "blocked", Text: "User message with `code`.", CreatedAt: now}},
		Results:  []Result{{Path: "docs/result.md", Summary: "User result summary"}},
	}
	dependency := &Task{ID: "DEP001", Title: "User dependency title", State: stateDone}
	graph := &Graph{
		Tasks: map[string]*Task{epic.ID: epic, child.ID: child, dependency.ID: dependency},
		Deps: map[string]map[string]struct{}{
			epic.ID:       {},
			child.ID:      {dependency.ID: {}},
			dependency.ID: {},
		},
	}
	journal := []JournalEntry{
		newJournalEntry(child.ID, "block", "agent@host", "User message with `code`.", now),
		{Version: journalVersion, TaskID: child.ID, Kind: "result", At: formatTime(now.Add(time.Second)), Text: "User result summary", File: &JournalFile{Path: "docs/result.md", SHA256: "abc"}},
	}

	render := func(color bool, outcome ShowOutcome) string {
		var output bytes.Buffer
		RenderShow(&output, outcome, color)
		return output.String()
	}
	cases := []struct {
		name    string
		outcome ShowOutcome
	}{
		{name: "leaf", outcome: ShowOutcome{Graph: graph, Task: child, Journal: journal, ProjectDir: "/project"}},
		{name: "epic", outcome: ShowOutcome{Graph: graph, Task: epic, Children: []*Task{child}, Journal: journal, ProjectDir: "/project"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plain := render(false, test.outcome)
			colored := render(true, test.outcome)
			if strings.Contains(plain, "\x1b[") {
				t.Fatalf("plain document contains ANSI: %q", plain)
			}
			if !strings.Contains(colored, "\x1b[") {
				t.Fatalf("colored document contains no ANSI: %q", colored)
			}
			if got := stripANSICodes(colored); got != plain {
				t.Fatalf("stripped colored document differs from plain\ncolored: %q\nplain:   %q", got, plain)
			}
			for _, literal := range []string{
				child.Title, child.Body, dependency.Title,
			} {
				if !strings.Contains(colored, literal) {
					t.Fatalf("colored document changed user-authored text %q:\n%s", literal, colored)
				}
			}
			if test.name == "leaf" && !strings.Contains(colored, journal[0].Text) {
				t.Fatalf("leaf document changed journal text %q:\n%s", journal[0].Text, colored)
			}
		})
	}
}

func TestColoredClaimDocumentIncludesStyledNextCommandsAndReducesToPlain(t *testing.T) {
	task := &Task{
		ID: "TASK01", Title: "Claimed task", State: stateDoing, ClaimedBy: "agent",
		Body: "User body\n", Messages: []Message{{Kind: "claim", Text: "User message"}},
		Results: []Result{{Path: "result.txt", Summary: "User summary"}},
	}
	dependency := &Task{ID: "DEP001", Title: "Dependency", State: stateDone}
	graph := &Graph{
		Tasks: map[string]*Task{task.ID: task, dependency.ID: dependency},
		Deps:  map[string]map[string]struct{}{task.ID: {dependency.ID: {}}, dependency.ID: {}},
	}
	outcome := ClaimOutcome{Graph: graph, Task: task, ProjectDir: "/project"}
	var plain, colored bytes.Buffer
	RenderClaim(&plain, outcome, false)
	RenderClaim(&colored, outcome, true)

	if got := stripANSICodes(colored.String()); got != plain.String() {
		t.Fatalf("stripped colored claim differs from plain\ncolored: %q\nplain:   %q", got, plain.String())
	}
	for _, command := range []string{
		"ergo done TASK01", "ergo block TASK01", "ergo cancel TASK01", "ergo release TASK01",
	} {
		want := colorGreen + command + colorReset
		if !strings.Contains(colored.String(), want) {
			t.Fatalf("claim command is not styled %q:\n%s", command, colored.String())
		}
	}
}
