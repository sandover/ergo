// Tests for tree view rendering and formatting.
// Verifies list/show output structure for tasks and epics.
package ergo

import (
	"bytes"
	"strings"
	"testing"
)

// TestDerivedEpicState verifies the pure function that computes epic state from children.
func TestDerivedEpicState(t *testing.T) {
	tests := []struct {
		name     string
		children []*treeNode
		want     string
	}{
		{
			name:     "empty epic",
			children: nil,
			want:     "empty",
		},
		{
			name: "all tasks done",
			children: []*treeNode{
				{task: &Task{ID: "T1", State: stateDone}},
				{task: &Task{ID: "T2", State: stateDone}},
			},
			want: "done",
		},
		{
			name: "all tasks canceled",
			children: []*treeNode{
				{task: &Task{ID: "T1", State: stateCanceled}},
				{task: &Task{ID: "T2", State: stateCanceled}},
			},
			want: "canceled",
		},
		{
			name: "mixed done and canceled counts as done",
			children: []*treeNode{
				{task: &Task{ID: "T1", State: stateDone}},
				{task: &Task{ID: "T2", State: stateCanceled}},
			},
			want: "done",
		},
		{
			name: "has active work (todo)",
			children: []*treeNode{
				{task: &Task{ID: "T1", State: stateDone}},
				{task: &Task{ID: "T2", State: stateTodo}},
			},
			want: "active",
		},
		{
			name: "has active work (doing)",
			children: []*treeNode{
				{task: &Task{ID: "T1", State: stateDone}},
				{task: &Task{ID: "T2", State: stateDoing}},
			},
			want: "active",
		},
		{
			name: "has active work (blocked)",
			children: []*treeNode{
				{task: &Task{ID: "T1", State: stateCanceled}},
				{task: &Task{ID: "T2", State: stateBlocked}},
			},
			want: "active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := derivedEpicState(tt.children)
			if got != tt.want {
				t.Errorf("derivedEpicState() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFilterAndCollapseNodes verifies the filtering and collapsing logic.
func TestFilterAndCollapseNodes(t *testing.T) {
	t.Run("hides orphan done and canceled tasks", func(t *testing.T) {
		nodes := []*treeNode{
			{task: &Task{ID: "T1", State: stateTodo}},
			{task: &Task{ID: "T2", State: stateCanceled}},
			{task: &Task{ID: "T3", State: stateDone}},
		}

		filtered := filterAndCollapseNodes(nodes)

		if len(filtered) != 1 {
			t.Fatalf("expected 1 node, got %d", len(filtered))
		}
		// Only T1 should remain (T2 canceled, T3 done - both orphans)
		if filtered[0].task.ID != "T1" {
			t.Errorf("expected T1; got %s", filtered[0].task.ID)
		}
	})

	t.Run("hides fully-canceled epics", func(t *testing.T) {
		nodes := []*treeNode{
			{
				task: &Task{ID: "E1", IsEpic: true},
				children: []*treeNode{
					{task: &Task{ID: "T1", State: stateCanceled}},
					{task: &Task{ID: "T2", State: stateCanceled}},
				},
			},
			{task: &Task{ID: "T3", State: stateTodo}},
		}

		filtered := filterAndCollapseNodes(nodes)

		if len(filtered) != 1 {
			t.Fatalf("expected 1 node (epic hidden), got %d", len(filtered))
		}
		if filtered[0].task.ID != "T3" {
			t.Errorf("expected T3, got %s", filtered[0].task.ID)
		}
	})

	t.Run("hides fully-done epics", func(t *testing.T) {
		nodes := []*treeNode{
			{
				task: &Task{ID: "E1", IsEpic: true},
				children: []*treeNode{
					{task: &Task{ID: "T1", State: stateDone}},
					{task: &Task{ID: "T2", State: stateDone}},
					{task: &Task{ID: "T3", State: stateCanceled}}, // counts toward done
				},
			},
		}

		filtered := filterAndCollapseNodes(nodes)

		if len(filtered) != 0 {
			t.Fatalf("expected fully-done epic to be hidden, got %d nodes", len(filtered))
		}
	})

	t.Run("active epics show done tasks for progress visibility", func(t *testing.T) {
		nodes := []*treeNode{
			{
				task: &Task{ID: "E1", IsEpic: true},
				children: []*treeNode{
					{task: &Task{ID: "T1", State: stateDone}},     // kept for progress context
					{task: &Task{ID: "T2", State: stateTodo}},     // active
					{task: &Task{ID: "T3", State: stateCanceled}}, // hidden (abandoned)
				},
			},
		}

		filtered := filterAndCollapseNodes(nodes)

		if len(filtered) != 1 {
			t.Fatalf("expected 1 node, got %d", len(filtered))
		}
		epic := filtered[0]
		if epic.collapsed {
			t.Error("expected epic not to be collapsed (has active work)")
		}
		// T1 and T2 should remain (done task kept for progress, canceled hidden)
		if len(epic.children) != 2 {
			t.Errorf("expected 2 children, got %d", len(epic.children))
		}
		ids := []string{epic.children[0].task.ID, epic.children[1].task.ID}
		if ids[0] != "T1" || ids[1] != "T2" {
			t.Errorf("expected T1, T2; got %v", ids)
		}
	})
}

// TestCountTasks verifies the task counting helper.
func TestCountTasks(t *testing.T) {
	nodes := []*treeNode{
		{task: &Task{ID: "T1"}},
		{
			task: &Task{ID: "E1", IsEpic: true}, // epic doesn't count
			children: []*treeNode{
				{task: &Task{ID: "T2"}},
				{task: &Task{ID: "T3"}},
			},
		},
	}

	count := countTasks(nodes)
	if count != 3 {
		t.Errorf("expected 3 tasks, got %d", count)
	}
}

// TestHideDoneEpicsInActiveView verifies done epics are hidden in active view.
func TestHideDoneEpicsInActiveView(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"E1": {ID: "E1", IsEpic: true, Title: "My Epic"},
			"T1": {ID: "T1", EpicID: "E1", State: stateDone},
			"T2": {ID: "T2", EpicID: "E1", State: stateDone},
		},
		Deps: map[string]map[string]struct{}{},
	}

	var buf bytes.Buffer
	roots := buildListRoots(graph, false, false, "")
	renderTreeView(&buf, roots, graph, "/repo", false)

	output := buf.String()
	if strings.Contains(output, "E1") {
		t.Errorf("expected done epic to be hidden in active view, got: %s", output)
	}
}

func TestRenderTreeRootRowsNoConnectors(t *testing.T) {
	graph := &Graph{
		Tasks: map[string]*Task{
			"E1": {ID: "E1", IsEpic: true, Title: "Epic"},
			"T1": {ID: "T1", EpicID: "E1", State: stateTodo, Title: "Child 1"},
			"T2": {ID: "T2", EpicID: "E1", State: stateDone, Title: "Child 2"},
			"O1": {ID: "O1", State: stateTodo, Title: "Orphan"},
		},
		Deps: map[string]map[string]struct{}{},
	}

	var buf bytes.Buffer
	roots := buildListRoots(graph, false, false, "")
	renderTreeView(&buf, roots, graph, "/repo", false)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	findLine := func(needle string) string {
		for _, line := range lines {
			if strings.Contains(line, needle) {
				return line
			}
		}
		return ""
	}

	orphanLine := findLine("O1")
	if orphanLine == "" {
		t.Fatalf("expected root task line for O1")
	}
	if strings.HasPrefix(orphanLine, "├") || strings.HasPrefix(orphanLine, "└") || strings.HasPrefix(orphanLine, "│") {
		t.Errorf("expected root task line to avoid connectors, got: %q", orphanLine)
	}

	epicLine := findLine("E1")
	if epicLine == "" {
		t.Fatalf("expected epic line for E1")
	}
	if strings.HasPrefix(epicLine, "├") || strings.HasPrefix(epicLine, "└") || strings.HasPrefix(epicLine, "│") {
		t.Errorf("expected root epic line to avoid connectors, got: %q", epicLine)
	}

	childLine := findLine("T1")
	if childLine == "" {
		t.Fatalf("expected child task line for T1")
	}
	if !(strings.HasPrefix(childLine, "├") || strings.HasPrefix(childLine, "└")) {
		t.Errorf("expected child task line to start with connector, got: %q", childLine)
	}
}

// TestFilterNodesByReady verifies that --ready filtering works correctly.
func TestFilterNodesByReady(t *testing.T) {
	// Create a graph with various task states
	graph := &Graph{
		Tasks: map[string]*Task{
			"T1": {ID: "T1", State: stateTodo, ClaimedBy: ""},        // ready
			"T2": {ID: "T2", State: stateDone, ClaimedBy: ""},        // done, not ready
			"T3": {ID: "T3", State: stateCanceled, ClaimedBy: ""},    // canceled, not ready
			"T4": {ID: "T4", State: stateDoing, ClaimedBy: "agent1"}, // doing, not ready
			"T5": {ID: "T5", State: stateTodo, ClaimedBy: ""},        // blocked by T6
			"T6": {ID: "T6", State: stateTodo, ClaimedBy: ""},        // ready
			"T7": {ID: "T7", State: stateBlocked, ClaimedBy: ""},     // explicitly blocked
		},
		Deps: map[string]map[string]struct{}{
			"T5": {"T6": {}}, // T5 depends on T6
		},
		RDeps: map[string]map[string]struct{}{},
	}

		t.Run("readyOnly filters out non-ready tasks", func(t *testing.T) {
			nodes := []*treeNode{
				{task: graph.Tasks["T1"]}, // ready
			{task: graph.Tasks["T2"]}, // done, not ready
			{task: graph.Tasks["T3"]}, // canceled, not ready
			{task: graph.Tasks["T4"]}, // doing, not ready
			{task: graph.Tasks["T5"]}, // blocked by T6, not ready
			{task: graph.Tasks["T6"]}, // ready
		}

			filtered := filterNodesByReady(nodes, graph)

		// Should only have T1 and T6 (both ready)
		if len(filtered) != 2 {
			t.Fatalf("expected 2 ready tasks, got %d", len(filtered))
		}
		ids := []string{filtered[0].task.ID, filtered[1].task.ID}
		if ids[0] != "T1" || ids[1] != "T6" {
			t.Errorf("expected T1 and T6, got %v", ids)
		}
	})

		t.Run("readyOnly excludes epics with no ready children", func(t *testing.T) {
			nodes := []*treeNode{
				{
				task: &Task{ID: "E1", IsEpic: true},
				children: []*treeNode{
					{task: graph.Tasks["T2"]}, // done, not ready
					{task: graph.Tasks["T3"]}, // canceled, not ready
				},
			},
			{
				task: &Task{ID: "E2", IsEpic: true},
				children: []*treeNode{
					{task: graph.Tasks["T1"]}, // ready
					{task: graph.Tasks["T2"]}, // done, not ready
				},
			},
		}

			filtered := filterNodesByReady(nodes, graph)

		// Should only have E2 (has ready child T1), not E1
		if len(filtered) != 1 {
			t.Fatalf("expected 1 epic with ready children, got %d", len(filtered))
		}
		if filtered[0].task.ID != "E2" {
			t.Errorf("expected E2, got %s", filtered[0].task.ID)
		}
		// E2 should have only T1 as child (T2 filtered out)
		if len(filtered[0].children) != 1 {
			t.Errorf("expected 1 child in E2, got %d", len(filtered[0].children))
		}
		if filtered[0].children[0].task.ID != "T1" {
			t.Errorf("expected T1 as child, got %s", filtered[0].children[0].task.ID)
		}
	})

	}

// TestFormatTreeLineTruncation verifies that long lines are truncated to prevent wrapping.
func TestFormatTreeLineTruncation(t *testing.T) {
	task := &Task{
		ID:        "ABC123",
		Title:     "Test task",
		State:     stateDoing,
		ClaimedBy: "very-long-username@very-long-hostname.example.com",
	}

	// With a narrow terminal width, the annotation should be truncated
	termWidth := 60
	line := formatTreeLine(
		"",                             // prefix
		"├",                            // connector
		true,                           // showConnector
		"◐",                            // icon
		task.ID,                        // id
		"Test task",                    // title
		[]string{"@" + task.ClaimedBy}, // annotations
		"",                             // blockerAnnotation
		task,                           // task
		true,                           // isReady
		false,                          // useColor
		termWidth,                      // termWidth
	)

	// Line visible length should not exceed terminal width
	lineVisLen := visibleLen(line)
	if lineVisLen > termWidth {
		t.Errorf("visible line length %d exceeds terminal width %d: %q", lineVisLen, termWidth, line)
	}

	// The line should contain the ID
	if !strings.Contains(line, "ABC123") {
		t.Errorf("line should contain task ID, got: %q", line)
	}

	// The annotation should be truncated (contains ellipsis)
	if !strings.Contains(line, "…") {
		t.Errorf("expected truncation ellipsis in line: %q", line)
	}
}
