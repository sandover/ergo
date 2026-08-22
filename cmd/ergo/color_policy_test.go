package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/sandover/ergo/v4/internal/ergo"
)

const ansiEscape = "\x1b["
const colorGreenForTest = "\x1b[32m"
const colorResetForTest = "\x1b[0m"

var ansiColorPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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

func TestColorModesAcrossTerminalAndRedirectedStreams(t *testing.T) {
	tests := []struct {
		mode     colorMode
		terminal bool
		want     bool
	}{
		{mode: colorModeAuto, terminal: true, want: true},
		{mode: colorModeAuto, terminal: false, want: false},
		{mode: colorModeAlways, terminal: true, want: true},
		{mode: colorModeAlways, terminal: false, want: true},
		{mode: colorModeNever, terminal: true, want: false},
		{mode: colorModeNever, terminal: false, want: false},
	}
	for _, test := range tests {
		name := string(test.mode) + "/redirected"
		if test.terminal {
			name = string(test.mode) + "/terminal"
		}
		t.Run(name, func(t *testing.T) {
			if got := resolveColor(test.mode, Streams{StdoutTerminal: test.terminal}); got != test.want {
				t.Fatalf("resolveColor(%s, terminal=%v) = %v, want %v",
					test.mode, test.terminal, got, test.want)
			}
		})
	}
}

func TestShowColorModesAcrossTerminalAndRedirectedStreams(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runNewTaskWithBody(t, dir, "Literal body\n", "Color matrix")
	if code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr)
	}
	id := strings.TrimSpace(stdout)
	plain, stderr, code := runRootForColorTestInDir(t, dir, Streams{}, "--color=never", "show", id)
	if code != 0 || stderr != "" {
		t.Fatalf("plain baseline: code=%d stderr=%q", code, stderr)
	}

	tests := []struct {
		name    string
		mode    string
		streams Streams
		colored bool
	}{
		{name: "auto terminal", mode: "auto", streams: Streams{StdoutTerminal: true}, colored: true},
		{name: "auto redirected", mode: "auto"},
		{name: "always terminal", mode: "always", streams: Streams{StdoutTerminal: true}, colored: true},
		{name: "always redirected", mode: "always", colored: true},
		{name: "never terminal", mode: "never", streams: Streams{StdoutTerminal: true}},
		{name: "never redirected", mode: "never"},
		{name: "auto NO_COLOR", mode: "auto", streams: Streams{StdoutTerminal: true, NoColor: true}},
		{name: "auto TERM dumb", mode: "auto", streams: Streams{StdoutTerminal: true, Term: "dumb"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, stderr, code := runRootForColorTestInDir(
				t, dir, test.streams, "--color="+test.mode, "show", id,
			)
			if code != 0 || stderr != "" {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
			if test.colored {
				if !strings.Contains(got, ansiEscape) {
					t.Fatalf("colored output contains no ANSI: %q", got)
				}
				if stripped := ansiColorPattern.ReplaceAllString(got, ""); stripped != plain {
					t.Fatalf("stripped output differs from plain\nstripped: %q\nplain:    %q", stripped, plain)
				}
				return
			}
			if got != plain {
				t.Fatalf("plain output differs from baseline\ngot:  %q\nwant: %q", got, plain)
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

func TestColorAutoHonorsEnvironmentSuppression(t *testing.T) {
	tests := []struct {
		name    string
		streams Streams
	}{
		{name: "NO_COLOR", streams: Streams{StdoutTerminal: true, NoColor: true}},
		{name: "TERM dumb", streams: Streams{StdoutTerminal: true, Term: "dumb"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, code := runRootForColorTest(t, test.streams, "quickstart")
			if code != 0 || stderr != "" || strings.Contains(stdout, ansiEscape) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
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

func TestRedirectedOutputIsPlainByDefault(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runNewTask(t, dir, "Redirected task")
	if code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr)
	}
	id := strings.TrimSpace(stdout)
	for _, command := range [][]string{
		{"list"},
		{"show", id},
		{"claim", id, "--agent", "agent@test"},
	} {
		t.Run(command[0], func(t *testing.T) {
			stdout, stderr, code := runErgo(t, dir, "", command...)
			if code != 0 || stderr != "" || strings.Contains(stdout, ansiEscape) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestColorPolicyAppliesToShowAndClaimDocuments(t *testing.T) {
	dir := setupErgo(t)
	body := "Literal {{CYAN}} body\n\n# User heading\n"
	stdout, stderr, code := runNewTaskWithBody(t, dir, body, "Colored document")
	if code != 0 {
		t.Fatalf("create: code=%d stderr=%q", code, stderr)
	}
	id := strings.TrimSpace(stdout)

	plain, stderr, code := runErgo(t, dir, "", "--color=never", "show", id)
	if code != 0 || stderr != "" || strings.Contains(plain, ansiEscape) {
		t.Fatalf("plain show: code=%d stdout=%q stderr=%q", code, plain, stderr)
	}
	colored, stderr, code := runErgo(t, dir, "", "--color=always", "show", id)
	if code != 0 || stderr != "" || !strings.Contains(colored, ansiEscape) {
		t.Fatalf("colored show: code=%d stdout=%q stderr=%q", code, colored, stderr)
	}
	if got := ansiColorPattern.ReplaceAllString(colored, ""); got != plain {
		t.Fatalf("stripped colored show differs from plain\ncolored: %q\nplain:   %q", got, plain)
	}
	if !strings.Contains(colored, body) {
		t.Fatalf("show changed user-authored body:\n%s", colored)
	}

	plainClaim, stderr, code := runErgo(t, dir, "", "--color=never", "claim", id, "--agent", "agent@test")
	if code != 0 || stderr != "" || strings.Contains(plainClaim, ansiEscape) {
		t.Fatalf("plain claim: code=%d stdout=%q stderr=%q", code, plainClaim, stderr)
	}
	claimed, stderr, code := runErgo(t, dir, "", "--color=always", "claim", id, "--agent", "agent@test")
	if code != 0 || stderr != "" || !strings.Contains(claimed, ansiEscape) {
		t.Fatalf("colored claim: code=%d stdout=%q stderr=%q", code, claimed, stderr)
	}
	if got := ansiColorPattern.ReplaceAllString(claimed, ""); got != plainClaim {
		t.Fatalf("stripped colored claim differs from plain\ncolored: %q\nplain:   %q", got, plainClaim)
	}
	for _, command := range []string{"ergo done " + id, "ergo block " + id, "ergo cancel " + id, "ergo open " + id} {
		if !strings.Contains(claimed, colorGreenForTest+command+colorResetForTest) {
			t.Fatalf("claim command is not styled %q:\n%s", command, claimed)
		}
	}
	if !strings.Contains(claimed, body) {
		t.Fatalf("claim changed user-authored body:\n%s", claimed)
	}
}

func runRootForColorTest(t *testing.T, capabilities Streams, args ...string) (string, string, int) {
	return runRootForColorTestInDir(t, "", capabilities, args...)
}

func runRootForColorTestInDir(t *testing.T, dir string, capabilities Streams, args ...string) (string, string, int) {
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
	root := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{StartDir: dir}), streams, "test")
	code := runCommand(root, args, streams)
	return stdout.String(), stderr.String(), code
}
