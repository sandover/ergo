// Purpose: Lock the complete v2 mutation vocabulary to real CLI behavior.
// Exports: none.
// Role: Black-box regressions for legacy logs, malformed calls, and cutover hints.
// Invariants: fixtures stay isolated and both supported event filenames work.
// Invariants: no removed or guessed command can silently succeed.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2LegacyEventsFileLifecycleNormalization(t *testing.T) {
	dir := setupErgoWithEventsOnly(t)
	errorID := createLifecycleTask(t, dir)
	_, stderr, code := runErgo(t, dir, "", "claim", errorID, "--agent", "legacy@local")
	if code != 0 {
		t.Fatalf("claim legacy task failed: %s", stderr)
	}
	appendLegacyErrorState(t, dir, errorID)
	_, stderr, code = runErgo(t, dir, "", "release", errorID)
	if code != 0 {
		t.Fatalf("release legacy error failed: %s", stderr)
	}
	shown := showTaskJSON(t, dir, errorID)
	if shown["state"] != "todo" || shown["claimed_by"] != "" {
		t.Fatalf("legacy error was not normalized: %v", shown)
	}

	blockedID := createLifecycleTask(t, dir)
	_, _, code = runErgo(t, dir, "", "claim", blockedID, "--agent", "legacy@local")
	if code != 0 {
		t.Fatal("claim legacy blocked task failed")
	}
	appendLegacyStateForConformance(t, dir, blockedID, "blocked")
	_, stderr, code = runErgo(t, dir, "", "block", blockedID)
	if code != 0 {
		t.Fatalf("repeat block failed: %s", stderr)
	}
	shown = showTaskJSON(t, dir, blockedID)
	if shown["state"] != "blocked" || shown["claimed_by"] != "" {
		t.Fatalf("claimed-blocked task was not normalized: %v", shown)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ergo", "plans.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("v2 rewrote legacy events.jsonl into plans.jsonl: %v", err)
	}
}

func TestV2MalformedMutationCallsAreActionable(t *testing.T) {
	dir := setupErgo(t)
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"claim", "A", "B", "--agent", "agent@local"}, "usage: ergo claim"},
		{[]string{"done"}, "usage: ergo done"},
		{[]string{"block"}, "usage: ergo block"},
		{[]string{"cancel"}, "usage: ergo cancel"},
		{[]string{"release"}, "usage: ergo release"},
		{[]string{"title", "ABCDEF"}, "usage: ergo title"},
		{[]string{"body", "A", "B"}, "| ergo body <id>"},
		{[]string{"move", "ABCDEF"}, "usage: ergo move"},
	}
	for _, test := range tests {
		_, stderr, code := runErgo(t, dir, "", test.args...)
		if code == 0 || !strings.Contains(stderr, test.want) || !strings.Contains(stderr, "hint:") {
			t.Errorf("args=%v code=%d stderr=%q", test.args, code, stderr)
		}
	}
}

func appendLegacyStateForConformance(t *testing.T, dir, id, state string) {
	t.Helper()
	path := getEventFilePath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	appendLegacyErrorState(t, dir, id)
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	last := strings.TrimPrefix(string(updated), string(data))
	last = strings.Replace(last, `"state":"error"`, `"state":"`+state+`"`, 1)
	if err := os.WriteFile(path, append(data, []byte(last)...), 0644); err != nil {
		t.Fatal(err)
	}
}
