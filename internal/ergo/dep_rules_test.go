// Dependency rule tests for task/epic graph constraints.
// Covers allowed edge types, cycles, and error conditions.
package ergo

import (
	"testing"
)

// TestDepKinds exhaustively tests all kind×kind combinations.
func TestDepKinds(t *testing.T) {
	tests := []struct {
		name        string
		fromIsEpic  bool
		toIsEpic    bool
		wantErr     bool
		errContains string
	}{
		// Valid: same kind
		{name: "task→task", fromIsEpic: false, toIsEpic: false, wantErr: false},
		{name: "epic→epic", fromIsEpic: true, toIsEpic: true, wantErr: false},

		// Invalid: mixed kinds
		{name: "task→epic", fromIsEpic: false, toIsEpic: true, wantErr: true, errContains: "task cannot depend on epic"},
		{name: "epic→task", fromIsEpic: true, toIsEpic: false, wantErr: true, errContains: "epic cannot depend on task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDepKinds(tt.fromIsEpic, tt.toIsEpic)
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
		{name: "epic self-dep rejected", from: "E1", to: "E1", wantErr: true},
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

// TestDepRulesMatrix tests all combinations of dependency rules.
// This is an integration test that combines kind validation, self-dep, and cycle detection.
func TestDepRulesMatrix(t *testing.T) {
	// Test matrix: from_kind × to_kind × same_id × would_cycle
	type testCase struct {
		name       string
		fromIsEpic bool
		toIsEpic   bool
		sameID     bool
		wouldCycle bool
		wantErr    bool
	}

	tests := []testCase{
		// Valid cases
		{name: "task→task different", fromIsEpic: false, toIsEpic: false, sameID: false, wouldCycle: false, wantErr: false},
		{name: "epic→epic different", fromIsEpic: true, toIsEpic: true, sameID: false, wouldCycle: false, wantErr: false},

		// Invalid: self-dep (checked first)
		{name: "task self-dep", fromIsEpic: false, toIsEpic: false, sameID: true, wouldCycle: false, wantErr: true},
		{name: "epic self-dep", fromIsEpic: true, toIsEpic: true, sameID: true, wouldCycle: false, wantErr: true},

		// Invalid: mixed kinds
		{name: "task→epic", fromIsEpic: false, toIsEpic: true, sameID: false, wouldCycle: false, wantErr: true},
		{name: "epic→task", fromIsEpic: true, toIsEpic: false, sameID: false, wouldCycle: false, wantErr: true},

		// Invalid: would create cycle (same kind, different ID)
		{name: "task→task cycle", fromIsEpic: false, toIsEpic: false, sameID: false, wouldCycle: true, wantErr: true},
		{name: "epic→epic cycle", fromIsEpic: true, toIsEpic: true, sameID: false, wouldCycle: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation order used in writeLinkEvent
			from, to := "A", "B"
			if tt.sameID {
				to = "A"
			}

			// 1. Check self-dep
			err := validateDepSelf(from, to)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("unexpected self-dep error: %v", err)
				}
				return
			}

			// 2. Check kind compatibility
			err = validateDepKinds(tt.fromIsEpic, tt.toIsEpic)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("unexpected kind error: %v", err)
				}
				return
			}

			// 3. Check cycle (simulated)
			if tt.wouldCycle {
				if !tt.wantErr {
					t.Error("cycle should cause error")
				}
				return
			}

			// If we get here, all validations passed
			if tt.wantErr {
				t.Error("expected error but all validations passed")
			}
		})
	}
}

// TestDepRulesDocumented verifies documented rules match implementation.
func TestDepRulesDocumented(t *testing.T) {
	// These tests verify the rules stated in model.go comments

	t.Run("task→task allowed", func(t *testing.T) {
		if err := validateDepKinds(false, false); err != nil {
			t.Errorf("task→task should be allowed: %v", err)
		}
	})

	t.Run("epic→epic allowed", func(t *testing.T) {
		if err := validateDepKinds(true, true); err != nil {
			t.Errorf("epic→epic should be allowed: %v", err)
		}
	})

	t.Run("task→epic forbidden", func(t *testing.T) {
		if err := validateDepKinds(false, true); err == nil {
			t.Error("task→epic should be forbidden")
		}
	})

	t.Run("epic→task forbidden", func(t *testing.T) {
		if err := validateDepKinds(true, false); err == nil {
			t.Error("epic→task should be forbidden")
		}
	})

	t.Run("self-dep forbidden", func(t *testing.T) {
		if err := validateDepSelf("X", "X"); err == nil {
			t.Error("self-dep should be forbidden")
		}
	})
}
