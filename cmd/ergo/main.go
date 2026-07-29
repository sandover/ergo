// Purpose: Provide the program entrypoint and connect the CLI to process I/O.
// Exports: main.
// Role: Binary entrypoint for the ergo CLI.
// Invariants: Process state and process exit are confined to this file.
// Notes: version is injected via ldflags.
package main

import (
	"os"

	"github.com/sandover/ergo/internal/ergo"
	"golang.org/x/term"
)

// version is set by goreleaser via ldflags
var version = "dev"

func main() {
	streams := processStreams()
	root := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{}), streams, version)
	os.Exit(runCommand(root, os.Args[1:], streams))
}

func processStreams() Streams {
	stdinTerminal := false
	if info, err := os.Stdin.Stat(); err == nil {
		stdinTerminal = info.Mode()&os.ModeCharDevice != 0
	}
	stdoutTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	width := 80
	if stdoutTerminal {
		if columns, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && columns > 0 {
			width = columns
		}
	}
	return Streams{
		In:             os.Stdin,
		Out:            os.Stdout,
		Err:            os.Stderr,
		StdinTerminal:  stdinTerminal,
		StdoutTerminal: stdoutTerminal,
		NoColor:        envPresent("NO_COLOR"),
		Term:           os.Getenv("TERM"),
		Width:          width,
	}
}

func envPresent(name string) bool {
	_, present := os.LookupEnv(name)
	return present
}
