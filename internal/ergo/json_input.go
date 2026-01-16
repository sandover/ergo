// JSON input parsing and validation for task/epic creation and updates.
//
// Agents pipe JSON to stdin for all mutations:
//
//	echo '{"title":"Do X"}' | ergo new task
//	echo '{"state":"done"}' | ergo set T-xyz
//
// This design:
//   - Eliminates shell escaping issues (no inline JSON in args)
//   - Uses one schema for both new and set (only `title` requirement differs)
//   - Returns structured validation errors for agent consumption
package ergo

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// TaskInput is the unified JSON schema for creating and updating tasks/epics.
//
// For `new task` / `new epic`: title and body are required.
// For `set`: all fields are optional; provided fields override existing values.
//
// Validation rules (apply to both new and set):
//   - title: cannot be empty if provided
//   - body: cannot be empty if provided
//   - state=doing requires claim
//   - state=error requires claim
//   - result_path and result_summary must be provided together
type TaskInput struct {
	Title  *string `json:"title,omitempty"`  // required for new; optional for set
	Body   *string `json:"body,omitempty"`   // required for new; optional for set
	Epic   *string `json:"epic,omitempty"`   // epic ID or "" to unassign
	Worker *string `json:"worker,omitempty"` // any|agent|human
	State  *string `json:"state,omitempty"`  // todo|doing|done|blocked|canceled|error
	Claim  *string `json:"claim,omitempty"`  // agent ID or "" to unclaim

	// Result attachment (both required together)
	ResultPath    *string `json:"result_path,omitempty"`    // path to result file
	ResultSummary *string `json:"result_summary,omitempty"` // one-line summary
}

// ValidationError is a structured error for JSON input validation.
// Returned as JSON to stdout when validation fails (with exit code 1).
type ValidationError struct {
	Error   string            `json:"error"`             // "validation_failed" or "parse_error"
	Message string            `json:"message"`           // human-readable summary
	Missing []string          `json:"missing,omitempty"` // list of missing required fields
	Invalid map[string]string `json:"invalid,omitempty"` // field -> reason map
}

func (e *ValidationError) GoError() error {
	// Build a detailed error message that includes missing/invalid field info
	parts := []string{e.Message}

	if len(e.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing required: %s", strings.Join(e.Missing, ", ")))
	}

	for field, reason := range e.Invalid {
		parts = append(parts, fmt.Sprintf("%s: %s", field, reason))
	}

	return fmt.Errorf("%s", strings.Join(parts, "; "))
}

// WriteJSON writes the validation error as JSON to the given writer.
func (e *ValidationError) WriteJSON(w io.Writer) error {
	return writeJSON(w, e)
}

// readJSONFromStdin reads and parses JSON from stdin.
// Returns nil if stdin is a terminal (no piped input).
func readJSONFromStdin() ([]byte, error) {
	if !stdinIsPiped() {
		return nil, nil
	}
	return io.ReadAll(os.Stdin)
}

// ParseTaskInput reads JSON from stdin and parses it into TaskInput.
// Returns an error if stdin is empty or JSON is malformed.
func ParseTaskInput() (*TaskInput, *ValidationError) {
	jsonBytes, err := readJSONFromStdin()
	if err != nil {
		return nil, &ValidationError{
			Error:   "io_error",
			Message: fmt.Sprintf("failed to read stdin: %v", err),
		}
	}
	if jsonBytes == nil || len(jsonBytes) == 0 {
		return nil, &ValidationError{
			Error:   "parse_error",
			Message: "no input: pipe JSON to stdin",
		}
	}

	var input TaskInput
	if err := json.Unmarshal(jsonBytes, &input); err != nil {
		return nil, &ValidationError{
			Error:   "parse_error",
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}
	}

	return &input, nil
}

// ValidateForNewTask validates TaskInput for new task creation.
func (t *TaskInput) ValidateForNewTask() *ValidationError {
	return t.validate(true, false)
}

// ValidateForNewEpic validates TaskInput for new epic creation.
func (t *TaskInput) ValidateForNewEpic() *ValidationError {
	return t.validate(true, true)
}

// ValidateForSet validates TaskInput for updating an existing task/epic.
func (t *TaskInput) ValidateForSet() *ValidationError {
	return t.validate(false, false)
}

