//go:build !windows

// Purpose: Flush an event-log directory after an atomic rename on Unix hosts.
// Exports: none (package-internal helper).
// Role: Durability step used by atomic event-log replacement.
// Invariants: A successful return means the containing directory was synced.
package ergo

import "os"

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
