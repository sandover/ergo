// Purpose: Parse and validate JSON stdin for task/epic mutations.
// Exports: TaskInput, ValidationError, ParseTaskInput.
// Role: Input validation layer for mutation commands.
// Invariants: Unknown keys are rejected; required fields enforced by mode.
// Notes: Returns structured errors for agent consumption.
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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

var knownJSONFields = []string{
	"title",
	"body",
	"epic",
	"state",
	"claim",
	"result_path",
	"result_summary",
}

// TaskInput is the unified JSON schema for creating and updating tasks/epics.
//
// For `new task` / `new epic`: title is required; body is optional (details).
// For `set`: all fields are optional; provided fields override existing values.
//
// Validation rules (apply to both new and set):
//   - title: cannot be empty if provided
//   - body: cannot be empty if provided
//   - state=doing requires claim
//   - state=error requires claim
//   - result_path and result_summary must be provided together
type TaskInput struct {
	Title *string `json:"title,omitempty"` // required for new; optional for set
	Body  *string `json:"body,omitempty"`  // optional details (cannot be empty if provided)
	Epic  *string `json:"epic,omitempty"`  // epic ID or "" to unassign
	State *string `json:"state,omitempty"` // todo|doing|done|blocked|canceled|error
	Claim *string `json:"claim,omitempty"` // agent ID or "" to unclaim

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
// Returns an error if stdin is empty, JSON is malformed, or unknown keys appear.
func ParseTaskInput() (*TaskInput, *ValidationError) {
	jsonBytes, err := readJSONFromStdin()
	if err != nil {
		return nil, &ValidationError{
			Error:   "io_error",
			Message: fmt.Sprintf("failed to read stdin: %v", err),
		}
	}
	if len(jsonBytes) == 0 {
		return nil, &ValidationError{
			Error:   "parse_error",
			Message: "no input: pipe JSON to stdin",
		}
	}

	var input TaskInput
	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		unknownField, hasUnknown := extractUnknownField(err)
		if hasUnknown {
			message := fmt.Sprintf("invalid JSON: %v", err)
			invalid := map[string]string{
				unknownField: "unknown field",
			}
			if suggestion, ok := suggestFieldName(unknownField); ok {
				message = fmt.Sprintf("invalid JSON: unknown field %q (did you mean: %s?)", unknownField, suggestion)
				invalid[unknownField] = fmt.Sprintf("unknown field (did you mean: %s?)", suggestion)
			}
			return nil, &ValidationError{
				Error:   "parse_error",
				Message: message,
				Invalid: invalid,
			}
		}
		return nil, &ValidationError{
			Error:   "parse_error",
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}
	}
	// Ensure there's no trailing junk after the first JSON object.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, &ValidationError{
			Error:   "parse_error",
			Message: "invalid JSON: multiple JSON values provided",
		}
	}

	return &input, nil
}

func extractUnknownField(err error) (string, bool) {
	msg := err.Error()
	const prefix = "json: unknown field "
	if !strings.HasPrefix(msg, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(msg, prefix)
	if len(rest) < 2 || rest[0] != '"' {
		return "", false
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end == -1 {
		return "", false
	}
	return rest[:end], true
}

func suggestFieldName(unknown string) (string, bool) {
	unknown = strings.ToLower(unknown)
	best := ""
	bestDist := 99
	secondBest := 99
	for _, cand := range knownJSONFields {
		dist := levenshteinDistance(unknown, cand)
		if dist < bestDist {
			secondBest = bestDist
			bestDist = dist
			best = cand
			continue
		}
		if dist < secondBest {
			secondBest = dist
		}
	}
	if best == "" {
		return "", false
	}
	if bestDist <= 2 && (secondBest-bestDist >= 2 || secondBest > 3) {
		return best, true
	}
	return "", false
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = minInt(del, minInt(ins, sub))
		}
		prev = cur
	}
	return prev[len(b)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

	// Body validation - optional details
	if t.Body != nil && strings.TrimSpace(*t.Body) == "" {
		invalid["body"] = "cannot be empty"
	}

	// State validation
	if t.State != nil {
		if _, ok := validStates[*t.State]; !ok {
			invalid["state"] = fmt.Sprintf("invalid value %q, expected: todo, doing, done, blocked, canceled, error", *t.State)
		}
	}

	// Claim invariants (doing/error require claim)
	// Relaxed for implicit claim: we only reject if claim is explicitly empty ("").
	// If claim is nil (missing from JSON), the logic layer will handle implicit claim.
	if t.State != nil {
		state := *t.State
		isClaimNeeded := state == stateDoing || state == stateError
		isClaimEmpty := t.Claim != nil && *t.Claim == ""
		if isClaimNeeded && isClaimEmpty {
			invalid["claim"] = fmt.Sprintf("cannot be empty when state=%s", state)
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
		if t.State != nil {
			invalid["state"] = "epics do not have state (use epic-deps)"
		}
		if t.Claim != nil {
			invalid["claim"] = "epics cannot be claimed"
		}
	}

	if len(missing) > 0 || len(invalid) > 0 {
		message := "invalid input"
		if len(missing) > 0 && len(invalid) == 0 {
			message = "missing required fields"
		}
		return &ValidationError{
			Error:   "validation_failed",
			Message: message,
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
