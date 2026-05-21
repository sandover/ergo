// Tests for shared validation-error formatting and field-suggestion helpers.
// These are still used by the forward inline-JSON CLI parser.
package ergo

import (
	"strings"
	"testing"
)

func TestSuggestFieldNameFrom_AdjacentSwapFallback(t *testing.T) {
	t.Run("suggests single adjacent swap", func(t *testing.T) {
		suggestion, ok := suggestFieldNameFrom("aftre", []string{"title", "body", "after"})
		if !ok {
			t.Fatal("expected suggestion for aftre, got none")
		}
		if suggestion != "after" {
			t.Fatalf("expected after, got %q", suggestion)
		}
	})

	t.Run("rejects ambiguous adjacent swap", func(t *testing.T) {
		suggestion, ok := suggestFieldNameFrom("acbd", []string{"abcd", "acdb"})
		if ok {
			t.Fatalf("expected no ambiguous suggestion, got %q", suggestion)
		}
	})
}

func TestValidationError_GoError(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		contains []string
	}{
		{
			name: "missing fields included",
			err: ValidationError{
				Message: "invalid input",
				Missing: []string{"body"},
			},
			contains: []string{"invalid input", "missing required: body"},
		},
		{
			name: "invalid fields included",
			err: ValidationError{
				Message: "invalid input",
				Invalid: map[string]string{"state": "invalid value"},
			},
			contains: []string{"invalid input", "state: invalid value"},
		},
		{
			name: "both missing and invalid",
			err: ValidationError{
				Message: "invalid input",
				Missing: []string{"title"},
				Invalid: map[string]string{"state": "bad value"},
			},
			contains: []string{"missing required: title", "state: bad value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.GoError().Error()
			for _, s := range tt.contains {
				if !strings.Contains(errMsg, s) {
					t.Errorf("expected error to contain %q, got %q", s, errMsg)
				}
			}
		})
	}
}
