// Purpose: Provide debug logging for CLI operations.
// Exports: debugf.
// Role: Diagnostics helper used across command handlers.
// Invariants: Emits output only when opts.Verbose is true.
// Notes: Writes to stderr to avoid polluting stdout.
package ergo

import (
	"fmt"
	"os"
)

func debugf(opts GlobalOptions, format string, args ...any) {
	if !opts.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
