// Dependency rule tests for task graph constraints.
// Covers ancestry checks, self-deps, cycles, and error conditions.
package ergo

import (
	"testing"
)

// TestDepAncestry tests parent-child dependency rejection.
func TestDepAncestry(t *testing.T) {
	tests := []struct {
		name        string
		from        *Task
		to          *Task
		wantErr     bool
		errContains string
	}{
		{
			name:    "unrelated tasks allowed",
			from:    &Task{ID: "A", EpicID: ""},
			to:      &Task{ID: "B", EpicID: ""},
			wantErr: false,
		},
		{
			name:    "siblings in same container allowed",
			from:    &Task{ID: "A", EpicID: "E1"},
			to:      &Task{ID: "B", EpicID: "E1"},
			wantErr: false,
		},
		{
			name:        "child cannot depend on parent",
			from:        &Task{ID: "A", EpicID: "E1"},
			to:          &Task{ID: "E1", EpicID: ""},
			wantErr:     true,
			errContains: "task cannot depend on its own container",
		},
		{
			name:        "parent cannot depend on child",
			from:        &Task{ID: "E1", EpicID: ""},
			to:          &Task{ID: "A", EpicID: "E1"},
			wantErr:     true,
			errContains: "container cannot depend on its own child",
		},
		{
			name:    "cross-container dep allowed",
			from:    &Task{ID: "A", EpicID: "E1"},
			to:      &Task{ID: "B", EpicID: "E2"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDepAncestry(tt.from, tt.to)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %s", tt.name)
				} else if tt.errContains != "" && err.Error() != tt.errContains {
					t.Errorf("expected error %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestDepSelf tests self-dependency rejection.
func TestDepSelf(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		{name: "self-dep rejected", from: "T1", to: "T1", wantErr: true},
		{name: "different IDs allowed", from: "T1", to: "T2", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDepSelf(tt.from, tt.to)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %s→%s", tt.from, tt.to)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestDepRulesDocumented verifies documented rules match implementation.
func TestDepRulesDocumented(t *testing.T) {
	t.Run("unrelated tasks allowed", func(t *testing.T) {
		from := &Task{ID: "A"}
		to := &Task{ID: "B"}
		if err := validateDepAncestry(from, to); err != nil {
			t.Errorf("unrelated tasks should be allowed: %v", err)
		}
	})

	t.Run("child→parent forbidden", func(t *testing.T) {
		from := &Task{ID: "A", EpicID: "E1"}
		to := &Task{ID: "E1"}
		if err := validateDepAncestry(from, to); err == nil {
			t.Error("child→parent should be forbidden")
		}
	})

	t.Run("parent→child forbidden", func(t *testing.T) {
		from := &Task{ID: "E1"}
		to := &Task{ID: "A", EpicID: "E1"}
		if err := validateDepAncestry(from, to); err == nil {
			t.Error("parent→child should be forbidden")
		}
	})

	t.Run("self-dep forbidden", func(t *testing.T) {
		if err := validateDepSelf("X", "X"); err == nil {
			t.Error("self-dep should be forbidden")
		}
	})
}
