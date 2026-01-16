// Entry point and command dispatch, plus top-level error handling.
package main

// version is set by goreleaser via ldflags
var version = "dev"

func main() {
	// Execute the Cobra root command
	execute()
}
