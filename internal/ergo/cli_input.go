// Purpose: Parse optional stdin for task and epic creation.
// Exports: none; creation commands use these package-local helpers.
// Role: Keeps process input outside the typed application boundary.
// Invariants: stdin is read only when piped.
// Notes: Storage JSON is unrelated to this command-input boundary.
package ergo

import (
	"io"
	"os"
)

const (
	NewTaskUsage = `usage: ergo new task "<title>" [--epic <id>]; optional piped stdin becomes the body`
	NewEpicUsage = `usage: ergo new epic "<title>" --file <path>; optional piped stdin becomes the epic body`
)

func readOptionalBodyFromStdin() (string, bool, error) {
	if !stdinIsPiped() {
		return "", false, nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}
