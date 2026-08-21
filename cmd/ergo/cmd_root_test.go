package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandover/ergo/v4/internal/ergo"
	"github.com/spf13/cobra"
)

func TestRootCommandConstructionDoesNotLeakFlags(t *testing.T) {
	first := freshRoot(t, "first")
	second := freshRoot(t, "second")

	if err := first.PersistentFlags().Set("dir", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	firstDone := findCommand(t, first, "done")
	if err := firstDone.Flags().Set("message", "one"); err != nil {
		t.Fatal(err)
	}
	if err := firstDone.Flags().Set("message", "two"); err != nil {
		t.Fatal(err)
	}

	if second.PersistentFlags().Changed("dir") {
		t.Fatal("fresh root inherited --dir Changed state")
	}
	secondDone := findCommand(t, second, "done")
	if secondDone.Flags().Changed("message") {
		t.Fatal("fresh lifecycle command inherited Changed state")
	}
	messages, err := secondDone.Flags().GetStringArray("message")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("fresh lifecycle command inherited messages: %v", messages)
	}
}

func TestRootCommandExecutionDoesNotLeakWritersVersionsOrDirectories(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	var firstOut, firstErr, secondOut, secondErr bytes.Buffer
	firstStreams := Streams{
		In: strings.NewReader(""), Out: &firstOut, Err: &firstErr,
		StdinTerminal: true, Width: 80,
	}
	secondStreams := Streams{
		In: strings.NewReader(""), Out: &secondOut, Err: &secondErr,
		StdinTerminal: true, Width: 80,
	}
	first := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{}), firstStreams, "first")
	second := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{}), secondStreams, "second")

	if code := runCommand(first, []string{"init", firstDir}, firstStreams); code != 0 {
		t.Fatalf("first init exit=%d stderr=%q", code, firstErr.String())
	}
	if code := runCommand(second, []string{"init", secondDir}, secondStreams); code != 0 {
		t.Fatalf("second init exit=%d stderr=%q", code, secondErr.String())
	}
	if code := runCommand(freshRootWithStreams(secondStreams, "second"), []string{"version"}, secondStreams); code != 0 {
		t.Fatalf("second version exit=%d stderr=%q", code, secondErr.String())
	}

	if strings.Contains(firstOut.String(), "ergo second") {
		t.Fatalf("first writer received second command output: %q", firstOut.String())
	}
	if !strings.Contains(secondOut.String(), "ergo second") {
		t.Fatalf("second writer lacks its version: %q", secondOut.String())
	}
	if _, err := os.Stat(filepath.Join(firstDir, ".ergo")); err != nil {
		t.Fatalf("first repository was not initialized independently: %v", err)
	}
	if _, err := os.Stat(filepath.Join(secondDir, ".ergo")); err != nil {
		t.Fatalf("second repository was not initialized independently: %v", err)
	}
}

func TestCommandRunnerPreservesRemovedErrorsAndExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "summary", args: []string{"done", "ABCDEF", "--summary", "note"}, want: "use -m <message>"},
		{name: "plan", args: []string{"plan"}, want: "use ergo new epic"},
		{name: "set", args: []string{"set", "ABCDEF", "{}"}, want: "use claim, done, block"},
		{name: "reopen", args: []string{"reopen", "ABCDEF"}, want: "use claim <id> --agent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output, errors bytes.Buffer
			streams := Streams{
				In: strings.NewReader(""), Out: &output, Err: &errors,
				StdinTerminal: true, Width: 80,
			}
			if code := runCommand(freshRootWithStreams(streams, "test"), test.args, streams); code != 1 {
				t.Fatalf("exit=%d, want 1", code)
			}
			if output.Len() != 0 {
				t.Fatalf("stdout=%q, want empty", output.String())
			}
			if !strings.Contains(errors.String(), test.want) {
				t.Fatalf("stderr=%q, want %q", errors.String(), test.want)
			}
		})
	}
}

func TestRootHelpAndVersionUseProvidedWriters(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"version"}} {
		var output, errors bytes.Buffer
		streams := Streams{
			In: strings.NewReader(""), Out: &output, Err: &errors,
			StdinTerminal: true, Width: 80,
		}
		if code := runCommand(freshRootWithStreams(streams, "4.0.0"), args, streams); code != 0 {
			t.Fatalf("args=%v exit=%d stderr=%q", args, code, errors.String())
		}
		if output.Len() == 0 {
			t.Fatalf("args=%v produced no output", args)
		}
	}
}

func TestResolveBuildVersion(t *testing.T) {
	tests := []struct {
		name, linker, module, want string
	}{
		{name: "release linker wins", linker: "4.3.3", module: "v4.3.2", want: "4.3.3"},
		{name: "go install module", linker: "dev", module: "v4.3.3", want: "4.3.3"},
		{name: "local build", linker: "dev", module: "(devel)", want: "dev"},
		{name: "missing build info", linker: "dev", want: "dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveBuildVersion(test.linker, test.module); got != test.want {
				t.Fatalf("resolveBuildVersion(%q, %q) = %q, want %q", test.linker, test.module, got, test.want)
			}
		})
	}
}

func freshRoot(t *testing.T, version string) *cobra.Command {
	t.Helper()
	return freshRootWithStreams(
		Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, StdinTerminal: true, Width: 80},
		version,
	)
}

func freshRootWithStreams(streams Streams, version string) *cobra.Command {
	return NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{}), streams, version)
}
