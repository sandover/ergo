package ergo

import "testing"

func TestStateIconsDistinguishLifecycleAndReadiness(t *testing.T) {
	tests := []struct {
		name  string
		state string
		ready bool
		want  string
	}{
		{name: "ready", state: stateTodo, ready: true, want: "○"},
		{name: "waiting", state: stateTodo, ready: false, want: "◷"},
		{name: "doing", state: stateDoing, want: "↻"},
		{name: "blocked", state: stateBlocked, want: "!"},
		{name: "done", state: stateDone, want: "✓"},
		{name: "canceled", state: stateCanceled, want: "–"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &Task{State: test.state}
			if got := stateIcon(task, test.ready, false); got != test.want {
				t.Fatalf("stateIcon(%s, ready=%v) = %q, want %q", test.state, test.ready, got, test.want)
			}
		})
	}
}
