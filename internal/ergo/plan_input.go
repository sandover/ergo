// Purpose: Parse and validate JSON stdin for `ergo plan`.
// Exports: PlanInput, PlanTaskInput, ParsePlanInput.
// Role: Input contract layer for atomic epic+task graph creation.
// Invariants: Unknown keys are rejected; task-title references are local and acyclic.
// Notes: Parse errors use `parse_error`; semantic failures use `validation_failed`.
package ergo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

var knownPlanTopLevelJSONFields = []string{
	"title",
	"body",
	"tasks",
}

// PlanInput is the JSON schema for `ergo plan`.
type PlanInput struct {
	Title *string         `json:"title,omitempty"` // epic title (required)
	Body  *string         `json:"body,omitempty"`  // epic body (optional)
	Tasks []PlanTaskInput `json:"tasks"`           // required, non-empty
}

// PlanTaskInput describes one task in a plan payload.
type PlanTaskInput struct {
	Title *string  `json:"title,omitempty"` // required
	Body  *string  `json:"body,omitempty"`  // optional
	After []string `json:"after,omitempty"` // optional: task titles this task depends on
}

// ParsePlanInput reads JSON from stdin and parses it into PlanInput.
func ParsePlanInput() (*PlanInput, *ValidationError) {
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

	var input PlanInput
	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		unknownField, hasUnknown := extractUnknownField(err)
		if hasUnknown {
			message := fmt.Sprintf("invalid JSON: %v", err)
			invalid := map[string]string{
				unknownField: "unknown field",
			}
			if suggestion, ok := suggestFieldNameFrom(unknownField, knownPlanTopLevelJSONFields); ok {
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
	// Ensure there is only one top-level JSON value.
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, &ValidationError{
			Error:   "parse_error",
			Message: "invalid JSON: multiple JSON values provided",
		}
	}

	return &input, nil
}

// Validate validates the parsed plan payload.
func (p *PlanInput) Validate() *ValidationError {
	var missing []string
	invalid := map[string]string{}

	if p.Title == nil || strings.TrimSpace(*p.Title) == "" {
		missing = append(missing, "title")
	}
	if p.Body != nil && strings.TrimSpace(*p.Body) == "" {
		invalid["body"] = "cannot be empty"
	}
	if len(p.Tasks) == 0 {
		missing = append(missing, "tasks")
	}

	titleToIndex := map[string]int{}
	depsByTitle := map[string][]string{}

	for i, task := range p.Tasks {
		fieldPrefix := fmt.Sprintf("tasks[%d]", i)
		if task.Title == nil || strings.TrimSpace(*task.Title) == "" {
			missing = append(missing, fieldPrefix+".title")
		} else {
			taskTitle := *task.Title
			if prior, exists := titleToIndex[taskTitle]; exists {
				invalid[fieldPrefix+".title"] = fmt.Sprintf("duplicate title %q (already used by tasks[%d].title)", taskTitle, prior)
			} else {
				titleToIndex[taskTitle] = i
			}
		}

		if task.Body != nil && strings.TrimSpace(*task.Body) == "" {
			invalid[fieldPrefix+".body"] = "cannot be empty"
		}
	}

	for i, task := range p.Tasks {
		if task.Title == nil || strings.TrimSpace(*task.Title) == "" {
			continue
		}
		taskTitle := *task.Title
		seenDeps := map[string]struct{}{}
		for j, rawDep := range task.After {
			field := fmt.Sprintf("tasks[%d].after[%d]", i, j)
			if strings.TrimSpace(rawDep) == "" {
				invalid[field] = "cannot be empty"
				continue
			}
			depTitle := rawDep
			if err := validateDepSelf(taskTitle, depTitle); err != nil {
				invalid[field] = err.Error()
				continue
			}
			if _, ok := titleToIndex[depTitle]; !ok {
				invalid[field] = fmt.Sprintf("unknown task title %q", depTitle)
				continue
			}
			if _, duplicate := seenDeps[depTitle]; duplicate {
				continue
			}
			seenDeps[depTitle] = struct{}{}
			depsByTitle[taskTitle] = append(depsByTitle[taskTitle], depTitle)
		}
	}

	if len(invalid) == 0 && hasPlanCycle(titleToIndex, depsByTitle) {
		invalid["tasks"] = "after graph contains a cycle"
	}

	if len(missing) == 0 && len(invalid) == 0 {
		return nil
	}

	sort.Strings(missing)
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

func hasPlanCycle(titles map[string]int, depsByTitle map[string][]string) bool {
	const (
		visitUnseen uint8 = iota
		visitActive
		visitDone
	)
	state := map[string]uint8{}

	var visit func(node string) bool
	visit = func(node string) bool {
		switch state[node] {
		case visitActive:
			return true
		case visitDone:
			return false
		}
		state[node] = visitActive
		for _, dep := range depsByTitle[node] {
			if visit(dep) {
				return true
			}
		}
		state[node] = visitDone
		return false
	}

	for title := range titles {
		if state[title] != visitUnseen {
			continue
		}
		if visit(title) {
			return true
		}
	}
	return false
}
