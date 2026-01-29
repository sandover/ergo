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
	t.Run("hides canceled tasks", func(t *testing.T) {
		nodes := []*treeNode{
			{task: &Task{ID: "T1", State: stateTodo}},
			{task: &Task{ID: "T2", State: stateCanceled}},
			{task: &Task{ID: "T3", State: stateDone}},
		}

		filtered := filterAndCollapseNodes(nodes)

		if len(filtered) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(filtered))
		}
		// T1 and T3 should remain
		ids := []string{filtered[0].task.ID, filtered[1].task.ID}
		if ids[0] != "T1" || ids[1] != "T3" {
			t.Errorf("expected T1, T3; got %v", ids)
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

	t.Run("collapses fully-done epics", func(t *testing.T) {
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

		if len(filtered) != 1 {
			t.Fatalf("expected 1 node, got %d", len(filtered))
		}
		epic := filtered[0]
		if !epic.collapsed {
			t.Error("expected epic to be collapsed")
		}
		if epic.collapsedCount != 3 {
			t.Errorf("expected 3 tasks in collapsed count, got %d", epic.collapsedCount)
		}
		if len(epic.children) != 0 {
			t.Errorf("expected children cleared, got %d", len(epic.children))
		}
	})

	t.Run("active epics show filtered children", func(t *testing.T) {
		nodes := []*treeNode{
			{
				task: &Task{ID: "E1", IsEpic: true},
				children: []*treeNode{
					{task: &Task{ID: "T1", State: stateDone}},
					{task: &Task{ID: "T2", State: stateTodo}},     // active
					{task: &Task{ID: "T3", State: stateCanceled}}, // should be hidden
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
		// T1 and T2 should remain, T3 (canceled) should be filtered
		if len(epic.children) != 2 {
			t.Errorf("expected 2 children, got %d", len(epic.children))
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

// TestRenderCollapsedEpic verifies the collapsed epic rendering format.
func TestRenderCollapsedEpic(t *testing.T) {
	// Create an epic with 5 done tasks
	graph := &Graph{
		Tasks: map[string]*Task{
			"E1": {ID: "E1", IsEpic: true, Title: "My Epic"},
			"T1": {ID: "T1", EpicID: "E1", State: stateDone},
			"T2": {ID: "T2", EpicID: "E1", State: stateDone},
			"T3": {ID: "T3", EpicID: "E1", State: stateDone},
			"T4": {ID: "T4", EpicID: "E1", State: stateDone},
			"T5": {ID: "T5", EpicID: "E1", State: stateDone},
		},
		Deps: map[string]map[string]struct{}{},
	}

	var buf bytes.Buffer
	// showAll=false triggers collapsing of done epics
	renderTreeView(&buf, graph, "/repo", false, false)

	output := buf.String()
	// Should show the checkmark icon and task count
	if !strings.Contains(output, "✓") {
		t.Error("expected checkmark icon in collapsed epic")
	}
	if !strings.Contains(output, "[5 tasks]") {
		t.Errorf("expected '[5 tasks]' in output, got: %s", output)
	}
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
		"◐",                            // icon
		task.ID,                        // id
		"Test task",                    // title
		"",                             // workerIndicator
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
