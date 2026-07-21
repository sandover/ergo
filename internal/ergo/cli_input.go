// Purpose: Parse the forward CLI contract for task creation and plan commands.
// Exports: readOptionalBodyFromStdin and inline JSON input helpers.
// Role: Converts optional positional JSON args plus optional stdin body into mutations.
// Invariants: Unknown keys are rejected with suggestions; stdin body is only read when piped.
// Notes: This is the hard-cutover surface; old flag-driven body modes are gone.
package ergo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var knownInlineTaskFields = []string{"title", "epic", "state", "claim", "result"}
var knownPlanCommandFields = []string{"title"}

type InlineTaskInput struct {
	Title  *string `json:"title,omitempty"`
	Epic   *string `json:"epic,omitempty"`
	State  *string `json:"state,omitempty"`
	Claim  *string `json:"claim,omitempty"`
	Result *string `json:"result,omitempty"`
}

type PlanCommandInput struct {
	Title *string `json:"title,omitempty"`
}

func readOptionalBodyFromStdin() (string, bool, error) {
	if !stdinIsPiped() {
		return "", false, nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

func parseInlineTaskArgs(args []string, usage string) (*InlineTaskInput, *ValidationError, error) {
	input := &InlineTaskInput{}
	if len(args) == 0 {
		return input, nil, nil
	}
	if len(args) > 1 {
		return nil, nil, errors.New(usage)
	}
	if verr := parseInlineJSONArg(args[0], input, knownInlineTaskFields); verr != nil {
		return nil, verr, nil
	}
	return input, nil, nil
}

func parsePlanCommandArgs(args []string, usage string) (*PlanCommandInput, *ValidationError, error) {
	input := &PlanCommandInput{}
	if len(args) == 0 {
		return input, nil, nil
	}
	if len(args) > 1 {
		return nil, nil, errors.New(usage)
	}
	if verr := parseInlineJSONArg(args[0], input, knownPlanCommandFields); verr != nil {
		return nil, verr, nil
	}
	return input, nil, nil
}

func parseInlineJSONArg(arg string, dest any, candidates []string) *ValidationError {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return &ValidationError{Error: "parse_error", Message: "empty JSON argument"}
	}

	decoder := json.NewDecoder(bytes.NewBufferString(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		unknownField, hasUnknown := extractUnknownField(err)
		if hasUnknown {
			message := fmt.Sprintf("invalid JSON: %v", err)
			invalid := map[string]string{unknownField: "unknown field"}
			if suggestion, ok := suggestFieldNameFrom(unknownField, candidates); ok {
				message = fmt.Sprintf("invalid JSON: unknown field %q (did you mean: %s?)", unknownField, suggestion)
				invalid[unknownField] = fmt.Sprintf("unknown field (did you mean: %s?)", suggestion)
			}
			return &ValidationError{Error: "parse_error", Message: message, Invalid: invalid}
		}
		return &ValidationError{Error: "parse_error", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return &ValidationError{Error: "parse_error", Message: "invalid JSON: multiple JSON values provided"}
	}
	return nil
}

func (input *InlineTaskInput) ValidateForNew() *ValidationError {
	return input.validate(true)
}

func (input *InlineTaskInput) validate(requireTitle bool) *ValidationError {
	if input == nil {
		if requireTitle {
			return &ValidationError{Error: "validation_failed", Message: "invalid task input", Missing: []string{"title"}}
		}
		return nil
	}

	var missing []string
	invalid := map[string]string{}

	if requireTitle {
		if input.Title == nil || strings.TrimSpace(*input.Title) == "" {
			missing = append(missing, "title")
		}
	} else if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		invalid["title"] = "cannot be empty"
	}

	if input.State != nil {
		state := strings.TrimSpace(*input.State)
		if err := validateForwardState(state); err != nil {
			invalid["state"] = "must be one of: todo, doing, done, blocked, canceled; error is legacy-only"
		}
	}

	if input.Result != nil && strings.TrimSpace(*input.Result) == "" {
		invalid["result"] = "cannot be empty"
	}

	if len(missing) == 0 && len(invalid) == 0 {
		return nil
	}
	return &ValidationError{Error: "validation_failed", Message: "invalid task input", Missing: missing, Invalid: invalid}
}

func (input *PlanCommandInput) Validate() *ValidationError {
	if input != nil && input.Title != nil && strings.TrimSpace(*input.Title) != "" {
		return nil
	}
	return &ValidationError{Error: "validation_failed", Message: "invalid plan input", Missing: []string{"title"}}
}

func (input *InlineTaskInput) ToUpdates() map[string]string {
	updates := map[string]string{}
	if input == nil {
		return updates
	}
	if input.Title != nil {
		updates["title"] = strings.TrimSpace(*input.Title)
	}
	if input.Epic != nil {
		updates["epic"] = *input.Epic
	}
	if input.State != nil {
		updates["state"] = strings.TrimSpace(*input.State)
	}
	if input.Claim != nil {
		updates["claim"] = *input.Claim
	}
	if input.Result != nil {
		updates["result.path"] = strings.TrimSpace(*input.Result)
	}
	return updates
}
