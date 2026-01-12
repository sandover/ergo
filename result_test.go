package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateResultSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"valid short", "Fix complete", false},
		{"valid with spaces", "  Fix complete  ", false}, // trimmed before storing
		{"max length", strings.Repeat("a", 120), false},
		{"too long", strings.Repeat("a", 121), true},
		{"newline not allowed", "Line1\nLine2", true},
		{"carriage return not allowed", "Line1\rLine2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResultSummary(tt.summary)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResultSummary(%q) error = %v, wantErr %v", tt.summary, err, tt.wantErr)
			}
		})
	}
}

func TestValidateResultPath(t *testing.T) {
	repoDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(repoDir, "docs", "report.md")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create .ergo directory
	ergoDir := filepath.Join(repoDir, ".ergo")
	if err := os.MkdirAll(ergoDir, 0755); err != nil {
		t.Fatalf("failed to create .ergo dir: %v", err)
	}
	ergoFile := filepath.Join(ergoDir, "internal.txt")
	if err := os.WriteFile(ergoFile, []byte("internal"), 0644); err != nil {
		t.Fatalf("failed to create internal file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid relative path", "docs/report.md", false},
		{"absolute path rejected", "/docs/report.md", true},
		{"parent traversal rejected", "../outside/file.txt", true},
		{"hidden traversal rejected", "docs/../../../etc/passwd", true},
		{"file not found", "docs/missing.md", true},
		{"directory rejected", "docs", true},
		{".ergo path rejected", ".ergo/internal.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateResultPath(repoDir, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResultPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestCaptureResultEvidence(t *testing.T) {
	repoDir := t.TempDir()

	// Create test file with known content
	content := "Hello, World!"
	testFile := filepath.Join(repoDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	evidence, err := captureResultEvidence(repoDir, "test.txt")
	if err != nil {
		t.Fatalf("captureResultEvidence failed: %v", err)
	}

	// SHA256 of "Hello, World!" is known
	expectedSha256 := "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
	if evidence.Sha256AtAttach != expectedSha256 {
		t.Errorf("sha256 = %q, want %q", evidence.Sha256AtAttach, expectedSha256)
	}

	if evidence.MtimeAtAttach == "" {
		t.Error("mtime_at_attach should not be empty")
	}

	// GitCommitAtAttach is optional (may be empty if not a git repo)
}

func TestGetGitHead(t *testing.T) {
	// Test with non-git directory
	tmpDir := t.TempDir()
	result := getGitHead(tmpDir)
	if result != "" {
		t.Errorf("expected empty string for non-git dir, got %q", result)
	}

	// We don't test with a real git repo since that would require git setup
}

func TestDeriveFileURL(t *testing.T) {
	tests := []struct {
		relPath string
		repoDir string
		want    string
	}{
		{"docs/report.md", "/home/user/project", "file:///home/user/project/docs/report.md"},
		{"file.txt", "/tmp", "file:///tmp/file.txt"},
		{"path with spaces/file.txt", "/home/user", "file:///home/user/path%20with%20spaces/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			got := deriveFileURL(tt.relPath, tt.repoDir)
			if got != tt.want {
				t.Errorf("deriveFileURL(%q, %q) = %q, want %q", tt.relPath, tt.repoDir, got, tt.want)
			}
		})
	}
}

func TestResultEventReplay(t *testing.T) {
	// Create events including a result
	now := time.Now().UTC()
	nowStr := formatTime(now)

	createEvent, _ := newEvent("new_task", now, NewTaskEvent{
		ID:        "T1",
		UUID:      "uuid-1",
		EpicID:    "",
		Body:      "Test task",
		State:     stateTodo,
		Worker:    "any",
		CreatedAt: nowStr,
	})
	result1Event, _ := newEvent("result", now.Add(time.Hour), ResultEvent{
		TaskID:         "T1",
		Summary:        "First result",
		Path:           "docs/result1.md",
		Sha256AtAttach: "abc123",
		TS:             formatTime(now.Add(time.Hour)),
	})
	result2Event, _ := newEvent("result", now.Add(2*time.Hour), ResultEvent{
		TaskID:         "T1",
		Summary:        "Second result",
		Path:           "docs/result2.md",
		Sha256AtAttach: "def456",
		TS:             formatTime(now.Add(2 * time.Hour)),
	})

	events := []Event{createEvent, result1Event, result2Event}

	graph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	task := graph.Tasks["T1"]
	if task == nil {
		t.Fatal("task T1 not found")
	}

	// Results should be in reverse chronological order (newest first)
	if len(task.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(task.Results))
	}
	if task.Results[0].Summary != "Second result" {
		t.Errorf("first result should be newest: got %q", task.Results[0].Summary)
	}
	if task.Results[0].Path != "docs/result2.md" {
		t.Errorf("first result path = %q, want %q", task.Results[0].Path, "docs/result2.md")
	}
	if task.Results[1].Summary != "First result" {
		t.Errorf("second result should be oldest: got %q", task.Results[1].Summary)
	}
}

func TestResultCompaction(t *testing.T) {
	// Create graph with results
	now := time.Now().UTC()
	graph := &Graph{
		Tasks: map[string]*Task{
			"T1": {
				ID:        "T1",
				UUID:      "uuid-1",
				State:     stateDone,
				Body:      "Test task",
				Worker:    workerAny,
				CreatedAt: now,
				UpdatedAt: now.Add(time.Hour),
				Results: []Result{
					{
						Summary:        "Second result",
						Path:           "docs/result2.md",
						Sha256AtAttach: "def456",
						CreatedAt:      now.Add(time.Hour),
					},
					{
						Summary:        "First result",
						Path:           "docs/result1.md",
						Sha256AtAttach: "abc123",
						CreatedAt:      now.Add(30 * time.Minute),
					},
				},
			},
		},
		Deps:  map[string]map[string]struct{}{},
		RDeps: map[string]map[string]struct{}{},
		Meta: map[string]*TaskMeta{
			"T1": {
				CreatedBody:  "Test task",
				CreatedState: stateTodo,
				CreatedAt:    now,
			},
		},
	}

	// Compact and replay
	events, err := compactEvents(graph)
	if err != nil {
		t.Fatalf("compactEvents failed: %v", err)
	}

	replayedGraph, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents failed: %v", err)
	}

	task := replayedGraph.Tasks["T1"]
	if task == nil {
		t.Fatal("task T1 not found after replay")
	}

	// Should preserve both results in correct order
	if len(task.Results) != 2 {
		t.Fatalf("expected 2 results after compaction, got %d", len(task.Results))
	}
	if task.Results[0].Summary != "Second result" {
		t.Errorf("first result should be 'Second result', got %q", task.Results[0].Summary)
	}
	if task.Results[1].Summary != "First result" {
		t.Errorf("second result should be 'First result', got %q", task.Results[1].Summary)
	}
}