func (t *TaskInput) validate(requireTitle bool, isEpic bool) *ValidationError {
	var missing []string
	invalid := make(map[string]string)

	// Title validation - required for new tasks/epics
	hasTitle := t.Title != nil && strings.TrimSpace(*t.Title) != ""
	if requireTitle {
		if !hasTitle {
			missing = append(missing, "title")
		}
	} else if t.Title != nil && strings.TrimSpace(*t.Title) == "" {
		invalid["title"] = "cannot be empty"
	}

	// Body validation - optional if title provided (title becomes the body)
	if t.Body != nil && strings.TrimSpace(*t.Body) == "" {
		invalid["body"] = "cannot be empty"
	}

	// Worker validation
	if t.Worker != nil {
		if _, err := ParseWorker(*t.Worker); err != nil {
			invalid["worker"] = fmt.Sprintf("invalid value %q, expected: any, agent, human", *t.Worker)
		}
	}

	// State validation
	if t.State != nil {
		if _, ok := validStates[*t.State]; !ok {
			invalid["state"] = fmt.Sprintf("invalid value %q, expected: todo, doing, done, blocked, canceled, error", *t.State)
		}
	}

	// Claim invariants (doing/error require claim)
	if t.State != nil {
		state := *t.State
		hasClaim := t.Claim != nil && *t.Claim != ""
		if (state == stateDoing || state == stateError) && !hasClaim {
			invalid["claim"] = fmt.Sprintf("required when state=%s", state)
		}
	}

	// Result path/summary must be together
	hasPath := t.ResultPath != nil
	hasSummary := t.ResultSummary != nil
	if hasPath && !hasSummary {
		invalid["result_summary"] = "required when result_path is provided"
	}
	if hasSummary && !hasPath {
		invalid["result_path"] = "required when result_summary is provided"
	}

	// Epic-only restrictions
	if isEpic {
		if t.Epic != nil {
			invalid["epic"] = "epics cannot be assigned to other epics"
		}
		if t.Worker != nil {
			invalid["worker"] = "epics do not have workers"
		}
		if t.State != nil {
			invalid["state"] = "epics do not have state (use epic-deps)"
		}
		if t.Claim != nil {
			invalid["claim"] = "epics cannot be claimed"
		}
	}

	if len(missing) > 0 || len(invalid) > 0 {
		return &ValidationError{
			Error:   "validation_failed",
			Message: "invalid input",
			Missing: missing,
			Invalid: invalid,
		}
	}
	return nil
}

// ToKeyValueMap converts TaskInput to a map compatible with existing set logic.
// Only includes fields that were explicitly provided (non-nil).
func (t *TaskInput) ToKeyValueMap() map[string]string {
	m := make(map[string]string)

	if t.Title != nil {
		m["title"] = *t.Title
	}
	if t.Body != nil {
		m["body"] = *t.Body
	}
	if t.Epic != nil {
		m["epic"] = *t.Epic
	}
	if t.Worker != nil {
		m["worker"] = *t.Worker
	}
	if t.State != nil {
		m["state"] = *t.State
	}
	if t.Claim != nil {
		m["claim"] = *t.Claim
	}
	if t.ResultPath != nil {
		m["result.path"] = *t.ResultPath
	}
	if t.ResultSummary != nil {
		m["result.summary"] = *t.ResultSummary
	}

	return m
}

// GetTitle returns the title or empty string if not set.
func (t *TaskInput) GetTitle() string {
	if t.Title != nil {
		return *t.Title
	}
	return ""
}

// GetBody returns body or empty string if not set.
func (t *TaskInput) GetBody() string {
	if t.Body != nil {
		return *t.Body
	}
	return ""
}

// GetEpic returns epic ID or empty string.
func (t *TaskInput) GetEpic() string {
	if t.Epic != nil {
		return *t.Epic
	}
	return ""
}

// GetWorker returns worker or workerAny if not set.
func (t *TaskInput) GetWorker() Worker {
	if t.Worker != nil {
		w, _ := ParseWorker(*t.Worker) // already validated
		return w
	}
	return workerAny
}

// GetFullBody returns the combined title + body for storage.
// The internal model stores everything in Body, with title as first line.
func (t *TaskInput) GetFullBody() string {
	title := t.GetTitle()
	body := t.GetBody()
	if body == "" {
		return title
	}
	return title + "\n" + body
}
