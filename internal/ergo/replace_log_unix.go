//go:build !windows

// Purpose: Replace a synced authoritative log on Unix.
// Role: Platform-specific replacement step for compaction.
// Invariants: The caller syncs the temporary file and then syncs its directory.
package ergo

import "os"

func replaceLogFile(source, destination string) error {
	return os.Rename(source, destination)
}
