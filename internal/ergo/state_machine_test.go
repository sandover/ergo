// Purpose: Verify lifecycle postconditions and exact claim ownership rules.
// Exports: none.
// Role: Unit coverage for the v2 state model independent of CLI routing.
// Invariants: every forward target is reachable from every readable state.
// Invariants: doing is claimed and every other forward state is unclaimed.
package ergo

import "testing"

func TestLifecycleStatePostconditionsAcceptEveryReadableSource(t *testing.T) {
	sources := []string{stateTodo, stateDoing, stateBlocked, stateDone, stateCanceled, stateError}
	targets := []string{stateTodo, stateDoing, stateBlocked, stateDone, stateCanceled}
	for _, source := range sources {
		for _, target := range targets {
			t.Run(source+"-to-"+target, func(t *testing.T) {
				mutation := taskMutation{State: target, StateSet: true}
				if target == stateDoing {
					mutation.Claim = "agent-1"
					mutation.ClaimSet = true
				}
				state, claim, err := mutationPostcondition(&Task{State: source, ClaimedBy: legacyClaim(source)}, mutation, "")
				if err != nil {
					t.Fatalf("unexpected postcondition error: %v", err)
				}
				if state != target {
					t.Fatalf("state = %q, want %q", state, target)
				}
				if target == stateDoing && claim != "agent-1" {
					t.Fatalf("doing claim = %q, want agent-1", claim)
				}
				if target != stateDoing && claim != "" {
					t.Fatalf("non-doing claim = %q, want empty", claim)
				}
			})
		}
	}
}

func TestClaimInvariant(t *testing.T) {
	tests := []struct {
		state string
		claim string
		ok    bool
	}{
		{stateDoing, "agent-1", true},
		{stateDoing, "", false},
		{stateTodo, "", true},
		{stateBlocked, "", true},
		{stateDone, "", true},
		{stateCanceled, "", true},
		{stateTodo, "agent-1", false},
		{stateBlocked, "agent-1", false},
		{stateDone, "agent-1", false},
		{stateCanceled, "agent-1", false},
	}
	for _, test := range tests {
		err := validateClaimInvariant(test.state, test.claim)
		if test.ok && err != nil {
			t.Errorf("state=%s claim=%q: %v", test.state, test.claim, err)
		}
		if !test.ok && err == nil {
			t.Errorf("state=%s claim=%q: expected error", test.state, test.claim)
		}
	}
}

func TestForwardWritesRejectLegacyError(t *testing.T) {
	if err := validateForwardState(stateError); err == nil {
		t.Fatal("expected legacy error target to be rejected")
	}
}

func legacyClaim(state string) string {
	if state == stateDoing || state == stateBlocked || state == stateError {
		return "legacy-agent"
	}
	return ""
}
