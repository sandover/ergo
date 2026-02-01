// Purpose: Detect stdin/stdout terminal state for CLI behavior.
// Exports: stdinIsPiped, stdoutIsTTY.
// Role: I/O behavior toggles for rendering and input handling.
// Invariants: Returns false on stat errors (conservative default).
// Notes: Uses os.ModeCharDevice checks for portability.
package ergo

import (
	"os"
)

// stdinIsPiped returns true if stdin has piped input (not a terminal).
func stdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

// stdoutIsTTY returns true if stdout is a terminal (supports color, interactive).
func stdoutIsTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
