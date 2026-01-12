package main

import (
	"testing"
)

// TestValidTransitions exhaustively tests all state×state combinations.
func TestValidTransitions(t *testing.T) {
	allStates := []string{stateTodo, stateDoing, stateDone, stateBlocked, stateCanceled, stateError}

	// Expected valid transitions (from → to)
	expected := map[string]map[string]bool{
		stateTodo:     {stateDoing: true, stateDone: true, stateBlocked: true, stateCanceled: true},
		stateDoing:    {stateTodo: true, stateDone: true, stateBlocked: true, stateCanceled: true, stateError: true},
		stateBlocked:  {stateTodo: true, stateDoing: true, stateDone: true, stateCanceled: true},
		stateDone:     {stateTodo: true}, // reopen only
		stateCanceled: {stateTodo: true}, // reopen only
		stateError:    {stateTodo: true, stateDoing: true, stateCanceled: true}, // retry, reassign, or give up
	}

	for _, from := range allStates {
		for _, to := range allStates {
			t.Run(from+"→"+to, func(t *testing.T) {
				err := validateTransition(from, to)

				if from == to {
					// no-op always valid
					if err != nil {
						t.Errorf("no-op %s→%s should be valid, got: %v", from, to, err)
					}
					return
				}

				shouldBeValid := expected[from][to]
				if shouldBeValid && err != nil {
					t.Errorf("%s→%s should be valid, got: %v", from, to, err)
				}
				if !shouldBeValid && err == nil {
					t.Errorf("%s→%s should be invalid, but was allowed", from, to)
				}
			})
		}
	}
}

// TestClaimInvariants tests the claim/state relationship.
func TestClaimInvariants(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		claimedBy string
		wantErr   bool
	}{
		// doing requires claim
		{name: "doing with claim", state: stateDoing, claimedBy: "agent-1", wantErr: false},
		{name: "doing without claim", state: stateDoing, claimedBy: "", wantErr: true},

		// error requires claim (shows who failed)
		{name: "error with claim", state: stateError, claimedBy: "agent-1", wantErr: false},
		{name: "error without claim", state: stateError, claimedBy: "", wantErr: true},

		// todo/done/canceled must have no claim
		{name: "todo without claim", state: stateTodo, claimedBy: "", wantErr: false},
		{name: "todo with claim", state: stateTodo, claimedBy: "agent-1", wantErr: true},
		{name: "done without claim", state: stateDone, claimedBy: "", wantErr: false},
		{name: "done with claim", state: stateDone, claimedBy: "agent-1", wantErr: true},
		{name: "canceled without claim", state: stateCanceled, claimedBy: "", wantErr: false},
		{name: "canceled with claim", state: stateCanceled, claimedBy: "agent-1", wantErr: true},

		// blocked can have or not have claim
		{name: "blocked without claim", state: stateBlocked, claimedBy: "", wantErr: false},
		{name: "blocked with claim", state: stateBlocked, claimedBy: "agent-1", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClaimInvariant(tt.state, tt.claimedBy)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for state=%s claimedBy=%q", tt.state, tt.claimedBy)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestInvalidTransitionsFromTerminalStates verifies done/canceled/error can only go to limited states.
func TestInvalidTransitionsFromTerminalStates(t *testing.T) {
	// done/canceled can only go to todo
	for _, from := range []string{stateDone, stateCanceled} {
		for _, to := range []string{stateDoing, stateBlocked, stateDone, stateCanceled, stateError} {
			if from == to {
				continue // no-op is always valid
			}
			t.Run(from+"→"+to, func(t *testing.T) {
				err := validateTransition(from, to)
				if err == nil {
					t.Errorf("%s→%s should be invalid", from, to)
				}
			})
		}
	}

	// error can go to todo, doing, canceled but NOT done or blocked
	for _, to := range []string{stateDone, stateBlocked} {
		t.Run(stateError+"→"+to, func(t *testing.T) {
			err := validateTransition(stateError, to)
			if err == nil {
				t.Errorf("error→%s should be invalid", to)
			}
		})
	}
}
