//go:build windows

// Purpose: Complete atomic event-log replacement on Windows hosts.
// Exports: none (package-internal helper).
// Role: Platform counterpart to Unix directory syncing.
// Invariants: File contents are synced before rename; directory fsync is unavailable through os.File on Windows.
package ergo

func syncDir(string) error {
	return nil
}
