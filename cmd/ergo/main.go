// Purpose: Provide the program entrypoint and invoke command execution.
// Exports: main.
// Role: Binary entrypoint for the ergo CLI.
// Invariants: Only delegates to execute(); version is injected via ldflags.
// Notes: Errors are handled by cmd helpers in this package.
package main

// version is set by goreleaser via ldflags
var version = "dev"

func main() {
	// Execute the Cobra root command
	execute()
}
