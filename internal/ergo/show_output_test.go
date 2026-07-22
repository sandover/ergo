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
		ClaimedBy: "agent@host", Title: "Implement login", Body: "Line one\n\n- literal Markdown",
		CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		Deps: []string{"BEFORE"}, RDeps: []string{"AFTER1"},
		Messages: []Message{{Kind: "release", Text: "Retry with the new token.", CreatedAt: now.Add(time.Minute)}},
		Results: []Result{
			{Path: "docs/new.md", Summary: "docs/new.md"},
			{Path: "docs/legacy.md", Summary: "Legacy caption"},
		},
	}
	graph := &Graph{Tasks: map[string]*Task{
		"ABCDEF": task,
		"BEFORE": {ID: "BEFORE", Title: "Prepare schema"},
		"AFTER1": {ID: "AFTER1", Title: "Run rollout"},
	}}
	meta := &TaskMeta{LastClaimAt: now.Add(30 * time.Second)}

	var buf bytes.Buffer
	printTaskDocument(&buf, task, meta, graph, "/repo")
	output := buf.String()
	if !strings.HasPrefix(output, "---\nid: \"ABCDEF\"\n") {
		t.Fatalf("ID is not fixed at the start of front matter: %s", output)
	}
	for _, want := range []string{
		"state: \"doing\"", "container_id: \"PARENT\"", "claimed_by: \"agent@host\"",
		"Line one\n\n- literal Markdown", "depends on `BEFORE`: Prepare schema",
		"blocks `AFTER1`: Run rollout", "## Messages", "Retry with the new token.",
		"[docs/new.md](file:///repo/docs/new.md)", "[docs/legacy.md](file:///repo/docs/legacy.md): Legacy caption",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "[docs/new.md](file:///repo/docs/new.md): docs/new.md") {
		t.Fatalf("path-only result repeated its path as a caption: %s", output)
	}
}
