package ergo

import (
	"testing"
)

// Test ParseWorker - domain validation
func TestParseWorker(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Worker
		wantErr  bool
	}{
		{name: "any", input: "any", expected: workerAny, wantErr: false},
		{name: "agent", input: "agent", expected: workerAgent, wantErr: false},
		{name: "human", input: "human", expected: workerHuman, wantErr: false},
		{name: "empty defaults to any", input: "", expected: workerAny, wantErr: false},
		{name: "case insensitive", input: "AGENT", expected: workerAgent, wantErr: false},
		{name: "whitespace trimmed", input: " human ", expected: workerHuman, wantErr: false},
		{name: "invalid worker", input: "robot", expected: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWorker(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

// Test parseKind - domain validation
func TestParseKind(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Kind
		wantErr  bool
	}{
		{name: "any", input: "any", expected: kindAny, wantErr: false},
		{name: "task", input: "task", expected: kindTask, wantErr: false},
		{name: "epic", input: "epic", expected: kindEpic, wantErr: false},
		{name: "empty defaults to any", input: "", expected: kindAny, wantErr: false},
		{name: "case insensitive", input: "TASK", expected: kindTask, wantErr: false},
		{name: "whitespace trimmed", input: " epic ", expected: kindEpic, wantErr: false},
		{name: "invalid kind", input: "story", expected: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseKind(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}
