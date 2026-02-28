// Purpose: Validate `ergo plan` JSON parsing and semantic validation behavior.
// Exports: none.
// Role: Unit tests for plan-input contract correctness and error classification.
// Invariants: Parse failures return parse_error; semantic failures return validation_failed.
// Notes: Uses stdin helpers to mirror real CLI input handling.
package ergo

import (
	"strings"
	"testing"
)

func TestParsePlanInput_RejectsUnknownField_WithSuggestion(t *testing.T) {
	restoreStdin := setStdin(t, `{"title":"Epic","tasks":[{"title":"A"}],"boddy":"oops"}`)
	defer restoreStdin()

	_, err := ParsePlanInput()
	if err == nil {
		t.Fatal("expected parse error for unknown field, got nil")
	}
	if err.Error != "parse_error" {
		t.Fatalf("expected parse_error, got %q", err.Error)
	}
	if !strings.Contains(err.Message, "did you mean: body") {
		t.Fatalf("expected suggestion in message, got %q", err.Message)
	}
	if err.Invalid["boddy"] == "" {
		t.Fatalf("expected invalid map entry for boddy, got %v", err.Invalid)
	}
}

func TestParsePlanInput_RejectsUnknownNestedField(t *testing.T) {
	restoreStdin := setStdin(t, `{"title":"Epic","tasks":[{"title":"A","aftre":["B"]}]}`)
	defer restoreStdin()

	_, err := ParsePlanInput()
	if err == nil {
		t.Fatal("expected parse error for unknown nested field, got nil")
	}
	if err.Error != "parse_error" {
		t.Fatalf("expected parse_error, got %q", err.Error)
	}
	if !strings.Contains(err.Message, "unknown field") {
		t.Fatalf("expected unknown field parse error, got %q", err.Message)
	}
}

func TestParsePlanInput_UnknownTopLevelDoesNotSuggestNestedField(t *testing.T) {
	restoreStdin := setStdin(t, `{"title":"Epic","tasks":[{"title":"A"}],"aftr":"x"}`)
	defer restoreStdin()

	_, err := ParsePlanInput()
	if err == nil {
		t.Fatal("expected parse error for unknown field, got nil")
	}
	if err.Error != "parse_error" {
		t.Fatalf("expected parse_error, got %q", err.Error)
	}
	if strings.Contains(err.Message, "did you mean: after") {
		t.Fatalf("expected no suggestion for nested-only field, got %q", err.Message)
	}
}

func TestParsePlanInput_RejectsMultipleJSONValues(t *testing.T) {
	restoreStdin := setStdin(t, `{"title":"Epic","tasks":[{"title":"A"}]}{"title":"Extra"}`)
	defer restoreStdin()

	_, err := ParsePlanInput()
	if err == nil {
		t.Fatal("expected parse error for multiple JSON values, got nil")
	}
	if err.Error != "parse_error" {
		t.Fatalf("expected parse_error, got %q", err.Error)
	}
	if !strings.Contains(err.Message, "multiple JSON values") {
		t.Fatalf("expected multiple JSON values error, got %q", err.Message)
	}
}

func TestPlanInputValidate_ValidPayload(t *testing.T) {
	title := "Epic"
	taskA := "Task A"
	taskB := "Task B"
	taskC := "Task C"
	input := &PlanInput{
		Title: &title,
		Tasks: []PlanTaskInput{
			{Title: &taskA},
			{Title: &taskB, After: []string{"Task A"}},
			{Title: &taskC, After: []string{"Task A", "Task B"}},
		},
	}

	if err := input.Validate(); err != nil {
		t.Fatalf("expected valid payload, got error: %+v", err)
	}
}

func TestPlanInputValidate_MissingRequiredFields(t *testing.T) {
	input := &PlanInput{}
	err := input.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if err.Error != "validation_failed" {
		t.Fatalf("expected validation_failed, got %q", err.Error)
	}
	if err.Message != "missing required fields" {
		t.Fatalf("expected missing required fields message, got %q", err.Message)
	}
	if len(err.Missing) != 2 {
		t.Fatalf("expected 2 missing fields, got %d (%v)", len(err.Missing), err.Missing)
	}
}

func TestPlanInputValidate_DuplicateTaskTitle(t *testing.T) {
	title := "Epic"
	task := "Task A"
	input := &PlanInput{
		Title: &title,
		Tasks: []PlanTaskInput{
			{Title: &task},
			{Title: &task},
		},
	}

	err := input.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if err.Invalid["tasks[1].title"] == "" {
		t.Fatalf("expected duplicate-title error, got %v", err.Invalid)
	}
}

func TestPlanInputValidate_DanglingAfterReference(t *testing.T) {
	title := "Epic"
	taskA := "Task A"
	taskB := "Task B"
	input := &PlanInput{
		Title: &title,
		Tasks: []PlanTaskInput{
			{Title: &taskA},
			{Title: &taskB, After: []string{"Task Missing"}},
		},
	}

	err := input.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Invalid["tasks[1].after[0]"], "unknown task title") {
		t.Fatalf("expected dangling reference error, got %v", err.Invalid)
	}
}

func TestPlanInputValidate_CycleInAfterGraph(t *testing.T) {
	title := "Epic"
	taskA := "Task A"
	taskB := "Task B"
	input := &PlanInput{
		Title: &title,
		Tasks: []PlanTaskInput{
			{Title: &taskA, After: []string{"Task B"}},
			{Title: &taskB, After: []string{"Task A"}},
		},
	}

	err := input.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Invalid["tasks"], "cycle") {
		t.Fatalf("expected cycle error, got %v", err.Invalid)
	}
}

func TestPlanInputValidate_SelfDependencyRejected(t *testing.T) {
	title := "Epic"
	taskA := "Task A"
	input := &PlanInput{
		Title: &title,
		Tasks: []PlanTaskInput{
			{Title: &taskA, After: []string{"Task A"}},
		},
	}

	err := input.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Invalid["tasks[0].after[0]"], "cannot depend on self") {
		t.Fatalf("expected self-dependency error, got %v", err.Invalid)
	}
}
