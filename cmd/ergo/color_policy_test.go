package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sandover/ergo/internal/ergo"
)

const ansiEscape = "\x1b["

func TestColorAutoResolution(t *testing.T) {
	tests := []struct {
		name    string
		streams Streams
		want    bool
	}{
		{name: "terminal", streams: Streams{StdoutTerminal: true}, want: true},
		{name: "redirected", streams: Streams{}, want: false},
		{name: "NO_COLOR set", streams: Streams{StdoutTerminal: true, NoColor: true}, want: false},
		{name: "dumb terminal", streams: Streams{StdoutTerminal: true, Term: "dumb"}, want: false},
		{name: "other terminal", streams: Streams{StdoutTerminal: true, Term: "xterm-256color"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveColor(colorModeAuto, test.streams); got != test.want {
				t.Fatalf("resolveColor(auto) = %v, want %v", got, test.want)
			}
		})
	}
}

func TestColorExplicitModeOverridesEnvironmentAndTerminal(t *testing.T) {
	disabled := Streams{StdoutTerminal: false, NoColor: true, Term: "dumb"}
	if !resolveColor(colorModeAlways, disabled) {
		t.Fatal("always did not enable color")
	}
	enabled := Streams{StdoutTerminal: true, Term: "xterm-256color"}
	if resolveColor(colorModeNever, enabled) {
		t.Fatal("never did not disable color")
	}
}

func TestColorFlagRejectsUnknownMode(t *testing.T) {
	_, stderr, code := runRootForColorTest(t, Streams{}, "--color=sometimes", "version")
	if code == 0 || !strings.Contains(stderr, `invalid color mode "sometimes"; expected auto, always, or never`) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestColorPolicyAppliesToRootHelpAndQuickstart(t *testing.T) {
	for _, command := range [][]string{{"--help"}, {"quickstart"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			stdout, stderr, code := runRootForColorTest(t, Streams{}, append([]string{"--color=always"}, command...)...)
			if code != 0 || stderr != "" || !strings.Contains(stdout, ansiEscape) {
				t.Fatalf("always: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}

			stdout, stderr, code = runRootForColorTest(t, Streams{StdoutTerminal: true}, append([]string{"--color=never"}, command...)...)
			if code != 0 || stderr != "" || strings.Contains(stdout, ansiEscape) {
				t.Fatalf("never: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestColorPolicyDoesNotLeakAcrossRootCommands(t *testing.T) {
	colored, _, _ := runRootForColorTest(t, Streams{}, "--color=always", "quickstart")
	plain, _, _ := runRootForColorTest(t, Streams{}, "quickstart")
	if !strings.Contains(colored, ansiEscape) {
		t.Fatal("forced-color command was plain")
	}
	if strings.Contains(plain, ansiEscape) {
		t.Fatal("fresh auto command inherited forced color")
	}
}

func TestColorModesKeepExactBodyPlain(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runNewTaskWithBody(t, dir, "literal \x1b[not-added\n", "Body")
	if code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr)
	}
	id := strings.TrimSpace(stdout)
	want := "literal \x1b[not-added\n"
	for _, mode := range []string{"auto", "always", "never"} {
		stdout, stderr, code = runErgo(t, dir, "", "--color="+mode, "show", id, "--body")
		if code != 0 || stderr != "" || stdout != want {
			t.Fatalf("%s: code=%d stdout=%q want=%q stderr=%q", mode, code, stdout, want, stderr)
		}
	}
}

func TestColorPolicyAppliesToListAndPrune(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runNewTask(t, dir, "Colored task")
	if code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr)
	}
	id := strings.TrimSpace(stdout)
	if _, stderr, code = runErgo(t, dir, "", "done", id); code != 0 {
		t.Fatalf("done: code=%d stderr=%q", code, stderr)
	}

	for _, command := range [][]string{{"list", "--all"}, {"prune"}} {
		t.Run(command[0], func(t *testing.T) {
			args := append([]string{"--color=always"}, command...)
			stdout, stderr, code := runErgo(t, dir, "", args...)
			if code != 0 || stderr != "" || !strings.Contains(stdout, ansiEscape) {
				t.Fatalf("always: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			args = append([]string{"--color=never"}, command...)
			stdout, stderr, code = runErgo(t, dir, "", args...)
			if code != 0 || stderr != "" || strings.Contains(stdout, ansiEscape) {
				t.Fatalf("never: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func runRootForColorTest(t *testing.T, capabilities Streams, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	streams := Streams{
		In:             strings.NewReader(""),
		Out:            &stdout,
		Err:            &stderr,
		StdinTerminal:  true,
		StdoutTerminal: capabilities.StdoutTerminal,
		NoColor:        capabilities.NoColor,
		Term:           capabilities.Term,
		Width:          80,
	}
	root := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{}), streams, "test")
	code := runCommand(root, args, streams)
	return stdout.String(), stderr.String(), code
}
