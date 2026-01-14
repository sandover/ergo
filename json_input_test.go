package main

import (
	"testing"
)

func ptr(s string) *string {
	return &s
}

func TestTaskInput_ValidateForNewTask(t *testing.T) {
	tests := []struct {
		name        string
		input       TaskInput
		expectError bool
		missing     []string
		invalidKeys []string
	}{
		{
			name:        "valid minimal",
			input:       TaskInput{Title: ptr("Do something")},
			expectError: false,
		},
		{
			name:        "valid with all fields",
			input:       TaskInput{Title: ptr("Do X"), Epic: ptr("E1"), Worker: ptr("agent")},
			expectError: false,
		},
		{
			name:        "missing title",
			input:       TaskInput{},
			expectError: true,
			missing:     []string{"title"},
		},
		{
			name:        "empty title",
			input:       TaskInput{Title: ptr("")},
			expectError: true,
			missing:     []string{"title"},
		},
		{
			name:        "whitespace title",
			input:       TaskInput{Title: ptr("   ")},
			expectError: true,
			missing:     []string{"title"},
		},
		{
			name:        "invalid worker",
			input:       TaskInput{Title: ptr("X"), Worker: ptr("robot")},
			expectError: true,
			invalidKeys: []string{"worker"},
		},
		{
			name:        "invalid state",
			input:       TaskInput{Title: ptr("X"), State: ptr("running")},
			expectError: true,
			invalidKeys: []string{"state"},
		},
		{
			name:        "state=doing without claim",
			input:       TaskInput{Title: ptr("X"), State: ptr("doing")},
			expectError: true,
			invalidKeys: []string{"claim"},
		},
		{
			name:        "state=doing with claim",
			input:       TaskInput{Title: ptr("X"), State: ptr("doing"), Claim: ptr("agent-1")},
			expectError: false,
		},
		{
			name:        "state=error without claim",
			input:       TaskInput{Title: ptr("X"), State: ptr("error")},
			expectError: true,
			invalidKeys: []string{"claim"},
		},
		{
			name:        "result_path without result_summary",
			input:       TaskInput{Title: ptr("X"), ResultPath: ptr("docs/x.md")},
			expectError: true,
			invalidKeys: []string{"result_summary"},
		},
		{
			name:        "result_summary without result_path",
			input:       TaskInput{Title: ptr("X"), ResultSummary: ptr("Summary")},
			expectError: true,
			invalidKeys: []string{"result_path"},
		},
		{
			name:        "result_path and result_summary together",
			input:       TaskInput{Title: ptr("X"), ResultPath: ptr("docs/x.md"), ResultSummary: ptr("Summary")},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := tt.input.ValidateForNewTask()

			if tt.expectError {
				if verr == nil {
					t.Error("expected validation error, got nil")
					return
				}
				// Check missing fields
				for _, m := range tt.missing {
					found := false
					for _, got := range verr.Missing {
						if got == m {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected missing field %q, got missing=%v", m, verr.Missing)
					}
				}
				// Check invalid keys
				for _, k := range tt.invalidKeys {
					if _, ok := verr.Invalid[k]; !ok {
						t.Errorf("expected invalid field %q, got invalid=%v", k, verr.Invalid)
					}
				}
			} else {
				if verr != nil {
					t.Errorf("expected no error, got %v", verr)
				}
			}
		})
	}
}

func TestTaskInput_ValidateForNewEpic(t *testing.T) {
	tests := []struct {
		name        string
		input       TaskInput
		expectError bool
		invalidKeys []string
	}{
		{
			name:        "valid minimal",
			input:       TaskInput{Title: ptr("Auth system")},
			expectError: false,
		},
		{
			name:        "epic cannot have epic",
			input:       TaskInput{Title: ptr("X"), Epic: ptr("E1")},
			expectError: true,
			invalidKeys: []string{"epic"},
		},
		{
			name:        "epic cannot have worker",
			input:       TaskInput{Title: ptr("X"), Worker: ptr("agent")},
			expectError: true,
			invalidKeys: []string{"worker"},
		},
		{
			name:        "epic cannot have state",
			input:       TaskInput{Title: ptr("X"), State: ptr("todo")},
			expectError: true,
			invalidKeys: []string{"state"},
		},
		{
			name:        "epic cannot have claim",
			input:       TaskInput{Title: ptr("X"), Claim: ptr("agent-1")},
			expectError: true,
			invalidKeys: []string{"claim"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := tt.input.ValidateForNewEpic()

			if tt.expectError {
				if verr == nil {
					t.Error("expected validation error, got nil")
					return
				}
				for _, k := range tt.invalidKeys {
					if _, ok := verr.Invalid[k]; !ok {
						t.Errorf("expected invalid field %q, got invalid=%v", k, verr.Invalid)
					}
				}
			} else {
				if verr != nil {
					t.Errorf("expected no error, got %v", verr)
				}
			}
		})
	}
}

func TestTaskInput_ValidateForSet(t *testing.T) {
	tests := []struct {
		name        string
		input       TaskInput
		expectError bool
		invalidKeys []string
	}{
		{
			name:        "empty is valid (no changes)",
			input:       TaskInput{},
			expectError: false,
		},
		{
			name:        "title update",
			input:       TaskInput{Title: ptr("New title")},
			expectError: false,
		},
		{
			name:        "empty title rejected",
			input:       TaskInput{Title: ptr("")},
			expectError: true,
			invalidKeys: []string{"title"},
		},
		{
			name:        "state change",
			input:       TaskInput{State: ptr("done")},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := tt.input.ValidateForSet()

			if tt.expectError {
				if verr == nil {
					t.Error("expected validation error, got nil")
					return
				}
				for _, k := range tt.invalidKeys {
					if _, ok := verr.Invalid[k]; !ok {
						t.Errorf("expected invalid field %q, got invalid=%v", k, verr.Invalid)
					}
				}
			} else {
				if verr != nil {
					t.Errorf("expected no error, got %v", verr)
				}
			}
		})
	}
}

func TestTaskInput_ToKeyValueMap(t *testing.T) {
	input := TaskInput{
		Title:         ptr("My Task"),
		Body:          ptr("Description"),
		Epic:          ptr("E1"),
		Worker:        ptr("agent"),
		State:         ptr("doing"),
		Claim:         ptr("agent-1"),
		ResultPath:    ptr("docs/x.md"),
		ResultSummary: ptr("Summary"),
	}

	m := input.ToKeyValueMap()

	expected := map[string]string{
		"title":          "My Task",
		"body":           "Description",
		"epic":           "E1",
		"worker":         "agent",
		"state":          "doing",
		"claim":          "agent-1",
		"result.path":    "docs/x.md",
		"result.summary": "Summary",
	}

	if len(m) != len(expected) {
		t.Errorf("expected %d keys, got %d", len(expected), len(m))
	}

	for k, v := range expected {
		if m[k] != v {
			t.Errorf("expected %s=%q, got %q", k, v, m[k])
		}
	}
}

func TestTaskInput_ToKeyValueMap_OnlySetFields(t *testing.T) {
	// Only title is set
	input := TaskInput{Title: ptr("Just title")}
	m := input.ToKeyValueMap()

	if len(m) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(m), m)
	}
	if m["title"] != "Just title" {
		t.Errorf("expected title='Just title', got %q", m["title"])
	}
}
